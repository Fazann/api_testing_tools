// Package security evaluates the business-security tests: race-condition
// bursts and permission matrices. It only interprets results; running the
// requests is the runner's job and orchestration is the service's.
package security

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
)

// StatusCount is how many responses had one status code; 0 means no response.
type StatusCount struct {
	Code  int `json:"code"`
	Count int `json:"count"`
}

// StateValue is a number read by a race test's state check.
type StateValue struct {
	Value float64
	// Err explains why the value could not be read; Value is then meaningless.
	Err string
}

// RaceReport is the outcome of one race test.
type RaceReport struct {
	RunID     string `json:"runId"`
	Passed    bool   `json:"passed"`
	Cancelled bool   `json:"cancelled"`
	Count     int    `json:"count"`
	// Successes counts 2xx responses.
	Successes    int           `json:"successes"`
	ServerErrors int           `json:"serverErrors"`
	NoResponse   int           `json:"noResponse"`
	StatusCounts []StatusCount `json:"statusCounts"`
	MinMs        int64         `json:"minMs"`
	MaxMs        int64         `json:"maxMs"`
	AvgMs        int64         `json:"avgMs"`
	// Before and After are the state check's readings, when it ran.
	Before *float64 `json:"before,omitempty"`
	After  *float64 `json:"after,omitempty"`
	// Checks lists every invariant with its outcome.
	Checks  []domain.AssertionResult `json:"checks"`
	Results []domain.RunResult       `json:"results"`
}

// IsSuccess reports whether a result counts as a successful call (2xx).
func IsSuccess(r domain.RunResult) bool {
	return r.Response != nil && r.Response.StatusCode >= 200 && r.Response.StatusCode < 300
}

// EvaluateRace computes the report for a burst. before and after are nil
// when the test has no state check.
func EvaluateRace(t domain.RaceTest, results []domain.RunResult, before, after *StateValue) RaceReport {
	rep := RaceReport{Count: len(results), Results: results, Checks: []domain.AssertionResult{}, StatusCounts: []StatusCount{}}
	counts := map[int]int{}
	var total int64
	for i, r := range results {
		if r.Status == domain.StatusCancelled {
			rep.Cancelled = true
		}
		code := 0
		if r.Response != nil {
			code = r.Response.StatusCode
		}
		counts[code]++
		switch {
		case code == 0:
			rep.NoResponse++
		case code >= 500:
			rep.ServerErrors++
		}
		if IsSuccess(r) {
			rep.Successes++
		}
		if i == 0 || r.DurationMs < rep.MinMs {
			rep.MinMs = r.DurationMs
		}
		if r.DurationMs > rep.MaxMs {
			rep.MaxMs = r.DurationMs
		}
		total += r.DurationMs
	}
	if len(results) > 0 {
		rep.AvgMs = total / int64(len(results))
	}
	for code, n := range counts {
		rep.StatusCounts = append(rep.StatusCounts, StatusCount{Code: code, Count: n})
	}
	sort.Slice(rep.StatusCounts, func(i, j int) bool { return rep.StatusCounts[i].Code < rep.StatusCounts[j].Code })

	add := func(desc string, passed bool, expected, actual, msg string) {
		rep.Checks = append(rep.Checks, domain.AssertionResult{Kind: domain.AssertScript, Description: desc,
			Passed: passed, Expected: expected, Actual: actual, Message: msg})
	}

	if t.SuccessOp != "" {
		ok := false
		var sym string
		switch t.SuccessOp {
		case domain.OpEq:
			ok, sym = rep.Successes == t.SuccessCount, "exactly"
		case domain.OpLte:
			ok, sym = rep.Successes <= t.SuccessCount, "at most"
		case domain.OpGte:
			ok, sym = rep.Successes >= t.SuccessCount, "at least"
		}
		msg := ""
		if !ok {
			msg = fmt.Sprintf("%d of %d parallel requests succeeded", rep.Successes, rep.Count)
			if t.SuccessOp != domain.OpGte && rep.Successes > t.SuccessCount {
				msg += " — the server accepted the same operation more than allowed (double spend / duplicate)"
			}
		}
		add(fmt.Sprintf("%s %d successful (2xx) responses", capitalize(sym), t.SuccessCount), ok,
			fmt.Sprintf("%s %d", sym, t.SuccessCount), strconv.Itoa(rep.Successes), msg)
	}

	if !t.AllowServerErrors {
		msg := ""
		if rep.ServerErrors > 0 {
			msg = fmt.Sprintf("%d requests got a 5xx under concurrency (deadlock, lock timeout or unhandled constraint violation?)", rep.ServerErrors)
		}
		add("No server errors (5xx)", rep.ServerErrors == 0, "0", strconv.Itoa(rep.ServerErrors), msg)
	}

	if rep.NoResponse > 0 && !rep.Cancelled {
		add("Every request got a response", false, strconv.Itoa(rep.Count), strconv.Itoa(rep.Count-rep.NoResponse),
			fmt.Sprintf("%d requests failed without a response (timeout, connection reset or script error)", rep.NoResponse))
	}

	if t.UniquePath != "" {
		add(uniqueCheck(t.UniquePath, results))
	}

	if t.Check != nil && before != nil && after != nil {
		desc := fmt.Sprintf("%s changed by %s per success", t.Check.Path, formatNum(t.Check.DeltaPerSuccess))
		switch {
		case before.Err != "":
			add(desc, false, "", "", "could not read the value before the burst: "+before.Err)
		case after.Err != "":
			b := before.Value
			rep.Before = &b
			add(desc, false, "", "", "could not read the value after the burst: "+after.Err)
		default:
			b, a := before.Value, after.Value
			rep.Before, rep.After = &b, &a
			want := float64(rep.Successes) * t.Check.DeltaPerSuccess
			got := a - b
			ok := math.Abs(got-want) <= t.Check.Tolerance+1e-9
			msg := ""
			if !ok {
				msg = fmt.Sprintf("%s went from %s to %s (change %s); with %d successes it should have changed by %s",
					t.Check.Path, formatNum(b), formatNum(a), formatNum(got), rep.Successes, formatNum(want))
				if math.Abs(got) < math.Abs(want) {
					msg += " — lost update: some successful requests were not applied"
				} else {
					msg += " — applied more often than the server reported success"
				}
			}
			add(desc, ok, formatNum(want), formatNum(got), msg)
		}
	}

	rep.Passed = !rep.Cancelled
	for _, c := range rep.Checks {
		if !c.Passed {
			rep.Passed = false
		}
	}
	return rep
}

