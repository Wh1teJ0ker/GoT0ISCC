package combat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"got0iscc/desktop/internal/application/httpx"
	accountdomain "got0iscc/desktop/internal/domain/accounts"
	domain "got0iscc/desktop/internal/domain/combat"
	"got0iscc/desktop/internal/platform/runtime"

	"golang.org/x/net/html"
)

const (
	baseURL            = "https://iscc.isclab.org.cn"
	combatPath         = "/measure"
	combatListPath     = "/measures"
	combatSubmitPath   = "/measure/submit"
	loginPath          = "/login"
	requestTimeout     = 20 * time.Second
	allAccountsValue   = "__all__"
	defaultCombatUser  = "Wh1teJ0ker"
	defaultCombatPass  = "a111111"
	combatCacheMetaKey = "combat.remote_cache.v1"
)

type AccountRepository interface {
	ListAccounts(ctx context.Context) ([]accountdomain.Account, error)
	MetaValue(ctx context.Context, key string) (string, error)
	SetMetaValue(ctx context.Context, key string, value string) error
}

type Service struct {
	layout      runtime.Layout
	accountRepo AccountRepository
}

func NewService(layout runtime.Layout, accountRepo AccountRepository) *Service {
	return &Service{
		layout:      layout,
		accountRepo: accountRepo,
	}
}

func (s *Service) Snapshot(ctx context.Context) (domain.Payload, error) {
	if cached, err := s.readCache(ctx); err == nil && cached.SnapshotAt != "" {
		return cached, nil
	}
	return s.Sync(ctx)
}

func (s *Service) Sync(ctx context.Context) (domain.Payload, error) {
	accountName, username, password, err := s.defaultAccount(ctx)
	if err != nil {
		return domain.Payload{}, err
	}

	client, err := newClient()
	if err != nil {
		return domain.Payload{}, err
	}
	if err := login(client, username, password); err != nil {
		return domain.Payload{}, err
	}

	doc, htmlText, err := fetchHTML(client, baseURL+combatPath)
	if err != nil {
		return domain.Payload{}, err
	}

	challenges, err := s.fetchChallenges(client)
	if err != nil {
		return domain.Payload{}, err
	}

	payload := domain.Payload{
		Account:    accountName,
		Username:   username,
		SourceURL:  baseURL + combatPath,
		SnapshotAt: nowTS(),
		Nonce:      extractInputValue(doc, "nonce"),
		Summary: domain.Summary{
			LastUpdatedAt: nowTS(),
		},
		Submission: domain.SubmissionForm{
			Enabled:        true,
			Action:         combatSubmitPath,
			FlagField:      "key",
			NonceField:     "nonce",
			ChallengeField: "id",
			ChallengeID:    extractInputValue(doc, "chal-id"),
		},
		Challenges: challenges,
	}

	payload.Resources = extractResources(doc)
	payload.Stages = extractStages(doc)
	payload.Scoreboard = extractScoreboard(doc)
	payload.Notices = extractCombatNotices(htmlText)
	payload.Summary.StageCount = len(payload.Stages)
	payload.Summary.ResourceCount = len(payload.Resources)
	payload.Summary.ScoreboardCount = len(payload.Scoreboard)
	payload.Summary.ChallengeCount = len(payload.Challenges)
	payload.Summary.CacheStatus = "fresh"
	payload.Summary.CacheUpdatedAt = payload.SnapshotAt
	payload.Summary.UsingCache = false

	if err := s.saveCache(ctx, payload); err != nil {
		return domain.Payload{}, err
	}
	return payload, nil
}

