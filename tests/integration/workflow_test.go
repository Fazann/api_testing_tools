// Package integration exercises the full engine (service, runner, storage,
// reports) against a local mock HTTP server.
package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

type fixture struct {
	svc   *service.Service
	store *storage.Store
	api   *mockAPI
	env   domain.Environment
	tree  domain.CollectionTree
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	svc, err := service.New(store)
	if err != nil {
		t.Fatal(err)
	}
	st := svc.GetSettings()
	st.ProxyMode = "none"
	if err := svc.SaveSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	api, srv := newMockAPI()
	t.Cleanup(srv.Close)

	data, err := os.ReadFile("../fixtures/wallet-demo.collection.json")
	if err != nil {
		t.Fatal(err)
	}
	imp, err := svc.ImportCollection(ctx, data)
	if err != nil {
		t.Fatalf("import demo collection: %v", err)
	}
	if imp.Requests != 6 || imp.Folders != 3 || imp.Environments != 1 {
		t.Fatalf("unexpected import summary %+v", imp)
	}
	env, err := svc.SaveEnvironment(ctx, domain.Environment{Name: "Integration", Variables: []domain.Variable{
		{Key: "base_url", Value: srv.URL, Enabled: true},
		{Key: "token", Value: testToken, Secret: true, Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := svc.GetCollectionTree(ctx, imp.Collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: svc, store: store, api: api, env: env, tree: tree}
}

func (f *fixture) request(t *testing.T, name string) domain.Request {
	for _, r := range f.tree.Requests {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("request %q not found", name)
	return domain.Request{}
}

func failures(run domain.Run) []string {
	var out []string
	for _, r := range run.Results {
		if r.Error != "" {
			out = append(out, r.Request.Name+": "+r.Error)
		}
		for _, a := range r.AssertionResults {
			if !a.Passed {
				out = append(out, r.Request.Name+": "+a.Message)
			}
		}
		if r.Regression != nil && !r.Regression.Passed {
			out = append(out, r.Request.Name+": "+r.Regression.Message)
		}
	}
	return out
}

// Workflow A: create and run tests; everything passes against a healthy API.
func TestCollectionRunPasses(t *testing.T) {
	f := setup(t)
	var progress int
	run, err := f.svc.RunCollection(context.Background(), "op", f.tree.Collection.ID, "", f.env.ID, false,
		func(done, total int, _ domain.RunResult) { progress = done })
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.StatusPassed || run.PassedCount != 6 {
		t.Fatalf("expected all 6 to pass, got %s %d/%d/%d: %v", run.Status, run.PassedCount, run.FailedCount, run.ErrorCount, failures(run))
	}
	if progress != 6 {
		t.Fatalf("progress reported %d of 6", progress)
	}
	// Tree order: top-level request first, then folders.
	if run.Results[0].Request.Name != "Health check" {
		t.Fatalf("unexpected order, first was %q", run.Results[0].Request.Name)
	}
	stored, err := f.svc.GetRun(context.Background(), run.ID)
	if err != nil || len(stored.Results) != 6 {
		t.Fatalf("run not persisted: %v", err)
	}
}

// Workflow B: approve a baseline, change behavior, detect the regression.
func TestRegressionDetectedAgainstBaseline(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	req := f.request(t, "Get balance")

	res, err := f.svc.SendRequest(ctx, "op", req, f.env.ID, false)
	if err != nil || res.Status != domain.StatusPassed {
		t.Fatalf("first run: %v %+v", err, res)
	}
	if res.Regression != nil {
		t.Fatal("no baseline exists yet, so no comparison should run")
	}
	_, err = f.svc.ApproveBaseline(ctx, service.ApproveBaselineInput{
		RequestID: req.ID, EnvironmentID: f.env.ID, Body: res.Response.Body,
		IgnoredPaths: []string{"meta.request_id", "meta.generated_at"},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, _ = f.svc.SendRequest(ctx, "op", req, f.env.ID, false)
	if res.Status != domain.StatusPassed || res.Regression == nil || !res.Regression.Passed {
		t.Fatalf("unchanged API with dynamic fields ignored should pass: %+v", res.Regression)
	}

	// A code change returns 200 with a wrong balance and a type change.
	f.api.set(func(m *mockAPI) { m.balance = 99.99 })
	res, _ = f.svc.SendRequest(ctx, "op", req, f.env.ID, false)
	if res.Status != domain.StatusFailed || len(res.Regression.Diffs) != 1 {
		t.Fatalf("expected a balance diff, got %+v", res.Regression)
	}
	d := res.Regression.Diffs[0]
	if d.Path != "data.balance" || d.Kind != domain.DiffChanged || d.Before != "150.25" || d.After != "99.99" {
		t.Fatalf("unexpected diff %+v", d)
	}

	// Baselines are never updated automatically.
	b, _ := f.svc.GetBaseline(ctx, req.ID, f.env.ID)
	if !strings.Contains(string(b.Snapshot), "150.25") {
		t.Fatal("baseline changed without approval")
	}

	// Previous-run comparison reports the same change.
	cmp, err := f.svc.CompareWithPrevious(ctx, res.RunID, res.ID)
	if err != nil || !cmp.Found {
		t.Fatalf("compare with previous: %v %+v", err, cmp)
	}
	found := false
	for _, d := range cmp.Diffs {
		if d.Path == "data.balance" {
			found = true
		}
	}
	if !found {
		t.Fatalf("previous-run diff missing balance change: %+v", cmp.Diffs)
	}
}

// A 200 OK that leaks a sensitive field must fail.
func TestBusinessRuleRegression(t *testing.T) {
	f := setup(t)
	f.api.set(func(m *mockAPI) { m.leakPassword = true })
	run, err := f.svc.RunCollection(context.Background(), "", f.tree.Collection.ID, "", f.env.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.FailedCount != 1 {
		t.Fatalf("expected exactly the profile test to fail: %v", failures(run))
	}
}

// Workflow C: missing required variables stop the run before any request is sent.
func TestRequiredVariables(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	empty, _ := f.svc.SaveEnvironment(ctx, domain.Environment{Name: "Empty"})
	_, err := f.svc.RunCollection(ctx, "", f.tree.Collection.ID, "", empty.ID, false, nil)
	// base_url is a collection variable in the demo, so only token is missing.
	if err == nil || strings.Contains(err.Error(), "base_url") || !strings.Contains(err.Error(), "token") {
		t.Fatalf("expected missing required variables error for token only, got %v", err)
	}
	warnings, _ := f.svc.Preflight(ctx, f.tree.Requests, f.tree.Collection.RequiredVariables, empty.ID)
	if len(warnings) == 0 || warnings[0].Code != "missing_required" {
		t.Fatalf("preflight should report missing variables: %+v", warnings)
	}
}

func TestProtectedEnvironmentNeedsConfirmation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.env.Protected = true
	env, _ := f.svc.SaveEnvironment(ctx, f.env)

	_, err := f.svc.RunCollection(ctx, "", f.tree.Collection.ID, "", env.ID, false, nil)
	var confirm *service.ConfirmationRequiredError
	if !errors.As(err, &confirm) {
		t.Fatalf("destructive requests against a protected environment must require confirmation, got %v", err)
	}
	// Read-only requests do not need confirmation.
	if _, err := f.svc.SendRequest(ctx, "", f.request(t, "Get balance"), env.ID, false); err != nil {
		t.Fatalf("GET should not need confirmation: %v", err)
	}
	run, err := f.svc.RunCollection(ctx, "", f.tree.Collection.ID, "", env.ID, true, nil)
	if err != nil || run.PassedCount != 6 {
		t.Fatalf("confirmed run: %v %v", err, failures(run))
	}
}

func TestFolderRunAndCancellation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var reportsFolder string
	for _, fo := range f.tree.Folders {
		if fo.Name == "Reports" {
			reportsFolder = fo.ID
		}
	}
	run, err := f.svc.RunCollection(ctx, "", f.tree.Collection.ID, reportsFolder, f.env.ID, false, nil)
	if err != nil || len(run.Results) != 1 || run.Status != domain.StatusPassed {
		t.Fatalf("folder run: %v %+v", err, failures(run))
	}

	f.api.set(func(m *mockAPI) { m.slow = 5 * time.Second })
	slow := domain.Request{Name: "Slow", Method: "GET", URLTemplate: "{{base_url}}/slow", TimeoutMs: 10_000}
	time.AfterFunc(100*time.Millisecond, func() { f.svc.Cancel("slow-op") })
	start := time.Now()
	res, err := f.svc.SendRequest(ctx, "slow-op", slow, f.env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != domain.StatusCancelled || time.Since(start) > 2*time.Second {
		t.Fatalf("expected prompt cancellation, got %s after %v", res.Status, time.Since(start))
	}

	slow.TimeoutMs = 100
	res, _ = f.svc.SendRequest(ctx, "", slow, f.env.ID, false)
	if res.Status != domain.StatusError || !strings.Contains(res.Error, "timed out") {
		t.Fatalf("expected timeout error, got %s %q", res.Status, res.Error)
	}
}

// Reports and history must never contain the secret token.
func TestNoSecretsInHistoryOrReports(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	run, err := f.svc.RunCollection(ctx, "", f.tree.Collection.ID, "", f.env.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"json", "html"} {
		out, err := f.svc.ExportReport(ctx, run.ID, format)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), testToken) {
			t.Fatalf("%s report leaked the token", format)
		}
		if !strings.Contains(string(out), "Integration") || !strings.Contains(string(out), service.Version) {
			t.Fatalf("%s report missing environment name or version", format)
		}
	}
	stored, _ := f.svc.GetRun(ctx, run.ID)
	for _, r := range stored.Results {
		for _, h := range r.Request.Headers {
			if h.Key == "Authorization" && h.Value != "[REDACTED]" {
				t.Fatalf("history stored an authorization header: %q", h.Value)
			}
		}
	}

	// Collection export warns about and strips the secret variable.
	findings, _ := f.svc.ExportPreview(ctx, f.tree.Collection.ID, []string{f.env.ID})
	if len(findings) != 1 {
		t.Fatalf("expected one secret finding, got %+v", findings)
	}
	exported, _ := f.svc.ExportCollection(ctx, f.tree.Collection.ID, []string{f.env.ID}, false)
	if strings.Contains(string(exported), testToken) {
		t.Fatal("collection export leaked the token")
	}
	// The export can be imported again.
	if _, err := f.svc.ImportCollection(ctx, exported); err != nil {
		t.Fatalf("re-import exported collection: %v", err)
	}
}

func TestHistoryFiltersAndDeletion(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	req := f.request(t, "Health check")
	for i := 0; i < 3; i++ {
		if _, err := f.svc.SendRequest(ctx, "", req, f.env.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	runs, _ := f.svc.ListRuns(ctx, domain.RunFilter{RequestID: req.ID})
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs for request, got %d", len(runs))
	}
	future := time.Now().Add(time.Hour)
	if none, _ := f.svc.ListRuns(ctx, domain.RunFilter{From: &future}); len(none) != 0 {
		t.Fatal("date filter ignored")
	}
	if n, _ := f.svc.ClearHistory(ctx); n != 3 {
		t.Fatalf("cleared %d", n)
	}
}

// An OpenAPI description imports as a runnable collection with an environment.
func TestOpenAPIImportRunsAgainstAPI(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	data, err := os.ReadFile("../fixtures/wallet-mock.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	imp, err := f.svc.ImportCollection(ctx, data)
	if err != nil {
		t.Fatalf("import OpenAPI: %v", err)
	}
	if imp.Requests != 3 || imp.Folders != 2 || imp.Environments != 1 {
		t.Fatalf("unexpected import summary %+v", imp)
	}
	// Point the generated environment at the mock server and fill in the token.
	envs, _ := f.svc.ListEnvironments(ctx)
	var env domain.Environment
	for _, e := range envs {
		if e.Name == "Wallet mock (from OpenAPI)" {
			env = e
		}
	}
	for i, v := range env.Variables {
		switch v.Key {
		case "base_url":
			env.Variables[i].Value = f.env.Variables[0].Value
		case "token":
			if !v.Secret {
				t.Fatal("token placeholder should be secret")
			}
			env.Variables[i].Value = testToken
		}
	}
	if _, err := f.svc.SaveEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	run, err := f.svc.RunCollection(ctx, "", imp.Collection.ID, "", env.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.StatusPassed || run.PassedCount != 3 {
		t.Fatalf("imported collection should pass against the mock API: %v", failures(run))
	}
}

// Requests with "inherit" auth use the collection's Bearer token; changing it
// in one place affects all of them, and a request can still override it.
func TestCollectionAuthIsInherited(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c := f.tree.Collection
	if c.Auth.Type != domain.AuthBearer || c.Auth.Token != "{{token}}" {
		t.Fatalf("demo collection should define Bearer {{token}}, got %+v", c.Auth)
	}
	balance := f.request(t, "Get balance")
	if balance.Auth.Type != domain.AuthInherit {
		t.Fatalf("Get balance should inherit, got %+v", balance.Auth)
	}

	// Inherited token is sent (the run passes) and is redacted from history.
	res, err := f.svc.SendRequest(ctx, "", balance, f.env.ID, false)
	if err != nil || res.Status != domain.StatusPassed {
		t.Fatalf("inherited auth: %v %+v", err, res)
	}
	for _, h := range res.Request.Headers {
		if h.Key == "Authorization" && h.Value != "[REDACTED]" {
			t.Fatalf("authorization header not redacted: %q", h.Value)
		}
	}

	// Break the collection auth: every inheriting request now gets 401.
	c.Auth = domain.Auth{Type: domain.AuthBearer, Token: "wrong-token"}
	if _, err := f.svc.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	run, err := f.svc.RunCollection(ctx, "", c.ID, "", f.env.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.FailedCount != 4 || run.PassedCount != 2 {
		t.Fatalf("expected the 4 inheriting requests to fail and the 2 no-auth ones to pass: %v", failures(run))
	}

	// A request can override the collection auth.
	balance.Auth = domain.Auth{Type: domain.AuthBearer, Token: "{{token}}"}
	if res, _ := f.svc.SendRequest(ctx, "", balance, f.env.ID, false); res.Status != domain.StatusPassed {
		t.Fatalf("override should win over collection auth: %+v", res)
	}

	// A collection cannot itself inherit.
	c.Auth = domain.Auth{Type: domain.AuthInherit}
	if _, err := f.svc.SaveCollection(ctx, c); err == nil {
		t.Fatal("collection auth of type inherit must be rejected")
	}
}

func TestCollectionVariablesApplyToAllRequests(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c := f.tree.Collection
	base := ""
	for _, v := range f.env.Variables {
		if v.Key == "base_url" {
			base = v.Value
		}
	}

	// Collection-level base_url and token: every request follows them, even
	// with no environment selected.
	c.Variables = []domain.Variable{
		{Key: "base_url", Value: base, Enabled: true},
		{Key: "token", Value: testToken, Secret: true, Enabled: true},
	}
	if _, err := f.svc.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if w, err := f.svc.PreflightCollection(ctx, c.ID, "", ""); err != nil || hasCode(w, "missing_required") {
		t.Fatalf("collection variables should satisfy required variables: %v %+v", err, w)
	}
	run, err := f.svc.RunCollection(ctx, "", c.ID, "", "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.FailedCount != 0 || run.PassedCount != 6 {
		t.Fatalf("all requests should pass using collection variables: %v", failures(run))
	}
	full, _ := f.svc.GetRun(ctx, run.ID)
	for _, r := range full.Results {
		if strings.Contains(r.Request.URL+r.Response.Body, testToken) {
			t.Fatalf("secret collection variable leaked into history: %+v", r)
		}
	}

	// Environment variables override collection variables of the same name.
	env := f.env
	env.Variables = []domain.Variable{{Key: "token", Value: "env-wrong-token", Secret: true, Enabled: true}}
	if env, err = f.svc.SaveEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	run, _ = f.svc.RunCollection(ctx, "", c.ID, "", env.ID, false, nil)
	if run.FailedCount != 4 {
		t.Fatalf("environment token should override the collection token: %v", failures(run))
	}

	// Making every request inherit the collection auth.
	n, err := f.svc.InheritCollectionAuth(ctx, c.ID)
	if err != nil || n != 2 {
		t.Fatalf("expected the 2 no-auth requests to switch to inherit: %d %v", n, err)
	}
	tree, _ := f.svc.GetCollectionTree(ctx, c.ID)
	for _, r := range tree.Requests {
		if r.Auth.Type != domain.AuthInherit {
			t.Fatalf("%s still has its own auth: %+v", r.Name, r.Auth)
		}
	}
}

func hasCode(ws []service.Warning, code string) bool {
	for _, w := range ws {
		if w.Code == code {
			return true
		}
	}
	return false
}
