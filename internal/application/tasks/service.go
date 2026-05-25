package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	accountdomain "got0iscc/desktop/internal/domain/accounts"
	logdomain "got0iscc/desktop/internal/domain/logs"
	"got0iscc/desktop/internal/platform/runtime"
	pythonrunner "got0iscc/desktop/internal/platform/sandbox/python"
)

type JobRepository interface {
	UpsertJobs(ctx context.Context, jobs []logdomain.Job) error
	ListJobs(ctx context.Context, limit int) ([]logdomain.Job, error)
	JobsSummary(ctx context.Context) (logdomain.Summary, error)
	JobByID(ctx context.Context, id string) (logdomain.Job, error)
}

type AccountRepository interface {
	ListAccounts(ctx context.Context) ([]accountdomain.Account, error)
}

type SettingsRepository interface {
	MetaValue(ctx context.Context, key string) (string, error)
	SetMetaValue(ctx context.Context, key string, value string) error
}

type StartRequest struct {
	Command       string `json:"command"`
	Account       string `json:"account"`
	Section       string `json:"section"`
	IDs           string `json:"ids"`
	Flag          string `json:"flag"`
	Workers       int    `json:"workers"`
	Force         bool   `json:"force"`
	ForceDownload bool   `json:"force_download"`
	ForceSolve    bool   `json:"force_solve"`
	NoSubmit      bool   `json:"no_submit"`
}

type Payload struct {
	Summary          logdomain.Summary                `json:"summary"`
	Jobs             []logdomain.Job                  `json:"jobs"`
	AvailableTracks  []string                         `json:"available_tracks"`
	Commands         []CommandMeta                    `json:"commands"`
	Accounts         []TaskAccount                    `json:"accounts"`
	NetworkProxy     NetworkProxySettings             `json:"network_proxy"`
	ChallengeOptions map[string][]TaskChallengeOption `json:"challenge_options"`
	SandboxProfiles  []pythonrunner.Profile           `json:"sandbox_profiles"`
	ConfigPath       string                           `json:"config_path"`
	DatabasePath     string                           `json:"database_path"`
	LastUpdatedAt    string                           `json:"last_updated_at"`
}

type NetworkProxySettings struct {
	Enabled                bool   `json:"enabled"`
	Type                   string `json:"type"`
	Host                   string `json:"host"`
	Port                   int    `json:"port"`
	Username               string `json:"username"`
	Password               string `json:"password"`
	LoginAttempts          int    `json:"login_attempts"`
	LoginRetryDelaySeconds int    `json:"login_retry_delay_seconds"`
	UpdatedAt              string `json:"updated_at"`
}

type TaskAccount struct {
	Name      string `json:"name"`
	Username  string `json:"username"`
	Enabled   bool   `json:"enabled"`
	LoginOK   bool   `json:"login_ok"`
	LastLogin string `json:"last_login"`
	Priority  int    `json:"priority"`
}

type TaskChallengeOption struct {
	ID        string `json:"id"`
	Section   string `json:"section"`
	Title     string `json:"title"`
	Category  string `json:"category"`
	Kind      string `json:"kind"`
	UpdatedAt string `json:"updated_at"`
}

type CommandMeta struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Service struct {
	repo          JobRepository
	accountRepo   AccountRepository
	challengeRepo ChallengeRepository
	settingsRepo  SettingsRepository
	layout        runtime.Layout
	sandbox       []pythonrunner.Profile
	mu            sync.RWMutex
	running       map[string]*runningJob
	workspaceDir  string
	native        *nativeTaskService
}

type runningJob struct {
	id      string
	cancel  context.CancelFunc
	logPath string
	job     logdomain.Job
}

var supportedCommands = []CommandMeta{
	{ID: "sync", Label: "同步资产", Description: "Go 原生任务链同步挑战缓存与远端状态。"},
	{ID: "solve", Label: "解题流程", Description: "Go 调度 + Python 沙箱执行题目脚本。"},
	{ID: "status", Label: "状态扫描", Description: "仅检查本地缓存与待提交状态。"},
	{ID: "submit-flag", Label: "手动提交", Description: "直接向练武题或擂台题提交 flag。"},
}

var supportedTracks = []string{"challenges", "arena"}

const metaNetworkProxy = "network.proxy"

func NewService(repo JobRepository, accountRepo AccountRepository, layout runtime.Layout, runner pythonrunner.Runner, sandboxProfiles []pythonrunner.Profile) (*Service, error) {
	challengeRepo, ok := repo.(ChallengeRepository)
	if !ok {
		return nil, errors.New("任务服务缺少挑战仓储支持")
	}
	service := &Service{
		repo:          repo,
		accountRepo:   accountRepo,
		challengeRepo: challengeRepo,
		settingsRepo:  settingsRepository(repo),
		layout:        layout,
		sandbox:       sandboxProfiles,
		running:       map[string]*runningJob{},
		workspaceDir:  layout.WorkspaceRoot,
	}
	service.native = newNativeTaskService(layout, accountRepo, challengeRepo, runner, "")
	service.native.setNetworkProxy(service.loadNetworkProxy(context.Background()))
	service.reconcileStaleJobs(context.Background())
	return service, nil
}

