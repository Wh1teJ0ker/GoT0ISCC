package logs

type Job struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Source     string `json:"source"`
	SourceType string `json:"source_type"`
	Command    string `json:"command"`
	LogPath    string `json:"log_path"`
	Status     string `json:"status"`
	Account    string `json:"account"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	ReturnCode *int   `json:"return_code,omitempty"`
	PID        *int   `json:"pid,omitempty"`
	Tail       string `json:"tail"`
	UpdatedAt  string `json:"updated_at"`
}

type ServiceLog struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	SourceType  string `json:"source_type"`
	FilePath    string `json:"file_path"`
	SizeBytes   int64  `json:"size_bytes"`
	ModifiedAt  string `json:"modified_at"`
	Description string `json:"description"`
}

type Summary struct {
	TotalJobs       int `json:"total_jobs"`
	RunningJobs     int `json:"running_jobs"`
	FailedJobs      int `json:"failed_jobs"`
	FinishedJobs    int `json:"finished_jobs"`
	StoppedJobs     int `json:"stopped_jobs"`
	ServiceLogCount int `json:"service_log_count"`
}

type Content struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	Content     string `json:"content"`
	Truncated   bool   `json:"truncated"`
	SizeBytes   int64  `json:"size_bytes"`
	ModifiedAt  string `json:"modified_at"`
	Description string `json:"description"`
}

type Payload struct {
	Summary     Summary      `json:"summary"`
	Jobs        []Job        `json:"jobs"`
	ServiceLogs []ServiceLog `json:"service_logs"`
}
