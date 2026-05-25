package wp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"got0iscc/desktop/internal/application/httpx"
	domain "got0iscc/desktop/internal/domain/wp"
	"got0iscc/desktop/internal/platform/runtime"
)

const (
	informationBaseURL = "https://information.isclab.org.cn"
	monitorEndpoint    = informationBaseURL + "/wpupload"
	getDataEndpoint    = informationBaseURL + "/wpupload/getdata"
	maxRemotePages     = 30
	remoteCacheMetaKey = "wp.remote_cache.v1"
)

var htmlTagPattern = regexp.MustCompile(`(?is)<[^>]+>`)

type Service struct {
	layout runtime.Layout
	repo   MetaRepository
}

type MetaRepository interface {
	MetaValue(ctx context.Context, key string) (string, error)
	SetMetaValue(ctx context.Context, key string, value string) error
}

type accountIdentity struct {
	Name           string
	Username       string
	Password       string
	SubmitIdentity string
	Enabled        bool
}

type accountSync struct {
	Account       string
	Identity      accountIdentity
	Status        string
	Message       string
	LastSyncedAt  string
	Records       []domain.SubmissionRecord
	RemoteRecords int
}

type challengeAccountState struct {
	ChallengeKey     string
	Account          string
	PlatformSolved   bool
	PlatformSolvedAt string
	LastSubmittedAt  string
	LastFlag         string
	UpdatedAt        string
}

type remoteDataResponse struct {
	Data        []remoteRecord `json:"data"`
	IsEmpty     bool           `json:"isEmpty"`
	PageIndex   int            `json:"pageindex"`
	TotalPages  int            `json:"totalPages"`
	TotalOfThis int            `json:"totalofthis"`
}

type remoteRecord struct {
	Filename string `json:"Filename"`
}

type remoteCacheSnapshot struct {
	UpdatedAt string                   `json:"updated_at"`
	Accounts  []remoteCacheAccountSync `json:"accounts"`
}

type remoteCacheAccountSync struct {
	Account      string                    `json:"account"`
	Status       string                    `json:"status"`
	Message      string                    `json:"message"`
	LastSyncedAt string                    `json:"last_synced_at"`
	Records      []domain.SubmissionRecord `json:"records"`
}

func NewService(layout runtime.Layout, repo MetaRepository) *Service {
	return &Service{layout: layout, repo: repo}
}

func (s *Service) List(ctx context.Context) (domain.Payload, error) {
	return s.list(ctx, false)
}

func (s *Service) SyncRemote(ctx context.Context) (domain.Payload, error) {
	return s.list(ctx, true)
}

func (s *Service) list(ctx context.Context, refreshRemote bool) (domain.Payload, error) {
	challenges, err := s.loadChallenges(ctx)
	if err != nil {
		return domain.Payload{}, err
	}
	states, err := s.loadAccountStatesFromDB(ctx)
	if err != nil {
		return domain.Payload{}, err
	}
	if len(states) == 0 {
		states = fallbackAccountStates(challenges)
	}

	identities, err := s.loadAccountIdentities(ctx)
	if err != nil {
		return domain.Payload{}, err
	}

	challengeByKey := make(map[string]domain.Challenge)
	challengeByID := make(map[string]domain.Challenge)
	for _, challenge := range challenges {
		challengeByKey[challenge.Key] = challenge
		challengeByID[challenge.Section+":"+challenge.ChallengeID] = challenge
		challengeByID[challenge.ChallengeID] = challenge
	}

	requirements := make([]domain.Item, 0)
	neededAccounts := map[string]bool{}
	for _, state := range states {
		if !state.PlatformSolved {
			continue
		}
		challenge, ok := challengeByKey[state.ChallengeKey]
		if !ok {
			challenge = challengeByID[state.ChallengeKey]
		}
		if challenge.Key == "" {
			continue
		}
		identity := identityForAccount(identities, state.Account)
		expected := expectedFilename(identity.SubmitIdentity, challenge.Title)
		item := domain.Item{
			Key:              state.Account + ":" + challenge.Key,
			Account:          state.Account,
			SubmitIdentity:   identity.SubmitIdentity,
			ExpectedFilename: expected,
			Section:          challenge.Section,
			SectionLabel:     challenge.SectionLabel,
			Challenge:        challenge,
			Status:           "pending_sync",
			PlatformSolved:   true,
			PlatformSolvedAt: state.PlatformSolvedAt,
			LastSubmittedAt:  state.LastSubmittedAt,
			LastFlag:         state.LastFlag,
		}
		requirements = append(requirements, item)
		neededAccounts[state.Account] = true
	}

	accountSyncs, err := s.loadAccountSyncs(ctx, identities, neededAccounts, refreshRemote)
	if err != nil {
		return domain.Payload{}, err
	}
	for i := range requirements {
		sync := accountSyncs[requirements[i].Account]
		applyRemoteStatus(&requirements[i], sync)
	}

	records := collectRecords(accountSyncs)
	markUnmatchedRecords(records, requirements)
	accounts := summarizeAccounts(requirements, accountSyncs)
	summary := summarizePayload(requirements, accounts, records)
	return domain.Payload{Summary: summary, Accounts: accounts, Items: requirements, Records: records}, nil
}

