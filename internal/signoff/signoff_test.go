package signoff

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/rules"
)

var (
	reqA = domain.Request{ID: "a", Name: "Get account", Method: "GET",
		Assertions: []domain.Assertion{{Kind: domain.AssertStatus, Enabled: true}}}
	reqB = domain.Request{ID: "b", Name: "Withdraw", Method: "POST"}
	reqC = domain.Request{ID: "c", Name: "Report", Method: "GET", PostResponseScript: `pm.test("ok", () => {})`}
)

func result(req domain.Request, status domain.ResultStatus) domain.RunResult {
	return domain.RunResult{ID: "res-" + req.ID, RequestID: req.ID, Status: status,
		Request: domain.RequestSummary{Name: req.Name, Method: req.Method}, Response: &domain.ResponseSummary{StatusCode: 200}}
}

func data(run *domain.Run) Data {
	return Data{Release: "v2.3.0", PreparedBy: "Dana", Collection: "Shop", EnvironmentName: "Staging",
		Requests: []domain.Request{reqA, reqB, reqC}, Run: run, Races: map[string]*domain.Run{},
		Baselines: map[string]*domain.Baseline{"a": {Mode: domain.BaselineValues, ApprovedBy: "Dana",
			UpdatedAt: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), IgnoredPaths: []string{"meta.id"}}},
		AppName: "API test kits", AppVersion: "0.1.2", Now: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
}

func passingRun() *domain.Run {
	return &domain.Run{ID: "run1", Status: domain.StatusPassed, PassedCount: 3,
		Results: []domain.RunResult{result(reqA, domain.StatusPassed), result(reqB, domain.StatusPassed), result(reqC, domain.StatusPassed)}}
}

func TestReadyRelease(t *testing.T) {
	r := Build(data(passingRun()))
	if !r.Ready || len(r.Reasons) != 0 {
		t.Fatalf("expected ready, reasons %v", r.Reasons)
	}
	if r.Run.Covered != 3 || r.Run.Requests != 3 || len(r.NotRun) != 0 {
		t.Fatalf("coverage %+v not run %v", r.Run, r.NotRun)
	}
	if strings.Join(r.NoTests, ",") != "Withdraw" || strings.Join(r.NoBaseline, ",") != "Withdraw,Report" {
		t.Fatalf("no tests %v no baseline %v", r.NoTests, r.NoBaseline)
	}
	if b := r.Baselines[0]; !b.Has || b.ApprovedBy != "Dana" || b.Mode != "whole response" || b.Ignored != 1 {
		t.Fatalf("baseline row %+v", b)
	}
}

func TestNotReadyReasons(t *testing.T) {
	run := passingRun()
	run.Status = domain.StatusFailed
	a := result(reqA, domain.StatusFailed)
	a.Regression = &domain.RegressionResult{Passed: false, Message: "1 difference(s) from the baseline",
		Diffs: []domain.DiffEntry{{Path: "total", Kind: domain.DiffChanged, Before: "10", After: "11"}}}
	b := result(reqB, domain.StatusError)
	b.Response, b.Error = nil, "connection refused"
	rulesRes := domain.RunResult{ID: "rules", Status: domain.StatusFailed, Request: domain.RequestSummary{Name: rules.ResultName},
		AssertionResults: []domain.AssertionResult{{Kind: domain.AssertRule, Description: "Balance deducted", Message: "89 ≠ 90"}}}
	run.Results = []domain.RunResult{a, b, rulesRes}

	d := data(run)
	d.Races["b"] = &domain.Run{Kind: domain.RunKindRace, Status: domain.StatusFailed, Results: []domain.RunResult{
		{RequestID: "b", Status: domain.StatusFailed, Request: domain.RequestSummary{Name: RaceVerdictPrefix + ": 10 × Withdraw"},
			AssertionResults: []domain.AssertionResult{{Description: "At most 1 successful", Message: "8 of 10 succeeded"}}}}}
	d.Matrix = &domain.Run{Kind: domain.RunKindMatrix, Status: domain.StatusFailed, Results: []domain.RunResult{
		{Status: domain.StatusFailed, Request: domain.RequestSummary{Name: "[Bob] Get account"},
			AssertionResults: []domain.AssertionResult{{Description: "Access for Bob", Message: "broken access control"}}}}}

	r := Build(d)
	want := []string{"1 request failed", "1 request had an error", "collection rules failed: Balance deducted",
		"1 regression from its baseline", "race test failed: Withdraw", "permission matrix: 1 access mismatch"}
	if r.Ready || strings.Join(r.Reasons, "|") != strings.Join(want, "|") {
		t.Fatalf("reasons\n%v\nwant\n%v", r.Reasons, want)
	}
	if strings.Join(r.NotRun, ",") != "Report" || r.Run.Covered != 2 {
		t.Fatalf("not run %v covered %d", r.NotRun, r.Run.Covered)
	}
	if len(r.Regressions) != 1 || r.Regressions[0].Diffs[0].Path != "total" {
		t.Fatalf("regressions %+v", r.Regressions)
	}
}

func TestNoRunIsNotReady(t *testing.T) {
	r := Build(data(nil))
	if r.Ready || r.Reasons[0] != "there is no collection run in Staging" || len(r.NotRun) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestPerformanceRows(t *testing.T) {
	up, small := 35.0, 5.0
	d := data(passingRun())
	d.Times = []domain.ResponseTimeRow{
		{Name: "Get account", ChangePct: &up},
		{Name: "Withdraw", ChangePct: &small},
		{Name: "Report", ChangePct: &small, LastSlower: true},
	}
	r := Build(d)
	if len(r.Performance) != 2 || r.Performance[0].Name != "Get account" || !r.Performance[1].Slower {
		t.Fatalf("performance %+v", r.Performance)
	}
	if !r.Ready {
		t.Fatal("performance changes are listed, not blocking")
	}
}

func TestHTMLIsEscapedRedactedAndScriptFree(t *testing.T) {
	d := data(passingRun())
	d.Notes = `<script>alert(1)</script> token s3cr3t`
	d.Secrets = []string{"s3cr3t"}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, Build(d)); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, bad := range []string{"<script", "s3cr3t", "http://", "https://"} {
		if strings.Contains(html, bad) {
			t.Errorf("report contains %q", bad)
		}
	}
	for _, want := range []string{"Release sign-off: v2.3.0", "Ready for release", "Dana", "&lt;script&gt;", "Approved by"} {
		if !strings.Contains(html, want) {
			t.Errorf("report lacks %q", want)
		}
	}
}

func TestRecognisesLegacyBurstRuns(t *testing.T) {
	race := domain.Run{Kind: domain.RunKindBurst, Results: []domain.RunResult{{RequestID: "b",
		Request: domain.RequestSummary{Name: RaceVerdictPrefix + ": 5 × Withdraw"}}}}
	if id, ok := IsRaceRun(race); !ok || id != "b" || IsMatrixRun(race) {
		t.Fatal("old race run")
	}
	matrix := domain.Run{Kind: domain.RunKindBurst, Results: []domain.RunResult{{Request: domain.RequestSummary{Name: "[Bob] x"}}}}
	if _, ok := IsRaceRun(matrix); ok || !IsMatrixRun(matrix) {
		t.Fatal("old matrix run")
	}
	if _, ok := IsRaceRun(domain.Run{Kind: domain.RunKindNormal}); ok || IsMatrixRun(domain.Run{Kind: domain.RunKindNormal}) {
		t.Fatal("a normal run is neither")
	}
}
