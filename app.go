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
	"api-test-kits/internal/domain"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

// App exposes the service to the frontend through Wails bindings. It only
// adds desktop concerns: native file dialogs and progress events.
type App struct {
	ctx context.Context
	svc *service.Service
}

func NewApp(svc *service.Service) *App { return &App{svc: svc} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

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
func (a *App) ExportCollection(collectionID string, envIDs []string, includeSecrets bool) (string, error) {
	data, err := a.svc.ExportCollection(a.ctx, collectionID, envIDs, includeSecrets)
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
	res, err := a.svc.ImportCollection(a.ctx, data)
	if err != nil {
		return nil, err
	}
	return &res, nil
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
	return path, writeFileAtomic(path, data)
}

// writeFileAtomic writes to a temporary file and renames it into place so an
// interrupted write never leaves a half-written file behind.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
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