func (s *Service) loadChallenges(ctx context.Context) ([]domain.Challenge, error) {
	fromDB, err := s.loadChallengesFromDB(ctx)
	if err == nil && len(fromDB) > 0 {
		return fromDB, nil
	}
	return s.loadChallengesFromDirs(ctx)
}

func (s *Service) loadChallengesFromDirs(ctx context.Context) ([]domain.Challenge, error) {
	entries, err := os.ReadDir(s.layout.ChallengesRoot)
	if err != nil {
		return nil, fmt.Errorf("读取 challenges 目录失败: %w", err)
	}
	items := make([]domain.Challenge, 0)
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(s.layout.ChallengesRoot, entry.Name())
		items = append(items, s.inspectChallenge("", "challenges", "练武题", dir, entry.Name(), "", "", ""))
	}
	sortChallenges(items)
	return items, nil
}

func (s *Service) loadChallengesFromDB(ctx context.Context) ([]domain.Challenge, error) {
	dbPath := s.challengeDBPath()
	if dbPath == "" {
		return nil, nil
	}
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&immutable=1", dbPath))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
SELECT challenge_key, challenge_id, section_name, title, dir_name, dir_path, description_path, remote_summary_path, updated_at
FROM challenges
ORDER BY section_name ASC, CAST(challenge_id AS INTEGER) ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Challenge, 0)
	for rows.Next() {
		var key, id, section, title, dirName, dirPath, descriptionPath, remotePath, updatedAt string
		if err := rows.Scan(&key, &id, &section, &title, &dirName, &dirPath, &descriptionPath, &remotePath, &updatedAt); err != nil {
			return nil, err
		}
		if !requiresWriteupSection(section) {
			continue
		}
		if dirPath == "" {
			dirPath = filepath.Join(s.layout.ChallengesRoot, dirName)
		}
		item := s.inspectChallenge(key, section, sectionLabel(section), dirPath, dirName, id, title, updatedAt)
		item.DescriptionPath = descriptionPath
		item.RemotePath = remotePath
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortChallenges(items)
	return items, nil
}

func (s *Service) inspectChallenge(key string, section string, sectionDisplay string, dir string, name string, id string, title string, updatedAt string) domain.Challenge {
	item := domain.Challenge{
		Key:          fallbackString(key, section+":"+challengeID(name)),
		ChallengeID:  fallbackString(id, challengeID(name)),
		Section:      fallbackString(section, "challenges"),
		SectionLabel: fallbackString(sectionDisplay, sectionLabel(section)),
		Title:        fallbackString(title, challengeTitle(name)),
		DirPath:      dir,
		UpdatedAt:    updatedAt,
	}
	item.DescriptionPath = firstExisting(dir, "description.md")
	item.SolvePath = firstExisting(dir, "solve.py", "solver.py", "exp.py", "exploit.py", "poc.py")
	item.RemotePath = firstExisting(dir, "remote.txt")
	return item
}

