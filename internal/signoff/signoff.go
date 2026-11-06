// Package signoff builds the release sign-off report: a summary of one
// collection in one environment for QA hand-off and audits (verdict, newest
// run, regressions, security checks, performance, baselines, coverage). It
// only reads stored data; see docs/signoff-report.md.
package signoff

import (
	"fmt"
	"math"
	"strings"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/redact"
	"api-test-kits/internal/reports"
	"api-test-kits/internal/rules"
)

// Limits keep the report readable.
const (
	maxDiffsPerRegression = 10
	maxDetailsPerItem     = 10
	// PerfChangePct is the median change (in percent) that lists a request under Performance.
	PerfChangePct = 20.0
)

// RaceVerdictPrefix starts the name of a race test's verdict result.
const RaceVerdictPrefix = "Race test verdict"

// Data is everything the report is built from.
type Data struct {
	Release, PreparedBy, Notes string
	Collection                 string
	// Requests are the collection's requests in run order.
	Requests        []domain.Request
	EnvironmentName string
	// Run is the newest collection run in the environment, with results (nil for none).
	Run *domain.Run
	// Races is the newest race test run per request ID.
	Races map[string]*domain.Run
	// Matrix is the newest permission matrix run (nil for none).
	Matrix *domain.Run
	// Baselines per request ID in the environment (missing: no baseline).
	Baselines  map[string]*domain.Baseline
	Times      []domain.ResponseTimeRow
	AppName    string
	AppVersion string
	// Secrets are current secret values, redacted again as a safety net.
	Secrets []string
	Now     time.Time
}

// Report is the computed content of a sign-off report.
type Report struct {
	Release, PreparedBy, Notes  string
	Collection, Environment     string
	AppName, AppVersion         string
	GeneratedAt                 time.Time
	Ready                       bool
	Reasons                     []string
	Run                         *RunSection
	Regressions                 []Regression
	Races                       []Check
	Matrix                      *Check
	Performance                 []PerfRow
	Baselines                   []BaselineRow
	NoTests, NoBaseline, NotRun []string
}

// RunSection summarises the newest collection run.
type RunSection struct {
	ID                            string
	StartedAt                     time.Time
	Status                        domain.ResultStatus
	Passed, Failed, Errors, Total int
	Covered, Requests             int
	Failures                      []Failure
}

// Failure is a failed or errored result with its messages.
type Failure struct {
	Name, Method string
	Status       domain.ResultStatus
	Details      []string
}

// Regression is a request whose response differs from its baseline.
type Regression struct {
	Name, Method, Message string
	Diffs                 []domain.DiffEntry
	More                  int
}

// Check is the outcome of a race test or a permission matrix run.
type Check struct {
	Name      string
	StartedAt time.Time
	Status    domain.ResultStatus
	Details   []string
}

// Passed reports whether the check passed.
func (c Check) Passed() bool { return c.Status == domain.StatusPassed }

// PerfRow is a request whose response time changed noticeably.
type PerfRow struct {
	Name, Method     string
	MedianMs, LastMs int64
	ChangePct        *float64
	Slower           bool
}

// BaselineRow describes the baseline of one request.
type BaselineRow struct {
	Name, Method string
	Has          bool
	Mode         string
	ApprovedAt   time.Time
	ApprovedBy   string
	Ignored      int
}

