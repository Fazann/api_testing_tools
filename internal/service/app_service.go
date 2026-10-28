// Package service is the application facade used by the desktop bindings.
// It has no Wails dependency so that a CLI can reuse it.
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/collectionio"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/environments"
	"api-test-kits/internal/httpclient"
	"api-test-kits/internal/redact"
	"api-test-kits/internal/regression"
	"api-test-kits/internal/reports"
	"api-test-kits/internal/runner"
	"api-test-kits/internal/storage"
)

const AppName = "API Regression Guard"

// Version is set at build time with -ldflags "-X api-test-kits/internal/service.Version=…".
var Version = "0.1.0-dev"

type Service struct {
	store *storage.Store

	mu       sync.Mutex
	client   *httpclient.Client
	settings domain.Settings
	ops      map[string]context.CancelFunc
}

// New creates a service backed by store.
func New(store *storage.Store) (*Service, error) {
	s := &Service{store: store, ops: map[string]context.CancelFunc{}}
	st, err := store.GetSettings(context.Background())
	if err != nil {
		return nil, err
	}
	if err := s.applySettings(st); err != nil {
		// A bad CA path or proxy must not prevent the app from starting.
		st.CACertFile, st.ProxyMode, st.ProxyURL = "", "system", ""
		if err := s.applySettings(st); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Service) applySettings(st domain.Settings) error {
	c, err := httpclient.New(httpclient.Options{
		MaxResponseBytes:           st.MaxResponseBytes,
		InsecureSkipVerify:         st.InsecureSkipVerify,
		CACertFile:                 st.CACertFile,
		ProxyMode:                  httpclient.ProxyMode(st.ProxyMode),
		ProxyURL:                   st.ProxyURL,
		AllowSensitiveDestinations: st.AllowSensitiveDestinations,
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	old := s.client
	s.client, s.settings = c, st
	s.mu.Unlock()
	if old != nil {
		old.Close()
	}
	return nil
}

func (s *Service) current() (*httpclient.Client, domain.Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client, s.settings
}

// AppInfo describes the running application.
type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	DataDir string `json:"dataPath"`
}

func (s *Service) AppInfo() AppInfo {
	return AppInfo{Name: AppName, Version: Version, DataDir: s.store.Path()}
}

// ---- collections, folders, requests ----

func (s *Service) ListCollections(ctx context.Context) ([]domain.Collection, error) {
	return s.store.ListCollections(ctx)
}

func (s *Service) GetCollectionTree(ctx context.Context, id string) (domain.CollectionTree, error) {
	return s.store.GetTree(ctx, id)
}

func (s *Service) SaveCollection(ctx context.Context, c domain.Collection) (domain.Collection, error) {
	c.Name = strings.TrimSpace(c.Name)
	return s.store.SaveCollection(ctx, c)
}

// InheritCollectionAuth makes every request in the collection use the
// collection's auth, returning how many requests were changed.
func (s *Service) InheritCollectionAuth(ctx context.Context, collectionID string) (int, error) {
	return s.store.InheritCollectionAuth(ctx, collectionID)
}

func (s *Service) DuplicateCollection(ctx context.Context, id string) (domain.Collection, error) {
	c, err := s.store.GetCollection(ctx, id)
	if err != nil {
		return c, err
	}
	name := c.Name + " (copy)"
	if len(name) > domain.MaxNameLength {
		name = name[:domain.MaxNameLength]
	}
	return s.store.DuplicateCollection(ctx, id, name)
}

func (s *Service) DeleteCollection(ctx context.Context, id string) error {
	return s.store.DeleteCollection(ctx, id)
}

func (s *Service) SaveFolder(ctx context.Context, f domain.Folder) (domain.Folder, error) {
	f.Name = strings.TrimSpace(f.Name)
	return s.store.SaveFolder(ctx, f)
}

func (s *Service) DeleteFolder(ctx context.Context, id string) error {
	return s.store.DeleteFolder(ctx, id)
}

func (s *Service) SaveRequest(ctx context.Context, r domain.Request) (domain.Request, error) {
	r.Name = strings.TrimSpace(r.Name)
	return s.store.SaveRequest(ctx, r)
}

func (s *Service) DuplicateRequest(ctx context.Context, id string) (domain.Request, error) {
	return s.store.DuplicateRequest(ctx, id)
}

func (s *Service) DeleteRequest(ctx context.Context, id string) error {
	return s.store.DeleteRequest(ctx, id)
}

// ---- environments ----

func (s *Service) ListEnvironments(ctx context.Context) ([]domain.Environment, error) {
	return s.store.ListEnvironments(ctx)
}

func (s *Service) SaveEnvironment(ctx context.Context, e domain.Environment) (domain.Environment, error) {
	e.Name = strings.TrimSpace(e.Name)
	return s.store.SaveEnvironment(ctx, e)
}

func (s *Service) DuplicateEnvironment(ctx context.Context, id string) (domain.Environment, error) {
	e, err := s.store.GetEnvironment(ctx, id)
	if err != nil {
		return e, err
	}
	e.ID = ""
	e.CreatedAt = time.Time{}
	e.Name = e.Name + " (copy)"
	return s.store.SaveEnvironment(ctx, e)
}

func (s *Service) DeleteEnvironment(ctx context.Context, id string) error {
	return s.store.DeleteEnvironment(ctx, id)
}

func (s *Service) environment(ctx context.Context, id string) (*domain.Environment, error) {
	if id == "" {
		return nil, nil
	}
	e, err := s.store.GetEnvironment(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("the selected environment no longer exists")
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ---- preflight and confirmation ----

type WarningLevel string

const (
	LevelInfo    WarningLevel = "info"
	LevelWarning WarningLevel = "warning"
	LevelError   WarningLevel = "error"
)

// Warning is something the user should see before a run.
type Warning struct {
	Level WarningLevel `json:"level"`
	Code  string       `json:"code"`
	// RequiresConfirmation means the run is refused unless confirmed.
	RequiresConfirmation bool   `json:"requiresConfirmation"`
	Message              string `json:"message"`
}

// ConfirmationRequiredError is returned when a run needs explicit approval.
type ConfirmationRequiredError struct{ Warnings []Warning }

func (e *ConfirmationRequiredError) Error() string {
	var msgs []string
	for _, w := range e.Warnings {
		if w.RequiresConfirmation {
			msgs = append(msgs, w.Message)
		}
	}
	return "confirmation required: " + strings.Join(msgs, "; ")
}

// Preflight checks requests against an environment without sending anything.
func (s *Service) Preflight(ctx context.Context, reqs []domain.Request, required []string, envID string) ([]Warning, error) {
	env, err := s.environment(ctx, envID)
	if err != nil {
		return nil, err
	}
	if reqs, env, err = s.scope(ctx, reqs, env); err != nil {
		return nil, err
	}
	_, st := s.current()
	warnings := []Warning{}
	if missing := environments.MissingRequired(env, required); len(missing) > 0 {
		warnings = append(warnings, Warning{Level: LevelError, Code: "missing_required",
			Message: "required variables are missing or empty: " + strings.Join(missing, ", ")})
	}
	var destructive []string
	for _, r := range reqs {
		r.Normalize()
		if domain.IsDestructiveMethod(r.Method) {
			destructive = append(destructive, r.Method+" "+r.Name)
		}
		prepared, err := runner.Prepare(r, env)
		if err != nil {
			warnings = append(warnings, Warning{Level: LevelError, Code: "prepare", Message: r.Name + ": " + err.Error()})
			continue
		}
		if u, err := url.Parse(prepared.HTTP.URL); err == nil && !st.AllowSensitiveDestinations {
			if err := httpclient.CheckHost(u.Hostname()); err != nil {
				warnings = append(warnings, Warning{Level: LevelError, Code: "blocked_destination", Message: r.Name + ": " + err.Error()})
			}
		}
	}
	if len(destructive) > 0 {
		list := summarizeList(destructive, 5)
		if env != nil && env.Protected {
			warnings = append(warnings, Warning{Level: LevelWarning, Code: "protected_destructive", RequiresConfirmation: true,
				Message: fmt.Sprintf("%q is a protected environment and these requests may modify data: %s", env.Name, list)})
		} else {
			warnings = append(warnings, Warning{Level: LevelInfo, Code: "destructive",
				Message: "these requests may modify data: " + list})
		}
	} else if env != nil && env.Protected {
		warnings = append(warnings, Warning{Level: LevelInfo, Code: "protected",
			Message: fmt.Sprintf("running against protected environment %q", env.Name)})
	}
	if st.InsecureSkipVerify {
		warnings = append(warnings, Warning{Level: LevelWarning, Code: "tls_insecure",
			Message: "TLS certificate verification is disabled in Settings"})
	}
	return warnings, nil
}

// PreflightCollection runs Preflight for a collection or one of its folders.
func (s *Service) PreflightCollection(ctx context.Context, collectionID, folderID, envID string) ([]Warning, error) {
	tree, err := s.store.GetTree(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	return s.Preflight(ctx, OrderedRequests(tree, folderID), tree.Collection.RequiredVariables, envID)
}

// scope applies the collection settings that requests follow: inherited
// auth, and the collection's variables underneath env's.
func (s *Service) scope(ctx context.Context, reqs []domain.Request, env *domain.Environment) ([]domain.Request, *domain.Environment, error) {
	reqs, err := s.inheritAuth(ctx, reqs)
	if err != nil {
		return nil, nil, err
	}
	if len(reqs) == 0 || reqs[0].CollectionID == "" {
		return reqs, env, nil
	}
	c, err := s.store.GetCollection(ctx, reqs[0].CollectionID)
	if errors.Is(err, storage.ErrNotFound) {
		return reqs, env, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return reqs, domain.WithCollectionVariables(env, c.Variables), nil
}

// inheritAuth returns reqs with "inherit" auth replaced by their collection's
// auth. Drafts that are not in a collection fall back to no auth.
func (s *Service) inheritAuth(ctx context.Context, reqs []domain.Request) ([]domain.Request, error) {
	out := make([]domain.Request, len(reqs))
	cache := map[string]domain.Auth{}
	for i, r := range reqs {
		if r.Auth.Type == domain.AuthInherit {
			auth, ok := cache[r.CollectionID]
			if !ok && r.CollectionID != "" {
				c, err := s.store.GetCollection(ctx, r.CollectionID)
				if err != nil && !errors.Is(err, storage.ErrNotFound) {
					return nil, err
				}
				auth = c.Auth
				cache[r.CollectionID] = auth
			}
			r.Auth = r.Auth.Effective(auth)
		}
		out[i] = r
	}
	return out, nil
}

func summarizeList(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" and %d more", len(items)-max)
}

func (s *Service) requireConfirmation(ctx context.Context, reqs []domain.Request, required []string, envID string, confirmed bool) error {
	warnings, err := s.Preflight(ctx, reqs, required, envID)
	if err != nil {
		return err
	}
	if confirmed {
		return nil
	}
	for _, w := range warnings {
		if w.RequiresConfirmation {
			return &ConfirmationRequiredError{Warnings: warnings}
		}
	}
	return nil
}

// ---- running ----

func (s *Service) beginOp(ctx context.Context, opID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	if opID == "" {
		return ctx, cancel
	}
	s.mu.Lock()
	if prev, ok := s.ops[opID]; ok {
		prev()
	}
	s.ops[opID] = cancel
	s.mu.Unlock()
	return ctx, func() {
		cancel()
		s.mu.Lock()
		delete(s.ops, opID)
		s.mu.Unlock()
	}
}

// Cancel stops an in-progress operation started with opID.
func (s *Service) Cancel(opID string) bool {
	s.mu.Lock()
	cancel, ok := s.ops[opID]
	s.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// SendRequest runs one request (saved or an unsaved draft) and records it in history.
func (s *Service) SendRequest(ctx context.Context, opID string, req domain.Request, envID string, confirmed bool) (domain.RunResult, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return domain.RunResult{}, err
	}
	if err := s.requireConfirmation(ctx, []domain.Request{req}, nil, envID, confirmed); err != nil {
		return domain.RunResult{}, err
	}
	run, err := s.execute(ctx, opID, []domain.Request{req}, envID, "", "", nil)
	if err != nil {
		return domain.RunResult{}, err
	}
	res := run.Results[0]
	res.RunID = run.ID
	return res, nil
}

// RunCollection runs every request in a collection, or only those inside
// folderID (including nested folders) when it is set.
func (s *Service) RunCollection(ctx context.Context, opID, collectionID, folderID, envID string, confirmed bool, progress func(done, total int, res domain.RunResult)) (domain.Run, error) {
	tree, err := s.store.GetTree(ctx, collectionID)
	if err != nil {
		return domain.Run{}, err
	}
	reqs := OrderedRequests(tree, folderID)
	if len(reqs) == 0 {
		return domain.Run{}, errors.New("there are no requests to run")
	}
	if err := s.requireConfirmation(ctx, reqs, tree.Collection.RequiredVariables, envID, confirmed); err != nil {
		return domain.Run{}, err
	}
	if missing := s.missingRequired(ctx, envID, tree.Collection); len(missing) > 0 {
		return domain.Run{}, fmt.Errorf("required variables are missing or empty: %s", strings.Join(missing, ", "))
	}
	return s.execute(ctx, opID, reqs, envID, tree.Collection.ID, tree.Collection.Name, progress)
}

func (s *Service) missingRequired(ctx context.Context, envID string, c domain.Collection) []string {
	env, _ := s.environment(ctx, envID)
	return environments.MissingRequired(domain.WithCollectionVariables(env, c.Variables), c.RequiredVariables)
}

// OrderedRequests returns requests in tree order: top-level requests first,
// then each folder depth-first, each level sorted by sort order and name.
func OrderedRequests(t domain.CollectionTree, rootFolder string) []domain.Request {
	children := map[string][]domain.Folder{}
	for _, f := range t.Folders {
		children[f.ParentID] = append(children[f.ParentID], f)
	}
	byParent := map[string][]domain.Request{}
	for _, r := range t.Requests {
		byParent[r.ParentID] = append(byParent[r.ParentID], r)
	}
	var out []domain.Request
	seen := map[string]bool{}
	var walk func(folderID string)
	walk = func(folderID string) {
		if seen[folderID] {
			return
		}
		seen[folderID] = true
		out = append(out, byParent[folderID]...)
		for _, f := range children[folderID] {
			walk(f.ID)
		}
	}
	walk(rootFolder)
	return out
}

func (s *Service) execute(ctx context.Context, opID string, reqs []domain.Request, envID, collectionID, collectionName string, progress func(done, total int, res domain.RunResult)) (domain.Run, error) {
	env, err := s.environment(ctx, envID)
	if err != nil {
		return domain.Run{}, err
	}
	if reqs, env, err = s.scope(ctx, reqs, env); err != nil {
		return domain.Run{}, err
	}
	client, st := s.current()
	items := make([]runner.Item, len(reqs))
	for i, r := range reqs {
		items[i].Request = r
		if r.ID != "" {
			b, err := s.store.GetBaseline(ctx, r.ID, envID)
			if err != nil {
				return domain.Run{}, err
			}
			items[i].Baseline = b
		}
	}

	opCtx, done := s.beginOp(ctx, opID)
	defer done()

	run := domain.Run{
		ID:             uuid.NewString(),
		CollectionID:   collectionID,
		CollectionName: collectionName,
		StartedAt:      time.Now().UTC(),
	}
	if env != nil {
		run.EnvironmentName = env.Name
	}
	finished := 0
	run.Results = runner.New(client).RunAll(opCtx, items, env, st.Concurrency, func(_ int, res domain.RunResult) {
		finished++
		if progress != nil {
			progress(finished, len(items), res)
		}
	})
	end := time.Now().UTC()
	run.FinishedAt = &end
	run.DurationMs = end.Sub(run.StartedAt).Milliseconds()
	for i := range run.Results {
		run.Results[i].RunID = run.ID
	}
	run.Tally()
	// Persist even if the caller's context was cancelled mid-run.
	if err := s.store.SaveRun(context.WithoutCancel(ctx), run, st.HistoryBodyLimitBytes, st.HistoryRetentionRuns); err != nil {
		return run, fmt.Errorf("the run finished but could not be saved to history: %w", err)
	}
	return run, nil
}

// ---- baselines ----

func (s *Service) GetBaseline(ctx context.Context, requestID, envID string) (*domain.Baseline, error) {
	return s.store.GetBaseline(ctx, requestID, envID)
}

// ApproveBaselineInput is the user's explicit approval of a response as the
// new baseline.
type ApproveBaselineInput struct {
	RequestID     string `json:"requestId"`
	EnvironmentID string `json:"environmentId"`
	// Body is the reviewed (already redacted) response body.
	Body          string   `json:"body"`
	Truncated     bool     `json:"truncated"`
	SelectedPaths []string `json:"selectedPaths"`
	IgnoredPaths  []string `json:"ignoredPaths"`
	Tolerance     float64  `json:"tolerance"`
}

// ApproveBaseline saves a reviewed response as the baseline for a request.
func (s *Service) ApproveBaseline(ctx context.Context, in ApproveBaselineInput) (domain.Baseline, error) {
	if in.RequestID == "" {
		return domain.Baseline{}, errors.New("save the request before approving a baseline")
	}
	if _, err := s.store.GetRequest(ctx, in.RequestID); err != nil {
		return domain.Baseline{}, fmt.Errorf("request not found: %w", err)
	}
	if in.Truncated {
		return domain.Baseline{}, errors.New("the response was truncated; raise the response size limit before approving it as a baseline")
	}
	if in.Tolerance < 0 {
		return domain.Baseline{}, errors.New("tolerance must not be negative")
	}
	paths := cleanPaths(in.SelectedPaths)
	for _, p := range paths {
		if _, err := assertions.ParsePath(p); err != nil {
			return domain.Baseline{}, err
		}
	}
	env, err := s.environment(ctx, in.EnvironmentID)
	if err != nil {
		return domain.Baseline{}, err
	}
	// Bodies shown in the UI are already redacted; redact again in case the
	// caller passed something else.
	secrets := env.SecretValues()
	if r, err := s.store.GetRequest(ctx, in.RequestID); err == nil {
		if c, err := s.store.GetCollection(ctx, r.CollectionID); err == nil {
			secrets = domain.WithCollectionVariables(env, c.Variables).SecretValues()
		}
	}
	body := redact.New(secrets...).String(in.Body)
	snap, err := regression.Snapshot([]byte(body), paths)
	if err != nil {
		return domain.Baseline{}, err
	}
	return s.store.SaveBaseline(ctx, domain.Baseline{
		RequestID: in.RequestID, EnvironmentID: in.EnvironmentID, Snapshot: snap,
		SelectedPaths: paths, IgnoredPaths: cleanPaths(in.IgnoredPaths), Tolerance: in.Tolerance,
	})
}

// UpdateBaselineRules changes ignored paths and tolerance of an existing baseline.
func (s *Service) UpdateBaselineRules(ctx context.Context, id string, ignored []string, tolerance float64) error {
	if tolerance < 0 {
		return errors.New("tolerance must not be negative")
	}
	return s.store.UpdateBaselineRules(ctx, id, cleanPaths(ignored), tolerance)
}

func (s *Service) DeleteBaseline(ctx context.Context, id string) error {
	return s.store.DeleteBaseline(ctx, id)
}

func cleanPaths(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// ---- history ----

func (s *Service) ListRuns(ctx context.Context, f domain.RunFilter) ([]domain.Run, error) {
	return s.store.ListRuns(ctx, f)
}

func (s *Service) GetRun(ctx context.Context, id string) (domain.Run, error) {
	return s.store.GetRun(ctx, id)
}

func (s *Service) DeleteRun(ctx context.Context, id string) error { return s.store.DeleteRun(ctx, id) }

func (s *Service) ClearHistory(ctx context.Context) (int64, error) {
	return s.store.ClearHistory(ctx, nil)
}

// PreviousComparison compares a result with the previous run of the same request.
type PreviousComparison struct {
	Found             bool                `json:"found"`
	PreviousRunID     string              `json:"previousRunId"`
	PreviousStartedAt time.Time           `json:"previousStartedAt"`
	StatusBefore      domain.ResultStatus `json:"statusBefore"`
	StatusAfter       domain.ResultStatus `json:"statusAfter"`
	CodeBefore        int                 `json:"codeBefore"`
	CodeAfter         int                 `json:"codeAfter"`
	// NewlyFailing lists assertion messages that passed before and fail now.
	NewlyFailing []string           `json:"newlyFailing"`
	Diffs        []domain.DiffEntry `json:"diffs"`
	Message      string             `json:"message"`
}

func (s *Service) CompareWithPrevious(ctx context.Context, runID, resultID string) (PreviousComparison, error) {
	var out PreviousComparison
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return out, err
	}
	var cur *domain.RunResult
	for i := range run.Results {
		if run.Results[i].ID == resultID {
			cur = &run.Results[i]
		}
	}
	if cur == nil {
		return out, storage.ErrNotFound
	}
	if cur.RequestID == "" {
		out.Message = "this result is not linked to a saved request"
		return out, nil
	}
	prev, err := s.store.PreviousResult(ctx, cur.RequestID, runID)
	if err != nil {
		return out, err
	}
	if prev == nil {
		out.Message = "no previous run of this request"
		return out, nil
	}
	prevRun, _ := s.store.GetRun(ctx, prev.RunID)
	out.Found = true
	out.PreviousRunID = prev.RunID
	out.PreviousStartedAt = prevRun.StartedAt
	out.StatusBefore, out.StatusAfter = prev.Status, cur.Status
	if prev.Response != nil {
		out.CodeBefore = prev.Response.StatusCode
	}
	if cur.Response != nil {
		out.CodeAfter = cur.Response.StatusCode
	}
	passedBefore := map[string]bool{}
	for _, a := range prev.AssertionResults {
		if a.Passed {
			passedBefore[a.AssertionID] = true
		}
	}
	out.NewlyFailing = []string{}
	for _, a := range cur.AssertionResults {
		if !a.Passed && passedBefore[a.AssertionID] {
			out.NewlyFailing = append(out.NewlyFailing, a.Message)
		}
	}
	out.Diffs = []domain.DiffEntry{}
	if prev.Response != nil && cur.Response != nil && !prev.Response.Truncated && !cur.Response.Truncated {
		a, errA := assertions.DecodeJSON([]byte(prev.Response.Body))
		b, errB := assertions.DecodeJSON([]byte(cur.Response.Body))
		if errA == nil && errB == nil {
			ignored := []string{}
			if bl, _ := s.store.GetBaseline(ctx, cur.RequestID, ""); bl != nil {
				ignored = bl.IgnoredPaths
			}
			out.Diffs = regression.Diff(a, b, regression.Options{IgnoredPaths: ignored})
		}
	}
	out.Message = fmt.Sprintf("%d body difference(s), %d newly failing assertion(s)", len(out.Diffs), len(out.NewlyFailing))
	return out, nil
}

// ---- reports ----

// ExportReport renders a stored run as "json" or "html".
func (s *Service) ExportReport(ctx context.Context, runID, format string) ([]byte, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	_, st := s.current()
	rep := reports.Build(run, reports.Meta{AppName: AppName, AppVersion: Version, Settings: st, Secrets: s.allSecrets(ctx)}, time.Now())
	var buf bytes.Buffer
	switch format {
	case "json":
		err = reports.WriteJSON(&buf, rep)
	case "html":
		err = reports.WriteHTML(&buf, rep)
	default:
		return nil, fmt.Errorf("unknown report format %q", format)
	}
	return buf.Bytes(), err
}

// allSecrets collects secret values from every environment and collection, used as a final
// redaction pass on exported reports.
func (s *Service) allSecrets(ctx context.Context) []string {
	envs, err := s.store.ListEnvironments(ctx)
	if err != nil {
		return nil
	}
	var out []string
	for i := range envs {
		out = append(out, envs[i].SecretValues()...)
	}
	if cs, err := s.store.ListCollections(ctx); err == nil {
		for _, c := range cs {
			out = append(out, (&domain.Environment{Variables: c.Variables}).SecretValues()...)
		}
	}
	return out
}

// ---- import / export ----

// ExportPreview reports which secrets an export would contain.
func (s *Service) ExportPreview(ctx context.Context, collectionID string, envIDs []string) ([]collectionio.SecretFinding, error) {
	tree, envs, err := s.exportInputs(ctx, collectionID, envIDs)
	if err != nil {
		return nil, err
	}
	f := collectionio.ScanSecrets(tree, envs)
	if f == nil {
		f = []collectionio.SecretFinding{}
	}
	return f, nil
}

func (s *Service) exportInputs(ctx context.Context, collectionID string, envIDs []string) (domain.CollectionTree, []domain.Environment, error) {
	tree, err := s.store.GetTree(ctx, collectionID)
	if err != nil {
		return tree, nil, err
	}
	var envs []domain.Environment
	for _, id := range envIDs {
		e, err := s.store.GetEnvironment(ctx, id)
		if err != nil {
			return tree, nil, err
		}
		envs = append(envs, e)
	}
	return tree, envs, nil
}

// ExportCollection serializes a collection (and optional environments).
// Secrets are blanked unless includeSecrets is true.
func (s *Service) ExportCollection(ctx context.Context, collectionID string, envIDs []string, includeSecrets bool) ([]byte, error) {
	tree, envs, err := s.exportInputs(ctx, collectionID, envIDs)
	if err != nil {
		return nil, err
	}
	doc := collectionio.Export(tree, envs, includeSecrets, Version, time.Now())
	var buf bytes.Buffer
	err = collectionio.Write(&buf, doc)
	return buf.Bytes(), err
}

// ImportResult summarizes an import.
type ImportResult struct {
	Collection   domain.Collection `json:"collection"`
	Requests     int               `json:"requests"`
	Folders      int               `json:"folders"`
	Environments int               `json:"environments"`
}

// ImportCollection validates untrusted data and stores it with fresh IDs.
func (s *Service) ImportCollection(ctx context.Context, data []byte) (ImportResult, error) {
	imp, err := collectionio.Read(bytes.NewReader(data))
	if err != nil {
		return ImportResult{}, err
	}
	if err := s.store.ImportTree(ctx, imp.Tree); err != nil {
		return ImportResult{}, err
	}
	for _, e := range imp.Environments {
		if _, err := s.store.SaveEnvironment(ctx, e); err != nil {
			return ImportResult{}, fmt.Errorf("collection imported, but environment %q failed: %w", e.Name, err)
		}
	}
	c, err := s.store.GetCollection(ctx, imp.Tree.Collection.ID)
	return ImportResult{Collection: c, Requests: len(imp.Tree.Requests), Folders: len(imp.Tree.Folders), Environments: len(imp.Environments)}, err
}

// ---- settings and data management ----

func (s *Service) GetSettings() domain.Settings {
	_, st := s.current()
	return st
}

func (s *Service) SaveSettings(ctx context.Context, st domain.Settings) error {
	if err := st.Validate(); err != nil {
		return err
	}
	if err := s.applySettings(st); err != nil {
		return err
	}
	return s.store.SaveSettings(ctx, st)
}

func (s *Service) DataInfo(ctx context.Context) (storage.DataInfo, error) {
	return s.store.DataInfo(ctx)
}

func (s *Service) CompactDatabase(ctx context.Context) error { return s.store.Vacuum(ctx) }

// DeleteAllData wipes every local record and restores default settings.
func (s *Service) DeleteAllData(ctx context.Context) error {
	if err := s.store.DeleteAllData(ctx); err != nil {
		return err
	}
	if err := s.applySettings(domain.DefaultSettings()); err != nil {
		return err
	}
	return s.store.Vacuum(ctx)
}