func uniqueCheck(path string, results []domain.RunResult) (string, bool, string, string, string) {
	desc := "Unique " + path + " across successful responses"
	seen := map[string]int{}
	var dups []string
	missing := 0
	for _, r := range results {
		if !IsSuccess(r) {
			continue
		}
		v, err := ReadPath(r.Response.Body, path)
		if err != nil {
			missing++
			continue
		}
		seen[v]++
		if seen[v] == 2 {
			dups = append(dups, v)
		}
	}
	if len(dups) > 0 {
		return desc, false, "all different", fmt.Sprintf("%d duplicated", len(dups)),
			"the same value was returned to several successful requests: " + summarize(dups, 5)
	}
	if missing > 0 {
		return desc, false, "present in every successful response", fmt.Sprintf("missing in %d", missing),
			fmt.Sprintf("%d successful responses had no value at %s", missing, path)
	}
	return desc, true, "all different", "all different", ""
}

// ReadPath returns the first value at path in a JSON body as text.
func ReadPath(body, path string) (string, error) {
	doc, err := assertions.DecodeJSON([]byte(body))
	if err != nil {
		return "", fmt.Errorf("the response is not JSON")
	}
	m, err := assertions.Select(doc, path)
	if err != nil {
		return "", err
	}
	if len(m) == 0 {
		return "", fmt.Errorf("no value at %s", path)
	}
	switch v := m[0].Value.(type) {
	case string:
		return v, nil
	case nil:
		return "null", nil
	case json.Number:
		// Kept as text: large integer IDs would collide as float64.
		return v.String(), nil
	default:
		b, _ := json.Marshal(v)
		return string(b), nil
	}
}

// ReadNumber reads the number at path from a check request's result.
func ReadNumber(r domain.RunResult, path string) StateValue {
	if r.Response == nil {
		msg := r.Error
		if msg == "" {
			msg = "no response"
		}
		return StateValue{Err: msg}
	}
	if r.Response.StatusCode < 200 || r.Response.StatusCode >= 300 {
		return StateValue{Err: fmt.Sprintf("the check request returned %d", r.Response.StatusCode)}
	}
	doc, err := assertions.DecodeJSON([]byte(r.Response.Body))
	if err != nil {
		return StateValue{Err: "the check response is not JSON"}
	}
	m, err := assertions.Select(doc, path)
	if err != nil {
		return StateValue{Err: err.Error()}
	}
	if len(m) == 0 {
		return StateValue{Err: "no value at " + path}
	}
	f, ok := assertions.ToFloat(m[0].Value)
	if !ok {
		if s, isStr := m[0].Value.(string); isStr {
			if p, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				return StateValue{Value: p}
			}
		}
		return StateValue{Err: fmt.Sprintf("the value at %s is %s, not a number", path, assertions.TypeName(m[0].Value))}
	}
	return StateValue{Value: f}
}

func formatNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func summarize(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" and %d more", len(items)-max)
}
