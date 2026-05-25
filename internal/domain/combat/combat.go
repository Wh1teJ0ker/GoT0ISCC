package combat

type Payload struct {
	Account    string         `json:"account"`
	Username   string         `json:"username"`
	SourceURL  string         `json:"source_url"`
	SnapshotAt string         `json:"snapshot_at"`
	Nonce      string         `json:"nonce"`
	Summary    Summary        `json:"summary"`
	Resources  []Resource     `json:"resources"`
	Stages     []Stage        `json:"stages"`
	Scoreboard []ScoreEntry   `json:"scoreboard"`
	Notices    []string       `json:"notices"`
	Challenges []Challenge    `json:"challenges"`
	Submission SubmissionForm `json:"submission"`
}

type Summary struct {
	StageCount      int    `json:"stage_count"`
	ResourceCount   int    `json:"resource_count"`
	ScoreboardCount int    `json:"scoreboard_count"`
	ChallengeCount  int    `json:"challenge_count"`
	LastUpdatedAt   string `json:"last_updated_at"`
	CacheUpdatedAt  string `json:"cache_updated_at"`
	CacheStatus     string `json:"cache_status"`
	UsingCache      bool   `json:"using_cache"`
}

type Resource struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type Stage struct {
	Title       string   `json:"title"`
	Description []string `json:"description"`
}

type ScoreEntry struct {
	Team     string `json:"team"`
	PassedAt string `json:"passed_at"`
	Score    string `json:"score"`
}

type Challenge struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	Description   string   `json:"description"`
	Files         []string `json:"files"`
	FlagSourceURL string   `json:"flag_source_url"`
	Value         int      `json:"value"`
	Solves        int      `json:"solves"`
	VerifyMode    string   `json:"verify_mode"`
}

type SubmissionForm struct {
	Enabled        bool   `json:"enabled"`
	Action         string `json:"action"`
	FlagField      string `json:"flag_field"`
	NonceField     string `json:"nonce_field"`
	ChallengeField string `json:"challenge_field"`
	ChallengeID    string `json:"challenge_id"`
}

type SubmitRequest struct {
	AccountName  string   `json:"account_name"`
	ChallengeIDs []string `json:"challenge_ids"`
	Flag         string   `json:"flag"`
}

type SubmitResponse struct {
	AccountName  string         `json:"account_name"`
	Username     string         `json:"username"`
	Action       string         `json:"action"`
	Nonce        string         `json:"nonce"`
	SubmittedAt  string         `json:"submitted_at"`
	Total        int            `json:"total"`
	SuccessCount int            `json:"success_count"`
	FailureCount int            `json:"failure_count"`
	Results      []SubmitResult `json:"results"`
}

type SubmitResult struct {
	AccountName   string `json:"account_name"`
	Username      string `json:"username"`
	ChallengeID   string `json:"challenge_id"`
	ChallengeName string `json:"challenge_name"`
	VerifyMode    string `json:"verify_mode"`
	StatusCode    int    `json:"status_code"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	Raw           string `json:"raw"`
	SubmittedAt   string `json:"submitted_at"`
}
