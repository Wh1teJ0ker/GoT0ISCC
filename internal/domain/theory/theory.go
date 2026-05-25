package theory

import accountsdomain "got0iscc/desktop/internal/domain/accounts"

type Payload struct {
	Account         string                   `json:"account"`
	Username        string                   `json:"username"`
	SourceURL       string                   `json:"source_url"`
	SnapshotAt      string                   `json:"snapshot_at"`
	Question        Question                 `json:"question"`
	Match           Match                    `json:"match"`
	AI              AIInsight                `json:"ai"`
	TestAccount     TestAccount              `json:"test_account"`
	AnswerForm      AnswerForm               `json:"answer_form"`
	Statistics      Statistics               `json:"statistics"`
	ReviewDashboard ReviewDashboard          `json:"review_dashboard"`
	ReviewItems     []ReviewItem             `json:"review_items"`
	LastCaptured    *CapturedQuestion        `json:"last_captured,omitempty"`
	Accounts        []TheoryAccount          `json:"accounts"`
	SelectedAccount string                   `json:"selected_account"`
	CacheStatus     CacheStatus              `json:"cache_status"`
	CapturedHistory []CapturedQuestionRecord `json:"captured_history"`
	CacheHistory    []RuntimeSnapshotRecord  `json:"cache_history"`
}

type Question struct {
	Number        int      `json:"number"`
	Title         string   `json:"title"`
	Options       []Option `json:"options"`
	RawText       string   `json:"raw_text"`
	SelectionType string   `json:"selection_type"`
}

type Option struct {
	Key       string `json:"key"`
	Content   string `json:"content"`
	InputType string `json:"input_type"`
}

type Match struct {
	Status             string         `json:"status"`
	Method             string         `json:"method"`
	Confidence         float64        `json:"confidence"`
	MatchedReviewID    int64          `json:"matched_review_id,omitempty"`
	RecommendedOption  string         `json:"recommended_option"`
	RecommendedText    string         `json:"recommended_text"`
	RecommendedOptions []string       `json:"recommended_options"`
	RecommendedTexts   []string       `json:"recommended_texts"`
	ReferenceQuestion  string         `json:"reference_question"`
	Reason             string         `json:"reason"`
	IsMultiAnswer      bool           `json:"is_multi_answer"`
	Candidates         []MatchPreview `json:"candidates"`
}

type MatchPreview struct {
	Question           string   `json:"question"`
	RecommendedOption  string   `json:"recommended_option"`
	RecommendedText    string   `json:"recommended_text"`
	RecommendedOptions []string `json:"recommended_options"`
	RecommendedTexts   []string `json:"recommended_texts"`
	IsMultiAnswer      bool     `json:"is_multi_answer"`
	Confidence         float64  `json:"confidence"`
}

type TestAccount struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
	Builtin  bool   `json:"builtin"`
}

type AnswerForm struct {
	Action         string `json:"action"`
	Method         string `json:"method"`
	Nonce          string `json:"nonce"`
	NonceField     string `json:"nonce_field"`
	OptionField    string `json:"option_field"`
	NumberField    string `json:"number_field"`
	NumberValue    string `json:"number_value"`
	AllowsMultiple bool   `json:"allows_multiple"`
}

type ManualSubmitRequest struct {
	Account string   `json:"account"`
	Options []string `json:"options"`
}

type ManualSubmitResponse struct {
	Success            bool     `json:"success"`
	Message            string   `json:"message"`
	SubmittedOptions   []string `json:"submitted_options"`
	NextQuestion       string   `json:"next_question"`
	NextQuestionNumber int      `json:"next_question_number"`
	Payload            Payload  `json:"payload"`
}

type Statistics struct {
	BankSize                int    `json:"bank_size"`
	SearchableBankSize      int    `json:"searchable_bank_size"`
	DuplicateQuestionGroups int    `json:"duplicate_question_groups"`
	MultiAnswerCount        int    `json:"multi_answer_count"`
	GeneratedAt             string `json:"generated_at"`
	QuestionNumber          int    `json:"question_number"`
	CurrentScore            string `json:"current_score"`
	TotalScore              string `json:"total_score"`
	ScoreText               string `json:"score_text"`
	OptionCount             int    `json:"option_count"`
	DatabasePath            string `json:"database_path"`
	Answerable              bool   `json:"answerable"`
	Completed               bool   `json:"completed"`
	ProgressMessage         string `json:"progress_message"`
}

type BankOption struct {
	Key       string `json:"key"`
	Content   string `json:"content"`
	InputType string `json:"input_type,omitempty"`
	IsCorrect bool   `json:"is_correct"`
}

type BankSearchHit struct {
	ID                 string       `json:"id"`
	Question           string       `json:"question"`
	NormalizedQuestion string       `json:"normalized_question"`
	CorrectOptions     []string     `json:"correct_options"`
	CorrectTexts       []string     `json:"correct_texts"`
	Score              float64      `json:"score"`
	MatchReason        string       `json:"match_reason"`
	Keywords           []string     `json:"keywords"`
	DuplicateGroup     string       `json:"duplicate_group,omitempty"`
	MultiAnswer        bool         `json:"multi_answer"`
	Options            []BankOption `json:"options"`
}

