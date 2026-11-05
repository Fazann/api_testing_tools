package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/security"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

const (
	aliceToken = "alice-secret-token"
	bobToken   = "bob-secret-token"
	adminToken = "admin-secret-token"
)

// racyAPI has deliberate concurrency and access-control bugs, plus correct
// versions of the same endpoints.
type racyAPI struct {
	mu       sync.Mutex
	redeemed bool // racy coupon
	safeUsed bool // locked coupon
	balance  float64
	txSeq    int
}

func newRacyAPI() *httptest.Server {
	api := &racyAPI{balance: 100}
	user := func(r *http.Request) string {
		switch r.Header.Get("Authorization") {
		case "Bearer " + aliceToken:
			return "alice"
		case "Bearer " + bobToken:
			return "bob"
		case "Bearer " + adminToken:
			return "admin"
		}
		return ""
	}
	mux := http.NewServeMux()
	// Check-then-act without a lock: concurrent requests all see "not redeemed".
	mux.HandleFunc("POST /coupons/racy", func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		used := api.redeemed
		api.mu.Unlock()
		if used {
			writeJSON(w, 409, map[string]any{"error": "already redeemed"})
			return
		}
		time.Sleep(30 * time.Millisecond)
		api.mu.Lock()
		api.redeemed = true
		api.txSeq++
		id := api.txSeq
		api.mu.Unlock()
		writeJSON(w, 200, map[string]any{"tx": id})
	})
	mux.HandleFunc("POST /coupons/safe", func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		defer api.mu.Unlock()
		if api.safeUsed {
			writeJSON(w, 409, map[string]any{"error": "already redeemed"})
			return
		}
		api.safeUsed = true
		api.txSeq++
		writeJSON(w, 200, map[string]any{"tx": api.txSeq})
	})
	// Lost update: read, wait, write.
	mux.HandleFunc("POST /wallet/withdraw", func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		b := api.balance
		api.mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		api.mu.Lock()
		api.balance = b - 10
		api.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /wallet", func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		defer api.mu.Unlock()
		writeJSON(w, 200, map[string]any{"data": map[string]any{"balance": api.balance}})
	})
	// IDOR: any signed-in user can read Alice's wallet.
	mux.HandleFunc("GET /users/alice/wallet", func(w http.ResponseWriter, r *http.Request) {
		if user(r) == "" {
			writeJSON(w, 401, map[string]any{"error": "unauthorized"})
			return
		}
		writeJSON(w, 200, map[string]any{"owner": "alice", "balance": 5})
	})
	mux.HandleFunc("GET /admin/users", func(w http.ResponseWriter, r *http.Request) {
		switch user(r) {
		case "":
			writeJSON(w, 401, map[string]any{"error": "unauthorized"})
		case "admin":
			writeJSON(w, 200, []string{"alice", "bob"})
		default:
			writeJSON(w, 403, map[string]any{"error": "forbidden"})
		}
	})
	return httptest.NewServer(mux)
}

type secFixture struct {
	svc  *service.Service
	env  domain.Environment
	coll domain.Collection
	reqs map[string]domain.Request
}

