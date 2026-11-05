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

func TestJUnitReport(t *testing.T) {
	run := sampleRun()
	run.Results = append(run.Results,
		domain.RunResult{ID: "y", Status: domain.StatusError, Error: "network: connection refused",
			Request: domain.RequestSummary{Name: "Down", Method: "POST"}},
		domain.RunResult{ID: "z", Status: domain.StatusPassed, DurationMs: 1500,
			Request: domain.RequestSummary{Name: "Health", Method: "GET"}, Response: &domain.ResponseSummary{StatusCode: 200}})
	run.Tally()
	var buf bytes.Buffer
	if err := WriteJUnit(&buf, Build(run, Meta{Secrets: []string{"SUPERSECRET"}}, time.Now())); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`<testsuites name="Wallet" tests="3" failures="1" errors="1"`,
		`<property name="environment" value="Staging">`,
		`<testcase name="GET Balance &lt;script&gt;" classname="Wallet" time="0.042">`,
		`type="AssertionFailure"`,
		`~ echo changed: old →`,
		`<error message="network: connection refused" type="RequestError">`,
		`<testcase name="GET Health" classname="Wallet" time="1.500">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "SUPERSECRET") || strings.Contains(out, "<script>") {
		t.Fatalf("JUnit report leaked a secret or unescaped markup:\n%s", out)
	}
}

func TestReportsExplainSlowResponses(t *testing.T) {
	run := sampleRun()
	slow := domain.PerformanceResult{Mode: domain.PerformanceFail, Samples: 5, Needed: 5, ReferenceMs: 100,
		ThresholdMs: 200, Slower: true, Message: "450 ms is slower than usual"}
	warn := slow
	warn.Mode, warn.Message = domain.PerformanceWarn, "300 ms is slower than usual (warning)"
	run.Results = append(run.Results,
		domain.RunResult{ID: "slow", Status: domain.StatusFailed, DurationMs: 450, Performance: &slow,
			Request: domain.RequestSummary{Name: "Search", Method: "GET"}, Response: &domain.ResponseSummary{StatusCode: 200}},
		domain.RunResult{ID: "warn", Status: domain.StatusPassed, DurationMs: 300, Performance: &warn,
			Request: domain.RequestSummary{Name: "List", Method: "GET"}, Response: &domain.ResponseSummary{StatusCode: 200}})
	run.Tally()
	rep := Build(run, Meta{}, time.Now())

	var junit bytes.Buffer
	if err := WriteJUnit(&junit, rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(junit.String(), "response time: 450 ms is slower than usual") {
		t.Errorf("JUnit must explain a response-time failure:\n%s", junit.String())
	}
	if strings.Contains(junit.String(), "(warning)") {
		t.Error("a warning is not a JUnit failure")
	}
	var html bytes.Buffer
	if err := WriteHTML(&html, rep); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<span class="failed">450 ms is slower`, `<span class="warn">300 ms is slower`} {
		if !strings.Contains(html.String(), want) {
			t.Errorf("HTML report missing %q", want)
		}
	}
}