type BankSummary struct {
	RawCount         int    `json:"raw_count"`
	SearchableCount  int    `json:"searchable_count"`
	DuplicateGroups  int    `json:"duplicate_groups"`
	MultiAnswerCount int    `json:"multi_answer_count"`
	GeneratedAt      string `json:"generated_at"`
	SourcePath       string `json:"source_path"`
	IndexPath        string `json:"index_path"`
	DatabasePath     string `json:"database_path"`
	ReviewPending    int    `json:"review_pending"`
	CapturedCount    int    `json:"captured_count"`
	CaptureHits      int    `json:"capture_hits"`
}

type BankSearchResponse struct {
	Query           string          `json:"query"`
	NormalizedQuery string          `json:"normalized_query"`
	Total           int             `json:"total"`
	Limit           int             `json:"limit"`
	Summary         BankSummary     `json:"summary"`
	Items           []BankSearchHit `json:"items"`
}

type AISettings struct {
	Enabled         bool   `json:"enabled" yaml:"enabled"`
	BaseURL         string `json:"base_url" yaml:"base_url"`
	APIKey          string `json:"api_key" yaml:"api_key"`
	Model           string `json:"model" yaml:"model"`
	ReasoningEffort string `json:"reasoning_effort" yaml:"reasoning_effort"`
	Prompt          string `json:"prompt" yaml:"prompt"`
	UpdatedAt       string `json:"updated_at" yaml:"updated_at"`
}

type AISettingsPayload struct {
	Settings      AISettings `json:"settings"`
	ConfigPath    string     `json:"config_path"`
	MaskedAPIKey  string     `json:"masked_api_key"`
	Ready         bool       `json:"ready"`
	ProviderLabel string     `json:"provider_label"`
	SupportsModel []string   `json:"supports_model"`
}

type AIAvailability struct {
	OK             bool   `json:"ok"`
	Status         string `json:"status"`
	Model          string `json:"model"`
	BaseURL        string `json:"base_url"`
	LatencyMS      int64  `json:"latency_ms"`
	HTTPStatusCode int    `json:"http_status_code"`
	Message        string `json:"message"`
	CheckedAt      string `json:"checked_at"`
}

type AIInsight struct {
	Enabled            bool     `json:"enabled"`
	Ready              bool     `json:"ready"`
	Status             string   `json:"status"`
	Model              string   `json:"model"`
	RecommendedOptions []string `json:"recommended_options"`
	RecommendedTexts   []string `json:"recommended_texts"`
	Confidence         float64  `json:"confidence"`
	Reason             string   `json:"reason"`
	Error              string   `json:"error"`
}

type ReviewDashboard struct {
	TotalQuestions    int    `json:"total_questions"`
	ReviewedQuestions int    `json:"reviewed_questions"`
	PendingReview     int    `json:"pending_review"`
	CapturedQuestions int    `json:"captured_questions"`
	CaptureHits       int    `json:"capture_hits"`
	LastCapturedAt    string `json:"last_captured_at"`
	DatabasePath      string `json:"database_path"`
}

type ReviewItem struct {
	ID                 int64        `json:"id"`
	Question           string       `json:"question"`
	NormalizedQuestion string       `json:"normalized_question"`
	SelectionType      string       `json:"selection_type"`
	SourceKind         string       `json:"source_kind"`
	SourceRef          string       `json:"source_ref"`
	Options            []BankOption `json:"options"`
	AnswerKeys         []string     `json:"answer_keys"`
	AnswerTexts        []string     `json:"answer_texts"`
	NeedsReview        bool         `json:"needs_review"`
	ReviewStatus       string       `json:"review_status"`
	ReviewReason       string       `json:"review_reason"`
	Confidence         float64      `json:"confidence"`
	QuestionHash       string       `json:"question_hash"`
	CapturedAt         string       `json:"captured_at"`
	CreatedAt          string       `json:"created_at"`
	UpdatedAt          string       `json:"updated_at"`
}

type CapturedQuestion struct {
	Question        string       `json:"question"`
	SelectionType   string       `json:"selection_type"`
	Options         []BankOption `json:"options"`
	QuestionHash    string       `json:"question_hash"`
	MatchedReviewID int64        `json:"matched_review_id"`
	CapturedAt      string       `json:"captured_at"`
}

type TheoryAccount struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Enabled  bool   `json:"enabled"`
}

type CacheStatus struct {
	HasSnapshot      bool   `json:"has_snapshot"`
	HasQuestion      bool   `json:"has_question"`
	Answerable       bool   `json:"answerable"`
	Completed        bool   `json:"completed"`
	CachedAt         string `json:"cached_at"`
	LastRemoteSyncAt string `json:"last_remote_sync_at"`
	LastRemoteError  string `json:"last_remote_error"`
	Source           string `json:"source"`
	Message          string `json:"message"`
}

