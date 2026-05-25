package wp

type Payload struct {
	Summary  Summary            `json:"summary"`
	Accounts []AccountSummary   `json:"accounts"`
	Items    []Item             `json:"items"`
	Records  []SubmissionRecord `json:"records"`
}

type Summary struct {
	Total               int      `json:"total"`
	TotalAccounts       int      `json:"total_accounts"`
	TotalChallenges     int      `json:"total_challenges"`
	Submitted           int      `json:"submitted"`
	Missing             int      `json:"missing"`
	NeedsFix            int      `json:"needs_fix"`
	Warnings            int      `json:"warnings"`
	SyncedAccounts      int      `json:"synced_accounts"`
	FailedAccounts      int      `json:"failed_accounts"`
	RemoteRecords       int      `json:"remote_records"`
	UnmatchedRecords    int      `json:"unmatched_records"`
	MonitorEndpoint     string   `json:"monitor_endpoint"`
	LastScannedAt       string   `json:"last_scanned_at"`
	MissingChallengeIDs []string `json:"missing_challenge_ids"`
}

type AccountSummary struct {
	Account        string `json:"account"`
	SubmitIdentity string `json:"submit_identity"`
	Total          int    `json:"total"`
	Submitted      int    `json:"submitted"`
	Missing        int    `json:"missing"`
	NeedsFix       int    `json:"needs_fix"`
	Warnings       int    `json:"warnings"`
	RemoteRecords  int    `json:"remote_records"`
	SyncStatus     string `json:"sync_status"`
	SyncMessage    string `json:"sync_message"`
	LastSyncedAt   string `json:"last_synced_at"`
	LastSolvedAt   string `json:"last_solved_at"`
}

type Challenge struct {
	Key             string   `json:"key"`
	ChallengeID     string   `json:"challenge_id"`
	Section         string   `json:"section"`
	SectionLabel    string   `json:"section_label"`
	Title           string   `json:"title"`
	DirPath         string   `json:"dir_path"`
	DescriptionPath string   `json:"description_path"`
	WriteupPath     string   `json:"writeup_path"`
	SolvePath       string   `json:"solve_path"`
	RemotePath      string   `json:"remote_path"`
	Status          string   `json:"status"`
	Issues          []Issue  `json:"issues"`
	Warnings        []Issue  `json:"warnings"`
	CandidateFiles  []string `json:"candidate_files"`
	UpdatedAt       string   `json:"updated_at"`
}

type Item struct {
	Key              string             `json:"key"`
	Account          string             `json:"account"`
	SubmitIdentity   string             `json:"submit_identity"`
	ExpectedFilename string             `json:"expected_filename"`
	Section          string             `json:"section"`
	SectionLabel     string             `json:"section_label"`
	Challenge        Challenge          `json:"challenge"`
	Status           string             `json:"status"`
	PlatformSolved   bool               `json:"platform_solved"`
	PlatformSolvedAt string             `json:"platform_solved_at"`
	LastSubmittedAt  string             `json:"last_submitted_at"`
	LastFlag         string             `json:"last_flag"`
	RemoteSubmitted  bool               `json:"remote_submitted"`
	RemoteAttempts   int                `json:"remote_attempts"`
	RemoteRecords    []SubmissionRecord `json:"remote_records"`
	SyncStatus       string             `json:"sync_status"`
	SyncMessage      string             `json:"sync_message"`
	LastSyncedAt     string             `json:"last_synced_at"`
	Issues           []Issue            `json:"issues"`
	Warnings         []Issue            `json:"warnings"`
}

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SubmissionRecord struct {
	Account          string  `json:"account"`
	SubmitIdentity   string  `json:"submit_identity"`
	Filename         string  `json:"filename"`
	ChallengeID      string  `json:"challenge_id"`
	ChallengeTitle   string  `json:"challenge_title"`
	Section          string  `json:"section"`
	SectionLabel     string  `json:"section_label"`
	ExpectedFilename string  `json:"expected_filename"`
	MatchStatus      string  `json:"match_status"`
	PageIndex        int     `json:"page_index"`
	Sequence         int     `json:"sequence"`
	Issues           []Issue `json:"issues"`
	Warnings         []Issue `json:"warnings"`
}
