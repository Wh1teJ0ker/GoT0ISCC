package theory

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"got0iscc/desktop/internal/application/httpx"
	accountsdomain "got0iscc/desktop/internal/domain/accounts"
	domain "got0iscc/desktop/internal/domain/theory"
	"got0iscc/desktop/internal/platform/runtime"
	sqlitestore "got0iscc/desktop/internal/platform/storage/sqlite"

	"golang.org/x/net/html"
)

const (
	theoryBaseURL           = "https://iscc.isclab.org.cn"
	theoryLoginPath         = "/login"
	theoryPaperPath         = "/paper"
	defaultTheoryUser       = "Wh1teJ0ker"
	metaTheoryAccountSelect = "theory.selected_account"
	theoryLoginCooldown     = 3 * time.Second
	theoryFetchCooldown     = 2 * time.Second
	theorySubmitCooldown    = 4 * time.Second
	theoryQuestionCooldown  = 5 * time.Second
	theorySessionIdleTTL    = 10 * time.Minute
)

const (
	automationLocalConfidenceThreshold = 0.86
	automationAIConfidenceThreshold    = 0.80
	automationAIMaxAttempts            = 3
	automationAIRetryDelay             = 2 * time.Second
	automationAIHeartbeatInterval      = 5 * time.Second
)

var theoryScorePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:当前)?(?:得分|分数|积分|成绩)\s*[:：]?\s*([0-9]+(?:\.[0-9]+)?)\s*(?:/|／|分)?\s*([0-9]+(?:\.[0-9]+)?)?`),
	regexp.MustCompile(`(?:得分|分数|积分|成绩)\s*(?:是|为)\s*([0-9]+(?:\.[0-9]+)?)\s*分?`),
	regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*(?:/|／)\s*([0-9]+(?:\.[0-9]+)?)\s*分?`),
	regexp.MustCompile(`(?:正确|答对|已完成|完成)\s*[:：]?\s*([0-9]+)\s*(?:道|题)?\s*(?:/|／|共|总计)?\s*([0-9]+)?`),
	regexp.MustCompile(`(?:第|进度)\s*([0-9]+)\s*(?:/|／)\s*([0-9]+)`),
}

var theoryQuestionNumberPatterns = []*regexp.Regexp{
	regexp.MustCompile(`第\s*([0-9]+)\s*题`),
	regexp.MustCompile(`(?:当前|已到|进度|题号)\s*[:：]?\s*([0-9]+)\s*(?:/|／|题)?`),
}

var errTheorySessionInvalid = errors.New("理论题账号未登录或登录失效，请检查账号密码后刷新远端缓存")

type Repository interface {
	ReplaceTheoryQuestions(ctx context.Context, items []domain.ReviewItem, source string) (domain.ReviewDashboard, error)
	TheoryReviewDashboard(ctx context.Context) (domain.ReviewDashboard, error)
	ListTheoryReviewItems(ctx context.Context, limit int) (domain.ReviewListResponse, error)
	ListTheoryReviewItemsAll(ctx context.Context) ([]domain.ReviewItem, error)
	ListTheorySearchableItems(ctx context.Context) ([]domain.ReviewItem, error)
	CaptureTheoryQuestion(ctx context.Context, captured domain.CapturedQuestion, sourceURL string, account string) (domain.CapturedQuestion, error)
	SaveTheoryReviewDecision(ctx context.Context, decision domain.ReviewDecision) (domain.ReviewItem, error)
	TheoryReviewItemByHash(ctx context.Context, hash string) (domain.ReviewItem, error)
	ListAccounts(ctx context.Context) ([]accountsdomain.Account, error)
	MetaValue(ctx context.Context, key string) (string, error)
	SetMetaValue(ctx context.Context, key string, value string) error
	LoadTheoryRuntimeSnapshot(ctx context.Context, account string) (domain.Payload, domain.CacheStatus, error)
	SaveTheoryRuntimeSnapshot(ctx context.Context, account string, username string, payload domain.Payload, cache domain.CacheStatus) error
	ListTheoryCapturedQuestions(ctx context.Context, account string, limit int) ([]domain.CapturedQuestionRecord, error)
	ListTheoryRuntimeSnapshots(ctx context.Context) ([]domain.RuntimeSnapshotRecord, error)
}

type Service struct {
	layout        runtime.Layout
	repo          Repository
	sessionMu     sync.Mutex
	sessions      map[string]*theorySession
	reviewMu      sync.Mutex
	reviewRun     *aiReviewRun
	automationMu  sync.Mutex
	automationRun *automationRun
}

type theorySession struct {
	accountName string
	username    string
	password    string
	client      *http.Client
	headers     httpx.BrowserHeaderProfile
	lastUsedAt  time.Time
}

type aiReviewRun struct {
	cancel context.CancelFunc
	status domain.AIReviewStatus
}

type automationRun struct {
	cancel context.CancelFunc
	status domain.AutomationStatus
}

func NewService(layout runtime.Layout, repo Repository) *Service {
	return &Service{
		layout:   layout,
		repo:     repo,
		sessions: make(map[string]*theorySession),
	}
}

func (s *Service) EnsureLegacyMerged(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	return s.mergeLegacyTheorySQLite(ctx)
}

func (s *Service) PrimeBank(ctx context.Context) error {
	bank, err := loadBankStore(s.layout.TheoryBankPath)
	if err != nil {
		return err
	}
	if s.repo == nil {
		return nil
	}
	items := make([]domain.ReviewItem, 0, len(bank.items))
	for _, item := range bank.items {
		sourceKind := sourceKindFromRef(bank.sourcePath)
		reviewStatus := "approved"
		needsReview := false
		reviewReason := ""
		if len(item.CorrectOptions) == 0 || len(item.CorrectTexts) == 0 {
			needsReview = true
			reviewStatus = "pending"
			reviewReason = "题库答案不完整，等待人工复核"
		}
		items = append(items, domain.ReviewItem{
			Question:           item.Question,
			NormalizedQuestion: item.NormalizedQuestion,
			SelectionType:      selectionTypeFromOptions(item.CorrectOptions),
			SourceKind:         sourceKind,
			SourceRef:          bank.sourcePath,
			Options:            item.Options,
			AnswerKeys:         append([]string(nil), item.CorrectOptions...),
			AnswerTexts:        append([]string(nil), item.CorrectTexts...),
			NeedsReview:        needsReview,
			ReviewStatus:       reviewStatus,
			ReviewReason:       reviewReason,
			Confidence:         1,
			QuestionHash:       theoryQuestionHash(item.Question, item.Options),
		})
	}
	_, err = s.repo.ReplaceTheoryQuestions(ctx, items, s.layout.TheoryBankPath)
	return err
}

func (s *Service) mergeLegacyTheorySQLite(ctx context.Context) error {
	legacyPath := legacyTheoryDBPath(s.layout.AppDataRoot, s.layout.AppDatabasePath)
	if legacyPath == "" {
		return nil
	}
	if _, statErr := os.Stat(legacyPath); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		return statErr
	}

	legacyStore, err := sqlitestore.Open(legacyPath)
	if err != nil {
		return err
	}
	defer legacyStore.Close()

	items, err := legacyStore.ListTheoryReviewItemsAll(ctx)
	if err != nil {
		return err
	}
	if len(items) > 0 {
		if _, err := s.repo.ReplaceTheoryQuestions(ctx, items, legacyPath); err != nil {
			return err
		}
	}

	if legacyMeta, err := loadLegacyTheoryMeta(ctx, legacyStore); err == nil {
		for key, value := range legacyMeta {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if setErr := s.repo.SetMetaValue(ctx, key, value); setErr != nil {
				return setErr
			}
		}
	}

	return removeLegacyTheorySQLiteFiles(legacyPath)
}

func (s *Service) loadMergedBank(ctx context.Context) (*bankStore, error) {
	if s.repo == nil {
		return loadBankStore(s.layout.TheoryBankPath)
	}

	reviewItems, err := s.repo.ListTheorySearchableItems(ctx)
	if err != nil {
		return nil, err
	}
	dashboard, err := s.repo.TheoryReviewDashboard(ctx)
	if err != nil {
		return nil, err
	}

	merged := make([]bankIndexedItem, 0, len(reviewItems))
	duplicateBuckets := map[string]int{}
	for _, review := range reviewItems {
		indexed := indexedItemFromReview(review)
		merged = append(merged, indexed)
		if indexed.CompactQuestion != "" {
			duplicateBuckets[indexed.CompactQuestion]++
		}
	}

	duplicateGroups := 0
	multiAnswerCount := 0
	for _, count := range duplicateBuckets {
		if count > 1 {
			duplicateGroups++
		}
	}
	for index := range merged {
		if duplicateBuckets[merged[index].CompactQuestion] > 1 {
			merged[index].DuplicateGroup = merged[index].CompactQuestion
		}
		if merged[index].MultiAnswer {
			multiAnswerCount++
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		left := merged[i]
		right := merged[j]
		if left.NormalizedQuestion == right.NormalizedQuestion {
			return left.ID < right.ID
		}
		return left.NormalizedQuestion < right.NormalizedQuestion
	})

	return &bankStore{
		sourcePath: "sqlite",
		indexPath:  "sqlite",
		signature:  dashboard.DatabasePath,
		summary: domain.BankSummary{
			RawCount:         dashboard.TotalQuestions,
			SearchableCount:  len(merged),
			DuplicateGroups:  duplicateGroups,
			MultiAnswerCount: multiAnswerCount,
			GeneratedAt:      dashboard.LastCapturedAt,
			SourcePath:       "sqlite",
			IndexPath:        "sqlite",
			DatabasePath:     dashboard.DatabasePath,
			ReviewPending:    dashboard.PendingReview,
			CapturedCount:    dashboard.CapturedQuestions,
		},
		items: merged,
	}, nil
}

func (s *Service) SearchBank(ctx context.Context, query string) (domain.BankSearchResponse, error) {
	bank, err := s.loadMergedBank(ctx)
	if err != nil {
		return domain.BankSearchResponse{}, err
	}
	response := searchBank(bank, bankSearchRequest{
		Query: query,
		Limit: 12,
	})
	response.Summary.DatabasePath = s.layout.TheoryBankDBPath
	if s.repo != nil {
		if summary, err := s.repo.TheoryReviewDashboard(ctx); err == nil {
			response.Summary.DatabasePath = summary.DatabasePath
			response.Summary.ReviewPending = summary.PendingReview
			response.Summary.CapturedCount = summary.CapturedQuestions
			response.Summary.CaptureHits = summary.CaptureHits
		}
	}
	return response, nil
}

func (s *Service) Snapshot(ctx context.Context) (domain.Payload, error) {
	return s.SnapshotByRequest(ctx, domain.SnapshotRequest{})
}

func (s *Service) SnapshotByRequest(ctx context.Context, req domain.SnapshotRequest) (domain.Payload, error) {
	account, accounts, err := s.resolveTheoryAccount(ctx, req.Account)
	if err != nil {
		return domain.Payload{}, err
	}
	if s.repo != nil && !req.Refresh {
		cached, cacheStatus, loadErr := s.repo.LoadTheoryRuntimeSnapshot(ctx, account.Name)
		if loadErr != nil {
			return domain.Payload{}, loadErr
		}
		if cacheStatus.HasSnapshot {
			if !isProgressTheoryPayload(cached) {
				cacheStatus.HasSnapshot = false
				cacheStatus.LastRemoteError = "历史缓存不是有效理论题，已忽略；请刷新远端缓存。"
				if strings.TrimSpace(cacheStatus.Source) == "" {
					cacheStatus.Source = "invalid-cache"
				}
				return s.emptyLocalSnapshot(ctx, account, accounts, cacheStatus), nil
			}
			return s.decorateLocalSnapshot(ctx, cached, cacheStatus, account, accounts), nil
		}
		return s.emptyLocalSnapshot(ctx, account, accounts, cacheStatus), nil
	}

	session, err := s.theorySession(ctx, account)
	if err != nil {
		return domain.Payload{}, err
	}
	payload, err := s.snapshotWithSession(ctx, session, account, accounts, true)
	if err != nil {
		return domain.Payload{}, err
	}
	if s.repo != nil {
		cache := cacheStatusForPayload(payload, "remote")
		_ = s.repo.SaveTheoryRuntimeSnapshot(ctx, account.Name, account.Username, payload, cache)
		payload.CacheStatus = cache
		payload.CapturedHistory, _ = s.repo.ListTheoryCapturedQuestions(ctx, account.Name, 12)
		payload.CacheHistory, _ = s.repo.ListTheoryRuntimeSnapshots(ctx)
	}
	return payload, nil
}