// Build computes the report from d.
func Build(d Data) Report {
	red := redact.New(d.Secrets...)
	clean := func(s string) string { return red.String(s) }
	r := Report{
		Release: clean(d.Release), PreparedBy: clean(d.PreparedBy), Notes: clean(d.Notes),
		Collection: d.Collection, Environment: d.EnvironmentName,
		AppName: d.AppName, AppVersion: d.AppVersion, GeneratedAt: d.Now,
	}
	if r.Environment == "" {
		r.Environment = "No environment"
	}
	reqs := d.Requests

	// Newest run, failures and regressions.
	ran := map[string]bool{}
	var rulesFailed []string
	failedRequests, erroredRequests := 0, 0
	if run := d.Run; run != nil {
		sec := &RunSection{ID: run.ID, StartedAt: run.StartedAt, Status: run.Status, Passed: run.PassedCount,
			Failed: run.FailedCount, Errors: run.ErrorCount, Total: len(run.Results), Requests: len(reqs)}
		for _, res := range run.Results {
			if res.RequestID != "" {
				ran[res.RequestID] = true
			}
			isRules := res.RequestID == "" && res.Request.Name == rules.ResultName
			switch res.Status {
			case domain.StatusFailed:
				if isRules {
					for _, a := range res.AssertionResults {
						if !a.Passed {
							rulesFailed = append(rulesFailed, clean(a.Description))
						}
					}
				} else {
					failedRequests++
				}
			case domain.StatusError:
				erroredRequests++
			}
			if res.Status == domain.StatusFailed || res.Status == domain.StatusError {
				sec.Failures = append(sec.Failures, Failure{Name: clean(res.Request.Name), Method: res.Request.Method,
					Status: res.Status, Details: limit(cleanAll(reports.FailureDetails(res), red), maxDetailsPerItem)})
			}
			if reg := res.Regression; reg != nil && !reg.Passed {
				g := Regression{Name: clean(res.Request.Name), Method: res.Request.Method, Message: clean(reg.Message)}
				for i, diff := range reg.Diffs {
					if i == maxDiffsPerRegression {
						g.More = len(reg.Diffs) - i
						break
					}
					diff.Before, diff.After, diff.Path = clean(diff.Before), clean(diff.After), clean(diff.Path)
					g.Diffs = append(g.Diffs, diff)
				}
				r.Regressions = append(r.Regressions, g)
			}
		}
		for _, q := range reqs {
			if ran[q.ID] {
				sec.Covered++
			}
		}
		r.Run = sec
	}

	// Security checks.
	var raceFailed []string
	for _, q := range reqs {
		run, ok := d.Races[q.ID]
		if !ok || run == nil {
			continue
		}
		c := Check{Name: q.Name, StartedAt: run.StartedAt, Status: run.Status}
		if v := raceVerdict(*run); v != nil {
			c.Status = v.Status
			c.Details = limit(cleanAll(reports.FailureDetails(*v), red), maxDetailsPerItem)
		}
		if !c.Passed() {
			raceFailed = append(raceFailed, q.Name)
		}
		r.Races = append(r.Races, c)
	}
	var mismatches int
	if m := d.Matrix; m != nil {
		c := Check{Name: "Permission matrix", StartedAt: m.StartedAt, Status: m.Status}
		for _, res := range m.Results {
			if res.Status == domain.StatusFailed || res.Status == domain.StatusError {
				mismatches++
				for _, line := range reports.FailureDetails(res) {
					c.Details = append(c.Details, clean(res.Request.Name)+": "+clean(line))
				}
			}
		}
		c.Details = limit(c.Details, maxDetailsPerItem)
		r.Matrix = &c
	}

	// Performance.
	for _, t := range d.Times {
		big := t.ChangePct != nil && math.Abs(*t.ChangePct) >= PerfChangePct
		if t.LastSlower || big {
			r.Performance = append(r.Performance, PerfRow{Name: t.Name, Method: t.Method, MedianMs: t.Stats.MedianMs,
				LastMs: t.LastMs, ChangePct: t.ChangePct, Slower: t.LastSlower})
		}
	}

	// Baselines and coverage.
	for _, q := range reqs {
		if !q.Kind.IsSocket() {
			row := BaselineRow{Name: q.Name, Method: q.Kind.Badge(q.Method)}
			if b := d.Baselines[q.ID]; b != nil {
				row.Has, row.Mode, row.ApprovedAt, row.ApprovedBy = true, baselineMode(*b), b.UpdatedAt, clean(b.ApprovedBy)
				row.Ignored = len(b.IgnoredPaths)
			} else {
				r.NoBaseline = append(r.NoBaseline, q.Name)
			}
			r.Baselines = append(r.Baselines, row)
		}
		if !hasTests(q) {
			r.NoTests = append(r.NoTests, q.Name)
		}
		if d.Run != nil && !ran[q.ID] {
			r.NotRun = append(r.NotRun, q.Name)
		}
	}

	// Verdict.
	switch {
	case d.Run == nil:
		r.Reasons = append(r.Reasons, fmt.Sprintf("there is no collection run in %s", r.Environment))
	case d.Run.Status == domain.StatusCancelled:
		r.Reasons = append(r.Reasons, "the newest collection run was cancelled")
	}
	if failedRequests > 0 {
		r.Reasons = append(r.Reasons, plural(failedRequests, "request failed", "requests failed"))
	}
	if erroredRequests > 0 {
		r.Reasons = append(r.Reasons, plural(erroredRequests, "request had an error", "requests had errors"))
	}
	if len(rulesFailed) > 0 {
		r.Reasons = append(r.Reasons, "collection rules failed: "+strings.Join(rulesFailed, ", "))
	}
	if n := len(r.Regressions); n > 0 {
		r.Reasons = append(r.Reasons, plural(n, "regression from its baseline", "regressions from their baselines"))
	}
	if len(raceFailed) > 0 {
		r.Reasons = append(r.Reasons, "race test failed: "+strings.Join(raceFailed, ", "))
	}
	if r.Matrix != nil && !r.Matrix.Passed() {
		if mismatches > 0 {
			r.Reasons = append(r.Reasons, "permission matrix: "+plural(mismatches, "access mismatch", "access mismatches"))
		} else {
			r.Reasons = append(r.Reasons, "the newest permission matrix run did not pass")
		}
	}
	r.Ready = len(r.Reasons) == 0
	return r
}