func (s *Service) loadAccountStatesFromDB(ctx context.Context) ([]challengeAccountState, error) {
	dbPath := s.challengeDBPath()
	if dbPath == "" {
		return nil, nil
	}
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&immutable=1", dbPath))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
SELECT challenge_key, account_name, data_json, updated_at
FROM challenge_accounts
ORDER BY account_name ASC, challenge_key ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make([]challengeAccountState, 0)
	for rows.Next() {
		var state challengeAccountState
		var dataJSON string
		if err := rows.Scan(&state.ChallengeKey, &state.Account, &dataJSON, &state.UpdatedAt); err != nil {
			return nil, err
		}
		var data struct {
			PlatformSolved   bool   `json:"platform_solved"`
			PlatformSolvedAt string `json:"platform_solved_at"`
			LastSubmitOK     bool   `json:"last_submit_ok"`
			LastSubmittedAt  string `json:"last_submitted_at"`
			LastFlag         string `json:"last_flag"`
		}
		_ = json.Unmarshal([]byte(dataJSON), &data)
		state.PlatformSolved = data.PlatformSolved || data.LastSubmitOK
		state.PlatformSolvedAt = data.PlatformSolvedAt
		state.LastSubmittedAt = data.LastSubmittedAt
		state.LastFlag = data.LastFlag
		states = append(states, state)
	}
	return states, rows.Err()
}

func (s *Service) loadAccountIdentities(ctx context.Context) (map[string]accountIdentity, error) {
	results := map[string]accountIdentity{}
	for _, dbPath := range dedupeStrings([]string{s.layout.AppDatabasePath, s.challengeDBPath()}) {
		if strings.TrimSpace(dbPath) == "" {
			continue
		}
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		items, err := loadIdentitiesFromDB(ctx, dbPath)
		if err != nil {
			continue
		}
		for _, item := range items {
			if item.Name == "" {
				continue
			}
			existing, exists := results[item.Name]
			if !exists || existing.Password == "" {
				results[item.Name] = item
			}
		}
	}
	return results, nil
}

func loadIdentitiesFromDB(ctx context.Context, dbPath string) ([]accountIdentity, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&immutable=1", dbPath))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
SELECT name, username, password, enabled
FROM accounts
ORDER BY name ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]accountIdentity, 0)
	for rows.Next() {
		var item accountIdentity
		var enabled int
		if err := rows.Scan(&item.Name, &item.Username, &item.Password, &enabled); err != nil {
			return nil, err
		}
		item.Name = strings.TrimSpace(item.Name)
		item.Username = strings.TrimSpace(item.Username)
		item.SubmitIdentity = fallbackString(item.Username, item.Name)
		item.Enabled = enabled != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) syncAccounts(ctx context.Context, identities map[string]accountIdentity, needed map[string]bool) map[string]accountSync {
	results := make(map[string]accountSync, len(needed))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)

	for account := range needed {
		account := account
		identity := identityForAccount(identities, account)
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sync := s.syncAccount(ctx, identity)
			mu.Lock()
			results[account] = sync
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}

func (s *Service) loadAccountSyncs(ctx context.Context, identities map[string]accountIdentity, needed map[string]bool, refreshRemote bool) (map[string]accountSync, error) {
	if refreshRemote {
		results := s.syncAccounts(ctx, identities, needed)
		if err := s.saveRemoteCache(ctx, results); err != nil {
			return nil, err
		}
		return results, nil
	}

	results, err := s.readRemoteCache(ctx, identities, needed)
	if err != nil {
		return nil, err
	}
	for account := range needed {
		if _, ok := results[account]; ok {
			continue
		}
		identity := identityForAccount(identities, account)
		results[account] = accountSync{
			Account:       account,
			Identity:      identity,
			Status:        "idle",
			Message:       "尚未抓取远端 WP 记录，请手动点击同步按钮",
			LastSyncedAt:  "",
			Records:       nil,
			RemoteRecords: 0,
		}
	}
	return results, nil
}