func (s *Service) decorateLocalSnapshot(ctx context.Context, cached domain.Payload, cacheStatus domain.CacheStatus, account accountsdomain.Account, accounts []domain.TheoryAccount) domain.Payload {
	if bank, bankErr := s.loadMergedBank(ctx); bankErr == nil {
		cached.Match = matchTheoryQuestion(cached.Question, bank)
		cached.Statistics = statisticsFromBank(bank, cached.Question, cached.Statistics.CurrentScore, cached.Statistics.TotalScore, cached.Statistics.ScoreText, cached.Statistics.ProgressMessage, s.layout.TheoryBankDBPath)
	}
	if s.repo != nil {
		if reviewResponse, reviewErr := s.repo.ListTheoryReviewItems(ctx, 12); reviewErr == nil {
			cached.ReviewDashboard = reviewResponse.Summary
			cached.ReviewItems = reviewResponse.Items
		}
		cached.CapturedHistory, _ = s.repo.ListTheoryCapturedQuestions(ctx, account.Name, 12)
		cached.CacheHistory, _ = s.repo.ListTheoryRuntimeSnapshots(ctx)
	}
	cached.Account = account.Name
	cached.Username = account.Username
	cached.SourceURL = firstNonEmpty(cached.SourceURL, theoryBaseURL+theoryPaperPath)
	cached.SnapshotAt = firstNonEmpty(cached.SnapshotAt, cacheStatus.CachedAt, theoryNowTS())
	cached.Accounts = accounts
	cached.SelectedAccount = account.Name
	cached.TestAccount = theoryTestAccount(account)
	cached.AI = s.localOnlyAIInsight(ctx, cached.AI)
	cached.CacheStatus = localReadCacheStatus(cacheStatus)
	return cached
}

func (s *Service) emptyLocalSnapshot(ctx context.Context, account accountsdomain.Account, accounts []domain.TheoryAccount, cacheStatus domain.CacheStatus) domain.Payload {
	var reviewDashboard domain.ReviewDashboard
	var reviewItems []domain.ReviewItem
	var capturedHistory []domain.CapturedQuestionRecord
	var cacheHistory []domain.RuntimeSnapshotRecord
	var statistics domain.Statistics
	if bank, bankErr := s.loadMergedBank(ctx); bankErr == nil {
		statistics = statisticsFromBank(bank, domain.Question{}, "", "", "", "", s.layout.TheoryBankDBPath)
	}
	if s.repo != nil {
		if reviewResponse, reviewErr := s.repo.ListTheoryReviewItems(ctx, 12); reviewErr == nil {
			reviewDashboard = reviewResponse.Summary
			reviewItems = reviewResponse.Items
		}
		capturedHistory, _ = s.repo.ListTheoryCapturedQuestions(ctx, account.Name, 12)
		cacheHistory, _ = s.repo.ListTheoryRuntimeSnapshots(ctx)
	}
	cacheStatus.HasSnapshot = false
	if strings.TrimSpace(cacheStatus.Source) == "" {
		cacheStatus.Source = "local-empty"
	}
	if strings.TrimSpace(cacheStatus.LastRemoteError) == "" {
		cacheStatus.LastRemoteError = "本地暂无缓存，请点击刷新远端缓存。"
	}
	return domain.Payload{
		Account:         account.Name,
		Username:        account.Username,
		SourceURL:       theoryBaseURL + theoryPaperPath,
		SnapshotAt:      theoryNowTS(),
		AI:              s.localOnlyAIInsight(ctx, domain.AIInsight{}),
		TestAccount:     theoryTestAccount(account),
		Statistics:      statistics,
		ReviewDashboard: reviewDashboard,
		ReviewItems:     reviewItems,
		Accounts:        accounts,
		SelectedAccount: account.Name,
		CacheStatus:     localReadCacheStatus(cacheStatus),
		CapturedHistory: capturedHistory,
		CacheHistory:    cacheHistory,
	}
}

func (s *Service) localOnlyAIInsight(ctx context.Context, existing domain.AIInsight) domain.AIInsight {
	settings, err := s.loadAISettings(ctx)
	if err != nil {
		if strings.TrimSpace(existing.Status) == "" {
			existing.Status = "unknown"
		}
		if strings.TrimSpace(existing.Error) == "" && strings.TrimSpace(existing.Reason) == "" {
			existing.Error = err.Error()
		}
		return existing
	}
	if strings.TrimSpace(existing.Status) != "" || len(existing.RecommendedOptions) > 0 || strings.TrimSpace(existing.Reason) != "" || strings.TrimSpace(existing.Error) != "" {
		existing.Enabled = settings.Enabled
		existing.Ready = strings.TrimSpace(settings.APIKey) != ""
		if strings.TrimSpace(existing.Model) == "" {
			existing.Model = settings.Model
		}
		if settings.Enabled && len(existing.RecommendedOptions) == 0 && (existing.Status == "disabled" || strings.Contains(existing.Reason, "AI 判题未启用")) {
			return aiInsightForSettings(settings, "cached", "已读取本地缓存，未重新请求 AI；自动答题时会强制 AI 复核。")
		}
		return existing
	}
	return aiInsightForSettings(settings, "cached", "已读取本地缓存，未重新请求 AI；自动答题时会强制 AI 复核。")
}

func aiInsightForSettings(settings domain.AISettings, readyStatus string, readyReason string) domain.AIInsight {
	insight := domain.AIInsight{
		Enabled: settings.Enabled,
		Ready:   strings.TrimSpace(settings.APIKey) != "",
		Model:   settings.Model,
	}
	if !settings.Enabled {
		insight.Status = "disabled"
		insight.Reason = "AI 判题未启用。"
		return insight
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		insight.Status = "not_ready"
		insight.Error = "已启用 AI，但尚未填写 API Key。"
		return insight
	}
	insight.Status = readyStatus
	insight.Reason = readyReason
	return insight
}

func localReadCacheStatus(cacheStatus domain.CacheStatus) domain.CacheStatus {
	origin := strings.TrimSpace(cacheStatus.Source)
	if cacheStatus.HasSnapshot {
		cacheStatus.Source = "local-cache"
		if origin != "" && origin != cacheStatus.Source {
			cacheStatus.Source += "/" + origin
		}
		return cacheStatus
	}
	cacheStatus.Source = "local-empty"
	if origin != "" && origin != cacheStatus.Source {
		cacheStatus.Source += "/" + origin
	}
	return cacheStatus
}

func cacheStatusForPayload(payload domain.Payload, source string) domain.CacheStatus {
	cache := domain.CacheStatus{
		HasSnapshot:      isProgressTheoryPayload(payload),
		HasQuestion:      strings.TrimSpace(payload.Question.Title) != "",
		Answerable:       isValidTheoryPayload(payload),
		Completed:        payload.Statistics.Completed,
		CachedAt:         theoryNowTS(),
		LastRemoteSyncAt: theoryNowTS(),
		LastRemoteError:  "",
		Source:           source,
		Message:          payload.Statistics.ProgressMessage,
	}
	if !cache.HasSnapshot {
		cache.LastRemoteError = "远端页面未返回可识别的理论题进度"
	}
	if strings.TrimSpace(cache.Message) == "" {
		cache.Message = theoryProgressMessage(payload.Question, payload.Statistics.CurrentScore, payload.Statistics.TotalScore, payload.Statistics.ScoreText, cache.Answerable, cache.Completed)
	}
	return cache
}

func statisticsFromBank(bank *bankStore, question domain.Question, currentScore string, totalScore string, scoreText string, progressMessage string, databasePath string) domain.Statistics {
	answerable := isValidTheoryQuestion(question)
	completed := !answerable && isTheoryProgressComplete(question, currentScore, totalScore, scoreText, progressMessage)
	if strings.TrimSpace(progressMessage) == "" {
		progressMessage = theoryProgressMessage(question, currentScore, totalScore, scoreText, answerable, completed)
	}
	return domain.Statistics{
		BankSize:                bank.summary.RawCount,
		SearchableBankSize:      bank.summary.SearchableCount,
		DuplicateQuestionGroups: bank.summary.DuplicateGroups,
		MultiAnswerCount:        bank.summary.MultiAnswerCount,
		GeneratedAt:             bank.summary.GeneratedAt,
		QuestionNumber:          question.Number,
		CurrentScore:            currentScore,
		TotalScore:              totalScore,
		ScoreText:               scoreText,
		OptionCount:             len(question.Options),
		DatabasePath:            databasePath,
		Answerable:              answerable,
		Completed:               completed,
		ProgressMessage:         progressMessage,
	}
}

func isTheoryProgressComplete(question domain.Question, currentScore string, totalScore string, scoreText string, progressMessage string) bool {
	text := compactTheoryText(strings.Join([]string{question.Title, currentScore, totalScore, scoreText, progressMessage}, " "))
	if strings.Contains(text, "完成") || strings.Contains(text, "结束") || strings.Contains(text, "已答完") || strings.Contains(text, "提交成功") {
		return true
	}
	if question.Number >= 100 && !isValidTheoryQuestion(question) {
		return true
	}
	if strings.TrimSpace(currentScore) != "" && strings.TrimSpace(totalScore) != "" && currentScore == totalScore {
		return true
	}
	return false
}

func theoryProgressMessage(question domain.Question, currentScore string, totalScore string, scoreText string, answerable bool, completed bool) string {
	if completed {
		if scoreText != "" {
			return "远端显示理论题已完成，当前成绩 " + scoreText
		}
		return "远端显示理论题已完成或已无下一题"
	}
	if answerable {
		if question.Number > 0 {
			return fmt.Sprintf("远端当前可答第 %d 题", question.Number)
		}
		return "远端当前题可提交"
	}
	if scoreText != "" {
		return "远端已同步成绩 " + scoreText
	}
	if question.Number > 0 {
		return fmt.Sprintf("远端已同步到第 %d 题，但当前页面没有可提交选项", question.Number)
	}
	return "远端页面未返回可提交题目"
}

func theoryTestAccount(account accountsdomain.Account) domain.TestAccount {
	return domain.TestAccount{
		Name:     account.Name,
		Username: account.Username,
		Password: account.Password,
		Builtin:  account.Name == defaultTheoryUser && account.Username == defaultTheoryUser,
	}
}

func (s *Service) ReviewItems(ctx context.Context) (domain.ReviewListResponse, error) {
	if s.repo == nil {
		return domain.ReviewListResponse{}, fmt.Errorf("理论题仓库未初始化")
	}
	return s.repo.ListTheoryReviewItems(ctx, 80)
}

func (s *Service) SaveReview(ctx context.Context, input domain.ReviewDecision) (domain.ReviewItem, error) {
	if s.repo == nil {
		return domain.ReviewItem{}, fmt.Errorf("理论题仓库未初始化")
	}
	input.Question = cleanQuestionText(input.Question)
	if strings.TrimSpace(input.Question) == "" {
		return domain.ReviewItem{}, fmt.Errorf("题目不能为空")
	}
	if input.SelectionType == "" {
		if len(input.AnswerKeys) > 1 {
			input.SelectionType = "multiple"
		} else {
			input.SelectionType = "single"
		}
	}
	if strings.TrimSpace(input.ReviewStatus) == "" {
		input.ReviewStatus = "approved"
	}
	return s.repo.SaveTheoryReviewDecision(ctx, input)
}