type CapturedQuestionRecord struct {
	ID              int64        `json:"id"`
	Account         string       `json:"account"`
	Question        string       `json:"question"`
	SelectionType   string       `json:"selection_type"`
	Options         []BankOption `json:"options"`
	QuestionHash    string       `json:"question_hash"`
	MatchedReviewID int64        `json:"matched_review_id"`
	SourceURL       string       `json:"source_url"`
	CreatedAt       string       `json:"created_at"`
}

type RuntimeSnapshotRecord struct {
	Account          string `json:"account"`
	Username         string `json:"username"`
	QuestionTitle    string `json:"question_title"`
	QuestionNumber   int    `json:"question_number"`
	CachedAt         string `json:"cached_at"`
	LastRemoteSyncAt string `json:"last_remote_sync_at"`
	LastRemoteError  string `json:"last_remote_error"`
	Source           string `json:"source"`
}

type SnapshotRequest struct {
	Account string `json:"account"`
	Refresh bool   `json:"refresh"`
}

type ReviewListResponse struct {
	Summary ReviewDashboard `json:"summary"`
	Items   []ReviewItem    `json:"items"`
}

type ReviewDecision struct {
	ID            int64        `json:"id"`
	Question      string       `json:"question"`
	SelectionType string       `json:"selection_type"`
	Options       []BankOption `json:"options"`
	AnswerKeys    []string     `json:"answer_keys"`
	AnswerTexts   []string     `json:"answer_texts"`
	ReviewStatus  string       `json:"review_status"`
	ReviewReason  string       `json:"review_reason"`
}

type AIReviewRequest struct {
	Limit           int    `json:"limit"`
	BatchSize       int    `json:"batch_size"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	DryRun          bool   `json:"dry_run"`
	OnlyPending     bool   `json:"only_pending"`
	ReasoningEffort string `json:"reasoning_effort"`
}

type AIReviewStatus struct {
	Running         bool    `json:"running"`
	StartedAt       string  `json:"started_at"`
	FinishedAt      string  `json:"finished_at"`
	Status          string  `json:"status"`
	Message         string  `json:"message"`
	Reviewed        int     `json:"reviewed"`
	Approved        int     `json:"approved"`
	Remaining       int     `json:"remaining"`
	CurrentBatch    int     `json:"current_batch"`
	BatchSize       int     `json:"batch_size"`
	Limit           int     `json:"limit"`
	DryRun          bool    `json:"dry_run"`
	ReasoningEffort string  `json:"reasoning_effort"`
	LastError       string  `json:"last_error"`
	LastUpdatedAt   string  `json:"last_updated_at"`
	LastConfidence  float64 `json:"last_confidence"`
}

type AutomationRequest struct {
	MaxQuestions   int    `json:"max_questions"`
	AllowAI        bool   `json:"allow_ai"`
	StopOnNoAnswer bool   `json:"stop_on_no_answer"`
	Account        string `json:"account"`
}

type AutomationStep struct {
	QuestionNumber     int      `json:"question_number"`
	Question           string   `json:"question"`
	MatchStatus        string   `json:"match_status"`
	DecisionSource     string   `json:"decision_source"`
	DecisionStage      string   `json:"decision_stage"`
	SubmittedOptions   []string `json:"submitted_options"`
	SubmittedTexts     []string `json:"submitted_texts"`
	DecisionReason     string   `json:"decision_reason"`
	Confidence         float64  `json:"confidence"`
	AIAttempts         int      `json:"ai_attempts"`
	Success            bool     `json:"success"`
	Message            string   `json:"message"`
	NextQuestion       string   `json:"next_question"`
	NextQuestionNumber int      `json:"next_question_number"`
	SubmittedAt        string   `json:"submitted_at"`
}

type AutomationResult struct {
	StartedAt     string           `json:"started_at"`
	FinishedAt    string           `json:"finished_at"`
	Completed     int              `json:"completed"`
	StoppedReason string           `json:"stopped_reason"`
	FinalQuestion string           `json:"final_question"`
	FinalNumber   int              `json:"final_number"`
	Results       []AutomationStep `json:"results"`
}

type AutomationStatus struct {
	Running          bool              `json:"running"`
	StartedAt        string            `json:"started_at"`
	FinishedAt       string            `json:"finished_at"`
	Status           string            `json:"status"`
	Message          string            `json:"message"`
	Completed        int               `json:"completed"`
	MaxQuestions     int               `json:"max_questions"`
	CurrentNumber    int               `json:"current_number"`
	CurrentTitle     string            `json:"current_title"`
	CurrentStartedAt string            `json:"current_started_at"`
	LastUpdatedAt    string            `json:"last_updated_at"`
	LastError        string            `json:"last_error"`
	Result           *AutomationResult `json:"result,omitempty"`
}

func TheoryAccountsFromAccounts(items []accountsdomain.Account) []TheoryAccount {
	result := make([]TheoryAccount, 0, len(items))
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		result = append(result, TheoryAccount{
			Name:     item.Name,
			Username: item.Username,
			Enabled:  item.Enabled,
		})
	}
	return result
}