func (s *Service) List(ctx context.Context) (Payload, error) {
	summary, err := s.repo.JobsSummary(ctx)
	if err != nil {
		return Payload{}, err
	}
	jobs, err := s.repo.ListJobs(ctx, 200)
	if err != nil {
		return Payload{}, err
	}
	accounts, err := s.accountRepo.ListAccounts(ctx)
	if err != nil {
		return Payload{}, err
	}
	taskAccounts := make([]TaskAccount, 0, len(accounts))
	for _, account := range accounts {
		taskAccounts = append(taskAccounts, TaskAccount{
			Name:      account.Name,
			Username:  account.Username,
			Enabled:   account.Enabled,
			LoginOK:   account.Runtime != nil && account.Runtime.LoginStatus == "ok",
			LastLogin: accountRuntimeField(account.Runtime, func(state *accountdomain.RuntimeState) string { return state.LastLoginAt }),
			Priority:  account.SubmitPriority,
		})
	}
	challengeOptions := s.listChallengeOptions(ctx)
	networkProxy := s.loadNetworkProxy(ctx)
	s.native.setNetworkProxy(networkProxy)

	return Payload{
		Summary:          summary,
		Jobs:             jobs,
		AvailableTracks:  append([]string(nil), supportedTracks...),
		Commands:         supportedCommands,
		Accounts:         taskAccounts,
		NetworkProxy:     networkProxy,
		ChallengeOptions: challengeOptions,
		SandboxProfiles:  s.sandbox,
		ConfigPath:       s.layout.InitConfigPath,
		DatabasePath:     s.layout.AppDatabasePath,
		LastUpdatedAt:    nowTS(),
	}, nil
}

func (s *Service) NetworkProxy(ctx context.Context) (NetworkProxySettings, error) {
	return s.loadNetworkProxy(ctx), nil
}

func (s *Service) SaveNetworkProxy(ctx context.Context, input NetworkProxySettings) (NetworkProxySettings, error) {
	settings, err := normalizeNetworkProxy(input)
	if err != nil {
		return NetworkProxySettings{}, err
	}
	if s.settingsRepo == nil {
		return NetworkProxySettings{}, errors.New("网络代理配置仓库不可用")
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return NetworkProxySettings{}, err
	}
	if err := s.settingsRepo.SetMetaValue(ctx, metaNetworkProxy, string(data)); err != nil {
		return NetworkProxySettings{}, err
	}
	s.native.setNetworkProxy(settings)
	return settings, nil
}

func (s *Service) loadNetworkProxy(ctx context.Context) NetworkProxySettings {
	if s.settingsRepo == nil {
		return NetworkProxySettings{}
	}
	raw, err := s.settingsRepo.MetaValue(ctx, metaNetworkProxy)
	if err != nil || strings.TrimSpace(raw) == "" {
		return NetworkProxySettings{}
	}
	var settings NetworkProxySettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return NetworkProxySettings{}
	}
	settings, err = normalizeNetworkProxy(settings)
	if err != nil {
		return NetworkProxySettings{}
	}
	return settings
}

func normalizeNetworkProxy(input NetworkProxySettings) (NetworkProxySettings, error) {
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	input.Host = strings.TrimSpace(input.Host)
	input.Username = strings.TrimSpace(input.Username)
	if input.LoginAttempts <= 0 {
		input.LoginAttempts = nativeDefaultLoginAttempts
	}
	if input.LoginRetryDelaySeconds <= 0 {
		input.LoginRetryDelaySeconds = int(nativeDefaultLoginRetryDelay / time.Second)
	}
	if input.LoginAttempts > 20 {
		return NetworkProxySettings{}, errors.New("登录重试次数必须在 1-20 之间")
	}
	if input.LoginRetryDelaySeconds > 60 {
		return NetworkProxySettings{}, errors.New("登录重试等待倍率必须在 1-60 秒之间")
	}
	if input.Type == "" {
		input.Enabled = false
	}
	if !input.Enabled {
		input.Type = ""
		input.Host = ""
		input.Port = 0
		input.Username = ""
		input.Password = ""
		input.UpdatedAt = nowTS()
		return input, nil
	}
	switch input.Type {
	case "http", "https", "socks4", "socks5":
	default:
		return NetworkProxySettings{}, fmt.Errorf("统一代理类型不支持: %s", input.Type)
	}
	if input.Host == "" {
		return NetworkProxySettings{}, errors.New("统一代理已启用时必须填写代理主机")
	}
	if input.Port <= 0 || input.Port > 65535 {
		return NetworkProxySettings{}, errors.New("统一代理端口必须在 1-65535 之间")
	}
	input.UpdatedAt = nowTS()
	return input, nil
}

