package rules

import (
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
)

func res(id, name, reqBody, respBody string, code int, headers ...domain.KeyValue) domain.RunResult {
	return domain.RunResult{ID: "res-" + id, RequestID: id, Status: domain.StatusPassed,
		Request:  domain.RequestSummary{Name: name, Body: reqBody},
		Response: &domain.ResponseSummary{StatusCode: code, Body: respBody, Headers: headers}}
}

func resp(id, path string) domain.RuleValue {
	return domain.RuleValue{Source: domain.SourceResponse, RequestID: id, Path: path}
}

func plus(v domain.RuleValue) domain.RuleTerm  { return domain.RuleTerm{Sign: "+", Value: v} }
func minus(v domain.RuleValue) domain.RuleTerm { return domain.RuleTerm{Sign: "-", Value: v} }
func constant(s string) domain.RuleValue {
	return domain.RuleValue{Source: domain.SourceConstant, Literal: s}
}

var run = []domain.RunResult{
	res("before", "Balance before", "", `{"data":{"balance":100.0}}`, 200),
	res("withdraw", "Withdraw 10", `{"amount":10}`, `{"ok":true,"order":{"id":"ord_7"}}`, 201,
		domain.KeyValue{Key: "Location", Value: "/orders/ord_7"}),
	res("after", "Balance after", "", `{"data":{"balance":90}}`, 200),
	res("list", "List orders", "", `{"orders":[{"id":"ord_1","price":5},{"id":"ord_7","price":2.5}],"ids":[1,"42"]}`, 200),
}

func rule(kind domain.RuleKind, op string, left domain.RuleValue, right ...domain.RuleTerm) domain.Rule {
	return domain.Rule{ID: "r", Name: "Rule", Enabled: true, Kind: kind, Operator: op, Left: left, Right: right}
}

func check(t *testing.T, r domain.Rule) domain.AssertionResult {
	t.Helper()
	out := Check([]domain.Rule{r}, run)
	if len(out) != 1 || out[0].Skipped {
		t.Fatalf("rule skipped: %+v", out)
	}
	if out[0].Result.Kind != domain.AssertRule || out[0].Result.Description != "Rule" {
		t.Fatalf("result fields: %+v", out[0].Result)
	}
	return out[0].Result
}

func TestCompareRules(t *testing.T) {
	amount := domain.RuleValue{Source: domain.SourceRequest, RequestID: "withdraw", Path: "amount"}
	cases := []struct {
		name    string
		rule    domain.Rule
		passed  bool
		message string
	}{
		{"balance deducted", rule(domain.RuleCompare, domain.RuleEq, resp("after", "data.balance"),
			plus(resp("before", "data.balance")), minus(amount)), true, "90 = 100 − 10"},
		{"wrong amount", rule(domain.RuleCompare, domain.RuleEq, resp("after", "data.balance"),
			plus(resp("before", "data.balance")), minus(constant("20"))), false, "90 ≠ 100 − 20 = 80"},
		{"count grew", rule(domain.RuleCompare, domain.RuleEq,
			domain.RuleValue{Source: domain.SourceResponse, RequestID: "list", Path: "orders", Aggregate: "count"},
			plus(constant("1")), plus(constant("1"))), true, "2 = 1 + 1"},
		{"sum with tolerance", func() domain.Rule {
			r := rule(domain.RuleCompare, domain.RuleEq,
				domain.RuleValue{Source: domain.SourceResponse, RequestID: "list", Path: "orders[*].price", Aggregate: "sum"},
				plus(constant("7.501")))
			r.Tolerance = 0.005
			return r
		}(), true, "7.5 = 7.501"},
		{"strings equal", rule(domain.RuleCompare, domain.RuleEq, resp("withdraw", "order.id"),
			plus(constant(`"ord_7"`))), true, `"ord_7" = "ord_7"`},
		{"plain text constant", rule(domain.RuleCompare, domain.RuleNe, resp("withdraw", "order.id"),
			plus(constant("ord_8"))), true, `"ord_7" ≠ "ord_8"`},
		{"status", rule(domain.RuleCompare, domain.RuleEq,
			domain.RuleValue{Source: domain.SourceStatus, RequestID: "withdraw"}, plus(constant("201"))), true, "201 = 201"},
		{"header", rule(domain.RuleCompare, domain.RuleEq,
			domain.RuleValue{Source: domain.SourceHeader, RequestID: "withdraw", Path: "location"},
			plus(constant("/orders/ord_7"))), true, `"/orders/ord_7" = "/orders/ord_7"`},
		{"greater fails", rule(domain.RuleCompare, domain.RuleGt, resp("after", "data.balance"),
			plus(resp("before", "data.balance"))), false, "90 is not > 100"},
		{"lte", rule(domain.RuleCompare, domain.RuleLte, resp("after", "data.balance"),
			plus(resp("before", "data.balance"))), true, "90 ≤ 100"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := check(t, c.rule)
			if got.Passed != c.passed || got.Message != c.message {
				t.Fatalf("passed=%v message=%q, want %v %q", got.Passed, got.Message, c.passed, c.message)
			}
		})
	}
}