// raceVerdict returns the verdict result of a race test run.
func raceVerdict(run domain.Run) *domain.RunResult {
	for i := len(run.Results) - 1; i >= 0; i-- {
		if strings.HasPrefix(run.Results[i].Request.Name, RaceVerdictPrefix) {
			return &run.Results[i]
		}
	}
	return nil
}

// IsRaceRun reports whether a run is a race test: its own kind, or an older
// burst run with a verdict result. It returns the tested request's ID.
func IsRaceRun(run domain.Run) (string, bool) {
	if run.Kind != domain.RunKindRace && run.Kind != domain.RunKindBurst {
		return "", false
	}
	if v := raceVerdict(run); v != nil {
		return v.RequestID, true
	}
	return "", false
}

// IsMatrixRun reports whether a run is a permission matrix run: its own kind,
// or an older burst run without a race verdict.
func IsMatrixRun(run domain.Run) bool {
	if run.Kind == domain.RunKindMatrix {
		return true
	}
	_, race := IsRaceRun(run)
	return run.Kind == domain.RunKindBurst && !race
}

func hasTests(q domain.Request) bool {
	if q.Kind.IsSocket() {
		return q.Socket != nil && len(q.Socket.Steps) > 0
	}
	for _, a := range q.Assertions {
		if a.Enabled {
			return true
		}
	}
	return strings.Contains(q.PostResponseScript, "test(")
}

func baselineMode(b domain.Baseline) string {
	switch {
	case b.Mode == domain.BaselineSchema:
		return "structure"
	case len(b.SelectedPaths) > 0:
		return plural(len(b.SelectedPaths), "selected field", "selected fields")
	}
	return "whole response"
}

func cleanAll(list []string, red *redact.Redactor) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = red.String(s)
	}
	return out
}

func limit(list []string, n int) []string {
	if len(list) <= n {
		return list
	}
	return append(list[:n:n], fmt.Sprintf("… and %d more", len(list)-n))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
