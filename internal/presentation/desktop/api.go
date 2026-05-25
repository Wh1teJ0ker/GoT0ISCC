package desktop

import (
	"context"

	logapp "got0iscc/desktop/internal/application/logs"
	pythonenvapp "got0iscc/desktop/internal/application/pythonenv"
	"got0iscc/desktop/internal/application/sandbox"
	taskapp "got0iscc/desktop/internal/application/tasks"
	"got0iscc/desktop/internal/bootstrap"
	accountdomain "got0iscc/desktop/internal/domain/accounts"
	combat "got0iscc/desktop/internal/domain/combat"
	theory "got0iscc/desktop/internal/domain/theory"
)

type API struct {
	ctx context.Context
	app *bootstrap.Application
}

func NewAPI(app *bootstrap.Application) *API {
	return &API{app: app}
}

func (a *API) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *API) Shutdown(_ context.Context) {
	if a.app != nil {
		_ = a.app.Close()
	}
}

func (a *API) Health() any {
	return a.app.SystemService.Health()
}

func (a *API) Overview() any {
	return a.app.SystemService.Overview()
}

func (a *API) DashboardSummary() (any, error) {
	return a.app.DashboardService.Summary(a.context())
}

func (a *API) SandboxProfiles() any {
	return a.app.SandboxService.Profiles()
}

func (a *API) RunPythonSandbox(req sandbox.RunRequest) (any, error) {
	return a.app.SandboxService.Run(a.context(), req)
}

func (a *API) PythonEnvStatus() (any, error) {
	return a.app.PythonEnvService.Status(a.context())
}

func (a *API) InitializePythonEnv(req pythonenvapp.InitRequest) (any, error) {
	return a.app.PythonEnvService.Initialize(a.context(), req)
}

func (a *API) InstallPythonPackages(req pythonenvapp.InstallPackagesRequest) (any, error) {
	return a.app.PythonEnvService.InstallPackages(a.context(), req)
}

func (a *API) ExportMigrationBundle() (any, error) {
	return a.app.ExportService.CreateMigrationBundle(a.context())
}

func (a *API) Accounts() (any, error) {
	return a.app.AccountService.List(a.context())
}

func (a *API) SaveAccount(account accountdomain.Account) (any, error) {
	return a.app.AccountService.Save(a.context(), account)
}

func (a *API) DeleteAccount(id int64) error {
	return a.app.AccountService.Delete(a.context(), id)
}

func (a *API) Logs() (any, error) {
	return a.app.LogService.List(a.context())
}

func (a *API) LogContent(req logapp.ContentRequest) (any, error) {
	return a.app.LogService.Content(a.context(), req)
}

func (a *API) Tasks() (any, error) {
	return a.app.TaskService.List(a.context())
}

func (a *API) NetworkProxy() (any, error) {
	return a.app.TaskService.NetworkProxy(a.context())
}

func (a *API) SaveNetworkProxy(settings taskapp.NetworkProxySettings) (any, error) {
	return a.app.TaskService.SaveNetworkProxy(a.context(), settings)
}

func (a *API) StartTask(req taskapp.StartRequest) (any, error) {
	return a.app.TaskService.Start(a.context(), req)
}

func (a *API) StopTask(id string) (any, error) {
	return a.app.TaskService.Stop(a.context(), id)
}

func (a *API) StopAllTasks() (any, error) {
	return a.app.TaskService.StopAll(a.context())
}

func (a *API) PracticeTrack() (any, error) {
	return a.app.TrackService.Practice(a.context())
}

func (a *API) ArenaTrack() (any, error) {
	return a.app.TrackService.Arena(a.context())
}

func (a *API) TheoryTrack() (any, error) {
	return a.app.TheoryService.Snapshot(a.context())
}

func (a *API) TheoryTrackWithRequest(req theory.SnapshotRequest) (any, error) {
	return a.app.TheoryService.SnapshotByRequest(a.context(), req)
}

func (a *API) SearchTheoryBank(query string) (any, error) {
	return a.app.TheoryService.SearchBank(a.context(), query)
}

func (a *API) TheoryAISettings() (any, error) {
	return a.app.TheoryService.AISettings(a.context())
}

func (a *API) SaveTheoryAISettings(settings theory.AISettings) (any, error) {
	return a.app.TheoryService.SaveAISettings(a.context(), settings)
}

func (a *API) TestTheoryAISettings(settings theory.AISettings) (any, error) {
	return a.app.TheoryService.TestAISettings(a.context(), settings)
}

func (a *API) TheoryReviewItems() (any, error) {
	return a.app.TheoryService.ReviewItems(a.context())
}

func (a *API) SaveTheoryReview(decision theory.ReviewDecision) (any, error) {
	return a.app.TheoryService.SaveReview(a.context(), decision)
}

func (a *API) SubmitTheoryManual(req theory.ManualSubmitRequest) (any, error) {
	return a.app.TheoryService.ManualSubmit(a.context(), req)
}

func (a *API) TheoryAIReviewStatus() (any, error) {
	return a.app.TheoryService.AIReviewStatus(a.context())
}

func (a *API) StartTheoryAIReview(req theory.AIReviewRequest) (any, error) {
	return a.app.TheoryService.StartAIReview(a.context(), req)
}

func (a *API) StopTheoryAIReview() (any, error) {
	return a.app.TheoryService.StopAIReview(a.context())
}

func (a *API) RunTheoryAutomation(req theory.AutomationRequest) (any, error) {
	return a.app.TheoryService.RunAutomation(a.context(), req)
}

func (a *API) TheoryAutomationStatus() (any, error) {
	return a.app.TheoryService.AutomationStatus(a.context())
}

func (a *API) StartTheoryAutomation(req theory.AutomationRequest) (any, error) {
	return a.app.TheoryService.StartAutomation(a.context(), req)
}

func (a *API) StopTheoryAutomation() (any, error) {
	return a.app.TheoryService.StopAutomation(a.context())
}

func (a *API) CombatTrack() (any, error) {
	return a.app.CombatService.Snapshot(a.context())
}

func (a *API) RefreshCombatTrack() (any, error) {
	return a.app.CombatService.Sync(a.context())
}

func (a *API) SubmitCombat(req combat.SubmitRequest) (any, error) {
	return a.app.CombatService.Submit(a.context(), req)
}

func (a *API) Writeups() (any, error) {
	return a.app.WPService.List(a.context())
}

func (a *API) SyncWriteups() (any, error) {
	return a.app.WPService.SyncRemote(a.context())
}

func (a *API) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}
