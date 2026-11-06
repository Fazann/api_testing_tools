package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"api-test-kits/internal/compare"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/regression"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

// buildAPI mirrors docs/demo/compare_demo_api.py: the new build has a type
// change, a changed field, a changed header and a slow endpoint; both builds
// return values that change on every call.
func buildAPI(t *testing.T, newBuild bool, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("X-Request-Id", fmt.Sprint(n))
		stamp := time.Now().UTC().Format(time.RFC3339Nano) + fmt.Sprint(n)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/orders":
			writeJSON(w, http.StatusCreated, map[string]any{"id": n, "status": "created"})
		case r.URL.Path == "/account":
			var balance any = 150.25
			if newBuild {
				balance = "150.25"
			}
			writeJSON(w, 200, map[string]any{"id": "acc_1", "balance": balance, "currency": "USD",
				"request_id": fmt.Sprintf("req-%d", n), "server_time": stamp})
		case r.URL.Path == "/orders":
			status, cache := "shipped", "max-age=60"
			if newBuild {
				status, cache = "pending", "no-store"
			}
			w.Header().Set("Cache-Control", cache)
			writeJSON(w, 200, map[string]any{"orders": []any{map[string]any{"id": 1, "status": "paid"},
				map[string]any{"id": 2, "status": status}}, "generated_at": stamp})
		case r.URL.Path == "/health":
			writeJSON(w, 200, map[string]any{"status": "ok", "version": "1.0", "uptime_s": n, "checked_at": stamp})
		case r.URL.Path == "/prices":
			if newBuild {
				time.Sleep(150 * time.Millisecond)
			}
			writeJSON(w, 200, map[string]any{"items": []any{map[string]any{"sku": "A", "price": 10}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type compareFixture struct {
	svc        *service.Service
	coll       domain.Collection
	oldEnv     domain.Environment
	newEnv     domain.Environment
	reqs       map[string]domain.Request
	oldCalls   *atomic.Int64
	newCalls   *atomic.Int64
	oldBaseURL string
}

func setupCompare(t *testing.T) *compareFixture {
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
	f := &compareFixture{svc: svc, reqs: map[string]domain.Request{}, oldCalls: &atomic.Int64{}, newCalls: &atomic.Int64{}}
	oldSrv, newSrv := buildAPI(t, false, f.oldCalls), buildAPI(t, true, f.newCalls)
	f.oldBaseURL = oldSrv.URL
	for _, e := range []struct {
		name, url string
		dst       *domain.Environment
	}{{"Old build", oldSrv.URL, &f.oldEnv}, {"New build", newSrv.URL, &f.newEnv}} {
		env, err := svc.SaveEnvironment(ctx, domain.Environment{Name: e.name,
			Variables: []domain.Variable{{Key: "base_url", Value: e.url, Enabled: true}}})
		if err != nil {
			t.Fatal(err)
		}
		*e.dst = env
	}
	if f.coll, err = svc.SaveCollection(ctx, domain.Collection{Name: "Compare"}); err != nil {
		t.Fatal(err)
	}
	for i, r := range []struct{ name, method, path string }{
		{"Get account", "GET", "/account"},
		{"List orders", "GET", "/orders"},
		{"Health", "GET", "/health"},
		{"Get prices", "GET", "/prices"},
		{"Create order", "POST", "/orders"},
	} {
		saved, err := svc.SaveRequest(ctx, domain.Request{CollectionID: f.coll.ID, Name: r.name, Method: r.method,
			URLTemplate: "{{base_url}}" + r.path, SortOrder: i})
		if err != nil {
			t.Fatal(err)
		}
		f.reqs[r.name] = saved
	}
	return f
}

func (f *compareFixture) run(t *testing.T, cfg domain.EnvComparison) compare.Report {
	t.Helper()
	cfg.CollectionID, cfg.EnvA, cfg.EnvB = f.coll.ID, f.oldEnv.ID, f.newEnv.ID
	rep, err := f.svc.CompareEnvironments(context.Background(), "cmp", cfg, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func byName(rep compare.Report) map[string]compare.Result {
	m := map[string]compare.Result{}
	for _, r := range rep.Requests {
		m[r.Name] = r
	}
	return m
}

func diffPaths(entries []domain.DiffEntry) []string {
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func TestCompareEnvironmentsFindsRegressionsAndIgnoresChangingFields(t *testing.T) {
	f := setupCompare(t)
	rep := f.run(t, domain.EnvComparison{DetectChanging: true})
	got := byName(rep)

	if rep.A.Name != "Old build" || rep.B.Name != "New build" || rep.RunIDA == "" || rep.RunIDB == "" {
		t.Fatalf("report header %+v", rep)
	}
	account := got["Get account"]
	if account.Verdict != compare.Different || fmt.Sprint(diffPaths(account.Body)) != "[balance]" ||
		account.Body[0].Kind != domain.DiffTypeChanged {
		t.Errorf("Get account: %+v", account)
	}
	if fmt.Sprint(account.Changing) != "[request_id server_time]" {
		t.Errorf("Get account changing = %v", account.Changing)
	}
	orders := got["List orders"]
	if orders.Verdict != compare.Different || fmt.Sprint(diffPaths(orders.Body)) != "[orders[1].status]" ||
		fmt.Sprint(diffPaths(orders.Headers)) != "[Cache-Control]" {
		t.Errorf("List orders: body %+v headers %+v", orders.Body, orders.Headers)
	}
	health := got["Health"]
	if health.Verdict != compare.Same || fmt.Sprint(health.Changing) != "[checked_at uptime_s]" {
		t.Errorf("Health: %+v", health)
	}
	prices := got["Get prices"]
	if prices.Verdict != compare.Same || prices.Slower != "b" {
		t.Errorf("Get prices: %+v", prices)
	}
	create := got["Create order"]
	if create.Verdict != compare.Skipped || create.SkipReason == "" {
		t.Errorf("Create order must be skipped by default: %+v", create)
	}
	if rep.Same != 2 || rep.Different != 2 || rep.Skipped != 1 || rep.Errors != 0 || rep.Slower != 1 {
		t.Errorf("tally %+v", rep)
	}
	// Each environment got 4 read-only requests twice; the POST was never sent.
	if f.oldCalls.Load() != 8 || f.newCalls.Load() != 8 {
		t.Errorf("calls old %d new %d, want 8 each", f.oldCalls.Load(), f.newCalls.Load())
	}

	// The first pass of each environment is in history; the detection calls are not.
	for _, id := range []string{rep.RunIDA, rep.RunIDB} {
		run, err := f.svc.GetRun(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if len(run.Results) != 4 || run.Kind != domain.RunKindNormal {
			t.Errorf("history run %s: %d results, kind %s", id, len(run.Results), run.Kind)
		}
	}
	// The settings were saved with the collection.
	saved, err := f.svc.GetEnvComparison(context.Background(), f.coll.ID)
	if err != nil || saved.EnvA != f.oldEnv.ID || saved.EnvB != f.newEnv.ID {
		t.Errorf("saved settings %+v %v", saved, err)
	}
}

func TestCompareEnvironmentsWithoutDetectionReportsNoise(t *testing.T) {
	f := setupCompare(t)
	got := byName(f.run(t, domain.EnvComparison{DetectChanging: false}))
	if h := got["Health"]; h.Verdict != compare.Different || len(h.Changing) != 0 {
		t.Errorf("without detection, changing fields are differences: %+v", h)
	}
	if f.oldCalls.Load() != 4 {
		t.Errorf("without detection each request is sent once, got %d calls", f.oldCalls.Load())
	}
}

func TestCompareEnvironmentsIgnoredPathsAndModifying(t *testing.T) {
	f := setupCompare(t)
	got := byName(f.run(t, domain.EnvComparison{DetectChanging: true, IncludeModifying: true,
		IgnoredPaths: []string{"balance", "orders[*].status", "header:cache-control", "id"}}))
	for _, name := range []string{"Get account", "List orders", "Health", "Create order"} {
		if got[name].Verdict != compare.Same {
			t.Errorf("%s should be the same with these ignored paths: %+v", name, got[name])
		}
	}
	// The POST is sent once per environment, never twice.
	if f.oldCalls.Load() != 9 {
		t.Errorf("calls %d, want 4×2 read-only + 1 POST", f.oldCalls.Load())
	}
}

func TestCompareEnvironmentsUsesBaselineIgnoredPaths(t *testing.T) {
	f := setupCompare(t)
	ctx := context.Background()
	if _, err := f.svc.ApproveBaseline(ctx, service.ApproveBaselineInput{RequestID: f.reqs["Get account"].ID,
		EnvironmentID: f.newEnv.ID, Body: `{"balance":"1"}`, IgnoredPaths: []string{"balance"}}); err != nil {
		t.Fatal(err)
	}
	got := byName(f.run(t, domain.EnvComparison{DetectChanging: true}))
	if got["Get account"].Verdict != compare.Same {
		t.Errorf("a baseline's ignored paths apply to the comparison: %+v", got["Get account"])
	}
}

func TestCompareEnvironmentsValidation(t *testing.T) {
	f := setupCompare(t)
	ctx := context.Background()
	if _, err := f.svc.CompareEnvironments(ctx, "x", domain.EnvComparison{CollectionID: f.coll.ID, EnvA: f.oldEnv.ID,
		EnvB: f.oldEnv.ID}, false, nil); err == nil {
		t.Error("the same environment twice must be refused")
	}
	f.newEnv.Protected = true
	if _, err := f.svc.SaveEnvironment(ctx, f.newEnv); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.CompareEnvironments(ctx, "x", domain.EnvComparison{CollectionID: f.coll.ID, EnvA: f.oldEnv.ID,
		EnvB: f.newEnv.ID, IncludeModifying: true}, false, nil)
	var conf *service.ConfirmationRequiredError
	if !asConfirmation(err, &conf) {
		t.Errorf("a protected environment with a POST needs confirmation, got %v", err)
	}
	if f.oldCalls.Load()+f.newCalls.Load() != 0 {
		t.Error("nothing may be sent before validation passes")
	}
}

func TestDetectChangingFields(t *testing.T) {
	f := setupCompare(t)
	ctx := context.Background()
	rep, err := f.svc.DetectChangingFields(ctx, "d", service.DetectChangingInput{Request: f.reqs["Health"],
		EnvironmentID: f.oldEnv.ID})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(regression.ChangingPaths(rep.Fields)) != "[checked_at uptime_s]" || rep.Samples != 3 || rep.StatusChanged {
		t.Fatalf("got %+v", rep)
	}
	if f.oldCalls.Load() != 3 {
		t.Fatalf("calls = %d", f.oldCalls.Load())
	}
	// Already ignored patterns are not suggested again.
	rep, err = f.svc.DetectChangingFields(ctx, "d", service.DetectChangingInput{Request: f.reqs["Health"],
		EnvironmentID: f.oldEnv.ID, IgnoredPaths: []string{"**.checked_at"}})
	if err != nil || fmt.Sprint(regression.ChangingPaths(rep.Fields)) != "[uptime_s]" {
		t.Fatalf("got %+v %v", rep, err)
	}
	// Detection is not recorded in history.
	runs, err := f.svc.ListRuns(ctx, domain.RunFilter{})
	if err != nil || len(runs) != 0 {
		t.Fatalf("history must stay empty: %d %v", len(runs), err)
	}
}

func TestDetectChangingFieldsGuards(t *testing.T) {
	f := setupCompare(t)
	ctx := context.Background()
	_, err := f.svc.DetectChangingFields(ctx, "d", service.DetectChangingInput{Request: f.reqs["Create order"],
		EnvironmentID: f.oldEnv.ID})
	var conf *service.ConfirmationRequiredError
	if !asConfirmation(err, &conf) || f.oldCalls.Load() != 0 {
		t.Fatalf("a POST needs confirmation before it is repeated: %v, calls %d", err, f.oldCalls.Load())
	}
	if _, err := f.svc.DetectChangingFields(ctx, "d", service.DetectChangingInput{Request: f.reqs["Create order"],
		EnvironmentID: f.oldEnv.ID, Confirmed: true}); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
	missing := f.reqs["Health"]
	missing.URLTemplate = f.oldBaseURL + "/nope"
	_, err = f.svc.DetectChangingFields(ctx, "d", service.DetectChangingInput{Request: missing, EnvironmentID: f.oldEnv.ID})
	if err == nil {
		t.Fatal("a non-JSON response must be reported")
	}
}
