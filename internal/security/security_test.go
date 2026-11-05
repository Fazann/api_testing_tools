package security

import (
	"strings"
	"testing"

	"api-test-kits/internal/domain"
)

func resp(code int, body string) domain.RunResult {
	return domain.RunResult{Status: domain.StatusPassed, Response: &domain.ResponseSummary{StatusCode: code, Body: body}}
}

func TestEvaluateRaceSuccessRules(t *testing.T) {
	results := []domain.RunResult{resp(200, `{"id":1}`), resp(200, `{"id":2}`), resp(409, `{}`)}
	cases := []struct {
		op   domain.Operator
		n    int
		pass bool
	}{
		{domain.OpLte, 1, false}, {domain.OpLte, 2, true}, {domain.OpEq, 2, true},
		{domain.OpEq, 1, false}, {domain.OpGte, 3, false}, {"", 0, true},
	}
	for _, c := range cases {
		rep := EvaluateRace(domain.RaceTest{SuccessOp: c.op, SuccessCount: c.n}, results, nil, nil)
		if rep.Passed != c.pass || rep.Successes != 2 {
			t.Errorf("%s %d: passed=%v successes=%d, want %v", c.op, c.n, rep.Passed, rep.Successes, c.pass)
		}
	}
}

func TestEvaluateRaceServerErrorsAndNoResponse(t *testing.T) {
	results := []domain.RunResult{resp(200, `{}`), resp(500, `{}`), {Status: domain.StatusError, Error: "timeout"}}
	rep := EvaluateRace(domain.RaceTest{}, results, nil, nil)
	if rep.Passed || rep.ServerErrors != 1 || rep.NoResponse != 1 || len(rep.Checks) != 2 {
		t.Fatalf("unexpected report %+v", rep)
	}
	if rep.StatusCounts[0].Code != 0 || rep.StatusCounts[2].Code != 500 {
		t.Fatalf("status counts not sorted: %+v", rep.StatusCounts)
	}
	rep = EvaluateRace(domain.RaceTest{AllowServerErrors: true}, results[:2], nil, nil)
	if !rep.Passed {
		t.Fatalf("5xx is allowed: %+v", rep.Checks)
	}
}

func TestEvaluateRaceUniquePath(t *testing.T) {
	big := `{"tx":{"id":9007199254740993}}`
	results := []domain.RunResult{resp(201, big), resp(201, `{"tx":{"id":9007199254740992}}`), resp(409, big)}
	rep := EvaluateRace(domain.RaceTest{UniquePath: "tx.id"}, results, nil, nil)
	if !rep.Passed {
		t.Fatalf("large IDs differ and must not collide as floats: %+v", rep.Checks)
	}
	results[1] = resp(201, big)
	rep = EvaluateRace(domain.RaceTest{UniquePath: "tx.id"}, results, nil, nil)
	if rep.Passed || !strings.Contains(rep.Checks[1].Message, "9007199254740993") {
		t.Fatalf("duplicate not reported: %+v", rep.Checks)
	}
	results[1] = resp(201, `{}`)
	if rep := EvaluateRace(domain.RaceTest{UniquePath: "tx.id"}, results, nil, nil); rep.Passed {
		t.Fatal("a missing value must fail the uniqueness check")
	}
}

func TestEvaluateRaceStateCheck(t *testing.T) {
	results := []domain.RunResult{resp(200, ``), resp(200, ``), resp(422, ``)}
	check := &domain.RaceCheck{Path: "balance", DeltaPerSuccess: -10, Tolerance: 0.001}
	tt := domain.RaceTest{Check: check}
	rep := EvaluateRace(tt, results, &StateValue{Value: 100}, &StateValue{Value: 80})
	if !rep.Passed || *rep.Before != 100 || *rep.After != 80 {
		t.Fatalf("100 → 80 with 2 successes of -10 should pass: %+v", rep.Checks)
	}
	rep = EvaluateRace(tt, results, &StateValue{Value: 100}, &StateValue{Value: 90})
	if rep.Passed || !strings.Contains(rep.Checks[1].Message, "lost update") {
		t.Fatalf("lost update not detected: %+v", rep.Checks)
	}
	rep = EvaluateRace(tt, results, &StateValue{Value: 100}, &StateValue{Value: 70})
	if rep.Passed || !strings.Contains(rep.Checks[1].Message, "more often") {
		t.Fatalf("over-application not detected: %+v", rep.Checks)
	}
	rep = EvaluateRace(tt, results, &StateValue{Err: "401"}, &StateValue{Value: 70})
	if rep.Passed {
		t.Fatal("an unreadable state must fail")
	}
}

func TestReadNumber(t *testing.T) {
	if v := ReadNumber(resp(200, `{"a":{"b":"12.5"}}`), "a.b"); v.Err != "" || v.Value != 12.5 {
		t.Fatalf("numeric string: %+v", v)
	}
	if v := ReadNumber(resp(200, `{"a":true}`), "a"); v.Err == "" {
		t.Fatal("boolean must not be read as a number")
	}
	if v := ReadNumber(resp(500, `{"a":1}`), "a"); v.Err == "" {
		t.Fatal("a failed check request must not be read")
	}
}

func TestClassifyAndJudge(t *testing.T) {
	cases := []struct {
		code     int
		expected domain.Access
		outcome  Outcome
		verdict  Verdict
	}{
		{200, domain.AccessAllow, OutcomeAllowed, VerdictPass},
		{302, domain.AccessAllow, OutcomeAllowed, VerdictPass},
		{403, domain.AccessAllow, OutcomeDenied, VerdictFail},
		{200, domain.AccessDeny, OutcomeAllowed, VerdictFail},
		{401, domain.AccessDeny, OutcomeDenied, VerdictPass},
		{404, domain.AccessDeny, OutcomeNotFound, VerdictPass},
		{422, domain.AccessDeny, OutcomeRejected, VerdictWarn},
		{500, domain.AccessDeny, OutcomeServerError, VerdictFail},
		{200, domain.AccessUnset, OutcomeAllowed, VerdictUnset},
	}
	for _, c := range cases {
		o := Classify(resp(c.code, ""))
		v, _ := Judge(c.expected, o, c.code, "Bob")
		if o != c.outcome || v != c.verdict {
			t.Errorf("%d expected %q: got %s/%s, want %s/%s", c.code, c.expected, o, v, c.outcome, c.verdict)
		}
	}
	if o := Classify(domain.RunResult{Status: domain.StatusError}); o != OutcomeError {
		t.Fatalf("no response: %s", o)
	}
	if v, _ := Judge(domain.AccessAllow, OutcomeError, 0, "x"); v != VerdictError {
		t.Fatalf("no response verdict: %s", v)
	}
	if SuggestedAccess(OutcomeNotFound) != domain.AccessDeny || SuggestedAccess(OutcomeRejected) != domain.AccessUnset {
		t.Fatal("suggestions")
	}
}