func TestContainsRules(t *testing.T) {
	created := resp("withdraw", "order.id")
	in := check(t, rule(domain.RuleContains, domain.RuleIn, created, plus(resp("list", "orders[*].id"))))
	if !in.Passed || in.Message != `"ord_7" appears in List orders → orders[*].id` {
		t.Fatalf("in: %+v", in)
	}
	gone := check(t, rule(domain.RuleContains, domain.RuleNotIn, created, plus(resp("list", "orders[*].id"))))
	if gone.Passed {
		t.Fatalf("not_in must fail when present: %+v", gone)
	}
	missing := check(t, rule(domain.RuleContains, domain.RuleIn, constant(`"ord_9"`), plus(resp("list", "orders[*].id"))))
	if missing.Passed || !strings.Contains(missing.Message, `(2 values: "ord_1", "ord_7")`) {
		t.Fatalf("missing: %+v", missing)
	}
	// An array path is expanded; numbers and numeric strings compare by value.
	num := check(t, rule(domain.RuleContains, domain.RuleIn, constant("42"), plus(resp("list", "ids"))))
	if !num.Passed {
		t.Fatalf("42 in [1, \"42\"]: %+v", num)
	}
}

func TestValuesThatCannotBeRead(t *testing.T) {
	broken := append([]domain.RunResult{}, run...)
	broken = append(broken,
		domain.RunResult{ID: "e", RequestID: "down", Status: domain.StatusError, Error: "connection refused",
			Request: domain.RequestSummary{Name: "Down"}},
		res("html", "Html", "", "<html>", 200))
	cases := map[string]struct {
		left    domain.RuleValue
		message string
	}{
		"no response":    {resp("down", "a"), "Down had no response (connection refused)"},
		"not json":       {resp("html", "a"), "Html: the response body is not JSON"},
		"missing path":   {resp("after", "data.nope"), "Balance after: data.nope not found in the response body"},
		"many values":    {resp("list", "orders[*].id"), "List orders: orders[*].id matches 2 values; choose count, sum, min or max"},
		"missing header": {domain.RuleValue{Source: domain.SourceHeader, RequestID: "after", Path: "ETag"}, "Balance after: the response has no ETag header"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			out := Check([]domain.Rule{rule(domain.RuleCompare, domain.RuleEq, c.left, plus(constant("1")))}, broken)
			if out[0].Skipped || out[0].Result.Passed || out[0].Result.Message != c.message {
				t.Fatalf("got %+v", out[0])
			}
		})
	}
	nan := Check([]domain.Rule{rule(domain.RuleCompare, domain.RuleEq, resp("withdraw", "order.id"),
		plus(constant("1")), plus(constant("2")))}, run)
	if nan[0].Result.Passed || !strings.Contains(nan[0].Result.Message, "not a number") {
		t.Fatalf("arithmetic on a string: %+v", nan[0])
	}
}

func TestEvaluateSkipsAndBuildsResult(t *testing.T) {
	ok := rule(domain.RuleCompare, domain.RuleEq, resp("after", "data.balance"), plus(constant("90")))
	ok.ID = "ok"
	bad := rule(domain.RuleCompare, domain.RuleEq, resp("after", "data.balance"), plus(constant("91")))
	bad.ID = "bad"
	off := ok
	off.ID, off.Enabled = "off", false
	outside := rule(domain.RuleCompare, domain.RuleEq, resp("other-folder", "x"), plus(constant("1")))
	outside.ID = "outside"

	got := Evaluate([]domain.Rule{ok, bad, off, outside}, run, time.Unix(0, 0))
	if got == nil || got.Request.Name != ResultName || got.Request.Method != ResultMethod || got.RequestID != "" {
		t.Fatalf("result %+v", got)
	}
	if len(got.AssertionResults) != 2 || got.Status != domain.StatusFailed {
		t.Fatalf("only enabled rules with all requests in the run count: %+v", got)
	}
	if Evaluate([]domain.Rule{ok}, run, time.Now()).Status != domain.StatusPassed {
		t.Fatal("a passing rule passes the result")
	}
	if Evaluate([]domain.Rule{off, outside}, run, time.Now()) != nil {
		t.Fatal("no applicable rule means no result")
	}
	checked := Check([]domain.Rule{off, outside}, run)
	if !checked[0].Skipped || checked[0].Reason != "the rule is disabled" || !checked[1].Skipped ||
		checked[1].Reason != "1 of its requests were not part of the run" {
		t.Fatalf("skip reasons %+v", checked)
	}
}
