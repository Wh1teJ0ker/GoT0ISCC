package tracks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	domain "got0iscc/desktop/internal/domain/tracks"
	"got0iscc/desktop/internal/platform/runtime"

	_ "github.com/mattn/go-sqlite3"
)

type Service struct {
	layout runtime.Layout
}

func NewService(layout runtime.Layout) *Service {
	return &Service{layout: layout}
}

func (s *Service) Practice(_ context.Context) (domain.Payload, error) {
	return s.loadFromSQLite("challenges", "练武题", s.layout.AppDatabasePath)
}

func (s *Service) Arena(_ context.Context) (domain.Payload, error) {
	return s.loadFromSQLite("arena", "擂台题", s.layout.AppDatabasePath)
}

func (s *Service) Theory(_ context.Context) (domain.Payload, error) {
	payload, err := s.loadFromSQLite("challenges", "练武题", s.layout.AppDatabasePath)
	if err != nil {
		return domain.Payload{}, err
	}
	return filterPayload(payload, "theory", "理论题", isTheoryChallenge), nil
}

func (s *Service) Combat(_ context.Context) (domain.Payload, error) {
	payload, err := s.loadFromSQLite("challenges", "练武题", s.layout.AppDatabasePath)
	if err != nil {
		return domain.Payload{}, err
	}
	return filterPayload(payload, "combat", "实战题", isCombatChallenge), nil
}

func (s *Service) loadFromSQLite(section string, displayName string, dbPath string) (domain.Payload, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return domain.Payload{}, fmt.Errorf("未找到共享题库数据库: %w", err)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&immutable=1", dbPath))
	if err != nil {
		return domain.Payload{}, err
	}
	defer db.Close()

	challenges, err := loadChallengesFromDB(db, section)
	if err != nil {
		return domain.Payload{}, err
	}
	accountRows, err := loadChallengeAccountsFromDB(db, section)
	if err != nil {
		return domain.Payload{}, err
	}
	challenges, err = s.mergeLocalChallenges(section, challenges)
	if err != nil {
		return domain.Payload{}, err
	}

	payload := s.assemblePayload(section, displayName, "sqlite", dbPath, challenges, accountRows)
	return payload, nil
}