func (s *Service) ManualSubmit(ctx context.Context, input domain.ManualSubmitRequest) (domain.ManualSubmitResponse, error) {
	account, accounts, err := s.resolveTheoryAccount(ctx, input.Account)
	if err != nil {
		return domain.ManualSubmitResponse{}, err
	}
	options := normalizeSubmitOptions(input.Options)
	if len(options) == 0 {
		return domain.ManualSubmitResponse{}, fmt.Errorf("请至少选择一个答案")
	}
	session, err := s.theorySession(ctx, account)
	if err != nil {
		return domain.ManualSubmitResponse{}, err
	}
	snapshot, err := s.snapshotWithSession(ctx, session, account, accounts, false)
	if err != nil {
		return domain.ManualSubmitResponse{}, err
	}
	if len(options) > 1 && !snapshot.AnswerForm.AllowsMultiple {
		return domain.ManualSubmitResponse{}, fmt.Errorf("当前题为单选题，只能选择一个答案")
	}
	submitResp, err := s.withTheorySubmitRetry(ctx, session, account, func(active *theorySession) (theorySubmitResponse, error) {
		return theorySubmitAnswer(ctx, active, snapshot.AnswerForm, options)
	})
	if err != nil {
		return domain.ManualSubmitResponse{}, err
	}
	nextSnapshot, refreshErr := s.snapshotWithSession(ctx, session, account, accounts, false)
	if refreshErr == nil && isProgressTheoryPayload(nextSnapshot) {
		s.saveAutomationSnapshot(ctx, account, accounts, nextSnapshot)
	} else {
		nextSnapshot = snapshot
		s.saveAutomationSnapshot(ctx, account, accounts, snapshot)
	}
	message := submitResp.Message
	if refreshErr != nil {
		message = firstNonEmpty(message, "已提交，但刷新下一题失败") + "；" + refreshErr.Error()
	}
	return domain.ManualSubmitResponse{
		Success:            refreshErr == nil && isProgressTheoryPayload(nextSnapshot),
		Message:            message,
		SubmittedOptions:   options,
		NextQuestion:       firstNonEmpty(nextSnapshot.Question.Title, submitResp.NextQuestion),
		NextQuestionNumber: firstNonZero(nextSnapshot.Question.Number, submitResp.NextQuestionNumber),
		Payload:            nextSnapshot,
	}, nil
}

func (s *Service) AIReviewStatus(_ context.Context) (domain.AIReviewStatus, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	if s.reviewRun == nil {
		return domain.AIReviewStatus{
			Running:       false,
			Status:        "idle",
			Message:       "未启动",
			LastUpdatedAt: theoryNowTS(),
		}, nil
	}
	return s.reviewRun.status, nil
}

func (s *Service) StartAIReview(ctx context.Context, req domain.AIReviewRequest) (domain.AIReviewStatus, error) {
	if s.repo == nil {
		return domain.AIReviewStatus{}, fmt.Errorf("理论题仓库未初始化")
	}
	settings, err := s.loadAISettings(ctx)
	if err != nil {
		return domain.AIReviewStatus{}, err
	}
	if !settings.Enabled {
		return domain.AIReviewStatus{}, fmt.Errorf("AI 判题未启用")
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		return domain.AIReviewStatus{}, fmt.Errorf("AI API Key 未配置")
	}

	reasoningEffort := strings.TrimSpace(req.ReasoningEffort)
	if reasoningEffort == "" {
		reasoningEffort = settings.ReasoningEffort
	}
	if reasoningEffort == "" {
		reasoningEffort = defaultAIReasoningEffort
	}

	batchSize := req.BatchSize
	if batchSize <= 0 {
		batchSize = 12
	}
	if batchSize > 50 {
		batchSize = 50
	}
	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}

	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	if s.reviewRun != nil && s.reviewRun.status.Running {
		return s.reviewRun.status, fmt.Errorf("AI 批量复核正在运行")
	}

	runCtx, cancel := context.WithCancel(context.Background())
	status := domain.AIReviewStatus{
		Running:         true,
		StartedAt:       theoryNowTS(),
		Status:          "running",
		Message:         "AI 批量复核已启动",
		BatchSize:       batchSize,
		Limit:           req.Limit,
		DryRun:          req.DryRun,
		ReasoningEffort: reasoningEffort,
		LastUpdatedAt:   theoryNowTS(),
	}
	s.reviewRun = &aiReviewRun{
		cancel: cancel,
		status: status,
	}

	go s.runAIReview(runCtx, settings, domain.AIReviewRequest{
		Limit:           req.Limit,
		BatchSize:       batchSize,
		TimeoutSeconds:  timeoutSeconds,
		DryRun:          req.DryRun,
		OnlyPending:     req.OnlyPending,
		ReasoningEffort: reasoningEffort,
	})

	return s.reviewRun.status, nil
}

func (s *Service) StopAIReview(_ context.Context) (domain.AIReviewStatus, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	if s.reviewRun == nil {
		return domain.AIReviewStatus{
			Running:       false,
			Status:        "idle",
			Message:       "未启动",
			LastUpdatedAt: theoryNowTS(),
		}, nil
	}
	if s.reviewRun.cancel != nil {
		s.reviewRun.cancel()
	}
	s.reviewRun.status.Running = false
	s.reviewRun.status.Status = "stopped"
	s.reviewRun.status.Message = "已停止"
	s.reviewRun.status.FinishedAt = theoryNowTS()
	s.reviewRun.status.LastUpdatedAt = theoryNowTS()
	return s.reviewRun.status, nil
}

func (s *Service) RunAutomation(ctx context.Context, input domain.AutomationRequest) (domain.AutomationResult, error) {
	input = normalizeAutomationRequest(input)
	return s.runAutomationWithProgress(ctx, input, nil)
}

func (s *Service) AutomationStatus(_ context.Context) (domain.AutomationStatus, error) {
	s.automationMu.Lock()
	defer s.automationMu.Unlock()
	if s.automationRun == nil {
		return domain.AutomationStatus{
			Running:       false,
			Status:        "idle",
			Message:       "未启动",
			LastUpdatedAt: theoryNowTS(),
		}, nil
	}
	return s.automationRun.status, nil
}

