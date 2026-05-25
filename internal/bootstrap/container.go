package bootstrap

import (
	"context"

	accountservice "got0iscc/desktop/internal/application/accounts"
	combatservice "got0iscc/desktop/internal/application/combat"
	dashboardservice "got0iscc/desktop/internal/application/dashboard"
	exportservice "got0iscc/desktop/internal/application/export"
	initconfigservice "got0iscc/desktop/internal/application/initconfig"
	logservice "got0iscc/desktop/internal/application/logs"
	pythonenvservice "got0iscc/desktop/internal/application/pythonenv"
	"got0iscc/desktop/internal/application/sandbox"
	"got0iscc/desktop/internal/application/system"
	taskservice "got0iscc/desktop/internal/application/tasks"
	theoryservice "got0iscc/desktop/internal/application/theory"
	trackservice "got0iscc/desktop/internal/application/tracks"
	wpservice "got0iscc/desktop/internal/application/wp"
	"got0iscc/desktop/internal/platform/runtime"
	pythonrunner "got0iscc/desktop/internal/platform/sandbox/python"
	sqlitestore "got0iscc/desktop/internal/platform/storage/sqlite"
)

type Application struct {
	Layout           runtime.Layout
	SystemService    *system.Service
	SandboxService   *sandbox.Service
	PythonEnvService *pythonenvservice.Service
	ExportService    *exportservice.Service
	DashboardService *dashboardservice.Service
	AccountService   *accountservice.Service
	LogService       *logservice.Service
	TaskService      *taskservice.Service
	TrackService     *trackservice.Service
	CombatService    *combatservice.Service
	TheoryService    *theoryservice.Service
	WPService        *wpservice.Service
	Store            *sqlitestore.Store
}

func NewApplication() (*Application, error) {
	layout, err := runtime.DetectLayout()
	if err != nil {
		return nil, err
	}

	runner := pythonrunner.NewLocalRunner(layout.AppRuntimeRoot)
	store, err := sqlitestore.Open(layout.AppDatabasePath)
	if err != nil {
		return nil, err
	}
	accountSvc := accountservice.NewService(store, layout)
	initConfigSvc := initconfigservice.NewService(layout, store)
	if err := initConfigSvc.EnsureInitialized(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	logSvc := logservice.NewService(store, layout)
	taskSvc, err := taskservice.NewService(store, store, layout, runner, runner.Profiles())
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	theorySvc := theoryservice.NewService(layout, store)
	if err := theorySvc.EnsureLegacyMerged(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}

	pythonEnvSvc := pythonenvservice.NewService(layout, store)
	trackSvc := trackservice.NewService(layout)
	combatSvc := combatservice.NewService(layout, store)
	wpSvc := wpservice.NewService(layout, store)

	return &Application{
		Layout:           layout,
		SystemService:    system.NewService(layout),
		SandboxService:   sandbox.NewService(runner, pythonEnvSvc.ActivePythonBinary),
		PythonEnvService: pythonEnvSvc,
		ExportService:    exportservice.NewService(layout),
		DashboardService: dashboardservice.NewService(trackSvc, theorySvc, combatSvc, wpSvc),
		AccountService:   accountSvc,
		LogService:       logSvc,
		TaskService:      taskSvc,
		TrackService:     trackSvc,
		CombatService:    combatSvc,
		TheoryService:    theorySvc,
		WPService:        wpSvc,
		Store:            store,
	}, nil
}

func (a *Application) Close() error {
	var firstErr error
	if a.Store != nil {
		if err := a.Store.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