func settingsRepository(value any) SettingsRepository {
	repo, _ := value.(SettingsRepository)
	return repo
}

func (s *Service) listChallengeOptions(ctx context.Context) map[string][]TaskChallengeOption {
	options := make(map[string][]TaskChallengeOption, len(supportedTracks))
	for _, track := range supportedTracks {
		options[track] = []TaskChallengeOption{}
	}
	if s.challengeRepo == nil || s.challengeRepo.DB() == nil {
		return options
	}
	rows, err := s.challengeRepo.DB().QueryContext(ctx, `
SELECT challenge_id, section_name, title, category, challenge_kind, updated_at
FROM challenges
WHERE section_name IN (?, ?)
ORDER BY section_name ASC, challenge_id + 0 ASC, challenge_key ASC
`, "challenges", "arena")
	if err != nil {
		return options
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var item TaskChallengeOption
		if err := rows.Scan(&item.ID, &item.Section, &item.Title, &item.Category, &item.Kind, &item.UpdatedAt); err != nil {
			continue
		}
		item.ID = strings.TrimSpace(item.ID)
		item.Section = strings.TrimSpace(item.Section)
		item.Title = strings.TrimSpace(item.Title)
		if item.ID == "" || item.Section == "" {
			continue
		}
		key := item.Section + "\x00" + item.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		if item.Title == "" {
			item.Title = "未命名题目"
		}
		options[item.Section] = append(options[item.Section], item)
	}
	return options
}

func (s *Service) Start(ctx context.Context, req StartRequest) (logdomain.Job, error) {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		command = "solve"
	}
	if !commandSupported(command) {
		return logdomain.Job{}, errors.New("不支持的任务命令")
	}
	if command == "submit-flag" {
		if strings.TrimSpace(req.Flag) == "" {
			return logdomain.Job{}, errors.New("submit-flag 需要填写 flag")
		}
		if strings.TrimSpace(req.IDs) == "" {
			return logdomain.Job{}, errors.New("submit-flag 需要填写题目 ID")
		}
	}

	id := fmt.Sprintf("desktop:%d", time.Now().UnixNano())
	logDir := filepath.Join(s.layout.AppRuntimeRoot, "tasks", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return logdomain.Job{}, err
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%s.log", sanitizeFileName(id)))

	runCtx, cancel := context.WithCancel(ctx)
	logFile, err := os.Create(logPath)
	if err != nil {
		cancel()
		return logdomain.Job{}, err
	}

	writer := &tailWriter{limit: 24}
	logger := &jobLogger{file: logFile, writer: writer}

	startedAt := nowTS()
	job := logdomain.Job{
		ID:         id,
		Title:      strings.TrimSpace(command + " " + req.Account),
		Source:     s.layout.AppRoot,
		SourceType: "desktop",
		Command:    s.buildCommandText(req),
		LogPath:    logPath,
		Status:     "starting",
		Account:    strings.TrimSpace(req.Account),
		StartedAt:  startedAt,
		UpdatedAt:  startedAt,
	}
	if strings.TrimSpace(job.Title) == "" {
		job.Title = command
	}
	if err := s.repo.UpsertJobs(ctx, []logdomain.Job{job}); err != nil {
		_ = logger.Close()
		cancel()
		return logdomain.Job{}, err
	}

	job.Status = "running"
	job.UpdatedAt = nowTS()
	if err := s.repo.UpsertJobs(ctx, []logdomain.Job{job}); err != nil {
		_ = logger.Close()
		cancel()
		return logdomain.Job{}, err
	}

	s.mu.Lock()
	s.running[job.ID] = &runningJob{
		id:      job.ID,
		cancel:  cancel,
		logPath: logPath,
		job:     job,
	}
	s.mu.Unlock()

	go s.runJob(runCtx, job, req, logger)
	return job, nil
}

func (s *Service) Stop(ctx context.Context, id string) (logdomain.Job, error) {
	s.mu.RLock()
	item := s.running[id]
	s.mu.RUnlock()
	if item == nil {
		job, err := s.repo.JobByID(ctx, id)
		if err != nil {
			return logdomain.Job{}, errors.New("任务不存在")
		}
		return job, nil
	}

	item.cancel()
	job := item.job
	job.Status = "stopping"
	job.UpdatedAt = nowTS()
	job.Tail = strings.TrimSpace(readTail(item.logPath))
	_ = s.repo.UpsertJobs(ctx, []logdomain.Job{job})
	s.mu.Lock()
	current := s.running[id]
	if current != nil {
		current.job = job
	}
	s.mu.Unlock()
	return job, nil
}