func (s *Service) StartAutomation(ctx context.Context, input domain.AutomationRequest) (domain.AutomationStatus, error) {
	input = normalizeAutomationRequest(input)

	s.automationMu.Lock()
	defer s.automationMu.Unlock()
	if s.automationRun != nil && s.automationRun.status.Running {
		return s.automationRun.status, fmt.Errorf("理论题自动答题正在运行")
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.automationRun = &automationRun{
		cancel: cancel,
		status: domain.AutomationStatus{
			Running:          true,
			StartedAt:        theoryNowTS(),
			Status:           "running",
			Message:          "理论题自动答题已启动",
			MaxQuestions:     input.MaxQuestions,
			CurrentStartedAt: theoryNowTS(),
			LastUpdatedAt:    theoryNowTS(),
		},
	}

	go s.runAutomationAsync(runCtx, input)
	return s.automationRun.status, nil
}

func normalizeAutomationRequest(input domain.AutomationRequest) domain.AutomationRequest {
	if input.MaxQuestions <= 0 {
		input.MaxQuestions = 20
	}
	if input.MaxQuestions > 200 {
		input.MaxQuestions = 200
	}
	input.AllowAI = true
	input.StopOnNoAnswer = true
	return input
}

func (s *Service) StopAutomation(_ context.Context) (domain.AutomationStatus, error) {
	s.automationMu.Lock()
	defer s.automationMu.Unlock()
	if s.automationRun == nil {
		return domain.AutomationStatus{
			Running:       false,
			Status:        "idle",
			Message:       "未启动",
			LastUpdatedAt: theoryNowTS(),
		}, nil
	}
	if s.automationRun.cancel != nil {
		s.automationRun.cancel()
	}
	s.automationRun.status.Running = false
	s.automationRun.status.Status = "stopped"
	s.automationRun.status.Message = "已停止"
	s.automationRun.status.FinishedAt = theoryNowTS()
	s.automationRun.status.LastUpdatedAt = theoryNowTS()
	return s.automationRun.status, nil
}

func legacyTheoryDBPath(appDataRoot string, mainDBPath string) string {
	legacyPath := filepath.Join(appDataRoot, "theory-bank.sqlite")
	if legacyPath == mainDBPath {
		return ""
	}
	return legacyPath
}

func loadLegacyTheoryMeta(ctx context.Context, store *sqlitestore.Store) (map[string]string, error) {
	keys := []string{
		"theory.bank_last_import_source",
		"theory.bank_last_imported_at",
		"theory.ai_settings",
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		value, err := store.MetaValue(ctx, key)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, nil
}

func removeLegacyTheorySQLiteFiles(path string) error {
	paths := []string{path, path + "-wal", path + "-shm"}
	for _, item := range paths {
		if err := os.Remove(item); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *Service) runAIReview(ctx context.Context, settings domain.AISettings, req domain.AIReviewRequest) {
	finalize := func(update func(*domain.AIReviewStatus)) {
		s.reviewMu.Lock()
		defer s.reviewMu.Unlock()
		if s.reviewRun == nil {
			return
		}
		update(&s.reviewRun.status)
		s.reviewRun.status.LastUpdatedAt = theoryNowTS()
	}

	items, err := s.repo.ListTheoryReviewItemsAll(ctx)
	if err != nil {
		finalize(func(status *domain.AIReviewStatus) {
			status.Running = false
			status.Status = "failed"
			status.LastError = err.Error()
			status.Message = err.Error()
			status.FinishedAt = theoryNowTS()
		})
		return
	}

	pending := make([]domain.ReviewItem, 0, len(items))
	for _, item := range items {
		if req.OnlyPending && !(item.NeedsReview || item.ReviewStatus == "pending" || item.ReviewStatus == "captured") {
			continue
		}
		if item.NeedsReview || item.ReviewStatus == "pending" || item.ReviewStatus == "captured" {
			pending = append(pending, item)
		}
		if req.Limit > 0 && len(pending) >= req.Limit {
			break
		}
	}

	finalize(func(status *domain.AIReviewStatus) {
		status.Remaining = len(pending)
		status.Message = fmt.Sprintf("待复核 %d 条", len(pending))
	})
	if len(pending) == 0 {
		finalize(func(status *domain.AIReviewStatus) {
			status.Running = false
			status.Status = "completed"
			status.Message = "没有待复核题目"
			status.FinishedAt = theoryNowTS()
		})
		return
	}

	for start := 0; start < len(pending); start += req.BatchSize {
		select {
		case <-ctx.Done():
			finalize(func(status *domain.AIReviewStatus) {
				status.Running = false
				status.Status = "stopped"
				status.Message = "已停止"
				status.FinishedAt = theoryNowTS()
			})
			return
		default:
		}

		end := start + req.BatchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[start:end]
		decisions, batchErr := requestAIReviewBatch(ctx, settings, batch, req.ReasoningEffort, req.TimeoutSeconds)
		if batchErr != nil {
			finalize(func(status *domain.AIReviewStatus) {
				status.Running = false
				status.Status = "failed"
				status.LastError = batchErr.Error()
				status.Message = batchErr.Error()
				status.FinishedAt = theoryNowTS()
			})
			return
		}

		for _, decision := range decisions {
			if req.DryRun {
				finalize(func(status *domain.AIReviewStatus) {
					status.Reviewed++
					if decision.ReviewStatus == "approved" {
						status.Approved++
					}
					status.Remaining = maxPendingCount(len(pending) - status.Reviewed)
					status.CurrentBatch = start/req.BatchSize + 1
					status.LastConfidence = decision.Confidence
					status.Message = fmt.Sprintf("dry-run 已处理 %d/%d", status.Reviewed, len(pending))
				})
				continue
			}

			_, saveErr := s.repo.SaveTheoryReviewDecision(ctx, domain.ReviewDecision{
				ID:            decision.ID,
				Question:      decision.Question,
				SelectionType: decision.SelectionType,
				Options:       decision.Options,
				AnswerKeys:    decision.AnswerKeys,
				AnswerTexts:   decision.AnswerTexts,
				ReviewStatus:  decision.ReviewStatus,
				ReviewReason:  decision.ReviewReason,
			})
			if saveErr != nil {
				finalize(func(status *domain.AIReviewStatus) {
					status.Running = false
					status.Status = "failed"
					status.LastError = saveErr.Error()
					status.Message = saveErr.Error()
					status.FinishedAt = theoryNowTS()
				})
				return
			}

			finalize(func(status *domain.AIReviewStatus) {
				status.Reviewed++
				if decision.ReviewStatus == "approved" {
					status.Approved++
				}
				status.Remaining = maxPendingCount(len(pending) - status.Reviewed)
				status.CurrentBatch = start/req.BatchSize + 1
				status.LastConfidence = decision.Confidence
				status.Message = fmt.Sprintf("已处理 %d/%d", status.Reviewed, len(pending))
			})
		}
	}

	finalize(func(status *domain.AIReviewStatus) {
		status.Running = false
		status.Status = "completed"
		status.Message = "AI 批量复核完成"
		status.FinishedAt = theoryNowTS()
	})
}

func (s *Service) runAutomationAsync(ctx context.Context, input domain.AutomationRequest) {
	finalize := func(update func(*domain.AutomationStatus)) {
		s.automationMu.Lock()
		defer s.automationMu.Unlock()
		if s.automationRun == nil {
			return
		}
		update(&s.automationRun.status)
		s.automationRun.status.LastUpdatedAt = theoryNowTS()
	}

	result, err := s.runAutomationWithProgress(ctx, input, func(questionNumber int, questionTitle string, completed int, message string) {
		finalize(func(status *domain.AutomationStatus) {
			if status.CurrentNumber != questionNumber || status.CurrentTitle != questionTitle {
				status.CurrentStartedAt = theoryNowTS()
			}
			status.CurrentNumber = questionNumber
			status.CurrentTitle = questionTitle
			status.Completed = completed
			if strings.TrimSpace(message) != "" {
				status.Message = message
			}
		})
	})

	if err != nil {
		finalize(func(status *domain.AutomationStatus) {
			status.Running = false
			status.Status = "failed"
			status.LastError = err.Error()
			status.Message = err.Error()
			status.FinishedAt = theoryNowTS()
			status.Result = &result
		})
		return
	}

	finalize(func(status *domain.AutomationStatus) {
		status.Running = false
		if result.StoppedReason == "已手动停止" {
			status.Status = "stopped"
		} else {
			status.Status = "completed"
		}
		if result.StoppedReason != "" {
			status.Message = result.StoppedReason
		} else {
			status.Message = "理论题自动答题已完成"
		}
		status.Completed = result.Completed
		status.CurrentNumber = result.FinalNumber
		status.CurrentTitle = result.FinalQuestion
		status.FinishedAt = theoryNowTS()
		status.Result = &result
	})
}

func (s *Service) runAutomationWithProgress(ctx context.Context, input domain.AutomationRequest, progress func(questionNumber int, questionTitle string, completed int, message string)) (domain.AutomationResult, error) {
	maxQuestions := input.MaxQuestions
	if maxQuestions <= 0 {
		maxQuestions = 20
	}
	if maxQuestions > 200 {
		maxQuestions = 200
	}
	result := domain.AutomationResult{
		StartedAt: theoryNowTS(),
		Results:   make([]domain.AutomationStep, 0, maxQuestions),
	}
	account, accounts, err := s.resolveTheoryAccount(ctx, input.Account)
	if err != nil {
		return result, err
	}

	session, err := s.theorySession(ctx, account)
	if err != nil {
		return result, err
	}
	if err := waitTheoryCooldown(ctx, theoryLoginCooldown); err != nil {
		if errors.Is(err, context.Canceled) {
			result.FinishedAt = theoryNowTS()
			result.StoppedReason = "已手动停止"
			return result, nil
		}
		result.FinishedAt = theoryNowTS()
		result.StoppedReason = err.Error()
		return result, err
	}

	seen := map[string]struct{}{}
	var prefetchedSnapshot *domain.Payload
	for idx := 0; idx < maxQuestions; idx++ {
		select {
		case <-ctx.Done():
			result.FinishedAt = theoryNowTS()
			result.StoppedReason = "已手动停止"
			return result, nil
		default:
		}

		if idx > 0 {
			if progress != nil {
				progress(result.FinalNumber, result.FinalQuestion, result.Completed, fmt.Sprintf("题间固定等待 %ds", int(theoryQuestionCooldown.Seconds())))
			}
			if err := waitTheoryCooldown(ctx, theoryQuestionCooldown); err != nil {
				if errors.Is(err, context.Canceled) {
					result.FinishedAt = theoryNowTS()
					result.StoppedReason = "已手动停止"
					return result, nil
				}
				result.FinishedAt = theoryNowTS()
				result.StoppedReason = err.Error()
				return result, err
			}
		}

		var snapshot domain.Payload
		snapshotAlreadyCached := false
		if prefetchedSnapshot != nil {
			snapshot = *prefetchedSnapshot
			prefetchedSnapshot = nil
			snapshotAlreadyCached = true
		} else {
			if idx > 0 && progress != nil {
				progress(result.FinalNumber, result.FinalQuestion, result.Completed, fmt.Sprintf("抓题前固定等待 %ds", int(theoryFetchCooldown.Seconds())))
			}
			if err := waitTheoryCooldown(ctx, theoryFetchCooldown); err != nil {
				if errors.Is(err, context.Canceled) {
					result.FinishedAt = theoryNowTS()
					result.StoppedReason = "已手动停止"
					return result, nil
				}
				result.FinishedAt = theoryNowTS()
				result.StoppedReason = err.Error()
				return result, err
			}
			snapshot, err = s.snapshotWithSession(ctx, session, account, accounts, false)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					result.FinishedAt = theoryNowTS()
					result.StoppedReason = "已手动停止"
					return result, nil
				}
				result.FinishedAt = theoryNowTS()
				result.StoppedReason = err.Error()
				return result, err
			}
		}
		if !snapshotAlreadyCached {
			s.saveAutomationSnapshot(ctx, account, accounts, snapshot)
		}
		if progress != nil {
			progress(snapshot.Question.Number, snapshot.Question.Title, result.Completed, "正在处理当前题目")
		}
		if !isValidTheoryPayload(snapshot) {
			result.StoppedReason = firstNonEmpty(snapshot.Statistics.ProgressMessage, "远端当前没有可提交的理论题")
			result.FinalQuestion = snapshot.Question.Title
			result.FinalNumber = snapshot.Question.Number
			if progress != nil {
				progress(snapshot.Question.Number, snapshot.Question.Title, result.Completed, result.StoppedReason)
			}
			break
		}
		questionKey := fmt.Sprintf("%d|%s", snapshot.Question.Number, normalizeTheoryText(snapshot.Question.Title))
		if _, exists := seen[questionKey]; exists {
			result.StoppedReason = "检测到题目未继续推进，自动化已停止"
			result.FinalQuestion = snapshot.Question.Title
			result.FinalNumber = snapshot.Question.Number
			break
		}
		seen[questionKey] = struct{}{}

		decision := s.decideAutomationAnswer(ctx, snapshot, input.AllowAI, progress, result.Completed)
		if hasAutomationAIInsight(decision.Insight) {
			snapshot.AI = decision.Insight
			s.saveAutomationSnapshot(ctx, account, accounts, snapshot)
		}
		if len(decision.Options) == 0 {
			step := automationStepFromDecision(snapshot, decision)
			step.Success = false
			step.Message = "没有达到自动提交阈值的答案"
			if strings.TrimSpace(decision.Reason) != "" {
				step.Message = decision.Reason
			}
			result.Results = append(result.Results, step)
			result.StoppedReason = step.Message
			result.FinalQuestion = snapshot.Question.Title
			result.FinalNumber = snapshot.Question.Number
			if progress != nil {
				progress(snapshot.Question.Number, snapshot.Question.Title, result.Completed, result.StoppedReason)
			}
			break
		}

		if progress != nil {
			progress(snapshot.Question.Number, snapshot.Question.Title, result.Completed, fmt.Sprintf("提交前固定等待 %ds", int(theorySubmitCooldown.Seconds())))
		}
		if err := waitTheoryCooldown(ctx, theorySubmitCooldown); err != nil {
			if errors.Is(err, context.Canceled) {
				result.FinishedAt = theoryNowTS()
				result.StoppedReason = "已手动停止"
				return result, nil
			}
			result.FinishedAt = theoryNowTS()
			result.StoppedReason = err.Error()
			return result, err
		}
		submitResp, submitErr := s.withTheorySubmitRetry(ctx, session, account, func(active *theorySession) (theorySubmitResponse, error) {
			return theorySubmitAnswer(ctx, active, snapshot.AnswerForm, decision.Options)
		})
		step := automationStepFromDecision(snapshot, decision)
		if submitErr != nil {
			step.Success = false
			step.Message = submitErr.Error()
			result.Results = append(result.Results, step)
			result.StoppedReason = submitErr.Error()
			result.FinalQuestion = snapshot.Question.Title
			result.FinalNumber = snapshot.Question.Number
			if progress != nil {
				progress(snapshot.Question.Number, snapshot.Question.Title, result.Completed, result.StoppedReason)
			}
			return result, submitErr
		}

		step.Success = true
		step.Message = submitResp.Message
		step.NextQuestion = submitResp.NextQuestion
		step.NextQuestionNumber = submitResp.NextQuestionNumber
		result.Results = append(result.Results, step)

		if submitResp.Advanced {
			if submitResp.Completed {
				step.Message = firstNonEmpty(submitResp.Message, "理论题已完成或已无下一题")
				step.NextQuestion = submitResp.NextQuestion
				step.NextQuestionNumber = firstNonZero(submitResp.NextQuestionNumber, snapshot.Question.Number)
				result.Results[len(result.Results)-1] = step
				result.Completed++
				result.StoppedReason = step.Message
				result.FinalQuestion = step.NextQuestion
				result.FinalNumber = step.NextQuestionNumber
				if progress != nil {
					progress(step.NextQuestionNumber, step.NextQuestion, result.Completed, step.Message)
				}
				break
			}
			advancedQuestion := domain.Question{
				Number: submitResp.NextQuestionNumber,
				Title:  submitResp.NextQuestion,
			}
			if submitResp.document != nil {
				nextSnapshot, snapshotErr := s.snapshotFromDocument(ctx, submitResp.document, account, accounts, false)
				if snapshotErr == nil && isValidTheoryPayload(nextSnapshot) && !sameTheoryQuestion(snapshot.Question, nextSnapshot.Question) {
					nextSnapshot.Statistics.ProgressMessage = firstNonEmpty(submitResp.Message, nextSnapshot.Statistics.ProgressMessage)
					s.saveAutomationSnapshot(ctx, account, accounts, nextSnapshot)
					prefetchedSnapshot = &nextSnapshot
					step.Message = firstNonEmpty(submitResp.Message, "已提交，下一题页面已就绪")
					step.NextQuestion = nextSnapshot.Question.Title
					step.NextQuestionNumber = nextSnapshot.Question.Number
					result.Results[len(result.Results)-1] = step
				}
			}
			if prefetchedSnapshot == nil && strings.TrimSpace(submitResp.NextQuestion) != "" && !sameTheoryQuestion(snapshot.Question, advancedQuestion) {
				step.Message = firstNonEmpty(submitResp.Message, "已提交，等待下一题固定冷却后继续")
				step.NextQuestion = submitResp.NextQuestion
				step.NextQuestionNumber = firstNonZero(submitResp.NextQuestionNumber, snapshot.Question.Number+1)
				result.Results[len(result.Results)-1] = step
			}
			if prefetchedSnapshot == nil && sameTheoryQuestion(snapshot.Question, domain.Question{
				Number: step.NextQuestionNumber,
				Title:  step.NextQuestion,
			}) {
				step.Success = false
				step.Message = "已提交但未确认题目推进，自动化停在当前题等待人工确认"
				result.Results[len(result.Results)-1] = step
				result.StoppedReason = step.Message
				result.FinalQuestion = snapshot.Question.Title
				result.FinalNumber = snapshot.Question.Number
				if progress != nil {
					progress(snapshot.Question.Number, snapshot.Question.Title, result.Completed, step.Message)
				}
				break
			}
		}

		result.Completed++

		if progress != nil {
			nextTitle := step.NextQuestion
			if strings.TrimSpace(nextTitle) == "" {
				nextTitle = snapshot.Question.Title
			}
			nextNumber := step.NextQuestionNumber
			if nextNumber == 0 {
				nextNumber = snapshot.Question.Number
			}
			progress(nextNumber, nextTitle, result.Completed, submitResp.Message)
		}
		if !submitResp.Advanced {
			result.StoppedReason = submitResp.Message
			result.FinalQuestion = snapshot.Question.Title
			result.FinalNumber = snapshot.Question.Number
			break
		}
		result.FinalQuestion = step.NextQuestion
		result.FinalNumber = step.NextQuestionNumber
	}

	if result.StoppedReason == "" {
		result.StoppedReason = "达到本轮自动化上限"
	}
	result.FinishedAt = theoryNowTS()
	return result, nil
}

func (s *Service) saveAutomationSnapshot(ctx context.Context, account accountsdomain.Account, accounts []domain.TheoryAccount, snapshot domain.Payload) {
	if s.repo == nil {
		return
	}
	if !isProgressTheoryPayload(snapshot) {
		return
	}
	cache := cacheStatusForPayload(snapshot, "automation")
	snapshot.CacheStatus = cache
	snapshot.Accounts = accounts
	snapshot.SelectedAccount = account.Name
	_ = s.repo.SaveTheoryRuntimeSnapshot(ctx, account.Name, account.Username, snapshot, cache)
}

type automationDecision struct {
	Options    []string
	Texts      []string
	Source     string
	Stage      string
	Reason     string
	Confidence float64
	AIAttempts int
	Insight    domain.AIInsight
}

func (s *Service) decideAutomationAnswer(ctx context.Context, snapshot domain.Payload, allowAI bool, progress func(questionNumber int, questionTitle string, completed int, message string), completed int) automationDecision {
	localDecision := automationDecisionFromLocal(snapshot.Match)
	if !allowAI {
		if len(localDecision.Options) > 0 {
			return localDecision
		}
		return automationDecision{
			Source:     "none",
			Stage:      "local-bank",
			Reason:     firstNonEmpty(localDecision.Reason, fmt.Sprintf("题库置信度 %.2f 低于 %.2f，且未允许 AI 复核", snapshot.Match.Confidence, automationLocalConfidenceThreshold)),
			Confidence: localDecision.Confidence,
		}
	}

	settings, err := s.loadAISettings(ctx)
	if err != nil {
		return automationDecision{
			Source: "ai",
			Stage:  "ai-settings",
			Reason: err.Error(),
			Insight: domain.AIInsight{
				Status: "error",
				Error:  err.Error(),
			},
		}
	}
	if !settings.Enabled || strings.TrimSpace(settings.APIKey) == "" {
		return automationDecision{
			Source:  "ai",
			Stage:   "ai-settings",
			Reason:  "AI 复核为强制步骤，但 AI 判题未启用或 API Key 未配置",
			Insight: aiInsightForSettings(settings, "not_ready", "AI 复核为强制步骤，但 AI 判题未启用或 API Key 未配置"),
		}
	}

	var lastInsight domain.AIInsight
	stage := "ai-fallback"
	if len(localDecision.Options) > 0 {
		stage = "ai-review"
	}
	for attempt := 1; attempt <= automationAIMaxAttempts; attempt++ {
		if progress != nil {
			progress(snapshot.Question.Number, snapshot.Question.Title, completed, fmt.Sprintf("AI 复核第 %d/%d 次", attempt, automationAIMaxAttempts))
		}
		lastInsight = s.evaluateAutomationWithAI(ctx, settings, snapshot.Question, snapshot.Match, attempt, completed, progress)
		if len(lastInsight.RecommendedOptions) > 0 && lastInsight.Confidence >= automationAIConfidenceThreshold {
			reason := lastInsight.Reason
			if len(localDecision.Options) > 0 {
				reason = fmt.Sprintf("AI 已复核题库候选答案。题库建议 %s；AI 建议 %s。%s", strings.Join(localDecision.Options, ","), strings.Join(lastInsight.RecommendedOptions, ","), lastInsight.Reason)
			}
			lastInsight.Reason = reason
			return automationDecision{
				Options:    append([]string(nil), lastInsight.RecommendedOptions...),
				Texts:      append([]string(nil), lastInsight.RecommendedTexts...),
				Source:     "ai",
				Stage:      stage,
				Reason:     reason,
				Confidence: lastInsight.Confidence,
				AIAttempts: attempt,
				Insight:    lastInsight,
			}
		}
		if attempt < automationAIMaxAttempts {
			select {
			case <-ctx.Done():
				return automationDecision{
					Source:     "ai",
					Stage:      stage,
					Reason:     "已手动停止",
					Confidence: lastInsight.Confidence,
					AIAttempts: attempt,
					Insight:    lastInsight,
				}
			case <-time.After(automationAIRetryDelay):
			}
		}
	}

	reason := firstNonEmpty(lastInsight.Error, lastInsight.Reason, fmt.Sprintf("AI 复核置信度低于 %.2f，停止自动提交", automationAIConfidenceThreshold))
	if strings.TrimSpace(lastInsight.Status) == "" {
		lastInsight.Status = "error"
	}
	if strings.TrimSpace(lastInsight.Error) == "" && strings.TrimSpace(lastInsight.Reason) == "" {
		lastInsight.Reason = reason
	}
	return automationDecision{
		Source:     "ai",
		Stage:      stage,
		Reason:     reason,
		Confidence: lastInsight.Confidence,
		AIAttempts: automationAIMaxAttempts,
		Insight:    lastInsight,
	}
}

func (s *Service) evaluateAutomationWithAI(ctx context.Context, settings domain.AISettings, question domain.Question, match domain.Match, attempt int, completed int, progress func(questionNumber int, questionTitle string, completed int, message string)) domain.AIInsight {
	type aiResult struct {
		insight domain.AIInsight
	}
	resultCh := make(chan aiResult, 1)
	startedAt := time.Now()
	go func() {
		resultCh <- aiResult{insight: s.evaluateWithAI(ctx, settings, question, match)}
	}()

	ticker := time.NewTicker(automationAIHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case result := <-resultCh:
			return result.insight
		case <-ticker.C:
			if progress != nil {
				elapsed := int(time.Since(startedAt).Round(time.Second).Seconds())
				progress(question.Number, question.Title, completed, fmt.Sprintf("AI 复核第 %d/%d 次，等待返回 %ds", attempt, automationAIMaxAttempts, elapsed))
			}
		case <-ctx.Done():
			return domain.AIInsight{
				Enabled: settings.Enabled,
				Ready:   strings.TrimSpace(settings.APIKey) != "",
				Status:  "stopped",
				Model:   settings.Model,
				Error:   "已手动停止",
			}
		}
	}
}

