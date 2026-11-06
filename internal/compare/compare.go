// Package compare compares the results of the same requests run against two
// environments (for example Production and Staging, or the old and the new
// build) and reports only what differs. It has no dependency on the desktop
// layer.
package compare

import (
	"fmt"
	"sort"
	"strings"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/regression"
)

// Verdict is the outcome for one request.
type Verdict string

const (
	Same      Verdict = "same"
	Different Verdict = "different"
	Error     Verdict = "error"
	Skipped   Verdict = "skipped"
)

// Timing thresholds: one side is slower when it takes at least SlowerFactor
// times as long as the other and at least SlowerMinMs longer.
const (
	SlowerFactor = 1.5
	SlowerMinMs  = 100
)

// HeaderPrefix marks an ignored-path line that names a header.
const HeaderPrefix = "header:"

// volatileHeaders differ between responses by nature and are never compared.
var volatileHeaders = map[string]bool{
	"date": true, "age": true, "expires": true, "last-modified": true, "etag": true, "set-cookie": true,
	"content-length": true, "transfer-encoding": true, "connection": true, "keep-alive": true,
	"server-timing": true, "report-to": true, "nel": true, "alt-svc": true, "via": true,
	"x-request-id": true, "x-correlation-id": true, "x-trace-id": true, "traceparent": true, "tracestate": true,
	"cf-ray": true, "x-served-by": true, "x-timer": true, "x-runtime": true, "x-response-time": true,
}

// volatilePrefixes are header name prefixes that are never compared.
var volatilePrefixes = []string{"x-amzn-", "x-amz-cf-", "x-cache"}