func (s *Service) Submit(ctx context.Context, req domain.SubmitRequest) (domain.SubmitResponse, error) {
	flagValue := strings.TrimSpace(req.Flag)
	if flagValue == "" {
		return domain.SubmitResponse{}, errors.New("flag 不能为空")
	}

	ids := normalizeChallengeIDs(req.ChallengeIDs)
	if len(ids) == 0 {
		return domain.SubmitResponse{}, errors.New("至少选择一道实战题")
	}

	accounts, allAccounts, err := s.resolveSubmitAccounts(ctx, req.AccountName)
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	results := make([]domain.SubmitResult, 0, len(accounts)*len(ids))
	successCount := 0
	nonce := ""
	for _, account := range accounts {
		accountNonce, accountResults := s.submitForAccount(account, ids, flagValue)
		if nonce == "" {
			nonce = accountNonce
		}
		for _, item := range accountResults {
			if item.Success {
				successCount++
			}
			results = append(results, item)
		}
	}

	accountName := accounts[0].Name
	username := accounts[0].Username
	if allAccounts {
		accountName = "全部启用账号"
		username = fmt.Sprintf("%d 个账号", len(accounts))
	}

	return domain.SubmitResponse{
		AccountName:  accountName,
		Username:     username,
		Action:       combatSubmitPath,
		Nonce:        nonce,
		SubmittedAt:  nowTS(),
		Total:        len(results),
		SuccessCount: successCount,
		FailureCount: len(results) - successCount,
		Results:      results,
	}, nil
}

func (s *Service) submitForAccount(account accountdomain.Account, ids []string, flagValue string) (string, []domain.SubmitResult) {
	client, err := newClient()
	if err != nil {
		return "", failedResults(account, ids, err.Error(), nil)
	}
	if err := login(client, account.Username, account.Password); err != nil {
		return "", failedResults(account, ids, err.Error(), nil)
	}

	doc, _, err := fetchHTML(client, baseURL+combatPath)
	if err != nil {
		return "", failedResults(account, ids, err.Error(), nil)
	}
	nonce := extractInputValue(doc, "nonce")
	if strings.TrimSpace(nonce) == "" {
		return "", failedResults(account, ids, "未能获取实战题 nonce", nil)
	}

	challenges, err := s.fetchChallenges(client)
	if err != nil {
		return nonce, failedResults(account, ids, err.Error(), nil)
	}
	challengeMap := make(map[string]domain.Challenge, len(challenges))
	for _, item := range challenges {
		challengeMap[item.ID] = item
	}

	results := make([]domain.SubmitResult, 0, len(ids))
	for _, id := range ids {
		challenge := challengeMap[id]
		submitResult, submitErr := submitCombatFlag(client, id, flagValue, nonce)
		if submitErr != nil {
			results = append(results, domain.SubmitResult{
				AccountName:   account.Name,
				Username:      account.Username,
				ChallengeID:   id,
				ChallengeName: challenge.Name,
				VerifyMode:    challenge.VerifyMode,
				StatusCode:    0,
				Success:       false,
				Message:       submitErr.Error(),
				Raw:           "",
				SubmittedAt:   nowTS(),
			})
			continue
		}
		submitResult.AccountName = account.Name
		submitResult.Username = account.Username
		submitResult.ChallengeID = id
		submitResult.ChallengeName = challenge.Name
		submitResult.VerifyMode = challenge.VerifyMode
		results = append(results, submitResult)
	}
	return nonce, results
}

func failedResults(account accountdomain.Account, ids []string, message string, challengeMap map[string]domain.Challenge) []domain.SubmitResult {
	results := make([]domain.SubmitResult, 0, len(ids))
	for _, id := range ids {
		var challenge domain.Challenge
		if challengeMap != nil {
			challenge = challengeMap[id]
		}
		results = append(results, domain.SubmitResult{
			AccountName:   account.Name,
			Username:      account.Username,
			ChallengeID:   id,
			ChallengeName: challenge.Name,
			VerifyMode:    challenge.VerifyMode,
			Success:       false,
			Message:       message,
			SubmittedAt:   nowTS(),
		})
	}
	return results
}

func (s *Service) defaultAccount(ctx context.Context) (string, string, string, error) {
	accounts, err := s.accountRepo.ListAccounts(ctx)
	if err == nil {
		for _, item := range accounts {
			if !item.Enabled {
				continue
			}
			if strings.TrimSpace(item.Username) == "" || strings.TrimSpace(item.Password) == "" {
				continue
			}
			return item.Name, item.Username, item.Password, nil
		}
	}
	return defaultCombatUser, defaultCombatUser, defaultCombatPass, nil
}