func hasAutomationAIInsight(insight domain.AIInsight) bool {
	return insight.Enabled ||
		insight.Ready ||
		strings.TrimSpace(insight.Status) != "" ||
		strings.TrimSpace(insight.Model) != "" ||
		len(insight.RecommendedOptions) > 0 ||
		len(insight.RecommendedTexts) > 0 ||
		insight.Confidence > 0 ||
		strings.TrimSpace(insight.Reason) != "" ||
		strings.TrimSpace(insight.Error) != ""
}

func automationDecisionFromLocal(match domain.Match) automationDecision {
	options, texts, source, reason, confidence := decideTheoryAnswer(match, domain.AIInsight{}, false)
	if len(options) == 0 {
		return automationDecision{
			Source:     "local-bank",
			Stage:      "local-bank",
			Reason:     firstNonEmpty(reason, fmt.Sprintf("题库置信度低于 %.2f", automationLocalConfidenceThreshold)),
			Confidence: confidence,
		}
	}
	if confidence < automationLocalConfidenceThreshold {
		return automationDecision{
			Source:     "local-bank",
			Stage:      "local-bank",
			Reason:     fmt.Sprintf("题库置信度 %.2f 低于自动提交阈值 %.2f，转 AI 判题", confidence, automationLocalConfidenceThreshold),
			Confidence: confidence,
		}
	}
	return automationDecision{
		Options:    options,
		Texts:      texts,
		Source:     source,
		Stage:      "local-bank",
		Reason:     reason,
		Confidence: confidence,
	}
}

func automationStepFromDecision(snapshot domain.Payload, decision automationDecision) domain.AutomationStep {
	return domain.AutomationStep{
		QuestionNumber:   snapshot.Question.Number,
		Question:         snapshot.Question.Title,
		MatchStatus:      snapshot.Match.Status,
		DecisionSource:   decision.Source,
		DecisionStage:    decision.Stage,
		SubmittedOptions: append([]string(nil), decision.Options...),
		SubmittedTexts:   append([]string(nil), decision.Texts...),
		DecisionReason:   decision.Reason,
		Confidence:       decision.Confidence,
		AIAttempts:       decision.AIAttempts,
		SubmittedAt:      theoryNowTS(),
	}
}

func (s *Service) snapshotWithSession(ctx context.Context, session *theorySession, account accountsdomain.Account, accounts []domain.TheoryAccount, allowAI bool) (domain.Payload, error) {
	return s.withTheorySessionRetry(ctx, session, account, func(active *theorySession) (domain.Payload, error) {
		doc, err := theoryFetchHTML(ctx, active, theoryBaseURL+theoryPaperPath)
		if err != nil {
			return domain.Payload{}, err
		}
		return s.snapshotFromDocument(ctx, doc, account, accounts, allowAI)
	})
}

