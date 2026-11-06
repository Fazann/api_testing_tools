package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/rules"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

// walletAPI is a wallet with orders: withdrawals deduct the amount sent
// (or, with skimming, one unit more) and created orders appear in the list.
func walletAPI(t *testing.T, skim bool) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	balance := 100.0
	orders := []map[string]any{{"id": "ord_1"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/wallet":
			writeJSON(w, 200, map[string]any{"data": map[string]any{"balance": balance}})
		case r.Method == http.MethodPost && r.URL.Path == "/wallet/withdraw":
			var body struct{ Amount float64 }
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			balance -= body.Amount
			if skim {
				balance--
			}
			writeJSON(w, 200, map[string]any{"ok": true})
		case r.Method == http.MethodPost && r.URL.Path == "/orders":
			id := "ord_" + string(rune('1'+len(orders)))
			orders = append(orders, map[string]any{"id": id})
			writeJSON(w, 201, map[string]any{"id": id})
		case r.Method == http.MethodGet && r.URL.Path == "/orders":
			writeJSON(w, 200, map[string]any{"orders": orders})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type rulesFixture struct {
	svc    *service.Service
	coll   domain.Collection
	env    domain.Environment
	folder domain.Folder
	ids    map[string]string
}

func setupRules(t *testing.T, skim bool) *rulesFixture {
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
	srv := walletAPI(t, skim)
	f := &rulesFixture{svc: svc, ids: map[string]string{}}
	if f.env, err = svc.SaveEnvironment(ctx, domain.Environment{Name: "Local",
		Variables: []domain.Variable{{Key: "base_url", Value: srv.URL, Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	if f.coll, err = svc.SaveCollection(ctx, domain.Collection{Name: "Wallet flow"}); err != nil {
		t.Fatal(err)
	}
	if f.folder, err = svc.SaveFolder(ctx, domain.Folder{CollectionID: f.coll.ID, Name: "Orders", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	for i, r := range []struct{ name, method, path, body, folder string }{
		{"Balance before", "GET", "/wallet", "", ""},
		{"Withdraw", "POST", "/wallet/withdraw", `{"amount": 10}`, ""},
		{"Balance after", "GET", "/wallet", "", ""},
		{"Create order", "POST", "/orders", `{}`, f.folder.ID},
		{"List orders", "GET", "/orders", "", f.folder.ID},
	} {
		req := domain.Request{CollectionID: f.coll.ID, ParentID: r.folder, Name: r.name, Method: r.method,
			URLTemplate: "{{base_url}}" + r.path, SortOrder: i, Body: r.body}
		if r.body != "" {
			req.BodyType = domain.BodyJSON
		}
		saved, err := svc.SaveRequest(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		f.ids[r.name] = saved.ID
	}
	val := func(src domain.RuleSource, name, path string) domain.RuleValue {
		return domain.RuleValue{Source: src, RequestID: f.ids[name], Path: path}
	}
	list := []domain.Rule{
		{ID: "deducted", Name: "Withdrawal is deducted", Enabled: true, Kind: domain.RuleCompare, Operator: domain.RuleEq,
			Left: val(domain.SourceResponse, "Balance after", "data.balance"),
			Right: []domain.RuleTerm{{Sign: "+", Value: val(domain.SourceResponse, "Balance before", "data.balance")},
				{Sign: "-", Value: val(domain.SourceRequest, "Withdraw", "amount")}}},
		{ID: "listed", Name: "Created order is listed", Enabled: true, Kind: domain.RuleContains, Operator: domain.RuleIn,
			Left:  val(domain.SourceResponse, "Create order", "id"),
			Right: []domain.RuleTerm{{Sign: "+", Value: val(domain.SourceResponse, "List orders", "orders[*].id")}}},
	}
	if _, err := svc.SaveCollectionRules(ctx, f.coll.ID, list); err != nil {
		t.Fatal(err)
	}
	return f
}

func rulesResult(t *testing.T, run domain.Run) *domain.RunResult {
	t.Helper()
	for i, r := range run.Results {
		if r.Request.Name == rules.ResultName {
			return &run.Results[i]
		}
	}
	return nil
}

func TestCollectionRunEvaluatesRules(t *testing.T) {
	f := setupRules(t, false)
	ctx := context.Background()
	run, err := f.svc.RunCollection(ctx, "run", f.coll.ID, "", f.env.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := rulesResult(t, run)
	if res == nil || res.Status != domain.StatusPassed || len(res.AssertionResults) != 2 {
		t.Fatalf("rules result %+v", res)
	}
	if msg := res.AssertionResults[0].Message; msg != "90 = 100 − 10" {
		t.Errorf("message %q", msg)
	}
	if run.Status != domain.StatusPassed || len(run.Results) != 6 {
		t.Errorf("run %s with %d results", run.Status, len(run.Results))
	}
	// The rules result is stored in history with the run.
	stored, err := f.svc.GetRun(ctx, run.ID)
	if err != nil || rulesResult(t, stored) == nil {
		t.Fatalf("rules result not stored: %v", err)
	}

	// Checking edited rules against that run sends nothing and reports each rule.
	tree, _ := f.svc.GetCollectionTree(ctx, f.coll.ID)
	edited := tree.Collection.Rules
	edited[0].Right[1] = domain.RuleTerm{Sign: "-", Value: domain.RuleValue{Source: domain.SourceConstant, Literal: "20"}}
	edited[1].Enabled = false
	check, err := f.svc.CheckCollectionRules(ctx, f.coll.ID, edited)
	if err != nil {
		t.Fatal(err)
	}
	if check.RunID != run.ID || len(check.Outcomes) != 2 || check.Outcomes[0].Result.Passed ||
		check.Outcomes[0].Result.Message != "90 ≠ 100 − 20 = 80" || !check.Outcomes[1].Skipped {
		t.Fatalf("check %+v", check)
	}
}

func TestBrokenBusinessLogicFailsTheRun(t *testing.T) {
	f := setupRules(t, true)
	run, err := f.svc.RunCollection(context.Background(), "run", f.coll.ID, "", f.env.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := rulesResult(t, run)
	if res == nil || res.Status != domain.StatusFailed || run.Status != domain.StatusFailed {
		t.Fatalf("a skimmed withdrawal must fail the run: %+v / %s", res, run.Status)
	}
	if msg := res.AssertionResults[0].Message; msg != "89 ≠ 100 − 10 = 90" {
		t.Errorf("message %q", msg)
	}
	if !res.AssertionResults[1].Passed {
		t.Errorf("the order rule still holds: %+v", res.AssertionResults[1])
	}
}

func TestFolderRunOnlyEvaluatesRulesInside(t *testing.T) {
	f := setupRules(t, false)
	ctx := context.Background()
	run, err := f.svc.RunCollection(ctx, "run", f.coll.ID, f.folder.ID, f.env.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := rulesResult(t, run)
	if res == nil || len(res.AssertionResults) != 1 || res.AssertionResults[0].Description != "Created order is listed" {
		t.Fatalf("only the rule whose requests ran: %+v", res)
	}
	// A single send runs no rules.
	sent, err := f.svc.SendRequest(ctx, "send", domain.Request{ID: f.ids["Balance after"], CollectionID: f.coll.ID,
		Name: "Balance after", Method: "GET", URLTemplate: "{{base_url}}/wallet"}, f.env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := f.svc.GetRun(ctx, sent.RunID)
	if rulesResult(t, stored) != nil {
		t.Fatal("Send must not evaluate rules")
	}
}

func TestRulesTravelWithExportToTheCLI(t *testing.T) {
	f := setupRules(t, true)
	data, err := f.svc.ExportCollection(context.Background(), f.coll.ID, []string{f.env.ID}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"rules"`) {
		t.Fatal("the export has no rules")
	}
	path := filepath.Join(t.TempDir(), "wallet.collection.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	// Import regenerates request IDs; the rule must still find its requests and fail on the skim.
	got := runCLI(t, nil, "run", path, "--env", "Local")
	if got.code != 1 || !strings.Contains(got.stdout, "Collection rules ") ||
		!strings.Contains(got.stdout, "Withdrawal is deducted — 89 ≠ 100 − 10 = 90") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
	}
}