// IsVolatileHeader reports whether a header is left out of every comparison.
func IsVolatileHeader(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if volatileHeaders[n] {
		return true
	}
	for _, p := range volatilePrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// Options control the comparison of one request.
type Options struct {
	// IgnoredPaths are body path patterns, as in baselines.
	IgnoredPaths []string
	// IgnoredHeaders are header names (any case).
	IgnoredHeaders []string
	// MaxDiffs caps the body differences (default 500).
	MaxDiffs int
}

// SplitIgnored splits ignored-path lines into body patterns and header names
// ("header:X-Build-Id").
func SplitIgnored(lines []string) (paths, headers []string) {
	paths, headers = []string{}, []string{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		switch {
		case l == "":
		case strings.HasPrefix(strings.ToLower(l), HeaderPrefix):
			if h := strings.TrimSpace(l[len(HeaderPrefix):]); h != "" {
				headers = append(headers, h)
			}
		default:
			paths = append(paths, l)
		}
	}
	return paths, headers
}

// Side is what one environment returned for a request.
type Side struct {
	ResultID   string              `json:"resultId"`
	Status     domain.ResultStatus `json:"status"`
	StatusCode int                 `json:"statusCode"`
	DurationMs int64               `json:"durationMs"`
	Error      string              `json:"error,omitempty"`
}

// Result compares one request between environments A and B.
type Result struct {
	RequestID string  `json:"requestId"`
	Name      string  `json:"name"`
	Method    string  `json:"method"`
	Verdict   Verdict `json:"verdict"`
	// Slower is "a" or "b" when that side was much slower, else "".
	Slower string `json:"slower"`
	A      Side   `json:"a"`
	B      Side   `json:"b"`
	// StatusChanged is set when the status codes differ.
	StatusChanged bool `json:"statusChanged"`
	// Body lists field differences (Before is A, After is B).
	Body []domain.DiffEntry `json:"body"`
	// BodyMessage explains a body that was not compared field by field: it
	// differs as text, or was truncated and not compared at all.
	BodyMessage string `json:"bodyMessage,omitempty"`
	// Headers lists header differences; Path is the header name.
	Headers []domain.DiffEntry `json:"headers"`
	// Changing lists body paths left out because they change between calls.
	Changing []string `json:"changing"`
	// SkipReason says why a skipped request was not sent.
	SkipReason string `json:"skipReason,omitempty"`
}

// SkippedResult is the entry for a request that was not compared.
func SkippedResult(req domain.Request, reason string) Result {
	return Result{RequestID: req.ID, Name: req.Name, Method: req.Kind.Badge(req.Method), Verdict: Skipped,
		SkipReason: reason, Body: []domain.DiffEntry{}, Headers: []domain.DiffEntry{}, Changing: []string{}}
}

// Request compares the results a and b of the same request.
func Request(a, b domain.RunResult, opts Options) Result {
	res := Result{
		RequestID: a.RequestID, Name: a.Request.Name, Method: a.Request.Method,
		A: side(a), B: side(b), Body: []domain.DiffEntry{}, Headers: []domain.DiffEntry{}, Changing: []string{},
	}
	if res.Name == "" {
		res.Name = b.Request.Name
	}
	res.Slower = Slower(a.DurationMs, b.DurationMs)
	if a.Response == nil || b.Response == nil {
		res.Verdict = Error
		if a.Status == domain.StatusCancelled || b.Status == domain.StatusCancelled {
			res.Slower = ""
		}
		return res
	}
	res.StatusChanged = a.Response.StatusCode != b.Response.StatusCode
	res.Headers = Headers(a.Response.Headers, b.Response.Headers, opts.IgnoredHeaders)
	var bodyDiffers bool
	res.Body, res.BodyMessage, bodyDiffers = Body(*a.Response, *b.Response, opts)
	res.Verdict = Same
	if res.StatusChanged || len(res.Headers) > 0 || bodyDiffers {
		res.Verdict = Different
	}
	return res
}

func side(r domain.RunResult) Side {
	s := Side{ResultID: r.ID, Status: r.Status, DurationMs: r.DurationMs, Error: r.Error}
	if r.Response != nil {
		s.StatusCode = r.Response.StatusCode
	} else if s.Error == "" {
		s.Error = "no response"
	}
	return s
}

// Slower returns "a" or "b" for the side that was much slower, or "".
func Slower(aMs, bMs int64) string {
	switch {
	case float64(bMs) >= SlowerFactor*float64(aMs) && bMs-aMs >= SlowerMinMs:
		return "b"
	case float64(aMs) >= SlowerFactor*float64(bMs) && aMs-bMs >= SlowerMinMs:
		return "a"
	}
	return ""
}

// Body compares two response bodies. JSON bodies are compared field by field;
// otherwise as text. note explains a body that was not compared field by
// field; differs says whether the bodies differ. Truncated bodies are not
// compared (differs is false).
func Body(a, b domain.ResponseSummary, opts Options) (diffs []domain.DiffEntry, note string, differs bool) {
	none := []domain.DiffEntry{}
	if a.Truncated || b.Truncated {
		return none, "a body was truncated at the response size limit, so the bodies were not compared", false
	}
	ja, errA := assertions.DecodeJSON([]byte(a.Body))
	jb, errB := assertions.DecodeJSON([]byte(b.Body))
	if errA == nil && errB == nil {
		diffs = regression.Diff(ja, jb, regression.Options{IgnoredPaths: opts.IgnoredPaths, MaxDiffs: opts.MaxDiffs})
		return diffs, "", len(diffs) > 0
	}
	if a.Body == b.Body {
		return none, "", false
	}
	switch {
	case errA == nil:
		return none, "A returned JSON, B did not", true
	case errB == nil:
		return none, "B returned JSON, A did not", true
	}
	return none, fmt.Sprintf("the bodies differ as text (%d vs %d bytes)", len(a.Body), len(b.Body)), true
}

// Headers compares response headers case-insensitively, without volatile
// headers and those in ignored. Repeated headers are joined with ", ".
func Headers(a, b []domain.KeyValue, ignored []string) []domain.DiffEntry {
	skip := map[string]bool{}
	for _, h := range ignored {
		skip[strings.ToLower(strings.TrimSpace(h))] = true
	}
	ma, names := headerMap(a), map[string]string{}
	mb := headerMap(b)
	for _, m := range []map[string]header{ma, mb} {
		for k, h := range m {
			if _, ok := names[k]; !ok {
				names[k] = h.name
			}
		}
	}
	keys := make([]string, 0, len(names))
	for k := range names {
		if !skip[k] && !IsVolatileHeader(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := []domain.DiffEntry{}
	for _, k := range keys {
		ha, okA := ma[k]
		hb, okB := mb[k]
		switch {
		case okA && okB:
			if ha.value != hb.value {
				out = append(out, domain.DiffEntry{Path: names[k], Kind: domain.DiffChanged, Before: ha.value, After: hb.value})
			}
		case okA:
			out = append(out, domain.DiffEntry{Path: names[k], Kind: domain.DiffRemoved, Before: ha.value})
		default:
			out = append(out, domain.DiffEntry{Path: names[k], Kind: domain.DiffAdded, After: hb.value})
		}
	}
	return out
}

type header struct{ name, value string }

func headerMap(list []domain.KeyValue) map[string]header {
	m := map[string]header{}
	for _, kv := range list {
		k := strings.ToLower(strings.TrimSpace(kv.Key))
		if k == "" {
			continue
		}
		if h, ok := m[k]; ok {
			h.value += ", " + kv.Value
			m[k] = h
		} else {
			m[k] = header{name: kv.Key, value: kv.Value}
		}
	}
	return m
}

// Report is the outcome of comparing a collection between two environments.
type Report struct {
	A EnvRef `json:"a"`
	B EnvRef `json:"b"`
	// RunIDA and RunIDB are the history runs of each environment's first pass.
	RunIDA    string   `json:"runIdA"`
	RunIDB    string   `json:"runIdB"`
	Requests  []Result `json:"requests"`
	Same      int      `json:"same"`
	Different int      `json:"different"`
	Errors    int      `json:"errors"`
	Skipped   int      `json:"skipped"`
	Slower    int      `json:"slower"`
	Cancelled bool     `json:"cancelled"`
}

// EnvRef names an environment in a report.
type EnvRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Tally fills in the counts from Requests.
func (r *Report) Tally() {
	r.Same, r.Different, r.Errors, r.Skipped, r.Slower = 0, 0, 0, 0, 0
	for _, q := range r.Requests {
		switch q.Verdict {
		case Same:
			r.Same++
		case Different:
			r.Different++
		case Error:
			r.Errors++
		case Skipped:
			r.Skipped++
		}
		if q.Slower != "" {
			r.Slower++
		}
		if q.A.Status == domain.StatusCancelled || q.B.Status == domain.StatusCancelled {
			r.Cancelled = true
		}
	}
}