func (s *Service) StopAll(ctx context.Context) ([]logdomain.Job, error) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.running))
	for id := range s.running {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	results := make([]logdomain.Job, 0, len(ids))
	for _, id := range ids {
		job, err := s.Stop(ctx, id)
		if err == nil {
			results = append(results, job)
		}
	}
	return results, nil
}

func (s *Service) runJob(ctx context.Context, job logdomain.Job, req StartRequest, logger *jobLogger) {
	defer logger.Close()
	result := nativeTaskResult{Tail: "任务未执行", ReturnCode: 1}
	if s.native == nil {
		logger.Printf("native task service unavailable")
		result = nativeTaskResult{Tail: "native task service unavailable", ReturnCode: 1}
	} else {
		result = s.native.Execute(ctx, req, logger)
	}

	finishedAt := nowTS()
	job.FinishedAt = finishedAt
	job.UpdatedAt = finishedAt
	job.Tail = firstNonEmpty(result.Tail, logger.Last())
	code := result.ReturnCode
	job.ReturnCode = &code

	if ctx.Err() == context.Canceled {
		job.Status = "stopped"
	} else if result.ReturnCode == 0 {
		job.Status = "finished"
	} else {
		job.Status = "failed"
	}

	_ = s.repo.UpsertJobs(context.Background(), []logdomain.Job{job})
	s.mu.Lock()
	delete(s.running, job.ID)
	s.mu.Unlock()
}

func (s *Service) buildCommandText(req StartRequest) string {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		command = "solve"
	}
	argv := []string{command, "--db", s.layout.AppDatabasePath}
	if value := strings.TrimSpace(req.Account); value != "" {
		argv = append(argv, "--account", value)
	}
	if value := strings.TrimSpace(req.Section); value != "" {
		argv = append(argv, "--section", value)
	}
	if value := strings.TrimSpace(req.IDs); value != "" {
		argv = append(argv, "--ids", value)
	}
	argv = append(argv, "--workers", fmt.Sprint(normalizedWorkers(req.Workers)))
	if req.Force {
		argv = append(argv, "--force")
	}
	if req.ForceDownload {
		argv = append(argv, "--force-download")
	}
	if req.ForceSolve {
		argv = append(argv, "--force-solve")
	}
	if req.NoSubmit {
		argv = append(argv, "--no-submit")
	}
	if command == "submit-flag" && strings.TrimSpace(req.Flag) != "" {
		argv = append(argv, "--flag", strings.TrimSpace(req.Flag))
	}
	return strings.Join(argv, " ")
}

func accountRuntimeField(state *accountdomain.RuntimeState, getter func(*accountdomain.RuntimeState) string) string {
	if state == nil {
		return ""
	}
	return getter(state)
}

func commandSupported(value string) bool {
	for _, item := range supportedCommands {
		if item.ID == value {
			return true
		}
	}
	return false
}

func nowTS() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func sanitizeFileName(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return replacer.Replace(value)
}

func readTail(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

type tailWriter struct {
	limit int
	lines []string
	mu    sync.Mutex
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	text := strings.ReplaceAll(string(p), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		w.lines = append(w.lines, line)
		if len(w.lines) > w.limit {
			w.lines = w.lines[len(w.lines)-w.limit:]
		}
	}
	return len(p), nil
}

func (w *tailWriter) Last() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.lines) == 0 {
		return ""
	}
	return w.lines[len(w.lines)-1]
}

type jobLogger struct {
	file   *os.File
	writer *tailWriter
	mu     sync.Mutex
}

func (l *jobLogger) Printf(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.file.WriteString(text)
	_, _ = l.writer.Write([]byte(text))
}

func (l *jobLogger) Last() string {
	return l.writer.Last()
}

func (l *jobLogger) Close() error {
	if l.file == nil {
		return nil
	}
	return l.file.Close()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Service) reconcileStaleJobs(ctx context.Context) {
	jobs, err := s.repo.ListJobs(ctx, 200)
	if err != nil {
		return
	}
	stale := make([]logdomain.Job, 0, 8)
	for _, job := range jobs {
		switch strings.ToLower(strings.TrimSpace(job.Status)) {
		case "running", "starting", "stopping":
		default:
			continue
		}
		if _, ok := s.running[job.ID]; ok {
			continue
		}
		job.Status = "stopped"
		job.FinishedAt = firstNonEmpty(job.FinishedAt, job.UpdatedAt, nowTS())
		job.UpdatedAt = nowTS()
		job.Tail = firstNonEmpty(job.Tail, "任务记录已自动清理：应用重启后未发现对应运行实例，已标记为 stopped")
		job.PID = nil
		stale = append(stale, job)
	}
	if len(stale) == 0 {
		return
	}
	_ = s.repo.UpsertJobs(ctx, stale)
}
