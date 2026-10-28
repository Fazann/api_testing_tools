package reports

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
)

func sampleRun() domain.Run {
	return domain.Run{
		ID: "r1", CollectionName: "Wallet", EnvironmentName: "Staging", Status: domain.StatusFailed,
		StartedAt: time.Now(), PassedCount: 0, FailedCount: 1, DurationMs: 42,
		Results: []domain.RunResult{{
			ID: "x", Status: domain.StatusFailed, DurationMs: 42,
			Request: domain.RequestSummary{Name: "Balance <script>", Method: "GET", URL: "https://api.test/b?key=SUPERSECRET",
				Headers: []domain.KeyValue{{Key: "Authorization", Value: "Bearer SUPERSECRET"}, {Key: "Accept", Value: "application/json"}}},
			Response: &domain.ResponseSummary{StatusCode: 200, Status: "200 OK", Body: `{"echo":"SUPERSECRET","x":"<img src=x onerror=alert(1)>"}`,
				Headers: []domain.KeyValue{{Key: "Set-Cookie", Value: "sid=1"}}},
			AssertionResults: []domain.AssertionResult{{Kind: domain.AssertJSONPath, Path: "echo", Passed: false,
				Expected: `"a"`, Actual: `"SUPERSECRET"`, Message: "expected a but got SUPERSECRET"}},
			Regression: &domain.RegressionResult{Diffs: []domain.DiffEntry{{Path: "echo", Kind: domain.DiffChanged, Before: "old", After: "SUPERSECRET"}}},
		}},
	}
}

func TestReportsRedactSecretsAndDropAuthHeaders(t *testing.T) {
	rep := Build(sampleRun(), Meta{AppName: "Guard", AppVersion: "1.2.3", Settings: domain.DefaultSettings(), Secrets: []string{"SUPERSECRET"}}, time.Now())
	var j, h bytes.Buffer
	if err := WriteJSON(&j, rep); err != nil {
		t.Fatal(err)
	}
	if err := WriteHTML(&h, rep); err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{"json": j.String(), "html": h.String()} {
		if strings.Contains(out, "SUPERSECRET") {
			t.Fatalf("%s report leaked a secret", name)
		}
		if strings.Contains(out, "Authorization") || strings.Contains(out, "Set-Cookie") {
			t.Fatalf("%s report contains credential headers", name)
		}
		for _, want := range []string{"1.2.3", "Staging"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s report missing %q", name, want)
			}
		}
	}
	var decoded Report
	if err := json.Unmarshal(j.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Summary.Failed != 1 || decoded.Summary.Assertions != 1 || decoded.Summary.AssertionsFailed != 1 {
		t.Fatalf("summary counts wrong: %+v", decoded.Summary)
	}
}

func TestHTMLIsEscapedAndSelfContained(t *testing.T) {
	var h bytes.Buffer
	_ = WriteHTML(&h, Build(sampleRun(), Meta{Settings: domain.DefaultSettings()}, time.Now()))
	out := h.String()
	if strings.Contains(out, "<script>") || strings.Contains(out, "<img src=x") {
		t.Fatal("response content must be HTML-escaped")
	}
	for _, external := range []string{"<link", "src=\"http", "<script"} {
		if strings.Contains(out, external) {
			t.Fatalf("report must be self-contained, found %q", external)
		}
	}
}