func (s *Service) readRemoteCache(ctx context.Context, identities map[string]accountIdentity, needed map[string]bool) (map[string]accountSync, error) {
	results := make(map[string]accountSync, len(needed))
	if s.repo == nil {
		return results, nil
	}
	raw, err := s.repo.MetaValue(ctx, remoteCacheMetaKey)
	if err != nil {
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return results, nil
	}

	var snapshot remoteCacheSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return results, nil
	}
	for _, item := range snapshot.Accounts {
		if !needed[item.Account] {
			continue
		}
		identity := identityForAccount(identities, item.Account)
		results[item.Account] = accountSync{
			Account:       item.Account,
			Identity:      identity,
			Status:        fallbackString(item.Status, "idle"),
			Message:       item.Message,
			LastSyncedAt:  item.LastSyncedAt,
			Records:       item.Records,
			RemoteRecords: len(item.Records),
		}
	}
	return results, nil
}

func (s *Service) saveRemoteCache(ctx context.Context, syncs map[string]accountSync) error {
	if s.repo == nil {
		return nil
	}
	keys := make([]string, 0, len(syncs))
	for key := range syncs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	snapshot := remoteCacheSnapshot{
		UpdatedAt: time.Now().Format("2006-01-02 15:04:05"),
		Accounts:  make([]remoteCacheAccountSync, 0, len(keys)),
	}
	for _, key := range keys {
		item := syncs[key]
		snapshot.Accounts = append(snapshot.Accounts, remoteCacheAccountSync{
			Account:      item.Account,
			Status:       item.Status,
			Message:      item.Message,
			LastSyncedAt: item.LastSyncedAt,
			Records:      item.Records,
		})
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return s.repo.SetMetaValue(ctx, remoteCacheMetaKey, string(data))
}

func (s *Service) syncAccount(ctx context.Context, identity accountIdentity) accountSync {
	now := time.Now().Format("2006-01-02 15:04:05")
	result := accountSync{
		Account:      identity.Name,
		Identity:     identity,
		Status:       "failed",
		LastSyncedAt: now,
	}
	if strings.TrimSpace(identity.Username) == "" || strings.TrimSpace(identity.Password) == "" {
		result.Message = "账号缺少登录用户名或密码，无法同步远端 WP 记录"
		return result
	}

	client, err := newMonitorClient()
	if err != nil {
		result.Message = err.Error()
		return result
	}
	if err := loginInformation(ctx, client, identity.Username, identity.Password); err != nil {
		result.Message = err.Error()
		return result
	}
	records, err := fetchAllRemoteRecords(ctx, client, identity)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.Status = "synced"
	result.Message = "同步成功"
	result.Records = records
	result.RemoteRecords = len(records)
	return result
}

func newMonitorClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{Jar: jar, Timeout: 25 * time.Second}, nil
}

