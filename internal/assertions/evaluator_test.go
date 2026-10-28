package assertions

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
)

const walletBody = `{
  "data": {
    "balance": 150.25,
    "currency": "USD",
    "owner": {"id": 42, "role": "reader"},
    "tags": ["test", "wallet"],
    "items": [{"amount": 100.10}, {"amount": 50.15}],
    "total": 150.25,
    "total_count": 2
  },
  "meta": {"request-id": "abc"}
}`

func resp(body string) Response {
	h := http.Header{}
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Request-Id", "r-1")
	return Response{StatusCode: 200, Header: h, Body: []byte(body), Duration: 120 * time.Millisecond}
}

func a(kind domain.AssertionKind, path string, op domain.Operator, expected string) domain.Assertion {
	as := domain.Assertion{Kind: kind, Path: path, Operator: op, Enabled: true}
	if expected != "" {
		as.Expected = json.RawMessage(expected)
	}
	return as
}

func TestEvaluateOperators(t *testing.T) {
	cases := []struct {
		name string
		as   domain.Assertion
		pass bool
	}{
		{"status eq", a(domain.AssertStatus, "", domain.OpEq, "200"), true},
		{"status eq wrong", a(domain.AssertStatus, "", domain.OpEq, "201"), false},
		{"status lt", a(domain.AssertStatus, "", domain.OpLt, "300"), true},
		{"header exists", a(domain.AssertHeader, "x-request-id", domain.OpExists, ""), true},
		{"header missing", a(domain.AssertHeader, "X-Missing", domain.OpExists, ""), false},
		{"header not exists", a(domain.AssertHeader, "X-Missing", domain.OpNotExists, ""), true},
		{"header eq", a(domain.AssertHeader, "X-Request-Id", domain.OpEq, `"r-1"`), true},
		{"header contains", a(domain.AssertHeader, "Content-Type", domain.OpContains, `"json"`), true},
		{"header matches", a(domain.AssertHeader, "Content-Type", domain.OpMatches, `"^application/json"`), true},
		{"json exists", a(domain.AssertJSONPath, "data.balance", domain.OpExists, ""), true},
		{"json missing", a(domain.AssertJSONPath, "data.nope", domain.OpExists, ""), false},
		{"json not exists", a(domain.AssertJSONPath, "data.password", domain.OpNotExists, ""), true},
		{"json eq string", a(domain.AssertJSONPath, "data.currency", domain.OpEq, `"USD"`), true},
		{"json eq wrong", a(domain.AssertJSONPath, "data.currency", domain.OpEq, `"EUR"`), false},
		{"json neq", a(domain.AssertJSONPath, "data.currency", domain.OpNeq, `"EUR"`), true},
		{"json eq number int vs float", a(domain.AssertJSONPath, "data.owner.id", domain.OpEq, `42.0`), true},
		{"json eq string vs number", a(domain.AssertJSONPath, "data.owner.id", domain.OpEq, `"42"`), false},
		{"json eq object any key order", a(domain.AssertJSONPath, "data.owner", domain.OpEq, `{"role":"reader","id":42}`), true},
		{"json gt", a(domain.AssertJSONPath, "data.balance", domain.OpGt, `100`), true},
		{"json gte equal", a(domain.AssertJSONPath, "data.balance", domain.OpGte, `150.25`), true},
		{"json lt fails", a(domain.AssertJSONPath, "data.balance", domain.OpLt, `100`), false},
		{"json lte", a(domain.AssertJSONPath, "data.balance", domain.OpLte, `150.25`), true},
		{"numeric op on string", a(domain.AssertJSONPath, "data.currency", domain.OpGt, `1`), false},
		{"string contains", a(domain.AssertJSONPath, "data.currency", domain.OpContains, `"US"`), true},
		{"array contains", a(domain.AssertJSONPath, "data.tags", domain.OpContains, `"wallet"`), true},
		{"array not contains", a(domain.AssertJSONPath, "data.tags", domain.OpNotContains, `"admin"`), true},
		{"matches", a(domain.AssertJSONPath, "data.currency", domain.OpMatches, `"^[A-Z]{3}$"`), true},
		{"type number", a(domain.AssertJSONPath, "data.balance", domain.OpType, `"number"`), true},
		{"type integer accepted as number", a(domain.AssertJSONPath, "data.owner.id", domain.OpType, `"number"`), true},
		{"type integer", a(domain.AssertJSONPath, "data.owner.id", domain.OpType, `"integer"`), true},
		{"type decimal is not integer", a(domain.AssertJSONPath, "data.balance", domain.OpType, `"integer"`), false},
		{"type array", a(domain.AssertJSONPath, "data.tags", domain.OpType, `"array"`), true},
		{"wildcard all positive", a(domain.AssertJSONPath, "data.items[*].amount", domain.OpGt, `0`), true},
		{"wildcard one fails", a(domain.AssertJSONPath, "data.items[*].amount", domain.OpGt, `60`), false},
		{"quoted key", a(domain.AssertJSONPath, `meta["request-id"]`, domain.OpEq, `"abc"`), true},
		{"path ref", a(domain.AssertJSONPath, "data.balance", domain.OpEq, `{"$path":"data.total"}`), true},
		{"response time", a(domain.AssertResponseTime, "", domain.OpLt, `500`), true},
		{"response time slow", a(domain.AssertResponseTime, "", domain.OpLt, `100`), false},
		{"valid json", a(domain.AssertValidJSON, "", "", ""), true},
		{"required fields", a(domain.AssertRequiredFields, "", "", `["data.balance","data.currency"]`), true},
		{"required fields missing", a(domain.AssertRequiredFields, "", "", `["data.balance","data.ghost"]`), false},
		{"body contains", a(domain.AssertBody, "", domain.OpContains, `"wallet"`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := NewEvaluator(resp(walletBody)).Evaluate(tc.as)
			if res.Passed != tc.pass {
				t.Fatalf("passed=%v want %v; expected=%s actual=%s message=%s", res.Passed, tc.pass, res.Expected, res.Actual, res.Message)
			}
			if res.Message == "" {
				t.Fatal("result should always include a message")
			}
		})
	}
}

