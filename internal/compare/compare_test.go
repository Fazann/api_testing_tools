package compare

import (
	"reflect"
	"testing"

	"api-test-kits/internal/domain"
)

func result(code int, body string, ms int64, headers ...string) domain.RunResult {
	var hs []domain.KeyValue
	for i := 0; i+1 < len(headers); i += 2 {
		hs = append(hs, domain.KeyValue{Key: headers[i], Value: headers[i+1], Enabled: true})
	}
	return domain.RunResult{ID: "r", RequestID: "q1", Request: domain.RequestSummary{Name: "Get", Method: "GET"},
		Status: domain.StatusPassed, DurationMs: ms,
		Response: &domain.ResponseSummary{StatusCode: code, Body: body, Headers: hs}}
}

func TestSameWhenOnlyVolatileThingsDiffer(t *testing.T) {
	a := result(200, `{"a":1,"b":[1,2]}`, 100, "Date", "Mon", "Content-Type", "application/json", "X-Request-Id", "1")
	b := result(200, `{"b":[1,2],"a":1.0}`, 120, "date", "Tue", "content-type", "application/json", "X-Amzn-Trace-Id", "z")
	got := Request(a, b, Options{})
	if got.Verdict != Same || got.Slower != "" || len(got.Headers) != 0 || len(got.Body) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestDifferences(t *testing.T) {
	a := result(200, `{"total":120.5,"currency":"USD","id":"x1"}`, 100, "Cache-Control", "max-age=60", "HSTS", "on")
	b := result(500, `{"total":"120","id":"y2"}`, 900, "Cache-Control", "no-store", "X-Build", "2")
	got := Request(a, b, Options{IgnoredPaths: []string{"id"}, IgnoredHeaders: []string{"x-build"}})
	if got.Verdict != Different || !got.StatusChanged || got.Slower != "b" {
		t.Fatalf("got %+v", got)
	}
	wantBody := []domain.DiffEntry{
		{Path: "currency", Kind: domain.DiffRemoved, Before: `"USD"`},
		{Path: "total", Kind: domain.DiffTypeChanged, Before: "number 120.5", After: `string "120"`},
	}
	if !reflect.DeepEqual(got.Body, wantBody) {
		t.Errorf("body = %+v", got.Body)
	}
	wantHeaders := []domain.DiffEntry{
		{Path: "Cache-Control", Kind: domain.DiffChanged, Before: "max-age=60", After: "no-store"},
		{Path: "HSTS", Kind: domain.DiffRemoved, Before: "on"},
	}
	if !reflect.DeepEqual(got.Headers, wantHeaders) {
		t.Errorf("headers = %+v", got.Headers)
	}
}

func TestRepeatedHeadersAreJoined(t *testing.T) {
	a := []domain.KeyValue{{Key: "Vary", Value: "Accept"}, {Key: "Vary", Value: "Origin"}}
	b := []domain.KeyValue{{Key: "vary", Value: "Accept, Origin"}}
	if got := Headers(a, b, nil); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestTextAndTruncatedBodies(t *testing.T) {
	same := Request(result(200, "ok", 1), result(200, "ok", 1), Options{})
	if same.Verdict != Same {
		t.Fatalf("equal text: %+v", same)
	}
	text := Request(result(200, "ok", 1), result(200, "okay", 1), Options{})
	if text.Verdict != Different || text.BodyMessage == "" {
		t.Fatalf("text: %+v", text)
	}
	mixed := Request(result(200, `{"a":1}`, 1), result(200, "<html>", 1), Options{})
	if mixed.Verdict != Different || mixed.BodyMessage != "A returned JSON, B did not" {
		t.Fatalf("mixed: %+v", mixed)
	}
	ta, tb := result(200, `{"a":1}`, 1), result(200, `{"a":2}`, 1)
	tb.Response.Truncated = true
	trunc := Request(ta, tb, Options{})
	if trunc.Verdict != Same || trunc.BodyMessage == "" {
		t.Fatalf("a truncated body is noted, not a difference: %+v", trunc)
	}
}

func TestErrorWhenASideHasNoResponse(t *testing.T) {
	b := domain.RunResult{ID: "e", Status: domain.StatusError, Error: "connection refused"}
	got := Request(result(200, "{}", 5), b, Options{})
	if got.Verdict != Error || got.B.Error != "connection refused" || got.A.StatusCode != 200 {
		t.Fatalf("got %+v", got)
	}
}

func TestSlower(t *testing.T) {
	cases := []struct {
		a, b int64
		want string
	}{
		{100, 150, ""},   // 1.5x but only 50 ms
		{10, 400, "b"},   // much slower
		{1000, 1090, ""}, // 90 ms but under 1.5x
		{900, 100, "a"},
		{0, 99, ""},
		{0, 100, "b"},
	}
	for _, c := range cases {
		if got := Slower(c.a, c.b); got != c.want {
			t.Errorf("Slower(%d,%d) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestSplitIgnored(t *testing.T) {
	paths, headers := SplitIgnored([]string{" meta.id ", "", "header: X-Build-Id", "HEADER:etag", "header:"})
	if !reflect.DeepEqual(paths, []string{"meta.id"}) || !reflect.DeepEqual(headers, []string{"X-Build-Id", "etag"}) {
		t.Fatalf("paths %v headers %v", paths, headers)
	}
}

func TestTally(t *testing.T) {
	r := Report{Requests: []Result{
		{Verdict: Same, Slower: "b"}, {Verdict: Different}, {Verdict: Error, B: Side{Status: domain.StatusCancelled}},
		{Verdict: Skipped},
	}}
	r.Tally()
	if r.Same != 1 || r.Different != 1 || r.Errors != 1 || r.Skipped != 1 || r.Slower != 1 || !r.Cancelled {
		t.Fatalf("got %+v", r)
	}
}