func (s *Service) resolveAccount(ctx context.Context, accountName string) (accountdomain.Account, error) {
	name := strings.TrimSpace(accountName)
	if name == "" {
		return accountdomain.Account{}, errors.New("请选择提交账号")
	}
	accounts, err := s.accountRepo.ListAccounts(ctx)
	if err != nil {
		return accountdomain.Account{}, err
	}
	for _, item := range accounts {
		if item.Name != name {
			continue
		}
		if !item.Enabled {
			return accountdomain.Account{}, errors.New("该账号已停用")
		}
		if strings.TrimSpace(item.Username) == "" || strings.TrimSpace(item.Password) == "" {
			return accountdomain.Account{}, errors.New("该账号缺少用户名或密码")
		}
		return item, nil
	}
	return accountdomain.Account{}, errors.New("未找到对应账号")
}

func (s *Service) resolveSubmitAccounts(ctx context.Context, accountName string) ([]accountdomain.Account, bool, error) {
	name := strings.TrimSpace(accountName)
	if name == allAccountsValue {
		accounts, err := s.accountRepo.ListAccounts(ctx)
		if err != nil {
			return nil, false, err
		}
		results := make([]accountdomain.Account, 0, len(accounts))
		for _, item := range accounts {
			if !item.Enabled {
				continue
			}
			if strings.TrimSpace(item.Username) == "" || strings.TrimSpace(item.Password) == "" {
				continue
			}
			results = append(results, item)
		}
		if len(results) == 0 {
			return nil, true, errors.New("没有可提交的启用账号")
		}
		return results, true, nil
	}
	account, err := s.resolveAccount(ctx, name)
	if err != nil {
		return nil, false, err
	}
	return []accountdomain.Account{account}, false, nil
}

func (s *Service) fetchChallenges(client *http.Client) ([]domain.Challenge, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+combatListPath, nil)
	if err != nil {
		return nil, err
	}
	httpx.ApplyBrowserHeaders(req, baseURL+combatPath, false)
	listResp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer listResp.Body.Close()
	if listResp.StatusCode >= 400 {
		return nil, fmt.Errorf("实战题列表请求失败: %s", listResp.Status)
	}

	var payload struct {
		Game []struct {
			ID       int    `json:"id"`
			Category string `json:"category"`
			Value    int    `json:"value"`
		} `json:"game"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	items := make([]domain.Challenge, 0, len(payload.Game))
	for _, item := range payload.Game {
		detail, err := s.fetchChallengeDetail(client, item.ID)
		if err != nil {
			items = append(items, domain.Challenge{
				ID:          fmt.Sprintf("%d", item.ID),
				Name:        fmt.Sprintf("实战题 %d", item.ID),
				Category:    item.Category,
				Value:       item.Value,
				Description: err.Error(),
			})
			continue
		}
		if strings.TrimSpace(detail.Category) == "" {
			detail.Category = item.Category
		}
		if detail.Value == 0 {
			detail.Value = item.Value
		}
		items = append(items, detail)
	}
	return items, nil
}

func (s *Service) fetchChallengeDetail(client *http.Client, id int) (domain.Challenge, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/measure/%d", baseURL, id), nil)
	if err != nil {
		return domain.Challenge{}, err
	}
	httpx.ApplyBrowserHeaders(req, baseURL+combatPath, false)
	resp, err := client.Do(req)
	if err != nil {
		return domain.Challenge{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return domain.Challenge{}, fmt.Errorf("题目详情请求失败: %s", resp.Status)
	}

	var item struct {
		ID            int      `json:"id"`
		Name          string   `json:"name"`
		Category      string   `json:"category"`
		Description   string   `json:"description"`
		Files         []string `json:"files"`
		FlagSourceURL string   `json:"flag_source_url"`
		Value         int      `json:"value"`
		Solves        int      `json:"solves"`
		VerifyMode    string   `json:"verify_mode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return domain.Challenge{}, err
	}
	files := make([]string, 0, len(item.Files))
	for _, file := range item.Files {
		files = append(files, absolutize(file))
	}
	return domain.Challenge{
		ID:            fmt.Sprintf("%d", item.ID),
		Name:          item.Name,
		Category:      item.Category,
		Description:   strings.TrimSpace(item.Description),
		Files:         files,
		FlagSourceURL: strings.TrimSpace(item.FlagSourceURL),
		Value:         item.Value,
		Solves:        item.Solves,
		VerifyMode:    strings.TrimSpace(item.VerifyMode),
	}, nil
}