func TestAggregate(t *testing.T) {
	sum := a(domain.AssertAggregate, "data.items[*].amount", domain.OpEq, `{"$path":"data.total"}`)
	sum.Function = domain.AggSum
	sum.Tolerance = 0.001
	if r := NewEvaluator(resp(walletBody)).Evaluate(sum); !r.Passed {
		t.Fatalf("sum should equal total within tolerance: %s", r.Message)
	}

	count := a(domain.AssertAggregate, "data.items", domain.OpEq, `{"$path":"data.total_count"}`)
	count.Function = domain.AggCount
	if r := NewEvaluator(resp(walletBody)).Evaluate(count); !r.Passed {
		t.Fatalf("count should equal total_count: %s", r.Message)
	}

	max := a(domain.AssertAggregate, "data.items[*].amount", domain.OpLte, `100`)
	max.Function = domain.AggMax
	if r := NewEvaluator(resp(walletBody)).Evaluate(max); r.Passed {
		t.Fatal("max 100.10 should not be <= 100")
	}

	bad := a(domain.AssertAggregate, "data.tags", domain.OpEq, `0`)
	bad.Function = domain.AggSum
	r := NewEvaluator(resp(walletBody)).Evaluate(bad)
	if r.Passed || !strings.Contains(r.Message, "not a number") {
		t.Fatalf("sum of strings must fail clearly, got %q", r.Message)
	}
}

func TestFailureMessagesIncludeExpectedAndActual(t *testing.T) {
	r := NewEvaluator(resp(walletBody)).Evaluate(a(domain.AssertJSONPath, "data.currency", domain.OpEq, `"EUR"`))
	if r.Expected != `"EUR"` || r.Actual != `"USD"` {
		t.Fatalf("expected/actual not recorded: %+v", r)
	}
	if !strings.Contains(r.Message, "EUR") || !strings.Contains(r.Message, "USD") {
		t.Fatalf("message should mention both values: %q", r.Message)
	}
}

func TestMalformedJSONDoesNotPanic(t *testing.T) {
	bodies := []string{"", "not json", `{"a":`, `{"a":1} trailing`, "\x00\xff"}
	kinds := []domain.Assertion{
		a(domain.AssertJSONPath, "a", domain.OpEq, "1"),
		a(domain.AssertValidJSON, "", "", ""),
		a(domain.AssertRequiredFields, "", "", `["a"]`),
		func() domain.Assertion {
			x := a(domain.AssertAggregate, "a", domain.OpEq, "1")
			x.Function = domain.AggSum
			return x
		}(),
	}
	for _, b := range bodies {
		for _, k := range kinds {
			r := NewEvaluator(resp(b)).Evaluate(k)
			if r.Passed {
				t.Fatalf("body %q kind %s should fail", b, k.Kind)
			}
			if r.Message == "" {
				t.Fatalf("body %q kind %s: missing message", b, k.Kind)
			}
		}
	}
}

func TestTruncatedBodyIsNotParsed(t *testing.T) {
	r := resp(`{"a":1}`)
	r.Truncated = true
	res := NewEvaluator(r).Evaluate(a(domain.AssertValidJSON, "", "", ""))
	if res.Passed || !strings.Contains(res.Message, "truncated") {
		t.Fatalf("truncated body must not be treated as JSON: %+v", res)
	}
}

func TestInvalidAssertionIsReportedNotPanicking(t *testing.T) {
	res := NewEvaluator(resp(walletBody)).Evaluate(a(domain.AssertJSONPath, "data.currency", domain.OpMatches, `"([a-z"`))
	if res.Passed || !strings.Contains(res.Message, "invalid") {
		t.Fatalf("bad regex should produce an invalid-assertion message: %+v", res)
	}
}

func TestEvaluateSkipsDisabled(t *testing.T) {
	as := a(domain.AssertStatus, "", domain.OpEq, "500")
	as.Enabled = false
	if got := Evaluate(resp(walletBody), []domain.Assertion{as}); len(got) != 0 {
		t.Fatalf("disabled assertions must be skipped, got %d results", len(got))
	}
}

func TestParsePath(t *testing.T) {
	good := map[string]int{
		"a":          1,
		"$.a.b":      2,
		"a[0].b":     3,
		"a[*]":       2,
		"a.*":        2,
		`a["x.y"].z`: 3,
		`a['k']`:     2,
		"items[-1]":  2,
		"$":          0,
	}
	for p, n := range good {
		segs, err := ParsePath(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if len(segs) != n {
			t.Fatalf("%s: got %d segments want %d", p, len(segs), n)
		}
	}
	for _, p := range []string{"a.", "a[", "a[x]", "a..b", `a["x]`} {
		if _, err := ParsePath(p); err == nil {
			t.Fatalf("%s: expected error", p)
		}
	}
}

func TestSelectNegativeIndexAndDisplayPath(t *testing.T) {
	doc, _ := DecodeJSON([]byte(`{"items":[1,2,3],"we ird":{"k":true}}`))
	m, _ := Select(doc, "items[-1]")
	if len(m) != 1 || m[0].Path != "items[2]" {
		t.Fatalf("got %+v", m)
	}
	m, _ = Select(doc, `["we ird"].k`)
	if len(m) != 1 || m[0].Path != `["we ird"].k` {
		t.Fatalf("got %+v", m)
	}
}
