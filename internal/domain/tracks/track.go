package tracks

type Payload struct {
	Section     string          `json:"section"`
	DisplayName string          `json:"display_name"`
	SourceType  string          `json:"source_type"`
	SourcePath  string          `json:"source_path"`
	SnapshotAt  string          `json:"snapshot_at"`
	Summary     Summary         `json:"summary"`
	Accounts    []AccountSummary `json:"accounts"`
	Challenges  []Challenge     `json:"challenges"`
}

type Summary struct {
	TotalChallenges               int    `json:"total_challenges"`
	TotalAccounts                 int    `json:"total_accounts"`
	SolvedChallenges              int    `json:"solved_challenges"`
	PendingChallenges             int    `json:"pending_challenges"`
	ChangedChallenges             int    `json:"changed_challenges"`
	WarningChallenges             int    `json:"warning_challenges"`
	AttachmentMismatchChallenges  int    `json:"attachment_mismatch_challenges"`
	RemoteChallenges              int    `json:"remote_challenges"`
	AttachmentChallenges          int    `json:"attachment_challenges"`
	LastUpdatedAt                 string `json:"last_updated_at"`
}

type AccountSummary struct {
	Name           string `json:"name"`
	TotalChallenges int   `json:"total_challenges"`
	SolvedCount    int    `json:"solved_count"`
	PendingCount   int    `json:"pending_count"`
	ChangedCount   int    `json:"changed_count"`
	WarningCount   int    `json:"warning_count"`
	LastActiveAt   string `json:"last_active_at"`
}

type Challenge struct {
	Key                     string             `json:"key"`
	ChallengeID             string             `json:"challenge_id"`
	Section                 string             `json:"section"`
	Title                   string             `json:"title"`
	Category                string             `json:"category"`
	Kind                    string             `json:"kind"`
	DetailURL               string             `json:"detail_url"`
	DirPath                 string             `json:"dir_path"`
	DescriptionPath         string             `json:"description_path"`
	SolvePath               string             `json:"solve_path"`
	SolveScript             string             `json:"solve_script"`
	RemoteSummaryPath       string             `json:"remote_summary_path"`
	UpdatedAt               string             `json:"updated_at"`
	Changed                 bool               `json:"changed"`
	ExpectsAttachments      bool               `json:"expects_attachments"`
	ExpectsRemote           bool               `json:"expects_remote"`
	Submitted               bool               `json:"submitted"`
	SubmittedAccountCount   int                `json:"submitted_account_count"`
	AssetWarnings           []string           `json:"asset_warnings"`
	Attachments             []Attachment       `json:"attachments"`
	AttachmentVariants      []AttachmentVariant `json:"attachment_variants"`
	AttachmentMismatch      bool               `json:"attachment_mismatch"`
	RemoteTargets           []RemoteTarget     `json:"remote_targets"`
	Accounts                []ChallengeAccount `json:"accounts"`
}

type ChallengeAccount struct {
	Account           string   `json:"account"`
	Submitted         bool     `json:"submitted"`
	PlatformSolved    bool     `json:"platform_solved"`
	PlatformSolvedAt  string   `json:"platform_solved_at"`
	LastSubmitOK      bool     `json:"last_submit_ok"`
	LastSubmittedAt   string   `json:"last_submitted_at"`
	LastSeenAt        string   `json:"last_seen_at"`
	Changed           bool     `json:"changed"`
	SolverStatus      string   `json:"solver_status"`
	SubmissionMessage string   `json:"submission_message"`
	LastFlag          string   `json:"last_flag"`
	AttachmentCount   int      `json:"attachment_count"`
	RemoteTargetCount int      `json:"remote_target_count"`
	Warnings          []string `json:"warnings"`
}

type Attachment struct {
	Name        string `json:"name"`
	StoredName  string `json:"stored_name"`
	URL         string `json:"url"`
	LocalPath   string `json:"local_path"`
	SharedPath  string `json:"shared_path"`
	StorageMode string `json:"storage_mode"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Changed     bool   `json:"changed"`
}

type AttachmentVariant struct {
	Name            string   `json:"name"`
	StoredName      string   `json:"stored_name"`
	URL             string   `json:"url"`
	LocalPath       string   `json:"local_path"`
	SHA256          string   `json:"sha256"`
	Size            int64    `json:"size"`
	Changed         bool     `json:"changed"`
	Accounts        []string `json:"accounts"`
	HasHashMismatch bool     `json:"has_hash_mismatch"`
}

type RemoteTarget struct {
	Value  string `json:"value"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
}