func submitCombatFlag(client *http.Client, challengeID string, flagValue string, nonce string) (domain.SubmitResult, error) {
	form := url.Values{}
	form.Set("id", challengeID)
	form.Set("key", flagValue)
	form.Set("nonce", nonce)

	req, err := http.NewRequest(http.MethodPost, baseURL+combatSubmitPath, strings.NewReader(form.Encode()))
	if err != nil {
		return domain.SubmitResult{}, err
	}
	httpx.ApplyBrowserHeaders(req, baseURL+combatPath, true)

	resp, err := client.Do(req)
	if err != nil {
		return domain.SubmitResult{}, err
	}
	defer resp.Body.Close()

	var payload struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Success bool   `json:"success"`
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.SubmitResult{}, fmt.Errorf("读取提交响应失败: %w", err)
	}
	rawBody := strings.TrimSpace(string(bodyBytes))
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return domain.SubmitResult{}, fmt.Errorf("解析提交响应失败: %w", err)
	}

	success := payload.Success || strings.EqualFold(payload.Status, "success")
	message := strings.TrimSpace(payload.Message)
	if message == "" {
		if success {
			message = "提交成功"
		} else if strings.TrimSpace(payload.Status) != "" {
			message = strings.TrimSpace(payload.Status)
		} else {
			message = "提交失败"
		}
	}

	return domain.SubmitResult{
		StatusCode:  resp.StatusCode,
		Success:     success,
		Message:     message,
		Raw:         firstNonEmpty(rawBody, strings.TrimSpace(payload.Message), strings.TrimSpace(payload.Status)),
		SubmittedAt: nowTS(),
	}, nil
}

func normalizeChallengeIDs(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func newClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: requestTimeout,
		Jar:     jar,
	}, nil
}

func login(client *http.Client, username string, password string) error {
	form := strings.NewReader("name=" + url.QueryEscape(username) + "&password=" + url.QueryEscape(password))
	req, err := http.NewRequest(http.MethodPost, baseURL+loginPath, form)
	if err != nil {
		return err
	}
	httpx.ApplyBrowserHeaders(req, baseURL+"/", true)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("登录失败: %s", resp.Status)
	}
	return nil
}

func fetchHTML(client *http.Client, url string) (*html.Node, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	httpx.ApplyBrowserHeaders(req, baseURL+"/", false)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("请求失败: %s", resp.Status)
	}
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, "", err
	}
	var b strings.Builder
	renderText(doc, &b)
	return doc, b.String(), nil
}

func extractInputValue(root *html.Node, name string) string {
	var result string
	walk(root, func(node *html.Node) {
		if result != "" || node.Type != html.ElementNode || node.Data != "input" {
			return
		}
		if attr(node, "name") == name || attr(node, "id") == name {
			result = attr(node, "value")
		}
	})
	return strings.TrimSpace(result)
}

func extractResources(root *html.Node) []domain.Resource {
	var items []domain.Resource
	walk(root, func(node *html.Node) {
		if node.Type != html.ElementNode || node.Data != "a" {
			return
		}
		href := strings.TrimSpace(attr(node, "href"))
		label := strings.TrimSpace(textContent(node))
		if href == "" || label == "" {
			return
		}
		if strings.Contains(label, "VPN") || strings.Contains(label, "网盘") || strings.Contains(label, "直播") || strings.Contains(label, "靶场") {
			items = append(items, domain.Resource{Label: label, URL: absolutize(href)})
		}
	})
	return dedupeResources(items)
}

func extractStages(root *html.Node) []domain.Stage {
	var stages []domain.Stage
	var current *domain.Stage
	walk(root, func(node *html.Node) {
		if node.Type != html.ElementNode {
			return
		}
		txt := strings.TrimSpace(textContent(node))
		if txt == "" {
			return
		}
		if node.Data == "strong" && (strings.Contains(txt, "阶段一") || strings.Contains(txt, "阶段二") || strings.Contains(txt, "阶段三")) {
			stages = append(stages, domain.Stage{Title: txt})
			current = &stages[len(stages)-1]
			return
		}
		if current != nil && (node.Data == "p" || node.Data == "div" || node.Data == "br") {
			if txt != current.Title && len(txt) > 8 {
				current.Description = append(current.Description, txt)
			}
		}
	})
	return compactStages(stages)
}

