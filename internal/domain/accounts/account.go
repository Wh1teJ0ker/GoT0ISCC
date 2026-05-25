package accounts

type Account struct {
	ID             int64         `json:"id"`
	Name           string        `json:"name"`
	Username       string        `json:"username"`
	Password       string        `json:"password"`
	Enabled        bool          `json:"enabled"`
	SubmitPriority int           `json:"submit_priority"`
	CreatedAt      string        `json:"created_at"`
	UpdatedAt      string        `json:"updated_at"`
	Runtime        *RuntimeState `json:"runtime,omitempty"`
}

type RuntimeState struct {
	AccountName               string `json:"account_name"`
	CycleStatus               string `json:"cycle_status"`
	LoginStatus               string `json:"login_status"`
	LastError                 string `json:"last_error"`
	LastLoginAt               string `json:"last_login_at"`
	LastCycleStartedAt        string `json:"last_cycle_started_at"`
	LastCycleFinishedAt       string `json:"last_cycle_finished_at"`
	ProcessedChallenges       int    `json:"processed_challenges"`
	ProcessedSections         int    `json:"processed_sections"`
	RemoteSubmissionCount     int    `json:"remote_submission_count"`
	LastRemoteSubmissionsSync string `json:"last_remote_submissions_sync_at"`
	SessionTokenFile          string `json:"session_token_file"`
	SessionTokenExists        bool   `json:"session_token_exists"`
	Source                    string `json:"source"`
	UpdatedAt                 string `json:"updated_at"`
	RawJSON                   string `json:"raw_json,omitempty"`
}

type Summary struct {
	Total            int    `json:"total"`
	Enabled          int    `json:"enabled"`
	Disabled         int    `json:"disabled"`
	MissingPassword  int    `json:"missing_password"`
	RuntimeAvailable int    `json:"runtime_available"`
	LoginOK          int    `json:"login_ok"`
	SessionReady     int    `json:"session_ready"`
	RunningCycles    int    `json:"running_cycles"`
	DatabasePath     string `json:"database_path"`
}