func (s *Service) snapshotFromDocument(ctx context.Context, doc *html.Node, account accountsdomain.Account, accounts []domain.TheoryAccount, allowAI bool) (domain.Payload, error) {
	question := extractTheoryQuestion(doc)
	currentScore, totalScore, scoreText := extractTheoryScore(doc)
	form := extractTheoryAnswerForm(doc)
	nonce := form.Nonce
	number := form.NumberValue
	if isTheoryLoginPage(doc) {
		return domain.Payload{}, errTheorySessionInvalid
	}

	bank, err := s.loadMergedBank(ctx)
	if err != nil {
		return domain.Payload{}, err
	}
	if question.Number == 0 {
		question.Number = firstNonZero(parseTheoryNumber(number), extractTheoryQuestionNumber(doc))
	}
	answerable := isValidTheoryQuestion(question)
	if answerable && (strings.TrimSpace(nonce) == "" || strings.TrimSpace(number) == "") {
		return domain.Payload{}, fmt.Errorf("理论题提交表单缺少必要字段，请刷新远端缓存后重试")
	}
	progressMessage := extractTheoryProgressMessage(doc)
	if !answerable && strings.TrimSpace(progressMessage) == "" && strings.TrimSpace(currentScore) == "" && question.Number == 0 {
		return domain.Payload{}, fmt.Errorf("理论题页面未返回有效题目或进度，请确认账号已登录并刷新远端缓存")
	}

	match := matchTheoryQuestion(question, bank)

	settings, err := s.loadAISettings(ctx)
	if err != nil {
		return domain.Payload{}, err
	}
	aiInsight := domain.AIInsight{
		Enabled: settings.Enabled,
		Ready:   strings.TrimSpace(settings.APIKey) != "",
		Status:  "disabled",
		Model:   settings.Model,
		Reason:  "AI 判题未启用。",
	}
	if allowAI && settings.Enabled && answerable {
		aiInsight = s.evaluateWithAI(ctx, settings, question, match)
	} else if !answerable {
		aiInsight = aiInsightForSettings(settings, "completed", "远端当前没有可答题目，跳过 AI 判题。")
	}

	var reviewDashboard domain.ReviewDashboard
	var reviewItems []domain.ReviewItem
	var captured *domain.CapturedQuestion
	var capturedHistory []domain.CapturedQuestionRecord
	var cacheHistory []domain.RuntimeSnapshotRecord
	if s.repo != nil {
		if answerable {
			questionHash := theoryQuestionHash(question.Title, bankOptionsFromQuestion(question.Options))
			matchedReviewID := match.MatchedReviewID
			if matchedReviewID == 0 {
				if item, findErr := s.repo.TheoryReviewItemByHash(ctx, questionHash); findErr == nil {
					matchedReviewID = item.ID
				}
			}
			capturedRecord, captureErr := s.repo.CaptureTheoryQuestion(ctx, domain.CapturedQuestion{
				Question:        question.Title,
				SelectionType:   question.SelectionType,
				Options:         bankOptionsFromQuestion(question.Options),
				QuestionHash:    questionHash,
				MatchedReviewID: matchedReviewID,
			}, theoryBaseURL+theoryPaperPath, account.Name)
			if captureErr == nil {
				captured = &capturedRecord
			}
		}
		reviewResponse, reviewErr := s.repo.ListTheoryReviewItems(ctx, 12)
		if reviewErr == nil {
			reviewDashboard = reviewResponse.Summary
			reviewItems = reviewResponse.Items
		}
		capturedHistory, _ = s.repo.ListTheoryCapturedQuestions(ctx, account.Name, 12)
		cacheHistory, _ = s.repo.ListTheoryRuntimeSnapshots(ctx)
	}

	return domain.Payload{
		Account:    account.Name,
		Username:   account.Username,
		SourceURL:  theoryBaseURL + theoryPaperPath,
		SnapshotAt: theoryNowTS(),
		Question:   question,
		Match:      match,
		AI:         aiInsight,
		TestAccount: domain.TestAccount{
			Name:     account.Name,
			Username: account.Username,
			Password: account.Password,
			Builtin:  account.Name == defaultTheoryUser && account.Username == defaultTheoryUser,
		},
		AnswerForm:      form,
		Statistics:      statisticsFromBank(bank, question, currentScore, totalScore, scoreText, progressMessage, s.layout.TheoryBankDBPath),
		ReviewDashboard: reviewDashboard,
		ReviewItems:     reviewItems,
		LastCaptured:    captured,
		Accounts:        accounts,
		SelectedAccount: account.Name,
		CapturedHistory: capturedHistory,
		CacheHistory:    cacheHistory,
	}, nil
}

type theorySubmitResponse struct {
	Advanced           bool
	Message            string
	NextQuestion       string
	NextQuestionNumber int
	Completed          bool
	CurrentScore       string
	TotalScore         string
	ScoreText          string
	document           *html.Node
}

func theorySubmitAnswer(ctx context.Context, session *theorySession, form domain.AnswerForm, options []string) (theorySubmitResponse, error) {
	if session == nil || session.client == nil {
		return theorySubmitResponse{}, fmt.Errorf("理论题会话未初始化")
	}
	numberField := firstNonEmpty(form.NumberField, "number")
	optionField := firstNonEmpty(form.OptionField, "option")
	nonceField := firstNonEmpty(form.NonceField, "nonce")
	action := firstNonEmpty(form.Action, theoryPaperPath)
	if strings.TrimSpace(form.Nonce) == "" || strings.TrimSpace(form.NumberValue) == "" {
		return theorySubmitResponse{}, fmt.Errorf("当前题提交表单无效，请刷新远端缓存后重试")
	}

	values := url.Values{}
	values.Set(numberField, form.NumberValue)
	values.Set(nonceField, form.Nonce)
	for _, option := range options {
		if strings.TrimSpace(option) == "" {
			continue
		}
		values.Add(optionField, strings.TrimSpace(option))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, theoryBaseURL+action, strings.NewReader(values.Encode()))
	if err != nil {
		return theorySubmitResponse{}, err
	}
	httpx.ApplyBrowserHeadersWithProfile(req, theoryBaseURL+theoryPaperPath, httpx.BrowserFormRequest, session.headers)
	resp, err := session.client.Do(req)
	if err != nil {
		return theorySubmitResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return theorySubmitResponse{}, fmt.Errorf("理论题提交失败: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return theorySubmitResponse{}, err
	}
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return theorySubmitResponse{}, err
	}
	if theoryResponseWasLogin(resp) || isTheoryLoginPage(doc) {
		return theorySubmitResponse{}, errTheorySessionInvalid
	}
	session.lastUsedAt = time.Now()
	nextQuestion := extractTheoryQuestion(doc)
	message := "已提交并切换到下一题"
	advanced := true
	currentScore, totalScore, scoreText := extractTheoryScore(doc)
	progressMessage := extractTheoryProgressMessage(doc)
	completed := false
	if !isValidTheoryQuestion(nextQuestion) {
		nextQuestion.Number = firstNonZero(nextQuestion.Number, extractTheoryQuestionNumber(doc))
		completed = isTheoryProgressComplete(nextQuestion, currentScore, totalScore, scoreText, progressMessage)
		if completed {
			message = firstNonEmpty(progressMessage, "已提交，远端显示理论题已完成")
		} else {
			advanced = false
			message = "已提交，但未解析到下一题"
		}
	}
	return theorySubmitResponse{
		Advanced:           advanced,
		Message:            message,
		NextQuestion:       nextQuestion.Title,
		NextQuestionNumber: nextQuestion.Number,
		Completed:          completed,
		CurrentScore:       currentScore,
		TotalScore:         totalScore,
		ScoreText:          scoreText,
		document:           doc,
	}, nil
}

func decideTheoryAnswer(match domain.Match, ai domain.AIInsight, allowAI bool) ([]string, []string, string, string, float64) {
	if len(match.RecommendedOptions) > 0 {
		return append([]string(nil), match.RecommendedOptions...), append([]string(nil), match.RecommendedTexts...), "local-bank", match.Reason, match.Confidence
	}
	if strings.TrimSpace(match.RecommendedOption) != "" {
		options := splitAnswerOptions(match.RecommendedOption)
		return options, append([]string(nil), match.RecommendedTexts...), "local-bank", match.Reason, match.Confidence
	}
	if allowAI && len(ai.RecommendedOptions) > 0 {
		return append([]string(nil), ai.RecommendedOptions...), append([]string(nil), ai.RecommendedTexts...), "ai", ai.Reason, ai.Confidence
	}
	return nil, nil, "none", firstNonEmpty(ai.Error, ai.Reason, match.Reason, "未命中可用答案"), 0
}

func sameTheoryQuestion(left domain.Question, right domain.Question) bool {
	if left.Number != 0 && right.Number != 0 && left.Number == right.Number {
		return true
	}
	leftTitle := normalizeTheoryText(left.Title)
	rightTitle := normalizeTheoryText(right.Title)
	return leftTitle != "" && leftTitle == rightTitle
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func normalizeSubmitOptions(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		option := strings.ToUpper(strings.TrimSpace(value))
		if option == "" {
			continue
		}
		if _, exists := seen[option]; exists {
			continue
		}
		seen[option] = struct{}{}
		result = append(result, option)
	}
	sort.Strings(result)
	return result
}

func splitAnswerOptions(value string) []string {
	raw := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '|' || r == '/' || r == ' '
	})
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func indexedItemFromReview(review domain.ReviewItem) bankIndexedItem {
	options := make([]domain.BankOption, 0, len(review.Options))
	for _, option := range review.Options {
		options = append(options, domain.BankOption{
			Key:       option.Key,
			Content:   option.Content,
			InputType: option.InputType,
			IsCorrect: containsAnswerKey(review.AnswerKeys, option.Key),
		})
	}
	normalizedQuestion := normalizeTheoryText(review.Question)
	compactQuestion := compactTheoryText(review.Question)
	searchTextParts := []string{normalizedQuestion, compactQuestion}
	for _, option := range options {
		searchTextParts = append(searchTextParts, normalizeTheoryText(option.Content), compactTheoryText(option.Content))
	}
	searchTextParts = append(searchTextParts, review.AnswerTexts...)

	hash := strings.TrimSpace(review.QuestionHash)
	if hash == "" {
		hash = theoryQuestionHash(review.Question, options)
	}
	return bankIndexedItem{
		ID:                 hash,
		ReviewID:           review.ID,
		QuestionHash:       hash,
		Question:           review.Question,
		NormalizedQuestion: normalizedQuestion,
		CompactQuestion:    compactQuestion,
		SearchText:         strings.Join(uniqueTheoryStrings(searchTextParts), " "),
		Keywords:           uniqueTheoryStrings(searchTextParts),
		CorrectOptions:     append([]string(nil), review.AnswerKeys...),
		CorrectTexts:       append([]string(nil), review.AnswerTexts...),
		DuplicateGroup:     "",
		Options:            options,
		MultiAnswer:        len(review.AnswerKeys) > 1,
	}
}

func containsAnswerKey(items []string, target string) bool {
	target = strings.TrimSpace(strings.ToUpper(target))
	for _, item := range items {
		if strings.TrimSpace(strings.ToUpper(item)) == target {
			return true
		}
	}
	return false
}

func (s *Service) resolveTheoryAccount(ctx context.Context, requested string) (accountsdomain.Account, []domain.TheoryAccount, error) {
	if s.repo == nil {
		return accountsdomain.Account{}, nil, fmt.Errorf("理论题仓库未初始化")
	}
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return accountsdomain.Account{}, nil, err
	}
	enabled := make([]accountsdomain.Account, 0, len(accounts))
	for _, item := range accounts {
		if item.Enabled && strings.TrimSpace(item.Username) != "" && strings.TrimSpace(item.Password) != "" {
			enabled = append(enabled, item)
		}
	}

	builtin := accountsdomain.Account{
		Name:     defaultTheoryUser,
		Username: defaultTheoryUser,
		Password: "a111111",
		Enabled:  true,
	}
	hasBuiltin := false
	for _, item := range enabled {
		if item.Name == defaultTheoryUser || item.Username == defaultTheoryUser {
			hasBuiltin = true
			break
		}
	}
	if !hasBuiltin {
		enabled = append([]accountsdomain.Account{builtin}, enabled...)
	}
	if len(enabled) == 0 {
		return accountsdomain.Account{}, nil, fmt.Errorf("没有可用的理论题账号")
	}

	requested = strings.TrimSpace(requested)
	if requested == "" {
		if selected, err := s.repo.MetaValue(ctx, metaTheoryAccountSelect); err == nil {
			requested = strings.TrimSpace(selected)
		}
	}
	if requested == "" {
		for _, item := range enabled {
			if item.Name == defaultTheoryUser || item.Username == defaultTheoryUser {
				requested = item.Name
				break
			}
		}
	}
	if requested == "" {
		requested = enabled[0].Name
	}

	var selected accountsdomain.Account
	found := false
	for _, item := range enabled {
		if item.Name == requested || item.Username == requested {
			selected = item
			found = true
			break
		}
	}
	if !found {
		selected = enabled[0]
	}
	if err := s.repo.SetMetaValue(ctx, metaTheoryAccountSelect, selected.Name); err != nil {
		return accountsdomain.Account{}, nil, err
	}
	resultAccounts := domain.TheoryAccountsFromAccounts(enabled)
	return selected, resultAccounts, nil
}

func maxPendingCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func (s *Service) theorySession(ctx context.Context, account accountsdomain.Account) (*theorySession, error) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()

	s.evictExpiredTheorySessionsLocked()
	if existing := s.sessions[account.Name]; existing != nil && existing.username == account.Username && existing.password == account.Password {
		existing.lastUsedAt = time.Now()
		return existing, nil
	}

	client, err := newTheoryClient()
	if err != nil {
		return nil, err
	}
	session := &theorySession{
		accountName: account.Name,
		username:    account.Username,
		password:    account.Password,
		client:      client,
		headers:     httpx.NewBrowserHeaderProfile(),
		lastUsedAt:  time.Now(),
	}
	if err := theoryLogin(ctx, session, account.Username, account.Password); err != nil {
		return nil, err
	}
	s.sessions[account.Name] = session
	return session, nil
}

func (s *Service) resetTheorySession(accountName string) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	delete(s.sessions, accountName)
}

func (s *Service) evictExpiredTheorySessionsLocked() {
	if len(s.sessions) == 0 {
		return
	}
	now := time.Now()
	for key, session := range s.sessions {
		if session == nil || now.Sub(session.lastUsedAt) > theorySessionIdleTTL {
			delete(s.sessions, key)
		}
	}
}

func newTheoryClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: 20 * time.Second,
		Jar:     jar,
	}, nil
}

func theoryLogin(ctx context.Context, session *theorySession, username string, password string) error {
	if session == nil || session.client == nil {
		return fmt.Errorf("理论题会话未初始化")
	}
	body := strings.NewReader(encodeTheoryLoginForm(username, password))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, theoryBaseURL+theoryLoginPath, body)
	if err != nil {
		return err
	}
	httpx.ApplyBrowserHeadersWithProfile(req, theoryBaseURL+"/", httpx.BrowserFormRequest, session.headers)
	resp, err := session.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("理论题登录失败: %s", resp.Status)
	}
	session.lastUsedAt = time.Now()
	return nil
}

func encodeTheoryLoginForm(username string, password string) string {
	values := url.Values{}
	values.Set("name", username)
	values.Set("password", password)
	return values.Encode()
}

func theoryFetchHTML(ctx context.Context, session *theorySession, url string) (*html.Node, error) {
	if session == nil || session.client == nil {
		return nil, fmt.Errorf("理论题会话未初始化")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpx.ApplyBrowserHeadersWithProfile(req, theoryBaseURL+theoryPaperPath, httpx.BrowserNavigationRequest, session.headers)
	resp, err := session.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("理论题页面请求失败: %s", resp.Status)
	}
	session.lastUsedAt = time.Now()
	return html.Parse(resp.Body)
}

func waitTheoryCooldown(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) withTheorySessionRetry(ctx context.Context, session *theorySession, account accountsdomain.Account, fn func(*theorySession) (domain.Payload, error)) (domain.Payload, error) {
	if session == nil {
		return domain.Payload{}, fmt.Errorf("理论题会话未初始化")
	}
	payload, err := fn(session)
	if !errors.Is(err, errTheorySessionInvalid) {
		return payload, err
	}
	s.resetTheorySession(account.Name)
	refreshed, refreshErr := s.theorySession(ctx, account)
	if refreshErr != nil {
		return domain.Payload{}, refreshErr
	}
	return fn(refreshed)
}

func (s *Service) withTheorySubmitRetry(ctx context.Context, session *theorySession, account accountsdomain.Account, fn func(*theorySession) (theorySubmitResponse, error)) (theorySubmitResponse, error) {
	if session == nil {
		return theorySubmitResponse{}, fmt.Errorf("理论题会话未初始化")
	}
	resp, err := fn(session)
	if !errors.Is(err, errTheorySessionInvalid) {
		return resp, err
	}
	s.resetTheorySession(account.Name)
	refreshed, refreshErr := s.theorySession(ctx, account)
	if refreshErr != nil {
		return theorySubmitResponse{}, refreshErr
	}
	return fn(refreshed)
}

func validateTheoryPaperSnapshot(root *html.Node, question domain.Question, nonce string, number string) error {
	if isTheoryLoginPage(root) {
		return errTheorySessionInvalid
	}
	if !isValidTheoryQuestion(question) {
		return fmt.Errorf("理论题页面未返回有效题目，请确认账号已登录并刷新远端缓存")
	}
	if strings.TrimSpace(nonce) == "" || strings.TrimSpace(number) == "" {
		return fmt.Errorf("理论题提交表单缺少必要字段，请刷新远端缓存后重试")
	}
	return nil
}

func isValidTheoryPayload(payload domain.Payload) bool {
	return isValidTheoryQuestion(payload.Question) &&
		strings.TrimSpace(payload.AnswerForm.Nonce) != "" &&
		strings.TrimSpace(payload.AnswerForm.NumberValue) != ""
}

func isProgressTheoryPayload(payload domain.Payload) bool {
	return isValidTheoryPayload(payload) ||
		strings.TrimSpace(payload.Question.Title) != "" ||
		payload.Question.Number > 0 ||
		strings.TrimSpace(payload.Statistics.CurrentScore) != "" ||
		strings.TrimSpace(payload.Statistics.ScoreText) != "" ||
		strings.TrimSpace(payload.Statistics.ProgressMessage) != ""
}

func isValidTheoryQuestion(question domain.Question) bool {
	return strings.TrimSpace(question.Title) != "" &&
		!isLoginQuestionTitle(question.Title) &&
		validTheoryOptionCount(question.Options) > 0
}

func validTheoryOptionCount(options []domain.Option) int {
	count := 0
	for _, option := range options {
		if strings.TrimSpace(option.Key) != "" {
			count++
		}
	}
	return count
}

func isTheoryLoginPage(root *html.Node) bool {
	if root == nil {
		return false
	}
	text := compactTheoryText(theoryText(root))
	hasLoginText := strings.Contains(text, "登录") || strings.Contains(text, "登陆")
	hasForgotText := strings.Contains(text, "忘记密码") || strings.Contains(text, "找回密码")
	hasPasswordInput := false
	hasLoginForm := false
	walkTheory(root, func(node *html.Node) {
		if node.Type != html.ElementNode {
			return
		}
		switch node.Data {
		case "input":
			inputType := strings.ToLower(strings.TrimSpace(attrTheory(node, "type")))
			name := strings.ToLower(strings.TrimSpace(attrTheory(node, "name")))
			if inputType == "password" || name == "password" {
				hasPasswordInput = true
			}
		case "form":
			action := strings.ToLower(strings.TrimSpace(attrTheory(node, "action")))
			if strings.Contains(action, "login") {
				hasLoginForm = true
			}
		}
	})
	return hasPasswordInput && (hasLoginForm || hasForgotText || hasLoginText)
}

func isLoginQuestionTitle(title string) bool {
	text := compactTheoryText(title)
	hasLoginText := strings.Contains(text, "登录") || strings.Contains(text, "登陆")
	return hasLoginText && (strings.Contains(text, "忘记密码") || strings.Contains(text, "找回密码") || len([]rune(text)) <= 12)
}

func theoryResponseWasLogin(resp *http.Response) bool {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	path := strings.TrimRight(resp.Request.URL.Path, "/")
	return path == theoryLoginPath
}

func extractTheoryQuestion(root *html.Node) domain.Question {
	result := domain.Question{}
	form := locateTheoryAnswerForm(root)
	if form == nil {
		return result
	}
	var foundText string
	walkTheory(root, func(node *html.Node) {
		if node.Type != html.ElementNode {
			return
		}
		if node.Data == "strong" && result.Number == 0 {
			txt := strings.TrimSpace(theoryText(node))
			if strings.HasPrefix(txt, "第") && strings.Contains(txt, "题") {
				fmt.Sscanf(txt, "第%d题", &result.Number)
			}
		}
		if node != form {
			return
		}
		if foundText == "" {
			foundText = firstNonEmpty(strings.TrimSpace(previousSiblingText(node)), strings.TrimSpace(theoryQuestionTextNearForm(node)))
		}
		walkTheory(node, func(child *html.Node) {
			if child.Type != html.ElementNode || child.Data != "input" {
				return
			}
			inputType := strings.ToLower(strings.TrimSpace(attrTheory(child, "type")))
			if inputType != "radio" && inputType != "checkbox" {
				return
			}
			if result.SelectionType == "" {
				if inputType == "checkbox" {
					result.SelectionType = "multiple"
				} else {
					result.SelectionType = "single"
				}
			}
			key := strings.TrimSpace(attrTheory(child, "value"))
			label := cleanOptionText(strings.TrimSpace(optionLabelText(child)))
			result.Options = append(result.Options, domain.Option{
				Key:       key,
				Content:   label,
				InputType: inputType,
			})
		})
	})
	result.Title = cleanQuestionText(foundText)
	result.RawText = foundText
	if result.SelectionType == "" {
		result.SelectionType = "single"
	}
	return result
}

func extractTheoryQuestionNumber(root *html.Node) int {
	text := strings.Join(strings.Fields(theoryText(root)), " ")
	for _, pattern := range theoryQuestionNumberPatterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) > 1 {
			if value := parseTheoryNumber(match[1]); value > 0 {
				return value
			}
		}
	}
	return 0
}

func extractTheoryInput(root *html.Node, name string) string {
	var result string
	walkTheory(root, func(node *html.Node) {
		if result != "" || node.Type != html.ElementNode || node.Data != "input" {
			return
		}
		if attrTheory(node, "name") == name {
			result = strings.TrimSpace(attrTheory(node, "value"))
		}
	})
	return result
}

func extractTheoryAnswerForm(root *html.Node) domain.AnswerForm {
	formNode := locateTheoryAnswerForm(root)
	if formNode == nil {
		return domain.AnswerForm{}
	}
	form := domain.AnswerForm{
		Action:     normalizeTheoryAction(attrTheory(formNode, "action")),
		Method:     strings.ToUpper(firstNonEmpty(attrTheory(formNode, "method"), http.MethodPost)),
		NonceField: "nonce",
	}
	walkTheory(formNode, func(node *html.Node) {
		if node.Type != html.ElementNode || node.Data != "input" {
			return
		}
		inputType := strings.ToLower(strings.TrimSpace(attrTheory(node, "type")))
		name := strings.TrimSpace(attrTheory(node, "name"))
		value := strings.TrimSpace(attrTheory(node, "value"))
		switch inputType {
		case "radio", "checkbox":
			if form.OptionField == "" && name != "" {
				form.OptionField = name
			}
			if !form.AllowsMultiple && inputType == "checkbox" {
				form.AllowsMultiple = true
			}
		default:
			lowerName := strings.ToLower(name)
			switch {
			case form.Nonce == "" && (lowerName == "nonce" || strings.Contains(lowerName, "nonce")):
				form.NonceField = firstNonEmpty(name, form.NonceField)
				form.Nonce = value
			case form.NumberValue == "" && (lowerName == "number" || strings.Contains(lowerName, "number")):
				form.NumberField = firstNonEmpty(name, "number")
				form.NumberValue = value
			}
		}
	})
	if form.OptionField == "" {
		form.OptionField = "option"
	}
	if form.NumberField == "" {
		form.NumberField = "number"
	}
	if form.Action == "" {
		form.Action = theoryPaperPath
	}
	return form
}