func extractScoreboard(root *html.Node) []domain.ScoreEntry {
	var entries []domain.ScoreEntry
	var inFirstTable bool
	walk(root, func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "table" && !inFirstTable {
			inFirstTable = true
			for tr := node.FirstChild; tr != nil; tr = tr.NextSibling {
				if tr.Type != html.ElementNode || tr.Data != "tr" {
					continue
				}
				cells := tableRowTexts(tr)
				if len(cells) == 3 && cells[0] != "队伍名" {
					entries = append(entries, domain.ScoreEntry{
						Team:     cells[0],
						PassedAt: cells[1],
						Score:    cells[2],
					})
				}
			}
		}
	})
	return entries
}

func extractCombatNotices(text string) []string {
	lines := strings.Split(text, "\n")
	notes := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(strings.Join(strings.Fields(line), " "))
		if line == "" {
			continue
		}
		if strings.Contains(line, "评分规则") || strings.Contains(line, "奖金") || strings.Contains(line, "Windows 10") {
			notes = append(notes, line)
		}
	}
	return dedupeStrings(notes)
}

func walk(node *html.Node, fn func(*html.Node)) {
	fn(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walk(child, fn)
	}
}

func attr(node *html.Node, key string) string {
	for _, item := range node.Attr {
		if item.Key == key {
			return item.Val
		}
	}
	return ""
}

func textContent(node *html.Node) string {
	var parts []string
	walk(node, func(current *html.Node) {
		if current.Type == html.TextNode {
			value := strings.TrimSpace(current.Data)
			if value != "" {
				parts = append(parts, value)
			}
		}
	})
	return strings.Join(parts, " ")
}

func tableRowTexts(node *html.Node) []string {
	values := make([]string, 0, 3)
	for td := node.FirstChild; td != nil; td = td.NextSibling {
		if td.Type == html.ElementNode && (td.Data == "td" || td.Data == "th") {
			values = append(values, strings.TrimSpace(textContent(td)))
		}
	}
	return values
}

func renderText(node *html.Node, b *strings.Builder) {
	if node.Type == html.TextNode {
		text := strings.TrimSpace(node.Data)
		if text != "" {
			b.WriteString(text)
			b.WriteString("\n")
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderText(child, b)
	}
}

func compactStages(items []domain.Stage) []domain.Stage {
	result := make([]domain.Stage, 0, len(items))
	for _, item := range items {
		item.Description = dedupeStrings(item.Description)
		if item.Title == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func dedupeResources(items []domain.Resource) []domain.Resource {
	seen := map[string]bool{}
	result := make([]domain.Resource, 0, len(items))
	for _, item := range items {
		key := item.Label + "|" + item.URL
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func dedupeStrings(items []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func absolutize(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return baseURL + path
}

func firstNonEmpty(values ...string) string {
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return ""
}

func nowTS() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func (s *Service) readCache(ctx context.Context) (domain.Payload, error) {
	if s.accountRepo == nil {
		return domain.Payload{}, nil
	}
	raw, err := s.accountRepo.MetaValue(ctx, combatCacheMetaKey)
	if err != nil {
		return domain.Payload{}, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return domain.Payload{}, nil
	}

	var payload domain.Payload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return domain.Payload{}, nil
	}
	if strings.TrimSpace(payload.SnapshotAt) == "" {
		return domain.Payload{}, nil
	}
	if payload.Summary.LastUpdatedAt == "" {
		payload.Summary.LastUpdatedAt = payload.SnapshotAt
	}
	payload.Summary.CacheStatus = "cached"
	payload.Summary.CacheUpdatedAt = payload.SnapshotAt
	payload.Summary.UsingCache = true
	return payload, nil
}

func (s *Service) saveCache(ctx context.Context, payload domain.Payload) error {
	if s.accountRepo == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.accountRepo.SetMetaValue(ctx, combatCacheMetaKey, string(data))
}