func (s *Service) loadFromArchive(section string, displayName string, archivePath string) (domain.Payload, error) {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return domain.Payload{}, err
	}
	var snapshot struct {
		Challenges        []challengeRecord        `json:"challenges"`
		ChallengeAccounts []challengeAccountRecord `json:"challenge_accounts"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return domain.Payload{}, err
	}

	filteredChallenges := make([]challengeRecord, 0, len(snapshot.Challenges))
	for _, item := range snapshot.Challenges {
		if strings.TrimSpace(item.Section) == section {
			filteredChallenges = append(filteredChallenges, item)
		}
	}
	filteredAccounts := make([]challengeAccountRecord, 0, len(snapshot.ChallengeAccounts))
	for _, item := range snapshot.ChallengeAccounts {
		if strings.TrimSpace(item.Data.Section) == section || strings.HasPrefix(strings.TrimSpace(item.ChallengeKey), section+":") {
			filteredAccounts = append(filteredAccounts, item)
		}
	}

	payload := s.assemblePayload(section, displayName, "archive", archivePath, filteredChallenges, filteredAccounts)
	return payload, nil
}

type challengeRecord struct {
	Key                string                `json:"key"`
	ChallengeID        string                `json:"challenge_id"`
	Section            string                `json:"section"`
	Title              string                `json:"title"`
	Category           string                `json:"category"`
	ChallengeKind      string                `json:"challenge_kind"`
	ExpectsAttachments bool                  `json:"expects_attachments"`
	ExpectsRemote      bool                  `json:"expects_remote"`
	AssetWarnings      []string              `json:"asset_warnings"`
	DirPath            string                `json:"dir_path"`
	DetailURL          string                `json:"detail_url"`
	DescriptionPath    string                `json:"description_path"`
	SolvePath          string                `json:"solve_path"`
	SolveScript        string                `json:"solve_script"`
	RemoteSummaryPath  string                `json:"remote_summary_path"`
	RemoteTargets      []domain.RemoteTarget `json:"remote_targets"`
	Attachments        []domain.Attachment   `json:"attachments"`
	UpdatedAt          string                `json:"updated_at"`
	Changed            bool                  `json:"changed"`
}

type localChallengeMeta struct {
	ChallengeID        string `json:"challenge_id"`
	Section            string `json:"section"`
	Title              string `json:"title"`
	Category           string `json:"category"`
	ChallengeKind      string `json:"challenge_kind"`
	ExpectsAttachments bool   `json:"expects_attachments"`
	ExpectsRemote      bool   `json:"expects_remote"`
	DetailURL          string `json:"detail_url"`
	DirName            string `json:"dir_name"`
	DirPath            string `json:"dir_path"`
	UpdatedAt          string `json:"updated_at"`
}

type challengeAccountRecord struct {
	ChallengeKey string               `json:"challenge_key"`
	AccountName  string               `json:"account_name"`
	Data         challengeAccountData `json:"data"`
	UpdatedAt    string               `json:"updated_at"`
}

type challengeAccountData struct {
	ChallengeID        string                `json:"challenge_id"`
	Section            string                `json:"section"`
	Title              string                `json:"title"`
	Category           string                `json:"category"`
	LastFlag           string                `json:"last_flag"`
	LastSeenAt         string                `json:"last_seen_at"`
	LastSubmitOK       bool                  `json:"last_submit_ok"`
	LastSubmittedAt    string                `json:"last_submitted_at"`
	PlatformSolved     bool                  `json:"platform_solved"`
	PlatformSolvedAt   string                `json:"platform_solved_at"`
	Changed            bool                  `json:"changed"`
	ChallengeKind      string                `json:"challenge_kind"`
	ExpectsAttachments bool                  `json:"expects_attachments"`
	ExpectsRemote      bool                  `json:"expects_remote"`
	AssetWarnings      []string              `json:"asset_warnings"`
	RemoteTargets      []domain.RemoteTarget `json:"remote_targets"`
	Attachments        []domain.Attachment   `json:"attachments"`
	Solver             solverState           `json:"solver"`
	Submission         submissionState       `json:"submission"`
	PlatformSubmission platformSubmission    `json:"platform_submission"`
}

type solverState struct {
	Status string `json:"status"`
}

type submissionState struct {
	Accepted bool   `json:"accepted"`
	Skipped  bool   `json:"skipped"`
	Message  string `json:"message"`
}

type platformSubmission struct {
	SubmittedAt string `json:"submitted_at"`
}

func loadChallengesFromDB(db *sql.DB, section string) ([]challengeRecord, error) {
	rows, err := db.Query(`
SELECT challenge_key, challenge_id, section_name, title, category, challenge_kind,
       expects_attachments, expects_remote, asset_warnings_json, dir_path, detail_url,
       description_path, remote_summary_path, remote_targets_json, attachments_json, updated_at, changed
FROM challenges
WHERE section_name = ?
ORDER BY challenge_id + 0 ASC, challenge_key ASC
`, section)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]challengeRecord, 0)
	for rows.Next() {
		var item challengeRecord
		var warningsJSON string
		var remoteJSON string
		var attachmentJSON string
		var expectsAttachments int
		var expectsRemote int
		var changed int
		if err := rows.Scan(
			&item.Key,
			&item.ChallengeID,
			&item.Section,
			&item.Title,
			&item.Category,
			&item.ChallengeKind,
			&expectsAttachments,
			&expectsRemote,
			&warningsJSON,
			&item.DirPath,
			&item.DetailURL,
			&item.DescriptionPath,
			&item.RemoteSummaryPath,
			&remoteJSON,
			&attachmentJSON,
			&item.UpdatedAt,
			&changed,
		); err != nil {
			return nil, err
		}
		item.ExpectsAttachments = expectsAttachments == 1
		item.ExpectsRemote = expectsRemote == 1
		item.Changed = changed == 1
		_ = json.Unmarshal([]byte(warningsJSON), &item.AssetWarnings)
		_ = json.Unmarshal([]byte(remoteJSON), &item.RemoteTargets)
		_ = json.Unmarshal([]byte(attachmentJSON), &item.Attachments)
		item.SolvePath = firstExisting(item.DirPath, "solve.py", "solver.py", "exp.py", "exploit.py", "poc.py")
		item.SolveScript = readTextIfExists(item.SolvePath)
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadChallengeAccountsFromDB(db *sql.DB, section string) ([]challengeAccountRecord, error) {
	rows, err := db.Query(`
SELECT challenge_key, account_name, data_json, updated_at
FROM challenge_accounts
WHERE challenge_key LIKE ?
ORDER BY challenge_key ASC, account_name ASC
`, section+":%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]challengeAccountRecord, 0)
	for rows.Next() {
		var item challengeAccountRecord
		var rawJSON string
		if err := rows.Scan(&item.ChallengeKey, &item.AccountName, &rawJSON, &item.UpdatedAt); err != nil {
			return nil, err
		}
		rawJSON = strings.TrimSpace(rawJSON)
		if rawJSON == "" {
			item.Data = challengeAccountData{}
			items = append(items, item)
			continue
		}
		if err := json.Unmarshal([]byte(rawJSON), &item.Data); err != nil {
			item.Data = challengeAccountData{
				Section: section,
			}
			items = append(items, item)
			continue
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) mergeLocalChallenges(section string, base []challengeRecord) ([]challengeRecord, error) {
	entries, err := os.ReadDir(s.layout.ChallengesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return base, nil
		}
		return nil, err
	}

	index := make(map[string]int, len(base))
	for i := range base {
		index[base[i].Key] = i
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(s.layout.ChallengesRoot, entry.Name())
		_ = ensureLocalChallengeMeta(section, dirPath, entry.Name())
		record, ok := inspectLocalChallengeDir(section, dirPath, entry.Name())
		if !ok {
			continue
		}
		if idx, exists := index[record.Key]; exists {
			base[idx] = mergeChallengeRecord(base[idx], record)
			continue
		}
		index[record.Key] = len(base)
		base = append(base, record)
	}

	sort.Slice(base, func(i, j int) bool {
		return numericThenString(base[i].ChallengeID, base[j].ChallengeID, base[i].Key, base[j].Key)
	})
	return base, nil
}

func inspectLocalChallengeDir(section string, dirPath string, dirName string) (challengeRecord, bool) {
	meta := readLocalChallengeMeta(filepath.Join(dirPath, "challenge.meta.json"))
	if strings.TrimSpace(meta.Section) != "" && strings.TrimSpace(meta.Section) != section {
		return challengeRecord{}, false
	}

	challengeID := firstNonEmpty(meta.ChallengeID, challengeIDFromDir(dirName))
	if challengeID == "" {
		return challengeRecord{}, false
	}
	title := firstNonEmpty(meta.Title, challengeTitleFromDir(dirName))
	key := section + ":" + challengeID
	if strings.TrimSpace(meta.Section) != "" {
		key = strings.TrimSpace(meta.Section) + ":" + challengeID
	}
	record := challengeRecord{
		Key:                key,
		ChallengeID:        challengeID,
		Section:            firstNonEmpty(meta.Section, section),
		Title:              title,
		Category:           strings.TrimSpace(meta.Category),
		ChallengeKind:      strings.TrimSpace(meta.ChallengeKind),
		ExpectsAttachments: meta.ExpectsAttachments,
		ExpectsRemote:      meta.ExpectsRemote,
		DirPath:            firstNonEmpty(meta.DirPath, dirPath),
		DetailURL:          strings.TrimSpace(meta.DetailURL),
		DescriptionPath:    firstExisting(dirPath, "description.md"),
		RemoteSummaryPath:  firstExisting(dirPath, "remote.txt"),
		UpdatedAt:          firstNonEmpty(meta.UpdatedAt, dirUpdatedAt(dirPath)),
		Changed:            false,
	}
	record.SolvePath = firstExisting(record.DirPath, "solve.py", "solver.py", "exp.py", "exploit.py", "poc.py")
	record.SolveScript = readTextIfExists(record.SolvePath)
	record.Attachments = scanLocalAttachments(filepath.Join(record.DirPath, "attachments"))
	record.ExpectsAttachments = record.ExpectsAttachments || len(record.Attachments) > 0
	record.ExpectsRemote = record.ExpectsRemote || record.RemoteSummaryPath != ""
	return record, record.Section == section
}

func readLocalChallengeMeta(path string) localChallengeMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return localChallengeMeta{}
	}
	var meta localChallengeMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return localChallengeMeta{}
	}
	return meta
}

func ensureLocalChallengeMeta(section string, dirPath string, dirName string) error {
	metaPath := filepath.Join(dirPath, "challenge.meta.json")
	if _, err := os.Stat(metaPath); err == nil {
		return nil
	}
	meta := localChallengeMeta{
		ChallengeID: challengeIDFromDir(dirName),
		Section:     section,
		Title:       challengeTitleFromDir(dirName),
		DirName:     dirName,
		DirPath:     dirPath,
		UpdatedAt:   dirUpdatedAt(dirPath),
	}
	if meta.ChallengeID == "" {
		return nil
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0o644)
}

func challengeIDFromDir(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	parts := strings.SplitN(name, "_", 2)
	return strings.TrimSpace(parts[0])
}

func challengeTitleFromDir(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	parts := strings.SplitN(name, "_", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func dirUpdatedAt(dir string) string {
	info, err := os.Stat(dir)
	if err != nil {
		return ""
	}
	return info.ModTime().Format("2006-01-02 15:04:05")
}

func scanLocalAttachments(dir string) []domain.Attachment {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	items := make([]domain.Attachment, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, domain.Attachment{
			Name:       entry.Name(),
			StoredName: entry.Name(),
			LocalPath:  fullPath,
			Size:       info.Size(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].StoredName) < strings.ToLower(items[j].StoredName)
	})
	return items
}

func mergeChallengeRecord(base challengeRecord, local challengeRecord) challengeRecord {
	base.Title = firstNonEmpty(base.Title, local.Title)
	base.Category = firstNonEmpty(base.Category, local.Category)
	base.ChallengeKind = firstNonEmpty(base.ChallengeKind, local.ChallengeKind)
	base.DirPath = firstNonEmpty(local.DirPath, base.DirPath)
	base.DetailURL = firstNonEmpty(base.DetailURL, local.DetailURL)
	base.DescriptionPath = firstNonEmpty(base.DescriptionPath, local.DescriptionPath)
	base.RemoteSummaryPath = firstNonEmpty(base.RemoteSummaryPath, local.RemoteSummaryPath)
	base.SolvePath = firstNonEmpty(base.SolvePath, local.SolvePath)
	base.SolveScript = firstNonEmpty(base.SolveScript, local.SolveScript)
	base.UpdatedAt = maxTS(base.UpdatedAt, local.UpdatedAt)
	base.ExpectsAttachments = base.ExpectsAttachments || local.ExpectsAttachments
	base.ExpectsRemote = base.ExpectsRemote || local.ExpectsRemote
	if len(base.Attachments) == 0 && len(local.Attachments) > 0 {
		base.Attachments = local.Attachments
	}
	return base
}

func (s *Service) assemblePayload(section string, displayName string, sourceType string, sourcePath string, challengeRows []challengeRecord, accountRows []challengeAccountRecord) domain.Payload {
	challengeMap := make(map[string]*domain.Challenge, len(challengeRows))
	accountSummaryMap := map[string]*domain.AccountSummary{}
	lastUpdated := ""

	for _, item := range challengeRows {
		challenge := &domain.Challenge{
			Key:                item.Key,
			ChallengeID:        item.ChallengeID,
			Section:            item.Section,
			Title:              item.Title,
			Category:           item.Category,
			Kind:               item.ChallengeKind,
			DetailURL:          item.DetailURL,
			DirPath:            item.DirPath,
			DescriptionPath:    item.DescriptionPath,
			SolvePath:          item.SolvePath,
			SolveScript:        item.SolveScript,
			RemoteSummaryPath:  item.RemoteSummaryPath,
			UpdatedAt:          item.UpdatedAt,
			Changed:            item.Changed,
			ExpectsAttachments: item.ExpectsAttachments,
			ExpectsRemote:      item.ExpectsRemote,
			AssetWarnings:      item.AssetWarnings,
			Attachments:        item.Attachments,
			RemoteTargets:      item.RemoteTargets,
			Accounts:           []domain.ChallengeAccount{},
		}
		challengeMap[item.Key] = challenge
		lastUpdated = maxTS(lastUpdated, item.UpdatedAt)
	}

	for _, row := range accountRows {
		challenge := challengeMap[row.ChallengeKey]
		if challenge == nil {
			challenge = &domain.Challenge{
				Key:                row.ChallengeKey,
				ChallengeID:        row.Data.ChallengeID,
				Section:            firstNonEmpty(row.Data.Section, section),
				Title:              row.Data.Title,
				Category:           row.Data.Category,
				Kind:               row.Data.ChallengeKind,
				DirPath:            challengeDirFromKey(s.layout.ChallengesRoot, row.ChallengeKey),
				UpdatedAt:          row.UpdatedAt,
				Changed:            row.Data.Changed,
				ExpectsAttachments: row.Data.ExpectsAttachments,
				ExpectsRemote:      row.Data.ExpectsRemote,
				AssetWarnings:      row.Data.AssetWarnings,
				Attachments:        row.Data.Attachments,
				RemoteTargets:      row.Data.RemoteTargets,
				Accounts:           []domain.ChallengeAccount{},
			}
			challenge.DescriptionPath = firstExisting(challenge.DirPath, "description.md")
			challenge.SolvePath = firstExisting(challenge.DirPath, "solve.py", "solver.py", "exp.py", "exploit.py", "poc.py")
			challenge.SolveScript = readTextIfExists(challenge.SolvePath)
			challenge.RemoteSummaryPath = firstExisting(challenge.DirPath, "remote.txt")
			challengeMap[row.ChallengeKey] = challenge
		}

		account := domain.ChallengeAccount{
			Account:           row.AccountName,
			Submitted:         row.Data.PlatformSolved || row.Data.Submission.Accepted || row.Data.Submission.Skipped,
			PlatformSolved:    row.Data.PlatformSolved,
			PlatformSolvedAt:  firstNonEmpty(row.Data.PlatformSolvedAt, row.Data.PlatformSubmission.SubmittedAt),
			LastSubmitOK:      row.Data.LastSubmitOK,
			LastSubmittedAt:   row.Data.LastSubmittedAt,
			LastSeenAt:        row.Data.LastSeenAt,
			Changed:           row.Data.Changed,
			SolverStatus:      row.Data.Solver.Status,
			SubmissionMessage: row.Data.Submission.Message,
			LastFlag:          row.Data.LastFlag,
			AttachmentCount:   len(row.Data.Attachments),
			RemoteTargetCount: len(row.Data.RemoteTargets),
			Warnings:          row.Data.AssetWarnings,
		}
		challenge.Accounts = append(challenge.Accounts, account)
		challenge.Submitted = challenge.Submitted || account.Submitted
		if account.Submitted {
			challenge.SubmittedAccountCount++
		}
		challenge.Changed = challenge.Changed || account.Changed
		if len(account.Warnings) > 0 {
			challenge.AssetWarnings = mergeUnique(challenge.AssetWarnings, account.Warnings)
		}
		if len(challenge.Attachments) == 0 && len(row.Data.Attachments) > 0 {
			challenge.Attachments = row.Data.Attachments
		}
		if len(challenge.RemoteTargets) == 0 && len(row.Data.RemoteTargets) > 0 {
			challenge.RemoteTargets = row.Data.RemoteTargets
		}
		lastUpdated = maxTS(lastUpdated, row.UpdatedAt)

		summary := accountSummaryMap[row.AccountName]
		if summary == nil {
			summary = &domain.AccountSummary{Name: row.AccountName}
			accountSummaryMap[row.AccountName] = summary
		}
		summary.TotalChallenges++
		if account.Submitted {
			summary.SolvedCount++
		} else {
			summary.PendingCount++
		}
		if account.Changed {
			summary.ChangedCount++
		}
		if len(account.Warnings) > 0 {
			summary.WarningCount++
		}
		summary.LastActiveAt = maxTS(summary.LastActiveAt, firstNonEmpty(account.LastSeenAt, row.UpdatedAt))
	}

	challenges := make([]domain.Challenge, 0, len(challengeMap))
	for _, item := range challengeMap {
		item.AttachmentVariants = buildAttachmentVariants(item.Attachments, item.Accounts)
		item.AttachmentMismatch = attachmentMismatch(item.AttachmentVariants)
		sort.Slice(item.Accounts, func(i, j int) bool {
			return strings.ToLower(item.Accounts[i].Account) < strings.ToLower(item.Accounts[j].Account)
		})
		challenges = append(challenges, *item)
	}
	sort.Slice(challenges, func(i, j int) bool {
		return numericThenString(challenges[i].ChallengeID, challenges[j].ChallengeID, challenges[i].Key, challenges[j].Key)
	})

	accountSummaries := make([]domain.AccountSummary, 0, len(accountSummaryMap))
	for _, item := range accountSummaryMap {
		accountSummaries = append(accountSummaries, *item)
	}
	sort.Slice(accountSummaries, func(i, j int) bool {
		return strings.ToLower(accountSummaries[i].Name) < strings.ToLower(accountSummaries[j].Name)
	})

	summary := domain.Summary{
		TotalChallenges: len(challenges),
		TotalAccounts:   len(accountSummaries),
		LastUpdatedAt:   lastUpdated,
	}
	for _, item := range challenges {
		if item.Submitted {
			summary.SolvedChallenges++
		} else {
			summary.PendingChallenges++
		}
		if item.Changed {
			summary.ChangedChallenges++
		}
		if len(item.AssetWarnings) > 0 {
			summary.WarningChallenges++
		}
		if item.AttachmentMismatch {
			summary.AttachmentMismatchChallenges++
		}
		if len(item.RemoteTargets) > 0 {
			summary.RemoteChallenges++
		}
		if len(item.Attachments) > 0 {
			summary.AttachmentChallenges++
		}
	}

	return domain.Payload{
		Section:     section,
		DisplayName: displayName,
		SourceType:  sourceType,
		SourcePath:  sourcePath,
		SnapshotAt:  lastUpdated,
		Summary:     summary,
		Accounts:    accountSummaries,
		Challenges:  challenges,
	}
}

func filterPayload(payload domain.Payload, section string, displayName string, keep func(domain.Challenge) bool) domain.Payload {
	filteredChallenges := make([]domain.Challenge, 0, len(payload.Challenges))
	accountSummaryMap := map[string]*domain.AccountSummary{}
	lastUpdated := strings.TrimSpace(payload.SnapshotAt)

	summary := domain.Summary{
		LastUpdatedAt: strings.TrimSpace(payload.SnapshotAt),
	}

	for _, item := range payload.Challenges {
		if !keep(item) {
			continue
		}
		filteredChallenges = append(filteredChallenges, item)
		summary.TotalChallenges++
		if item.Submitted {
			summary.SolvedChallenges++
		} else {
			summary.PendingChallenges++
		}
		if item.Changed {
			summary.ChangedChallenges++
		}
		if len(item.AssetWarnings) > 0 {
			summary.WarningChallenges++
		}
		if item.AttachmentMismatch {
			summary.AttachmentMismatchChallenges++
		}
		if len(item.RemoteTargets) > 0 {
			summary.RemoteChallenges++
		}
		if len(item.Attachments) > 0 {
			summary.AttachmentChallenges++
		}
		lastUpdated = maxTS(lastUpdated, item.UpdatedAt)

		for _, account := range item.Accounts {
			entry := accountSummaryMap[account.Account]
			if entry == nil {
				entry = &domain.AccountSummary{Name: account.Account}
				accountSummaryMap[account.Account] = entry
			}
			entry.TotalChallenges++
			if account.Submitted {
				entry.SolvedCount++
			} else {
				entry.PendingCount++
			}
			if account.Changed {
				entry.ChangedCount++
			}
			if len(account.Warnings) > 0 {
				entry.WarningCount++
			}
			entry.LastActiveAt = maxTS(entry.LastActiveAt, firstNonEmpty(account.LastSeenAt, item.UpdatedAt))
		}
	}

	accountSummaries := make([]domain.AccountSummary, 0, len(accountSummaryMap))
	for _, item := range accountSummaryMap {
		accountSummaries = append(accountSummaries, *item)
	}
	sort.Slice(accountSummaries, func(i, j int) bool {
		return strings.ToLower(accountSummaries[i].Name) < strings.ToLower(accountSummaries[j].Name)
	})

	summary.TotalAccounts = len(accountSummaries)
	summary.LastUpdatedAt = lastUpdated

	return domain.Payload{
		Section:     section,
		DisplayName: displayName,
		SourceType:  payload.SourceType,
		SourcePath:  payload.SourcePath,
		SnapshotAt:  lastUpdated,
		Summary:     summary,
		Accounts:    accountSummaries,
		Challenges:  filteredChallenges,
	}
}

func challengeDirFromKey(root string, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	id := strings.TrimSpace(parts[1])
	if id == "" {
		return ""
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	prefix := id + "_"
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) {
			return filepath.Join(root, entry.Name())
		}
	}
	return ""
}

func readTextIfExists(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
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

func isCombatChallenge(item domain.Challenge) bool {
	kind := strings.ToLower(strings.TrimSpace(item.Kind))
	if item.ExpectsRemote || len(item.RemoteTargets) > 0 {
		return true
	}
	return kind == "web" || kind == "pwn"
}

func isTheoryChallenge(item domain.Challenge) bool {
	return !isCombatChallenge(item)
}

func buildAttachmentVariants(attachments []domain.Attachment, accounts []domain.ChallengeAccount) []domain.AttachmentVariant {
	if len(attachments) == 0 {
		return nil
	}
	accountNames := make([]string, 0, len(accounts))
	for _, item := range accounts {
		accountNames = append(accountNames, item.Account)
	}
	variants := make([]domain.AttachmentVariant, 0, len(attachments))
	for _, item := range attachments {
		variants = append(variants, domain.AttachmentVariant{
			Name:       item.Name,
			StoredName: item.StoredName,
			URL:        item.URL,
			LocalPath:  item.LocalPath,
			SHA256:     item.SHA256,
			Size:       item.Size,
			Changed:    item.Changed,
			Accounts:   accountNames,
		})
	}
	return variants
}

func attachmentMismatch(variants []domain.AttachmentVariant) bool {
	byName := map[string]string{}
	for _, item := range variants {
		key := strings.TrimSpace(firstNonEmpty(item.StoredName, item.Name))
		hash := strings.TrimSpace(item.SHA256)
		if key == "" || hash == "" {
			continue
		}
		if prev, ok := byName[key]; ok && prev != hash {
			return true
		}
		byName[key] = hash
	}
	return false
}

func mergeUnique(base []string, extra []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(base)+len(extra))
	for _, item := range append(base, extra...) {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func maxTS(a string, b string) string {
	if strings.TrimSpace(b) > strings.TrimSpace(a) {
		return strings.TrimSpace(b)
	}
	return strings.TrimSpace(a)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func numericThenString(left string, right string, leftFallback string, rightFallback string) bool {
	leftInt, leftErr := strconv.Atoi(strings.TrimSpace(left))
	rightInt, rightErr := strconv.Atoi(strings.TrimSpace(right))
	if leftErr == nil && rightErr == nil && leftInt != rightInt {
		return leftInt < rightInt
	}
	if strings.TrimSpace(left) != strings.TrimSpace(right) {
		return strings.TrimSpace(left) < strings.TrimSpace(right)
	}
	return strings.TrimSpace(leftFallback) < strings.TrimSpace(rightFallback)
}
