package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"api-test-kits/internal/collectionio"
	"api-test-kits/internal/compare"
	"api-test-kits/internal/contract"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/fsutil"
	"api-test-kits/internal/security"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

// App exposes the service to the frontend through Wails bindings. It only
// adds desktop concerns: native file dialogs and progress events.
type App struct {
	ctx context.Context
	svc *service.Service
	upd *updates
}

func NewApp(svc *service.Service) *App { return &App{svc: svc, upd: newUpdates()} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// shutdown closes open socket connections so servers see a clean disconnect.
func (a *App) shutdown(context.Context) { a.svc.CloseSockets() }

// RunProgress is emitted as "run:progress" while a collection runs.
type RunProgress struct {
	OpID   string           `json:"opId"`
	Done   int              `json:"done"`
	Total  int              `json:"total"`
	Result domain.RunResult `json:"result"`
}

func (a *App) AppInfo() service.AppInfo { return a.svc.AppInfo() }

// ---- collections ----

func (a *App) ListCollections() ([]domain.Collection, error) { return a.svc.ListCollections(a.ctx) }

func (a *App) GetCollectionTree(id string) (domain.CollectionTree, error) {
	return a.svc.GetCollectionTree(a.ctx, id)
}

func (a *App) SaveCollection(c domain.Collection) (domain.Collection, error) {
	return a.svc.SaveCollection(a.ctx, c)
}

func (a *App) RenameCollection(id, name string) (domain.Collection, error) {
	return a.svc.RenameCollection(a.ctx, id, name)
}

func (a *App) InheritCollectionAuth(collectionID string) (int, error) {
	return a.svc.InheritCollectionAuth(a.ctx, collectionID)
}

func (a *App) DuplicateCollection(id string) (domain.Collection, error) {
	return a.svc.DuplicateCollection(a.ctx, id)
}

func (a *App) DeleteCollection(id string) error { return a.svc.DeleteCollection(a.ctx, id) }

func (a *App) SaveFolder(f domain.Folder) (domain.Folder, error) { return a.svc.SaveFolder(a.ctx, f) }

func (a *App) DeleteFolder(id string) error { return a.svc.DeleteFolder(a.ctx, id) }

func (a *App) SaveRequest(r domain.Request) (domain.Request, error) {
	return a.svc.SaveRequest(a.ctx, r)
}

func (a *App) DuplicateRequest(id string) (domain.Request, error) {
	return a.svc.DuplicateRequest(a.ctx, id)
}

func (a *App) DeleteRequest(id string) error { return a.svc.DeleteRequest(a.ctx, id) }

// ---- environments ----

func (a *App) ListEnvironments() ([]domain.Environment, error) { return a.svc.ListEnvironments(a.ctx) }

func (a *App) SaveEnvironment(e domain.Environment) (domain.Environment, error) {
	return a.svc.SaveEnvironment(a.ctx, e)
}

func (a *App) DuplicateEnvironment(id string) (domain.Environment, error) {
	return a.svc.DuplicateEnvironment(a.ctx, id)
}

func (a *App) DeleteEnvironment(id string) error { return a.svc.DeleteEnvironment(a.ctx, id) }

// ---- running ----

func (a *App) PreflightRequest(r domain.Request, envID string) ([]service.Warning, error) {
	return a.svc.Preflight(a.ctx, []domain.Request{r}, nil, envID)
}

// ---- response times ----

func (a *App) GetPerformanceCheck(requestID string) (domain.PerformanceCheck, error) {
	return a.svc.GetPerformanceCheck(a.ctx, requestID)
}

func (a *App) SavePerformanceCheck(c domain.PerformanceCheck) (domain.PerformanceCheck, error) {
	return a.svc.SavePerformanceCheck(a.ctx, c)
}

func (a *App) GetResponseTimeTrend(requestID, envID string) (domain.ResponseTimeTrend, error) {
	return a.svc.GetResponseTimeTrend(a.ctx, requestID, envID)
}

func (a *App) GetCollectionResponseTimes(collectionID, envID string) ([]domain.ResponseTimeRow, error) {
	return a.svc.GetCollectionResponseTimes(a.ctx, collectionID, envID)
}

func (a *App) PreflightCollection(collectionID, folderID, envID string) ([]service.Warning, error) {
	return a.svc.PreflightCollection(a.ctx, collectionID, folderID, envID)
}

func (a *App) SendRequest(opID string, r domain.Request, envID string, confirmed bool) (domain.RunResult, error) {
	return a.svc.SendRequest(a.ctx, opID, r, envID, confirmed)
}

func (a *App) RunCollection(opID, collectionID, folderID, envID string, confirmed bool) (domain.Run, error) {
	return a.svc.RunCollection(a.ctx, opID, collectionID, folderID, envID, confirmed, func(done, total int, res domain.RunResult) {
		runtime.EventsEmit(a.ctx, "run:progress", RunProgress{OpID: opID, Done: done, Total: total, Result: res})
	})
}

func (a *App) Cancel(opID string) bool { return a.svc.Cancel(opID) }

// ---- WebSocket and Socket.IO console ----

// ConnectSocket opens a console connection; received messages and the end of
// the connection arrive as "socket:event" events (service.SocketEvent).
func (a *App) ConnectSocket(sessionID string, r domain.Request, envID string, confirmed bool) (service.SocketSession, error) {
	return a.svc.ConnectSocket(a.ctx, sessionID, r, envID, confirmed, func(ev service.SocketEvent) {
		runtime.EventsEmit(a.ctx, "socket:event", ev)
	})
}

func (a *App) SendSocketMessage(sessionID string, m domain.SocketMessage) (domain.SocketFrame, error) {
	return a.svc.SendSocketMessage(sessionID, m)
}

func (a *App) DisconnectSocket(sessionID string) bool { return a.svc.DisconnectSocket(sessionID) }

// StartSocketHeartbeat sends m now and then every intervalMs on an open
// console session; heartbeats arrive as "socket:event" events.
func (a *App) StartSocketHeartbeat(sessionID string, m domain.SocketMessage, intervalMs int) (service.HeartbeatStatus, error) {
	return a.svc.StartSocketHeartbeat(sessionID, m, intervalMs)
}

func (a *App) StopSocketHeartbeat(sessionID string) service.HeartbeatStatus {
	return a.svc.StopSocketHeartbeat(sessionID)
}

// ---- security tests ----

func (a *App) GetRaceTest(requestID string) (*domain.RaceTest, error) {
	return a.svc.GetRaceTest(a.ctx, requestID)
}

func (a *App) RunRaceTest(opID string, t domain.RaceTest, envID string, confirmed bool) (security.RaceReport, error) {
	return a.svc.RunRaceTest(a.ctx, opID, t, envID, confirmed)
}

func (a *App) GetPermissionMatrix(collectionID string) (domain.PermissionMatrix, error) {
	return a.svc.GetPermissionMatrix(a.ctx, collectionID)
}

func (a *App) SavePermissionMatrix(m domain.PermissionMatrix) (domain.PermissionMatrix, error) {
	return a.svc.SavePermissionMatrix(a.ctx, m)
}

// RunPermissionMatrix emits "matrix:progress" events while it runs.
func (a *App) RunPermissionMatrix(opID, collectionID, envID string, confirmed bool) (security.MatrixReport, error) {
	return a.svc.RunPermissionMatrix(a.ctx, opID, collectionID, envID, confirmed, func(done, total int, res domain.RunResult) {
		runtime.EventsEmit(a.ctx, "matrix:progress", RunProgress{OpID: opID, Done: done, Total: total, Result: res})
	})
}

// ---- release sign-off report ----

func (a *App) BuildSignoffReport(in service.SignoffInput) (service.SignoffResult, error) {
	return a.svc.BuildSignoffReport(a.ctx, in)
}

// SaveSignoffReport writes the previewed report HTML to a user-chosen file.
func (a *App) SaveSignoffReport(fileName, html string) (string, error) {
	if html == "" || len(html) > 20<<20 {
		return "", fmt.Errorf("nothing to save; preview the report first")
	}
	filter := runtime.FileFilter{DisplayName: "HTML report (*.html)", Pattern: "*.html"}
	return a.saveWithDialog(fileName, filter, []byte(html))
}

// ---- collection rules ----

func (a *App) SaveCollectionRules(collectionID string, list []domain.Rule) ([]domain.Rule, error) {
	return a.svc.SaveCollectionRules(a.ctx, collectionID, list)
}

func (a *App) CheckCollectionRules(collectionID string, list []domain.Rule) (service.RulesCheck, error) {
	return a.svc.CheckCollectionRules(a.ctx, collectionID, list)
}

// ---- environment comparison and changing fields ----

func (a *App) GetEnvComparison(collectionID string) (domain.EnvComparison, error) {
	return a.svc.GetEnvComparison(a.ctx, collectionID)
}

func (a *App) SaveEnvComparison(c domain.EnvComparison) (domain.EnvComparison, error) {
	return a.svc.SaveEnvComparison(a.ctx, c)
}

// CompareEnvironments emits "compare:progress" events while it runs.
func (a *App) CompareEnvironments(opID string, c domain.EnvComparison, confirmed bool) (compare.Report, error) {
	return a.svc.CompareEnvironments(a.ctx, opID, c, confirmed, func(done, total int, res domain.RunResult) {
		runtime.EventsEmit(a.ctx, "compare:progress", RunProgress{OpID: opID, Done: done, Total: total, Result: res})
	})
}

func (a *App) DetectChangingFields(opID string, in service.DetectChangingInput) (service.ChangingReport, error) {
	return a.svc.DetectChangingFields(a.ctx, opID, in)
}

// ---- baselines ----

func (a *App) GetBaseline(requestID, envID string) (*domain.Baseline, error) {
	return a.svc.GetBaseline(a.ctx, requestID, envID)
}

func (a *App) ApproveBaseline(in service.ApproveBaselineInput) (domain.Baseline, error) {
	return a.svc.ApproveBaseline(a.ctx, in)
}

func (a *App) UpdateBaselineRules(id string, ignored []string, tolerance float64) error {
	return a.svc.UpdateBaselineRules(a.ctx, id, ignored, tolerance)
}

func (a *App) DeleteBaseline(id string) error { return a.svc.DeleteBaseline(a.ctx, id) }

// ---- history ----

func (a *App) ListRuns(f domain.RunFilter) ([]domain.Run, error) { return a.svc.ListRuns(a.ctx, f) }

func (a *App) GetRun(id string) (domain.Run, error) { return a.svc.GetRun(a.ctx, id) }

func (a *App) DeleteRun(id string) error { return a.svc.DeleteRun(a.ctx, id) }

func (a *App) ClearHistory() (int64, error) { return a.svc.ClearHistory(a.ctx) }

func (a *App) CompareWithPrevious(runID, resultID string) (service.PreviousComparison, error) {
	return a.svc.CompareWithPrevious(a.ctx, runID, resultID)
}

// ExportReport asks for a destination and writes the report. It returns the
// saved path, or "" if the user cancelled.
func (a *App) ExportReport(runID, format string) (string, error) {
	data, err := a.svc.ExportReport(a.ctx, runID, format)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("api-report-%s.%s", time.Now().Format("20060102-150405"), format)
	filter := runtime.FileFilter{DisplayName: "HTML report (*.html)", Pattern: "*.html"}
	if format == "json" {
		filter = runtime.FileFilter{DisplayName: "JSON report (*.json)", Pattern: "*.json"}
	}
	return a.saveWithDialog(name, filter, data)
}

// ---- import / export ----

func (a *App) ExportPreview(collectionID string, envIDs []string) ([]collectionio.SecretFinding, error) {
	return a.svc.ExportPreview(a.ctx, collectionID, envIDs)
}

// ExportCollection writes the collection to a user-chosen file. The frontend
// must show ExportPreview findings and get confirmation before passing
// includeSecrets=true.
func (a *App) ExportCollection(collectionID string, envIDs []string, includeSecrets, includeBaselines bool) (string, error) {
	data, err := a.svc.ExportCollection(a.ctx, collectionID, envIDs, includeSecrets, includeBaselines)
	if err != nil {
		return "", err
	}
	c, err := a.svc.GetCollectionTree(a.ctx, collectionID)
	if err != nil {
		return "", err
	}
	name := safeFileName(c.Collection.Name) + ".collection.json"
	return a.saveWithDialog(name, runtime.FileFilter{DisplayName: "Collection (*.json)", Pattern: "*.json"}, data)
}

// ImportCollection asks for a collection export or an OpenAPI 3 / Swagger 2
// document and imports it. Returns nil if cancelled.
func (a *App) ImportCollection() (*service.ImportResult, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import collection or OpenAPI / Swagger",
		Filters: []runtime.FileFilter{
			{DisplayName: "Collection or OpenAPI (*.json, *.yaml, *.yml)", Pattern: "*.json;*.yaml;*.yml"},
		},
	})
	if err != nil || path == "" {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > collectionio.MaxFileBytes {
		return nil, fmt.Errorf("file is larger than %d MB", collectionio.MaxFileBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	res, err := a.svc.ImportCollection(a.ctx, data, filepath.Base(path))
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// ---- OpenAPI contract ----

func (a *App) GetContract(collectionID string) (service.ContractInfo, error) {
	return a.svc.GetContract(a.ctx, collectionID)
}

// AttachContract asks for an OpenAPI 3 / Swagger 2 file and attaches it to
// the collection. Returns nil if cancelled.
func (a *App) AttachContract(collectionID string) (*service.ContractInfo, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Attach OpenAPI / Swagger spec",
		Filters: []runtime.FileFilter{{DisplayName: "OpenAPI or Swagger (*.json, *.yaml, *.yml)", Pattern: "*.json;*.yaml;*.yml"}},
	})
	if err != nil || path == "" {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > contract.MaxSpecBytes {
		return nil, fmt.Errorf("file is larger than %d MB", contract.MaxSpecBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, err := a.svc.AttachContract(a.ctx, collectionID, data, filepath.Base(path))
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (a *App) SetContractOptions(collectionID string, enabled, allowUndocumented bool) (service.ContractInfo, error) {
	return a.svc.SetContractOptions(a.ctx, collectionID, enabled, allowUndocumented)
}

func (a *App) RemoveContract(collectionID string) error {
	return a.svc.RemoveContract(a.ctx, collectionID)
}

func (a *App) GetContractCoverage(collectionID, envID string) (service.ContractCoverage, error) {
	return a.svc.GetContractCoverage(a.ctx, collectionID, envID)
}

// ---- settings and data ----

func (a *App) GetSettings() domain.Settings { return a.svc.GetSettings() }

func (a *App) SaveSettings(s domain.Settings) error { return a.svc.SaveSettings(a.ctx, s) }

func (a *App) DataInfo() (storage.DataInfo, error) { return a.svc.DataInfo(a.ctx) }

func (a *App) CompactDatabase() error { return a.svc.CompactDatabase(a.ctx) }

func (a *App) DeleteAllData() error { return a.svc.DeleteAllData(a.ctx) }

// ChooseCACertFile lets the user pick a PEM bundle; returns "" if cancelled.
func (a *App) ChooseCACertFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Choose CA certificate bundle",
		Filters: []runtime.FileFilter{{DisplayName: "PEM certificates (*.pem, *.crt)", Pattern: "*.pem;*.crt"}},
	})
}

// ---- helpers ----

func (a *App) saveWithDialog(defaultName string, filter runtime.FileFilter, data []byte) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters:         []runtime.FileFilter{filter},
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, fsutil.WriteFileAtomic(path, data)
}

func safeFileName(name string) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" {
		return "collection"
	}
	return name
}
