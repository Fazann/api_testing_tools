package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"api-test-kits/internal/domain"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrationsAreIdempotentAndFilePrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "data.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	// New collections default to no auth through the migration's column default.
	c, err := s.SaveCollection(context.Background(), domain.Collection{Name: "C"})
	if err != nil || c.Auth.Type != domain.AuthNone {
		t.Fatalf("default collection auth: %v %+v", err, c.Auth)
	}
	s.Close()
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(path)
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("database file mode %v, want 0600", st.Mode().Perm())
		}
	}
}

func TestCollectionRequestLifecycle(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	c, err := s.SaveCollection(ctx, domain.Collection{Name: "Wallet", RequiredVariables: []string{"base_url"},
		Variables: []domain.Variable{{Key: "base_url", Value: "http://x", Enabled: true}},
		Auth:      domain.Auth{Type: domain.AuthBearer, Token: "{{token}}"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetCollection(ctx, c.ID); got.Auth.Type != domain.AuthBearer || got.Auth.Token != "{{token}}" ||
		len(got.Variables) != 1 || got.Variables[0].Value != "http://x" {
		t.Fatalf("collection auth or variables not persisted: %+v", got)
	}
	f, err := s.SaveFolder(ctx, domain.Folder{CollectionID: c.ID, Name: "Balances"})
	if err != nil {
		t.Fatal(err)
	}
	req := domain.Request{
		CollectionID: c.ID, ParentID: f.ID, Name: "Get balance", Method: "get", URLTemplate: "{{base_url}}/balance",
		Headers: []domain.KeyValue{{Key: "Accept", Value: "application/json", Enabled: true}},
		Auth:    domain.Auth{Type: domain.AuthBearer, Token: "{{token}}"},
		Assertions: []domain.Assertion{
			{Kind: domain.AssertStatus, Operator: domain.OpEq, Expected: json.RawMessage(`200`), Enabled: true},
			{Kind: domain.AssertAggregate, Path: "items[*].amount", Function: "sum", Operator: domain.OpEq, Expected: json.RawMessage(`{"$path":"total"}`), Tolerance: 0.01, Enabled: true},
		},
	}
	saved, err := s.SaveRequest(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Method != "GET" || len(saved.Assertions) != 2 || saved.Assertions[1].Function != "sum" || saved.Assertions[1].Tolerance != 0.01 {
		t.Fatalf("round trip lost data: %+v", saved)
	}

	other, _ := s.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "Own auth", Method: "GET", URLTemplate: "http://x"})
	if n, err := s.InheritCollectionAuth(ctx, c.ID); err != nil || n != 2 {
		t.Fatalf("inherit collection auth: %d %v", n, err)
	}
	if got, _ := s.GetRequest(ctx, other.ID); got.Auth.Type != domain.AuthInherit {
		t.Fatalf("request should now inherit: %+v", got.Auth)
	}
	if err := s.DeleteRequest(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	saved, _ = s.GetRequest(ctx, saved.ID)

	saved.Assertions = saved.Assertions[:1]
	saved.Name = "Renamed"
	if saved, err = s.SaveRequest(ctx, saved); err != nil || saved.Name != "Renamed" || len(saved.Assertions) != 1 {
		t.Fatalf("update failed: %v %+v", err, saved)
	}

	dup, err := s.DuplicateRequest(ctx, saved.ID)
	if err != nil || dup.ID == saved.ID || dup.Name != "Renamed (copy)" || dup.Assertions[0].ID == saved.Assertions[0].ID {
		t.Fatalf("duplicate: %v %+v", err, dup)
	}

	copyC, err := s.DuplicateCollection(ctx, c.ID, "Wallet 2")
	if err != nil {
		t.Fatal(err)
	}
	tree, _ := s.GetTree(ctx, copyC.ID)
	if len(tree.Folders) != 1 || len(tree.Requests) != 2 || tree.Requests[0].ParentID != tree.Folders[0].ID {
		t.Fatalf("duplicated tree wrong: %+v", tree)
	}

	// Deleting a folder deletes its requests; deleting a collection cascades.
	if err := s.DeleteFolder(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRequest(ctx, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("request should be gone with its folder: %v", err)
	}
	if err := s.DeleteCollection(ctx, copyC.ID); err != nil {
		t.Fatal(err)
	}
	info, _ := s.DataInfo(ctx)
	if info.Collections != 1 || info.Requests != 0 {
		t.Fatalf("unexpected counts after deletes: %+v", info)
	}
}

func TestFolderCycleRejected(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	c, _ := s.SaveCollection(ctx, domain.Collection{Name: "C"})
	a, _ := s.SaveFolder(ctx, domain.Folder{CollectionID: c.ID, Name: "A"})
	b, _ := s.SaveFolder(ctx, domain.Folder{CollectionID: c.ID, ParentID: a.ID, Name: "B"})
	a.ParentID = b.ID
	if _, err := s.SaveFolder(ctx, a); err == nil {
		t.Fatal("moving a folder into its own child must fail")
	}
}

func TestValidationRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	if _, err := s.SaveCollection(ctx, domain.Collection{Name: "  "}); err == nil {
		t.Fatal("empty name must fail")
	}
	c, _ := s.SaveCollection(ctx, domain.Collection{Name: "C"})
	if _, err := s.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "x", Method: "TRACE", URLTemplate: "http://x"}); err == nil {
		t.Fatal("unsupported method must fail")
	}
	if _, err := s.SaveEnvironment(ctx, domain.Environment{Name: "E", Variables: []domain.Variable{{Key: "a b", Value: "1"}}}); err == nil {
		t.Fatal("invalid variable name must fail")
	}
}

func TestBaselineUpsertAndRunHistory(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	c, _ := s.SaveCollection(ctx, domain.Collection{Name: "C"})
	r, _ := s.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "R", Method: "GET", URLTemplate: "http://x"})
	env, _ := s.SaveEnvironment(ctx, domain.Environment{Name: "Local"})

	b1, err := s.SaveBaseline(ctx, domain.Baseline{RequestID: r.ID, EnvironmentID: env.ID, Snapshot: json.RawMessage(`{"a":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := s.SaveBaseline(ctx, domain.Baseline{RequestID: r.ID, EnvironmentID: env.ID, Snapshot: json.RawMessage(`{"a":2}`), IgnoredPaths: []string{"ts"}})
	if b1.ID != b2.ID {
		t.Fatal("approving again must replace the baseline for the same request and environment")
	}
	got, _ := s.GetBaseline(ctx, r.ID, env.ID)
	if string(got.Snapshot) != `{"a":2}` || got.IgnoredPaths[0] != "ts" || got.Mode != domain.BaselineValues {
		t.Fatalf("got %+v", got)
	}
	if none, _ := s.GetBaseline(ctx, r.ID, ""); none != nil {
		t.Fatal("baselines are per environment")
	}

	for i := 0; i < 5; i++ {
		run := domain.Run{ID: "run" + string(rune('a'+i)), CollectionID: c.ID, CollectionName: "C", EnvironmentName: "Local",
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			Results: []domain.RunResult{{ID: "res" + string(rune('a'+i)), RequestID: r.ID, Status: domain.StatusPassed,
				Response: &domain.ResponseSummary{StatusCode: 200, Body: "0123456789"}, CreatedAt: time.Now()}}}
		run.Tally()
		if err := s.SaveRun(ctx, run, 4, 3); err != nil {
			t.Fatal(err)
		}
	}
	runs, _ := s.ListRuns(ctx, domain.RunFilter{CollectionID: c.ID})
	if len(runs) != 3 || runs[0].ID != "rune" {
		t.Fatalf("retention should keep the 3 newest runs, got %+v", runs)
	}
	full, _ := s.GetRun(ctx, "rune")
	if full.Results[0].Response.Body != "0123" || !full.Results[0].Response.Truncated {
		t.Fatalf("stored body should be truncated to the history limit: %+v", full.Results[0].Response)
	}
	byReq, _ := s.ListRuns(ctx, domain.RunFilter{RequestID: r.ID})
	if len(byReq) != 3 {
		t.Fatalf("filter by request: %d", len(byReq))
	}
	prev, _ := s.PreviousResult(ctx, r.ID, "rune")
	if prev == nil || prev.RunID != "rund" {
		t.Fatalf("previous result: %+v", prev)
	}

	// History survives deletion of the request it came from.
	if err := s.DeleteRequest(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	full, err = s.GetRun(ctx, "rune")
	if err != nil || full.Results[0].RequestID != "" {
		t.Fatalf("history should remain with request link cleared: %v %+v", err, full.Results)
	}
	n, _ := s.ClearHistory(ctx, nil)
	if n != 3 {
		t.Fatalf("cleared %d runs", n)
	}
}

func TestSettingsDefaultsAndDeleteAll(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	st, _ := s.GetSettings(ctx)
	if st != domain.DefaultSettings() {
		t.Fatal("expected defaults")
	}
	st.Concurrency = 4
	if err := s.SaveSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	st.Concurrency = 99
	if err := s.SaveSettings(ctx, st); err == nil {
		t.Fatal("invalid settings must be rejected")
	}
	got, _ := s.GetSettings(ctx)
	if got.Concurrency != 4 {
		t.Fatalf("got %+v", got)
	}
	_, _ = s.SaveCollection(ctx, domain.Collection{Name: "C"})
	if err := s.DeleteAllData(ctx); err != nil {
		t.Fatal(err)
	}
	info, _ := s.DataInfo(ctx)
	if info.Collections != 0 {
		t.Fatal("delete all data left records behind")
	}
}

func TestDamagedJSONColumnsDoNotCrash(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	c, _ := s.SaveCollection(ctx, domain.Collection{Name: "C"})
	r, _ := s.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "R", Method: "GET", URLTemplate: "http://x"})
	if _, err := s.db.Exec(`UPDATE requests SET headers_json = '{broken', auth_json = 'nope' WHERE id = ?`, r.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRequest(ctx, r.ID)
	if err != nil || got.Auth.Type != domain.AuthNone {
		t.Fatalf("damaged row should load with defaults: %v %+v", err, got)
	}
}

func TestRenameCollectionKeepsOtherFields(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	c, err := s.SaveCollection(ctx, domain.Collection{Name: "Old", Description: "d",
		Variables: []domain.Variable{{Key: "token", Value: "abc"}}, PreRequestScript: "pm.test('x')"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.RenameCollection(ctx, c.ID, "New")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "New" || got.Description != "d" || len(got.Variables) != 1 || got.Variables[0].Value != "abc" ||
		got.PreRequestScript != "pm.test('x')" {
		t.Fatalf("rename changed more than the name: %+v", got)
	}
	if _, err := s.RenameCollection(ctx, c.ID, "  "); err == nil {
		t.Fatal("empty name must fail")
	}
	if _, err := s.RenameCollection(ctx, "missing", "X"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: got %v, want ErrNotFound", err)
	}
}

func TestResponseTimesFilterKindEnvironmentAndLegacyRuns(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	c, _ := s.SaveCollection(ctx, domain.Collection{Name: "C"})
	r, _ := s.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "R", Method: "GET", URLTemplate: "http://x"})
	env, _ := s.SaveEnvironment(ctx, domain.Environment{Name: "Local"})
	at := time.Now().UTC().Add(-time.Hour)
	save := func(id, kind, envID, envName string, ms int64, status domain.ResultStatus, withResponse bool) {
		t.Helper()
		at = at.Add(time.Minute)
		res := domain.RunResult{ID: id + "-r", RequestID: r.ID, DurationMs: ms, Status: status, CreatedAt: at}
		if withResponse {
			res.Response = &domain.ResponseSummary{StatusCode: 200}
		}
		run := domain.Run{ID: id, Kind: kind, EnvironmentID: envID, EnvironmentName: envName, StartedAt: at,
			Status: status, Results: []domain.RunResult{res}}
		if err := s.SaveRun(ctx, run, -1, 0); err != nil {
			t.Fatal(err)
		}
	}
	save("legacy", domain.RunKindNormal, "", "Local", 10, domain.StatusPassed, true)
	if _, err := s.db.ExecContext(ctx, `UPDATE runs SET environment_id = NULL WHERE id = 'legacy'`); err != nil {
		t.Fatal(err)
	}
	save("a", domain.RunKindNormal, env.ID, "Local", 20, domain.StatusFailed, true)
	save("burst", domain.RunKindBurst, env.ID, "Local", 999, domain.StatusPassed, true)
	save("error", domain.RunKindNormal, env.ID, "Local", 999, domain.StatusError, false)
	save("other-env", domain.RunKindNormal, "someone-else", "Local", 999, domain.StatusPassed, true)
	save("no-env", domain.RunKindNormal, "", "", 30, domain.StatusPassed, true)
	save("b", domain.RunKindNormal, env.ID, "Local", 40, domain.StatusPassed, true)

	pts, err := s.ResponseTimes(ctx, r.ID, env.ID, env.Name, 10)
	if err != nil {
		t.Fatal(err)
	}
	var got []int64
	for _, p := range pts {
		got = append(got, p.DurationMs)
	}
	if fmt.Sprint(got) != "[10 20 40]" || pts[0].StatusCode != 200 {
		t.Fatalf("want legacy, a, b oldest first, got %v", got)
	}
	if pts, _ := s.ResponseTimes(ctx, r.ID, env.ID, env.Name, 2); len(pts) != 2 || pts[1].DurationMs != 40 {
		t.Fatalf("limit keeps the latest: %+v", pts)
	}
	if pts, _ := s.ResponseTimes(ctx, r.ID, "", "", 10); len(pts) != 1 || pts[0].DurationMs != 30 {
		t.Fatalf("no environment: %+v", pts)
	}
	byReq, err := s.CollectionResponseTimes(ctx, c.ID, env.ID, env.Name, 2)
	if err != nil || len(byReq[r.ID]) != 2 || byReq[r.ID][0].DurationMs != 20 {
		t.Fatalf("collection: %+v %v", byReq, err)
	}

	if _, err := s.SavePerformanceCheck(ctx, domain.PerformanceCheck{RequestID: r.ID, Mode: "loud"}); err == nil {
		t.Fatal("unknown mode must fail")
	}
	if _, err := s.SavePerformanceCheck(ctx, domain.PerformanceCheck{RequestID: "missing"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown request: %v", err)
	}
	saved, err := s.SavePerformanceCheck(ctx, domain.PerformanceCheck{RequestID: r.ID, Mode: domain.PerformanceFail})
	if err != nil || saved.Factor != 1.5 || saved.Window != 20 {
		t.Fatalf("defaults fill zero fields: %+v %v", saved, err)
	}
	if got, _ := s.GetPerformanceCheck(ctx, r.ID); got.Mode != domain.PerformanceFail {
		t.Fatalf("round trip: %+v", got)
	}
	if err := s.DeleteRequest(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetPerformanceCheck(ctx, r.ID); got.Mode != domain.PerformanceWarn {
		t.Fatal("the check is deleted with its request")
	}
}

// Migration 0007 marks race tests and permission matrices saved before it
// (runs that sent one request more than once) so trends leave them out.
func TestMigration0007MarksOldBurstRuns(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	c, _ := s.SaveCollection(ctx, domain.Collection{Name: "C"})
	r, _ := s.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "R", Method: "GET", URLTemplate: "http://x"})
	now := time.Now().UTC()
	res := func(id string) domain.RunResult {
		return domain.RunResult{ID: id, RequestID: r.ID, Status: domain.StatusPassed, CreatedAt: now}
	}
	for _, run := range []domain.Run{
		{ID: "normal", StartedAt: now, Results: []domain.RunResult{res("n1")}},
		{ID: "race", StartedAt: now, Results: []domain.RunResult{res("r1"), res("r2"), res("r3")}},
	} {
		if err := s.SaveRun(ctx, run, -1, 0); err != nil {
			t.Fatal(err)
		}
	}
	// Roll the schema back to before 0007, keeping the rows, and migrate again.
	for _, q := range []string{
		`ALTER TABLE runs DROP COLUMN environment_id`, `ALTER TABLE runs DROP COLUMN kind`,
		`ALTER TABLE run_results DROP COLUMN performance_json`, `DROP TABLE performance_checks`,
		`DELETE FROM schema_migrations WHERE version = 7`,
	} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := s.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"normal": domain.RunKindNormal, "race": domain.RunKindBurst} {
		run, err := s.GetRun(ctx, id)
		if err != nil || run.Kind != want || run.EnvironmentID != "" {
			t.Fatalf("%s: kind %q env %q, want %q (%v)", id, run.Kind, run.EnvironmentID, want, err)
		}
	}
}

func TestListRunsIncludesKind(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for _, r := range []domain.Run{
		{ID: "normal", Kind: domain.RunKindNormal, StartedAt: time.Now().Add(-time.Minute)},
		{ID: "burst", Kind: domain.RunKindBurst, StartedAt: time.Now()},
	} {
		if err := s.SaveRun(ctx, r, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := s.ListRuns(ctx, domain.RunFilter{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range runs {
		got[r.ID] = r.Kind
	}
	if got["normal"] != domain.RunKindNormal || got["burst"] != domain.RunKindBurst {
		t.Fatalf("run kinds in the list: %v", got)
	}
}
