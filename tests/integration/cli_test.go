package integration

import (
	"bytes"
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"api-test-kits/internal/cli"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/service"
)

type cliResult struct {
	code           int
	stdout, stderr string
}

// runCLI runs the CLI in-process with the given APIGUARD_VAR_* variables.
func runCLI(t *testing.T, environ []string, args ...string) cliResult {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Main(context.Background(), args, cli.Env{
		Stdout: &out, Stderr: &errOut,
		Getenv:  func(string) string { return "" },
		Environ: func() []string { return environ },
	})
	return cliResult{code, out.String(), errOut.String()}
}

const fixtureFile = "../fixtures/wallet-demo.collection.json"

func baseURL(f *fixture) string {
	for _, v := range f.env.Variables {
		if v.Key == "base_url" {
			return v.Value
		}
	}
	return ""
}

func TestCLIRunsCollectionFileAndWritesReports(t *testing.T) {
	f := setup(t)
	dir := t.TempDir()
	junit := filepath.Join(dir, "out", "junit.xml")
	html := filepath.Join(dir, "report.html")
	res := runCLI(t, []string{"APIGUARD_VAR_token=" + testToken, "PATH=/usr/bin"},
		"run", fixtureFile, "--var", "base_url="+baseURL(f), "--junit", junit, "--report", html)
	if res.code != cli.ExitPassed {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "PASSED — 6 passed, 0 failed, 0 errors") {
		t.Fatalf("summary missing:\n%s", res.stdout)
	}
	if strings.Contains(res.stderr, "token") {
		t.Fatalf("the secret was set, so there should be no empty-secret warning:\n%s", res.stderr)
	}
	data, err := os.ReadFile(junit)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tests  int `xml:"tests,attr"`
		Suites []struct {
			Cases []struct {
				Name string `xml:"name,attr"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil || doc.Tests != 6 || len(doc.Suites[0].Cases) != 6 {
		t.Fatalf("invalid JUnit report (%v): %s", err, data)
	}
	for _, p := range []string{junit, html} {
		b, _ := os.ReadFile(p)
		if bytes.Contains(b, []byte(testToken)) {
			t.Fatalf("%s leaked the token", p)
		}
	}
	if strings.Contains(res.stdout, testToken) {
		t.Fatal("stdout leaked the token")
	}
}

func TestCLIMissingSecretIsASetupError(t *testing.T) {
	f := setup(t)
	// The fixture lists token as a required variable, so the run does not start.
	res := runCLI(t, nil, "run", fixtureFile, "--var", "base_url="+baseURL(f))
	if res.code != cli.ExitUsage || !strings.Contains(res.stderr, "required variables are missing or empty: token") {
		t.Fatalf("expected a setup error, exit %d\n%s", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, `secret variable "token" is empty`) || !strings.Contains(res.stderr, "APIGUARD_VAR_token") {
		t.Fatalf("expected a hint about the empty secret:\n%s", res.stderr)
	}
}

func TestCLIFailsOnAssertions(t *testing.T) {
	f := setup(t)
	res := runCLI(t, []string{"APIGUARD_VAR_token=wrong-token"}, "run", fixtureFile, "--var", "base_url="+baseURL(f),
		"--folder", "Wallet")
	if res.code != cli.ExitFailed {
		t.Fatalf("with a wrong token the wallet tests should fail, exit %d\n%s\n%s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "✘ FAIL") || !strings.Contains(res.stdout, "2 request(s)") ||
		!strings.Contains(res.stdout, "expected 200, got 401") {
		t.Fatalf("expected two failing wallet requests with details:\n%s", res.stdout)
	}
}

// Baselines approved in the app travel in the export and catch a contract
// break in CI; --no-baselines turns the comparison off.
func TestCLIDetectsRegressionWithExportedBaselines(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	req := f.request(t, "Get balance")
	first, err := f.svc.SendRequest(ctx, "", req, f.env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ApproveBaseline(ctx, service.ApproveBaselineInput{RequestID: req.ID, EnvironmentID: f.env.ID,
		Body: first.Response.Body, Mode: domain.BaselineSchema}); err != nil {
		t.Fatal(err)
	}
	data, err := f.svc.ExportCollection(ctx, f.tree.Collection.ID, []string{f.env.ID}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(testToken)) {
		t.Fatal("export leaked the token")
	}
	file := filepath.Join(t.TempDir(), "wallet.collection.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"APIGUARD_VAR_token=" + testToken}

	if res := runCLI(t, env, "run", file, "--env", "Integration"); res.code != cli.ExitPassed {
		t.Fatalf("unchanged API should pass, exit %d\n%s\n%s", res.code, res.stdout, res.stderr)
	}
	if res := runCLI(t, env, "list", file); !strings.Contains(res.stdout, "[baseline: Integration]") {
		t.Fatalf("list should show the baseline:\n%s", res.stdout)
	}

	f.api.set(func(m *mockAPI) { m.balanceAsString = true })
	res := runCLI(t, env, "run", file, "--env", "integration", "-q")
	if res.code != cli.ExitFailed || !strings.Contains(res.stdout, "data.balance type changed: number → string") {
		t.Fatalf("expected a baseline failure, exit %d\n%s\n%s", res.code, res.stdout, res.stderr)
	}
	if strings.Contains(res.stdout, "✔ PASS") {
		t.Fatalf("--quiet should hide passing requests:\n%s", res.stdout)
	}
	// Without baselines only the request's own type assertion reports the change.
	res = runCLI(t, env, "run", file, "--env", "Integration", "--no-baselines")
	if res.code != cli.ExitFailed || strings.Contains(res.stdout, "baseline:") || !strings.Contains(res.stdout, "baselines off") {
		t.Fatalf("--no-baselines should skip the comparison, exit %d\n%s", res.code, res.stdout)
	}
}

// --collection reads the desktop app's database (secrets and baselines
// included) but never writes to it.
func TestCLIRunsFromAppDatabaseWithoutChangingIt(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	before, _ := f.svc.ListRuns(ctx, domain.RunFilter{})
	dataDir := filepath.Dir(f.store.Path())
	res := runCLI(t, nil, "run", "--collection", "wallet demo", "--data-dir", dataDir, "--env", "Integration")
	if res.code != cli.ExitPassed {
		t.Fatalf("exit %d\n%s\n%s", res.code, res.stdout, res.stderr)
	}
	after, _ := f.svc.ListRuns(ctx, domain.RunFilter{})
	if len(after) != len(before) {
		t.Fatalf("the CLI wrote %d runs into the app database", len(after)-len(before))
	}
	res = runCLI(t, nil, "run", "--collection", "Nope", "--data-dir", dataDir)
	if res.code != cli.ExitUsage || !strings.Contains(res.stderr, `"Wallet demo"`) {
		t.Fatalf("an unknown collection should list the existing ones:\n%s", res.stderr)
	}
}

func TestCLIProtectedEnvironmentNeedsConfirmation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.env.Protected = true
	if _, err := f.svc.SaveEnvironment(ctx, f.env); err != nil {
		t.Fatal(err)
	}
	data, _ := f.svc.ExportCollection(ctx, f.tree.Collection.ID, []string{f.env.ID}, false, false)
	file := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(file, data, 0o600)
	env := []string{"APIGUARD_VAR_token=" + testToken}

	res := runCLI(t, env, "run", file)
	if res.code != cli.ExitUsage || !strings.Contains(res.stderr, "--confirm-protected") {
		t.Fatalf("expected a refusal, exit %d\n%s", res.code, res.stderr)
	}
	if res := runCLI(t, env, "run", file, "--confirm-protected"); res.code != cli.ExitPassed {
		t.Fatalf("confirmed run should pass, exit %d\n%s\n%s", res.code, res.stdout, res.stderr)
	}
}

func TestCLIUsageErrors(t *testing.T) {
	setup(t)
	two := filepath.Join(t.TempDir(), "two.json")
	data, _ := os.ReadFile(fixtureFile)
	data = bytes.Replace(data, []byte(`"environments": [`), []byte(`"environments": [{"name": "Other", "variables": []},`), 1)
	os.WriteFile(two, data, 0o600)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"run"}, "give a collection file"},
		{[]string{"run", "missing.json"}, "no such file"},
		{[]string{"run", fixtureFile, "--report", "r.txt"}, "cannot tell the report format"},
		{[]string{"run", fixtureFile, "--folder", "Nope"}, `folders: "Authorization"`},
		{[]string{"run", two}, `choose one with --env: "Other", "Local (demo)"`},
		{[]string{"run", fixtureFile, "--var", "no-equals"}, "expected KEY=VALUE"},
		{[]string{"run", fixtureFile, "--concurrency", "20"}, "between 1 and 8"},
		{[]string{"frobnicate"}, "unknown command"},
	}
	for _, c := range cases {
		res := runCLI(t, nil, c.args...)
		if res.code != cli.ExitUsage || !strings.Contains(res.stderr, c.want) {
			t.Errorf("%v: exit %d, stderr %q, want %q", c.args, res.code, res.stderr, c.want)
		}
	}
	if res := runCLI(t, nil, "help", "run"); res.code != 0 || !strings.Contains(res.stdout, "--confirm-protected") {
		t.Errorf("help run: %d %s", res.code, res.stdout)
	}
}
