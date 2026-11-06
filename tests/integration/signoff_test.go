package integration

import (
	"context"
	"strings"
	"testing"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/service"
)

func TestSignoffReport(t *testing.T) {
	f := setupRules(t, false)
	ctx := context.Background()
	build := func() service.SignoffResult {
		t.Helper()
		res, err := f.svc.BuildSignoffReport(ctx, service.SignoffInput{CollectionID: f.coll.ID, EnvironmentID: f.env.ID,
			Release: "v1.0.0", PreparedBy: "Dana (QA)", Notes: "Smoke-tested on Staging"})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// Nothing has run yet.
	if res := build(); res.Ready || len(res.Reasons) != 1 || !strings.Contains(res.Reasons[0], "no collection run") {
		t.Fatalf("before any run: %+v", res.Reasons)
	}

	// Approve the first balance, by name, then run: ready.
	before, err := f.svc.SendRequest(ctx, "s", domain.Request{ID: f.ids["Balance before"], CollectionID: f.coll.ID,
		Name: "Balance before", Method: "GET", URLTemplate: "{{base_url}}/wallet"}, f.env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ApproveBaseline(ctx, service.ApproveBaselineInput{RequestID: f.ids["Balance before"],
		EnvironmentID: f.env.ID, Body: before.Response.Body, ApprovedBy: "  Dana (QA) "}); err != nil {
		t.Fatal(err)
	}
	if b, _ := f.svc.GetBaseline(ctx, f.ids["Balance before"], f.env.ID); b == nil || b.ApprovedBy != "Dana (QA)" {
		t.Fatalf("approver not stored: %+v", b)
	}
	if _, err := f.svc.RunCollection(ctx, "r1", f.coll.ID, "", f.env.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	res := build()
	if !res.Ready {
		t.Fatalf("first run should be ready: %v", res.Reasons)
	}
	for _, want := range []string{"Ready for release", "Release sign-off: v1.0.0", "Dana (QA)", "Smoke-tested on Staging",
		"covered 5 of 5 requests", "Balance after"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	if !strings.HasPrefix(res.FileName, "signoff-Wallet-flow-v1.0.0-") || !strings.HasSuffix(res.FileName, ".html") {
		t.Errorf("file name %q", res.FileName)
	}

	// The balance moved on, so the next run regresses; a failing race test also blocks.
	if _, err := f.svc.RunCollection(ctx, "r2", f.coll.ID, "", f.env.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RunRaceTest(ctx, "race", domain.RaceTest{RequestID: f.ids["Withdraw"], Count: 3,
		SuccessOp: domain.OpLte, SuccessCount: 1}, f.env.ID, true); err != nil {
		t.Fatal(err)
	}
	res = build()
	want := []string{"1 request failed", "1 regression from its baseline", "race test failed: Withdraw"}
	if res.Ready || strings.Join(res.Reasons, "|") != strings.Join(want, "|") {
		t.Fatalf("reasons %v, want %v", res.Reasons, want)
	}
	if !strings.Contains(res.HTML, "Not ready") || !strings.Contains(res.HTML, "Race test — Withdraw") {
		t.Error("report body does not show the failures")
	}

	// The race test is stored with its own kind, so it never counts as the collection run.
	runs, _ := f.svc.ListRuns(ctx, domain.RunFilter{CollectionID: f.coll.ID})
	if runs[0].Kind != domain.RunKindRace {
		t.Errorf("newest run kind %q", runs[0].Kind)
	}

	if _, err := f.svc.BuildSignoffReport(ctx, service.SignoffInput{CollectionID: f.coll.ID, Release: "  "}); err == nil {
		t.Error("a release name is required")
	}
}