func loginInformation(ctx context.Context, client *http.Client, username string, password string) error {
	bodyBytes, _ := json.Marshal(map[string]string{
		"username": strings.TrimSpace(username),
		"password": password,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, informationBaseURL+"/login", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	httpx.ApplyBrowserHeaders(req, informationBaseURL+"/wpupload", false)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("登录信息系统失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("登录信息系统失败: HTTP %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("登录信息系统失败: %s", responseHint(string(data)))
	}
	if errText := strings.TrimSpace(fmt.Sprint(payload["error"])); errText != "" && errText != "<nil>" {
		return fmt.Errorf("登录信息系统失败: %s", errText)
	}
	if redirect := strings.TrimSpace(fmt.Sprint(payload["redirect"])); redirect == "" || redirect == "<nil>" {
		return errors.New("登录信息系统失败: 未返回登录成功跳转")
	}
	return nil
}

func fetchAllRemoteRecords(ctx context.Context, client *http.Client, identity accountIdentity) ([]domain.SubmissionRecord, error) {
	records := make([]domain.SubmissionRecord, 0)
	totalPages := 1
	seenPages := map[int]bool{}
	for page := 1; page <= totalPages && page <= maxRemotePages; page++ {
		if seenPages[page] {
			break
		}
		seenPages[page] = true
		response, err := fetchRemotePage(ctx, client, page)
		if err != nil {
			return nil, err
		}
		if response.TotalPages > 0 {
			totalPages = response.TotalPages
		}
		if response.IsEmpty || response.TotalOfThis == 0 || len(response.Data) == 0 {
			break
		}
		for i, item := range response.Data {
			filename := strings.TrimSpace(item.Filename)
			if filename == "" {
				continue
			}
			record := parseRemoteFilename(filename, identity)
			record.PageIndex = page
			record.Sequence = (page-1)*10 + i + 1
			records = append(records, record)
		}
	}
	return records, nil
}

func fetchRemotePage(ctx context.Context, client *http.Client, page int) (remoteDataResponse, error) {
	bodyBytes, _ := json.Marshal(map[string]string{"pageindex": strconv.Itoa(page)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, getDataEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return remoteDataResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	httpx.ApplyBrowserHeaders(req, informationBaseURL+"/wpupload", false)
	resp, err := client.Do(req)
	if err != nil {
		return remoteDataResponse{}, fmt.Errorf("读取远端 WP 历史失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return remoteDataResponse{}, fmt.Errorf("读取远端 WP 历史失败: HTTP %d", resp.StatusCode)
	}
	if looksLikeLoginPage(resp.Request.URL.String(), string(data)) {
		return remoteDataResponse{}, errors.New("读取远端 WP 历史失败: 登录态无效")
	}
	var payload remoteDataResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return remoteDataResponse{}, fmt.Errorf("读取远端 WP 历史失败: 返回不是 JSON")
	}
	return payload, nil
}

func parseRemoteFilename(filename string, identity accountIdentity) domain.SubmissionRecord {
	record := domain.SubmissionRecord{
		Account:        identity.Name,
		SubmitIdentity: identity.SubmitIdentity,
		Filename:       filename,
		MatchStatus:    "unmatched",
	}
	base := strings.TrimSpace(filepath.Base(filename))
	withoutExt := strings.TrimSuffix(base, filepath.Ext(base))
	if !strings.EqualFold(filepath.Ext(base), ".docx") {
		record.Issues = append(record.Issues, domain.Issue{Code: "not_docx", Message: "远端记录不是 .docx 文件"})
	}
	prefix := identity.SubmitIdentity + "-"
	if !strings.HasPrefix(withoutExt, prefix) {
		record.Issues = append(record.Issues, domain.Issue{Code: "identity_mismatch", Message: "文件名没有使用本账号注册身份作为前缀"})
		record.ChallengeTitle = strings.TrimSpace(afterFirstDash(withoutExt))
		return record
	}
	record.ChallengeTitle = strings.TrimSpace(strings.TrimPrefix(withoutExt, prefix))
	if record.ChallengeTitle == "" {
		record.Issues = append(record.Issues, domain.Issue{Code: "missing_title", Message: "文件名缺少题目名称"})
	}
	return record
}

func applyRemoteStatus(item *domain.Item, sync accountSync) {
	item.SyncStatus = fallbackString(sync.Status, "failed")
	item.SyncMessage = sync.Message
	item.LastSyncedAt = sync.LastSyncedAt
	if sync.Status != "synced" {
		item.Status = "sync_failed"
		item.Issues = append(item.Issues, domain.Issue{Code: "sync_failed", Message: fallbackString(sync.Message, "未能同步远端 WP 历史")})
		return
	}

	for _, record := range sync.Records {
		if filenamesMatch(record.Filename, item.ExpectedFilename) || titleMatches(record.ChallengeTitle, item.Challenge.Title) {
			matched := record
			matched.ChallengeID = item.Challenge.ChallengeID
			matched.ChallengeTitle = item.Challenge.Title
			matched.Section = item.Section
			matched.SectionLabel = item.SectionLabel
			matched.ExpectedFilename = item.ExpectedFilename
			matched.MatchStatus = "matched"
			item.RemoteRecords = append(item.RemoteRecords, matched)
		}
	}
	item.RemoteAttempts = len(item.RemoteRecords)
	item.RemoteSubmitted = item.RemoteAttempts > 0
	if !item.RemoteSubmitted {
		item.Status = "missing"
		item.Issues = append(item.Issues, domain.Issue{Code: "missing_remote_writeup", Message: "该账号已解出本题，但远端历史中没有匹配的 WP"})
		return
	}
	for _, record := range item.RemoteRecords {
		if !filenamesMatch(record.Filename, item.ExpectedFilename) {
			item.Issues = append(item.Issues, domain.Issue{Code: "filename_mismatch", Message: "远端 WP 文件名应为 " + item.ExpectedFilename})
			break
		}
	}
	if item.RemoteAttempts > 3 {
		item.Issues = append(item.Issues, domain.Issue{Code: "too_many_attempts", Message: "该题远端提交记录超过 3 次"})
	}
	if len(item.Issues) > 0 {
		item.Status = "needs_fix"
		return
	}
	item.Status = "submitted"
}

func collectRecords(syncs map[string]accountSync) []domain.SubmissionRecord {
	records := make([]domain.SubmissionRecord, 0)
	for _, sync := range syncs {
		records = append(records, sync.Records...)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Account == records[j].Account {
			return records[i].Sequence < records[j].Sequence
		}
		return records[i].Account < records[j].Account
	})
	return records
}

func markUnmatchedRecords(records []domain.SubmissionRecord, items []domain.Item) {
	matched := map[string]bool{}
	for _, item := range items {
		for _, record := range item.RemoteRecords {
			matched[record.Account+"\x00"+record.Filename] = true
		}
	}
	for i := range records {
		if matched[records[i].Account+"\x00"+records[i].Filename] {
			records[i].MatchStatus = "matched"
			continue
		}
		records[i].MatchStatus = "unmatched"
		if len(records[i].Issues) == 0 {
			records[i].Warnings = append(records[i].Warnings, domain.Issue{Code: "unmatched_record", Message: "远端记录没有匹配到当前已解题目"})
		}
	}
}

func summarizePayload(items []domain.Item, accounts []domain.AccountSummary, records []domain.SubmissionRecord) domain.Summary {
	summary := domain.Summary{
		Total:           len(items),
		TotalAccounts:   len(accounts),
		MonitorEndpoint: monitorEndpoint,
		RemoteRecords:   len(records),
	}
	challengeSeen := map[string]bool{}
	lastScannedAt := ""
	for _, item := range items {
		challengeSeen[item.Challenge.Key] = true
		if item.LastSyncedAt > lastScannedAt {
			lastScannedAt = item.LastSyncedAt
		}
		switch item.Status {
		case "submitted":
			summary.Submitted++
		case "missing":
			summary.Missing++
			summary.MissingChallengeIDs = append(summary.MissingChallengeIDs, fmt.Sprintf("%s:%s", item.Account, item.Challenge.ChallengeID))
		case "needs_fix":
			summary.NeedsFix++
		case "sync_failed":
			summary.NeedsFix++
		}
		if len(item.Warnings) > 0 {
			summary.Warnings++
		}
	}
	for _, account := range accounts {
		if account.SyncStatus == "synced" {
			summary.SyncedAccounts++
		} else {
			summary.FailedAccounts++
		}
	}
	for _, record := range records {
		if record.MatchStatus != "matched" {
			summary.UnmatchedRecords++
		}
	}
	summary.TotalChallenges = len(challengeSeen)
	summary.LastScannedAt = lastScannedAt
	sort.Strings(summary.MissingChallengeIDs)
	return summary
}

func summarizeAccounts(items []domain.Item, syncs map[string]accountSync) []domain.AccountSummary {
	byAccount := make(map[string]*domain.AccountSummary)
	order := make([]string, 0)
	for _, item := range items {
		summary := byAccount[item.Account]
		if summary == nil {
			sync := syncs[item.Account]
			summary = &domain.AccountSummary{
				Account:        item.Account,
				SubmitIdentity: item.SubmitIdentity,
				SyncStatus:     fallbackString(sync.Status, "failed"),
				SyncMessage:    sync.Message,
				LastSyncedAt:   sync.LastSyncedAt,
				RemoteRecords:  sync.RemoteRecords,
			}
			byAccount[item.Account] = summary
			order = append(order, item.Account)
		}
		summary.Total++
		switch item.Status {
		case "submitted":
			summary.Submitted++
		case "missing":
			summary.Missing++
		case "needs_fix", "sync_failed":
			summary.NeedsFix++
		}
		if len(item.Warnings) > 0 {
			summary.Warnings++
		}
		if item.PlatformSolvedAt > summary.LastSolvedAt {
			summary.LastSolvedAt = item.PlatformSolvedAt
		}
	}
	sort.Strings(order)
	results := make([]domain.AccountSummary, 0, len(order))
	for _, account := range order {
		results = append(results, *byAccount[account])
	}
	return results
}

func fallbackAccountStates(challenges []domain.Challenge) []challengeAccountState {
	states := make([]challengeAccountState, 0, len(challenges))
	for _, challenge := range challenges {
		states = append(states, challengeAccountState{
			ChallengeKey:   challenge.Key,
			Account:        "default",
			PlatformSolved: true,
		})
	}
	return states
}

func identityForAccount(identities map[string]accountIdentity, account string) accountIdentity {
	if identity, ok := identities[account]; ok {
		if identity.SubmitIdentity == "" {
			identity.SubmitIdentity = fallbackString(identity.Username, identity.Name)
		}
		return identity
	}
	return accountIdentity{Name: account, Username: account, SubmitIdentity: account, Enabled: true}
}

func expectedFilename(identity string, title string) string {
	return safeFilenamePart(identity) + "-" + safeFilenamePart(title) + ".docx"
}

func filenamesMatch(actual string, expected string) bool {
	return strings.TrimSpace(filepath.Base(actual)) == strings.TrimSpace(expected)
}

func titleMatches(actual string, expected string) bool {
	return normalizeTitle(actual) == normalizeTitle(expected)
}

func normalizeTitle(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".docx")
	value = strings.TrimSpace(value)
	return strings.ToLower(value)
}

func afterFirstDash(value string) string {
	if idx := strings.Index(value, "-"); idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func safeFilenamePart(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(value)
}

func (s *Service) challengeDBPath() string {
	if _, err := os.Stat(s.layout.AppDatabasePath); err == nil {
		return s.layout.AppDatabasePath
	}
	return ""
}

func requiresWriteupSection(section string) bool {
	normalized := strings.ToLower(strings.TrimSpace(section))
	switch normalized {
	case "challenges", "challenge", "practice", "practices", "练武题", "arena", "arenas", "擂台题":
		return true
	default:
		return strings.Contains(normalized, "arena") || strings.Contains(normalized, "擂台") || strings.Contains(normalized, "练武")
	}
}

func sectionLabel(section string) string {
	normalized := strings.ToLower(strings.TrimSpace(section))
	if normalized == "challenges" || normalized == "challenge" || normalized == "practice" || strings.Contains(normalized, "练武") {
		return "练武题"
	}
	if normalized == "arena" || normalized == "arenas" || strings.Contains(normalized, "擂台") {
		return "擂台题"
	}
	if strings.TrimSpace(section) == "" {
		return "未知赛道"
	}
	return section
}

func fallbackString(primary string, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	results := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		results = append(results, value)
	}
	return results
}

func sortChallenges(items []domain.Challenge) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Section == items[j].Section {
			return numericPrefix(items[i].ChallengeID) < numericPrefix(items[j].ChallengeID)
		}
		return items[i].Section < items[j].Section
	})
}

func challengeID(name string) string {
	parts := strings.SplitN(name, "_", 2)
	return strings.TrimSpace(parts[0])
}

func challengeTitle(name string) string {
	parts := strings.SplitN(name, "_", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(name)
}

func numericPrefix(value string) int {
	var result int
	for _, r := range value {
		if r < '0' || r > '9' {
			break
		}
		result = result*10 + int(r-'0')
	}
	if result == 0 {
		return 999999
	}
	return result
}

func firstExisting(dir string, names ...string) string {
	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func looksLikeLoginPage(urlText string, body string) bool {
	parsed, err := url.Parse(urlText)
	if err == nil && strings.TrimRight(parsed.Path, "/") == "/login" {
		return true
	}
	return strings.Contains(body, "<h2 class=\"form-title\">登录</h2>") || (strings.Contains(body, "用户名/邮箱") && strings.Contains(body, "id=\"password\""))
}

func responseHint(body string) string {
	text := strings.TrimSpace(htmlTagPattern.ReplaceAllString(body, " "))
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > 240 {
		return string([]rune(text)[:240])
	}
	return text
}