func setupSecurity(t *testing.T) *secFixture {
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
	srv := newRacyAPI()
	t.Cleanup(srv.Close)

	env, err := svc.SaveEnvironment(ctx, domain.Environment{Name: "Local", Variables: []domain.Variable{
		{Key: "base_url", Value: srv.URL, Enabled: true},
		{Key: "token", Value: aliceToken, Secret: true, Enabled: true},
		{Key: "bob_token", Value: bobToken, Secret: true, Enabled: true},
		{Key: "admin_token", Value: adminToken, Secret: true, Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	coll, err := svc.SaveCollection(ctx, domain.Collection{Name: "Security",
		Auth: domain.Auth{Type: domain.AuthBearer, Token: "{{token}}"}})
	if err != nil {
		t.Fatal(err)
	}
	f := &secFixture{svc: svc, env: env, coll: coll, reqs: map[string]domain.Request{}}
	for i, r := range []struct{ name, method, path string }{
		{"Racy coupon", "POST", "/coupons/racy"},
		{"Safe coupon", "POST", "/coupons/safe"},
		{"Withdraw", "POST", "/wallet/withdraw"},
		{"Get wallet", "GET", "/wallet"},
		{"Alice wallet", "GET", "/users/alice/wallet"},
		{"List users", "GET", "/admin/users"},
	} {
		saved, err := svc.SaveRequest(ctx, domain.Request{CollectionID: coll.ID, Name: r.name, Method: r.method,
			URLTemplate: "{{base_url}}" + r.path, SortOrder: i, Auth: domain.Auth{Type: domain.AuthInherit}})
		if err != nil {
			t.Fatal(err)
		}
		f.reqs[r.name] = saved
	}
	return f
}

func TestRaceTestDetectsDoubleRedeem(t *testing.T) {
	f := setupSecurity(t)
	ctx := context.Background()
	rep, err := f.svc.RunRaceTest(ctx, "race", domain.RaceTest{RequestID: f.reqs["Racy coupon"].ID, Count: 10,
		SuccessOp: domain.OpLte, SuccessCount: 1, UniquePath: "tx"}, f.env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed || rep.Successes < 2 {
		t.Fatalf("the racy coupon should be redeemed more than once, got %d successes, passed=%v", rep.Successes, rep.Passed)
	}
	run, err := f.svc.GetRun(ctx, rep.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Results) != 11 || run.Status != domain.StatusFailed {
		t.Fatalf("expected 10 results plus a failed verdict, got %d results, status %s", len(run.Results), run.Status)
	}
	if !strings.HasPrefix(run.Results[10].Request.Name, "Race test verdict") {
		t.Fatalf("last result should be the verdict, got %q", run.Results[10].Request.Name)
	}
	saved, err := f.svc.GetRaceTest(ctx, f.reqs["Racy coupon"].ID)
	if err != nil || saved == nil || saved.Count != 10 {
		t.Fatalf("race test settings were not saved: %v %+v", err, saved)
	}
}

func TestRaceTestPassesForLockedEndpoint(t *testing.T) {
	f := setupSecurity(t)
	rep, err := f.svc.RunRaceTest(context.Background(), "race", domain.RaceTest{RequestID: f.reqs["Safe coupon"].ID, Count: 10,
		SuccessOp: domain.OpEq, SuccessCount: 1, UniquePath: "tx"}, f.env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Passed || rep.Successes != 1 {
		t.Fatalf("expected exactly one success and a pass, got %d, checks %+v", rep.Successes, rep.Checks)
	}
	// Bearer tokens never appear in the report.
	data, _ := json.Marshal(rep)
	if strings.Contains(string(data), aliceToken) {
		t.Fatal("report leaked the token")
	}
}

func TestRaceTestDetectsLostUpdate(t *testing.T) {
	f := setupSecurity(t)
	rep, err := f.svc.RunRaceTest(context.Background(), "race", domain.RaceTest{RequestID: f.reqs["Withdraw"].ID, Count: 5,
		Check: &domain.RaceCheck{RequestID: f.reqs["Get wallet"].ID, Path: "data.balance", DeltaPerSuccess: -10}}, f.env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Before == nil || rep.After == nil || *rep.Before != 100 {
		t.Fatalf("state check did not read the balance: %+v %+v", rep.Before, rep.After)
	}
	if rep.Passed || rep.Successes != 5 || *rep.After == 50 {
		t.Fatalf("expected a lost update (5 successes but balance not 50), got after=%v passed=%v", *rep.After, rep.Passed)
	}
}

func TestRaceTestRequiresConfirmationOnProtectedEnv(t *testing.T) {
	f := setupSecurity(t)
	ctx := context.Background()
	f.env.Protected = true
	if _, err := f.svc.SaveEnvironment(ctx, f.env); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.RunRaceTest(ctx, "race", domain.RaceTest{RequestID: f.reqs["Safe coupon"].ID, Count: 3}, f.env.ID, false)
	var conf *service.ConfirmationRequiredError
	if err == nil || !asConfirmation(err, &conf) {
		t.Fatalf("expected a confirmation error, got %v", err)
	}
}

func asConfirmation(err error, target **service.ConfirmationRequiredError) bool {
	c, ok := err.(*service.ConfirmationRequiredError)
	if ok {
		*target = c
	}
	return ok
}

func TestPermissionMatrixFindsIDOR(t *testing.T) {
	f := setupSecurity(t)
	ctx := context.Background()
	alice, users := f.reqs["Alice wallet"].ID, f.reqs["List users"].ID
	m, err := f.svc.SavePermissionMatrix(ctx, domain.PermissionMatrix{
		CollectionID: f.coll.ID,
		Roles: []domain.Role{
			{ID: "alice", Name: "Alice"},
			{ID: "bob", Name: "Bob", Variables: []domain.Variable{{Key: "token", Value: "{{bob_token}}", Enabled: true}}},
			{ID: "admin", Name: "Admin", Variables: []domain.Variable{{Key: "token", Value: "{{admin_token}}", Enabled: true}}},
			{ID: "anon", Name: "Anonymous", Anonymous: true},
		},
		RequestIDs: []string{alice, users},
		Expected: map[string]map[string]domain.Access{
			alice: {"alice": domain.AccessAllow, "bob": domain.AccessDeny, "anon": domain.AccessDeny},
			users: {"alice": domain.AccessDeny, "bob": domain.AccessDeny, "admin": domain.AccessAllow, "anon": domain.AccessDeny},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Roles) != 4 {
		t.Fatalf("roles not saved: %+v", m.Roles)
	}
	var progress int
	rep, err := f.svc.RunPermissionMatrix(ctx, "matrix", f.coll.ID, f.env.ID, false, func(done, total int, _ domain.RunResult) {
		progress = done
		if total != 8 {
			t.Errorf("total = %d, want 8", total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if progress != 8 || len(rep.Cells) != 8 {
		t.Fatalf("expected 8 cells, got %d (progress %d)", len(rep.Cells), progress)
	}
	cell := func(req, role string) security.MatrixCell {
		for _, c := range rep.Cells {
			if c.RequestID == req && c.RoleID == role {
				return c
			}
		}
		t.Fatalf("no cell %s/%s", req, role)
		return security.MatrixCell{}
	}
	if c := cell(alice, "bob"); c.Verdict != security.VerdictFail || c.StatusCode != 200 {
		t.Fatalf("Bob reading Alice's wallet should fail the matrix: %+v", c)
	}
	if c := cell(alice, "admin"); c.Verdict != security.VerdictUnset || c.Suggested != domain.AccessAllow {
		t.Fatalf("admin cell has no expectation: %+v", c)
	}
	if c := cell(users, "anon"); c.Verdict != security.VerdictPass || c.StatusCode != 401 {
		t.Fatalf("anonymous must be sent without the token: %+v", c)
	}
	if c := cell(users, "admin"); c.Verdict != security.VerdictPass {
		t.Fatalf("admin role variables were not applied: %+v", c)
	}
	if rep.Passed || rep.Failures != 1 || rep.Passes != 6 || rep.Unset != 1 {
		t.Fatalf("unexpected tally: %+v", rep)
	}
	run, err := f.svc.GetRun(ctx, rep.RunID)
	if err != nil || run.Status != domain.StatusFailed || len(run.Results) != 8 {
		t.Fatalf("run not stored as failed: %v %s %d", err, run.Status, len(run.Results))
	}
	data, _ := json.Marshal(run)
	for _, tok := range []string{aliceToken, bobToken, adminToken} {
		if strings.Contains(string(data), tok) {
			t.Fatalf("history leaked %q", tok)
		}
	}
}

func TestPermissionMatrixNeedsRolesAndRequests(t *testing.T) {
	f := setupSecurity(t)
	ctx := context.Background()
	if _, err := f.svc.RunPermissionMatrix(ctx, "m", f.coll.ID, f.env.ID, false, nil); err == nil {
		t.Fatal("an empty matrix must not run")
	}
	if _, err := f.svc.SavePermissionMatrix(ctx, domain.PermissionMatrix{CollectionID: f.coll.ID,
		Roles: []domain.Role{{ID: "a", Name: "Same"}, {ID: "b", Name: "same"}}}); err == nil {
		t.Fatal("duplicate role names must be rejected")
	}
}
