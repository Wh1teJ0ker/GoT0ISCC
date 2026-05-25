package system

import "got0iscc/desktop/internal/platform/runtime"

type Service struct {
	layout runtime.Layout
}

type Health struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type Overview struct {
	AppName         string           `json:"app_name"`
	DesktopStack    string           `json:"desktop_stack"`
	Workspace       Workspace        `json:"workspace"`
	Modules         []Module         `json:"modules"`
	Sandbox         SandboxSummary   `json:"sandbox"`
	MigrationPhases []MigrationPhase `json:"migration_phases"`
}

type Workspace struct {
	AppRoot         string `json:"app_root"`
	WorkspaceRoot   string `json:"workspace_root"`
	ChallengesRoot  string `json:"challenges_root"`
	AppDataRoot     string `json:"app_data_root"`
	AppRuntimeRoot  string `json:"app_runtime_root"`
	AppDatabasePath string `json:"app_database_path"`
}

type Module struct {
	Name        string `json:"name"`
	Layer       string `json:"layer"`
	Purpose     string `json:"purpose"`
	Status      string `json:"status"`
	Replacement string `json:"replacement"`
}

type SandboxSummary struct {
	Name                string   `json:"name"`
	EntryCommand        string   `json:"entry_command"`
	Capabilities        []string `json:"capabilities"`
	SecurityBoundaries  []string `json:"security_boundaries"`
	UpgradeDirections   []string `json:"upgrade_directions"`
	DefaultTimeoutNotes string   `json:"default_timeout_notes"`
}

type MigrationPhase struct {
	Name   string   `json:"name"`
	Goal   string   `json:"goal"`
	Items  []string `json:"items"`
	Status string   `json:"status"`
}

func NewService(layout runtime.Layout) *Service {
	return &Service{layout: layout}
}

func (s *Service) Health() Health {
	return Health{
		Status:  "ok",
		Message: "desktop skeleton ready",
	}
}

func (s *Service) Overview() Overview {
	return Overview{
		AppName:      "GoT0ISCC Desktop",
		DesktopStack: "Go 1.24 + Wails + React + Embedded Python Sandbox",
		Workspace: Workspace{
			AppRoot:         s.layout.AppRoot,
			WorkspaceRoot:   s.layout.WorkspaceRoot,
			ChallengesRoot:  s.layout.ChallengesRoot,
			AppDataRoot:     s.layout.AppDataRoot,
			AppRuntimeRoot:  s.layout.AppRuntimeRoot,
			AppDatabasePath: s.layout.AppDatabasePath,
		},
		Modules: []Module{
			{
				Name:        "Desktop Shell",
				Layer:       "presentation",
				Purpose:     "Use Wails to host the desktop window and bind Go services into the frontend.",
				Status:      "ready",
				Replacement: "Replaces the old browser-only control panel entrance.",
			},
			{
				Name:        "Application Services",
				Layer:       "application",
				Purpose:     "Centralize orchestrated use-cases such as task running, config changes, and sandbox execution.",
				Status:      "ready",
				Replacement: "Owns task orchestration, persistence, and desktop bindings inside the current Go runtime.",
			},
			{
				Name:        "Python Sandbox",
				Layer:       "platform",
				Purpose:     "Run temporary code or future solver scripts inside isolated per-run workspaces.",
				Status:      "ready",
				Replacement: "Replaces ad-hoc direct Python execution with a managed runner.",
			},
		},
		Sandbox: SandboxSummary{
			Name:         "Embedded Python Sandbox",
			EntryCommand: "python3 -I main.py",
			Capabilities: []string{
				"Ephemeral workdir per run",
				"Optional injected files",
				"Timeout control",
				"Captured stdout and stderr",
				"Profile-based execution entry",
			},
			SecurityBoundaries: []string{
				"Process isolation",
				"Temporary working directory",
				"Minimal environment variables",
				"No mutation of main project files by default",
			},
			UpgradeDirections: []string{
				"Linux nsjail/firejail profile",
				"macOS helper-based stronger isolation",
				"Remote worker or microVM execution",
			},
			DefaultTimeoutNotes: "Current local profile defaults to 15 seconds and is intended for controlled local execution.",
		},
		MigrationPhases: []MigrationPhase{
			{
				Name:   "Phase 1",
				Goal:   "Stabilize the new desktop shell and runtime model.",
				Status: "active",
				Items: []string{
					"Create Wails project",
					"Define runtime layout",
					"Integrate Python sandbox",
				},
			},
			{
				Name:   "Phase 2",
				Goal:   "Stabilize local persistence, accounts, and runtime state inside the current application.",
				Status: "active",
				Items: []string{
					"Accounts CRUD",
					"SQLite account persistence",
					"Runtime state management",
				},
			},
			{
				Name:   "Phase 3",
				Goal:   "Consolidate task scheduling, logs, and challenge overview in the current runtime.",
				Status: "active",
				Items: []string{
					"Task lifecycle management",
					"Service log center",
					"Challenge aggregate model",
				},
			},
		},
	}
}