func parseTheoryNumber(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return 0
	}
	return result
}

func extractTheoryScore(root *html.Node) (string, string, string) {
	text := strings.Join(strings.Fields(theoryText(root)), " ")
	for _, pattern := range theoryScorePatterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) > 1 {
			current := strings.TrimSpace(match[1])
			total := ""
			if len(match) > 2 {
				total = strings.TrimSpace(match[2])
			}
			if total != "" {
				return current, total, fmt.Sprintf("%s / %s", current, total)
			}
			return current, "", current
		}
	}
	return "", "", ""
}

func extractTheoryProgressMessage(root *html.Node) string {
	text := strings.Join(strings.Fields(theoryText(root)), " ")
	compact := compactTheoryText(text)
	switch {
	case strings.Contains(compact, "恭喜您完成选择题答题"):
		return "远端显示理论题已完成"
	case strings.Contains(compact, "已完成"):
		return "远端显示理论题已完成"
	case strings.Contains(compact, "答题结束"):
		return "远端显示理论题答题结束"
	case strings.Contains(compact, "全部完成"):
		return "远端显示理论题全部完成"
	case strings.Contains(compact, "没有题目") || strings.Contains(compact, "暂无题目"):
		return "远端当前没有可提交的理论题"
	default:
		return ""
	}
}

func matchTheoryQuestion(question domain.Question, bank *bankStore) domain.Match {
	if strings.TrimSpace(question.Title) == "" || bank == nil || len(bank.items) == 0 {
		return domain.Match{Status: "unmatched", Method: "local-bank"}
	}

	type candidate struct {
		entry  bankIndexedItem
		score  float64
		reason string
	}

	candidates := make([]candidate, 0, len(bank.items))
	for _, item := range bank.items {
		score, reason := scoreBankItem(normalizeTheoryText(question.Title), compactTheoryText(question.Title), item)
		candidates = append(candidates, candidate{
			entry:  item,
			score:  score,
			reason: reason,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	match := domain.Match{
		Status: "matched",
		Method: "local-bank",
	}
	limit := 3
	if len(candidates) < limit {
		limit = len(candidates)
	}
	for idx := 0; idx < limit; idx++ {
		item := candidates[idx]
		match.Candidates = append(match.Candidates, domain.MatchPreview{
			Question:           item.entry.Question,
			RecommendedOption:  strings.Join(alignOptions(question.Options, item.entry.CorrectTexts, item.entry.CorrectOptions), ","),
			RecommendedText:    strings.Join(item.entry.CorrectTexts, " / "),
			RecommendedOptions: append([]string(nil), item.entry.CorrectOptions...),
			RecommendedTexts:   append([]string(nil), item.entry.CorrectTexts...),
			IsMultiAnswer:      item.entry.MultiAnswer,
			Confidence:         round(item.score),
		})
	}
	if len(candidates) == 0 || candidates[0].score < 0.35 {
		match.Status = "weak_match"
		match.Reason = "题库中没有足够接近的题目，建议人工确认或后续接入 AI 判题。"
		return match
	}

	best := candidates[0]
	match.Confidence = round(best.score)
	match.MatchedReviewID = best.entry.ReviewID
	aligned := alignOptions(question.Options, best.entry.CorrectTexts, best.entry.CorrectOptions)
	match.RecommendedOptions = aligned
	match.RecommendedTexts = append([]string(nil), best.entry.CorrectTexts...)
	match.RecommendedOption = strings.Join(aligned, ",")
	match.RecommendedText = strings.Join(best.entry.CorrectTexts, " / ")
	match.ReferenceQuestion = best.entry.Question
	match.IsMultiAnswer = best.entry.MultiAnswer
	match.Reason = "当前答案来自本地清洗题库的相似题匹配。命中方式：" + best.reason + "。"
	return match
}

func alignOptions(options []domain.Option, expectedTexts []string, fallbackKeys []string) []string {
	if len(options) == 0 {
		return append([]string(nil), fallbackKeys...)
	}
	results := make([]string, 0, len(expectedTexts))
	used := map[string]struct{}{}
	for idx, expectedText := range expectedTexts {
		fallbackKey := ""
		if idx < len(fallbackKeys) {
			fallbackKey = fallbackKeys[idx]
		}
		bestKey := fallbackKey
		bestScore := 0.0
		for _, option := range options {
			if _, exists := used[option.Key]; exists {
				continue
			}
			score := similarity(expectedText, option.Content)
			if score > bestScore {
				bestScore = score
				bestKey = option.Key
			}
		}
		if bestKey != "" {
			used[bestKey] = struct{}{}
			results = append(results, bestKey)
		}
	}
	if len(results) == 0 {
		return append([]string(nil), fallbackKeys...)
	}
	sort.Strings(results)
	return results
}

func similarity(left string, right string) float64 {
	a := normalizeTheoryText(left)
	b := normalizeTheoryText(right)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	setA := strings.Fields(a)
	setB := strings.Fields(b)
	if len(setA) == 0 || len(setB) == 0 {
		if strings.Contains(a, b) || strings.Contains(b, a) {
			shorter := math.Min(float64(len([]rune(a))), float64(len([]rune(b))))
			longer := math.Max(float64(len([]rune(a))), float64(len([]rune(b))))
			return shorter / longer
		}
		return 0
	}
	seen := map[string]bool{}
	inter := 0.0
	for _, item := range setA {
		seen[item] = true
	}
	union := map[string]bool{}
	for _, item := range setA {
		union[item] = true
	}
	for _, item := range setB {
		if seen[item] {
			inter++
		}
		union[item] = true
	}
	jaccard := inter / float64(len(union))
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return math.Max(jaccard, 0.82)
	}
	return jaccard
}

func normalizeTheoryText(value string) string {
	replacer := strings.NewReplacer(
		"（", " ", "）", " ", "(", " ", ")", " ",
		"。", " ", "，", " ", "、", " ", "：", " ", "；", " ",
		"？", " ", "?", " ", ".", " ", ",", " ", "!", " ", "！", " ",
		"“", " ", "”", " ", "\"", " ", "'", " ", "‘", " ", "’", " ",
		"【", " ", "】", " ", "[", " ", "]", " ", "《", " ", "》", " ",
		"　", " ",
	)
	value = strings.ToLower(strings.TrimSpace(toHalfWidth(value)))
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = replacer.Replace(value)
	value = strings.ReplaceAll(value, "第1题", " ")
	value = strings.ReplaceAll(value, "第 1 题", " ")
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func compactTheoryText(value string) string {
	return strings.ReplaceAll(normalizeTheoryText(value), " ", "")
}

func toHalfWidth(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, char := range value {
		switch {
		case char == 12288:
			builder.WriteRune(' ')
		case char >= 65281 && char <= 65374:
			builder.WriteRune(char - 65248)
		default:
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func walkTheory(node *html.Node, fn func(*html.Node)) {
	fn(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkTheory(child, fn)
	}
}

func attrTheory(node *html.Node, key string) string {
	for _, item := range node.Attr {
		if item.Key == key {
			return item.Val
		}
	}
	return ""
}

func theoryText(node *html.Node) string {
	var parts []string
	walkTheory(node, func(current *html.Node) {
		if current.Type == html.TextNode {
			text := strings.TrimSpace(current.Data)
			if text != "" {
				parts = append(parts, text)
			}
		}
	})
	return strings.Join(parts, " ")
}

func previousSiblingText(node *html.Node) string {
	for prev := node.PrevSibling; prev != nil; prev = prev.PrevSibling {
		text := strings.TrimSpace(theoryText(prev))
		if text != "" {
			return text
		}
	}
	if node.Parent != nil {
		return strings.TrimSpace(theoryText(node.Parent))
	}
	return ""
}

func theoryQuestionTextNearForm(node *html.Node) string {
	if node == nil {
		return ""
	}
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		for child := parent.FirstChild; child != nil; child = child.NextSibling {
			if child == node {
				break
			}
			text := strings.TrimSpace(theoryText(child))
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func nextTextAfter(node *html.Node) string {
	if node.NextSibling != nil {
		return theoryText(node.NextSibling)
	}
	return ""
}

func optionLabelText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if id := strings.TrimSpace(attrTheory(node, "id")); id != "" {
		if label := findTheoryLabelByAttr(node, "for", id); label != "" {
			return label
		}
	}
	if parent := node.Parent; parent != nil && parent.Type == html.ElementNode && parent.Data == "label" {
		return strings.TrimSpace(theoryText(parent))
	}
	if label := firstNonEmpty(strings.TrimSpace(nextTextAfter(node)), strings.TrimSpace(theoryText(node.Parent))); label != "" {
		return label
	}
	return ""
}

func findTheoryLabelByAttr(root *html.Node, attrKey string, attrValue string) string {
	var result string
	for parent := root; parent != nil && result == ""; parent = parent.Parent {
		walkTheory(parent, func(node *html.Node) {
			if result != "" || node.Type != html.ElementNode || node.Data != "label" {
				return
			}
			if strings.TrimSpace(attrTheory(node, attrKey)) == attrValue {
				result = strings.TrimSpace(theoryText(node))
			}
		})
	}
	return result
}

func locateTheoryAnswerForm(root *html.Node) *html.Node {
	var best *html.Node
	bestScore := -1
	walkTheory(root, func(node *html.Node) {
		if node.Type != html.ElementNode || node.Data != "form" {
			return
		}
		score := 0
		walkTheory(node, func(child *html.Node) {
			if child.Type != html.ElementNode || child.Data != "input" {
				return
			}
			inputType := strings.ToLower(strings.TrimSpace(attrTheory(child, "type")))
			switch inputType {
			case "radio", "checkbox":
				score += 4
			case "hidden":
				name := strings.ToLower(strings.TrimSpace(attrTheory(child, "name")))
				if name == "nonce" || strings.Contains(name, "nonce") || name == "number" || strings.Contains(name, "number") {
					score += 2
				}
			}
		})
		if score > bestScore {
			best = node
			bestScore = score
		}
	})
	return best
}

func normalizeTheoryAction(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return theoryPaperPath
	}
	if strings.HasPrefix(action, "http://") || strings.HasPrefix(action, "https://") {
		parsed, err := url.Parse(action)
		if err == nil {
			return firstNonEmpty(parsed.Path, theoryPaperPath)
		}
	}
	if strings.HasPrefix(action, "/") {
		return action
	}
	return "/" + strings.TrimPrefix(action, "/")
}

func cleanQuestionText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "第1题")
	value = strings.TrimSpace(value)
	return value
}

func cleanOptionText(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"A.", "B.", "C.", "D.", "A．", "B．", "C．", "D．"} {
		value = strings.TrimPrefix(value, prefix)
	}
	return strings.TrimSpace(value)
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}

func theoryNowTS() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func bankOptionsFromQuestion(options []domain.Option) []domain.BankOption {
	result := make([]domain.BankOption, 0, len(options))
	for _, option := range options {
		result = append(result, domain.BankOption{
			Key:       option.Key,
			Content:   option.Content,
			InputType: option.InputType,
			IsCorrect: false,
		})
	}
	return result
}

func theoryQuestionHash(question string, options []domain.BankOption) string {
	parts := make([]string, 0, len(options))
	for _, option := range options {
		parts = append(parts, option.Key+":"+normalizeTheoryText(option.Content))
	}
	sum := sha1.Sum([]byte(normalizeTheoryText(question) + "|" + strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func selectionTypeFromOptions(answerKeys []string) string {
	if len(answerKeys) > 1 {
		return "multiple"
	}
	return "single"
}

func sourceKindFromRef(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".docx"):
		return "docx"
	case strings.Contains(lower, ".standardized."):
		return "standardized-json"
	case strings.Contains(lower, ".normalized."):
		return "normalized-json"
	default:
		return "raw-json"
	}
}
