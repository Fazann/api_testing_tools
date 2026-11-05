package security

import (
	"fmt"

	"api-test-kits/internal/domain"
)

// Outcome classifies what happened when a role called an endpoint.
type Outcome string

const (
	// OutcomeAllowed is a 2xx or 3xx response.
	OutcomeAllowed Outcome = "allowed"
	// OutcomeDenied is 401 or 403.
	OutcomeDenied Outcome = "denied"
	// OutcomeNotFound is 404, which many APIs use to hide resources that
	// belong to someone else. It satisfies a "deny" expectation.
	OutcomeNotFound Outcome = "not_found"
	// OutcomeRejected is any other 4xx (400, 405, 409, 422, …).
	OutcomeRejected    Outcome = "rejected"
	OutcomeServerError Outcome = "server_error"
	// OutcomeError means there was no response.
	OutcomeError Outcome = "error"
)

// Verdict is the result of comparing an outcome with the expectation.
type Verdict string

const (
	VerdictPass Verdict = "pass"
	VerdictFail Verdict = "fail"
	// VerdictWarn is suspicious but not proven wrong, for example a 400 where
	// "deny" was expected: the request got past authentication to validation.
	VerdictWarn Verdict = "warn"
	// VerdictUnset means there is no expectation for the cell yet.
	VerdictUnset Verdict = "unset"
	// VerdictError means the request could not be made.
	VerdictError Verdict = "error"
)

// Classify returns the outcome of one result.
func Classify(r domain.RunResult) Outcome {
	if r.Response == nil {
		return OutcomeError
	}
	c := r.Response.StatusCode
	switch {
	case c >= 200 && c < 400:
		return OutcomeAllowed
	case c == 401 || c == 403:
		return OutcomeDenied
	case c == 404:
		return OutcomeNotFound
	case c >= 400 && c < 500:
		return OutcomeRejected
	case c >= 500:
		return OutcomeServerError
	}
	return OutcomeError
}

// SuggestedAccess is the expectation that would make outcome pass, or unset
// when the outcome is ambiguous. It is used to approve a run's results.
func SuggestedAccess(o Outcome) domain.Access {
	switch o {
	case OutcomeAllowed:
		return domain.AccessAllow
	case OutcomeDenied, OutcomeNotFound:
		return domain.AccessDeny
	}
	return domain.AccessUnset
}

// Judge compares an outcome with the expectation and explains a mismatch.
func Judge(expected domain.Access, o Outcome, status int, role string) (Verdict, string) {
	if o == OutcomeError {
		return VerdictError, "no response"
	}
	switch expected {
	case domain.AccessAllow:
		switch o {
		case OutcomeAllowed:
			return VerdictPass, ""
		case OutcomeServerError:
			return VerdictFail, fmt.Sprintf("%s should be allowed but the server failed with %d", role, status)
		default:
			return VerdictFail, fmt.Sprintf("%s should be allowed but got %d — access was lost", role, status)
		}
	case domain.AccessDeny:
		switch o {
		case OutcomeDenied, OutcomeNotFound:
			return VerdictPass, ""
		case OutcomeAllowed:
			return VerdictFail, fmt.Sprintf("%s should be denied but got %d — broken access control", role, status)
		case OutcomeRejected:
			return VerdictWarn, fmt.Sprintf("%s got %d instead of 401/403: the request passed authorization and reached validation", role, status)
		default:
			return VerdictFail, fmt.Sprintf("%s should be denied but the server failed with %d", role, status)
		}
	}
	return VerdictUnset, ""
}

// MatrixCell is one role × request result.
type MatrixCell struct {
	RequestID  string        `json:"requestId"`
	RoleID     string        `json:"roleId"`
	Expected   domain.Access `json:"expected"`
	Outcome    Outcome       `json:"outcome"`
	StatusCode int           `json:"statusCode"`
	DurationMs int64         `json:"durationMs"`
	Verdict    Verdict       `json:"verdict"`
	Message    string        `json:"message,omitempty"`
	// ResultID links to the stored result in the run's history.
	ResultID string `json:"resultId"`
	// Suggested is the expectation that matches this outcome.
	Suggested domain.Access `json:"suggested"`
}

// MatrixRequest describes a row of the matrix.
type MatrixRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Method string `json:"method"`
}

// MatrixRole describes a column of the matrix, without its variables.
type MatrixRole struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Anonymous bool   `json:"anonymous"`
}

// MatrixReport is the outcome of one permission-matrix run.
type MatrixReport struct {
	RunID     string          `json:"runId"`
	Passed    bool            `json:"passed"`
	Cancelled bool            `json:"cancelled"`
	Roles     []MatrixRole    `json:"roles"`
	Requests  []MatrixRequest `json:"requests"`
	Cells     []MatrixCell    `json:"cells"`
	Passes    int             `json:"passes"`
	Failures  int             `json:"failures"`
	Warnings  int             `json:"warnings"`
	Unset     int             `json:"unset"`
	Errors    int             `json:"errors"`
}

// Tally counts verdicts and sets Passed.
func (m *MatrixReport) Tally() {
	m.Passes, m.Failures, m.Warnings, m.Unset, m.Errors = 0, 0, 0, 0, 0
	for _, c := range m.Cells {
		switch c.Verdict {
		case VerdictPass:
			m.Passes++
		case VerdictFail:
			m.Failures++
		case VerdictWarn:
			m.Warnings++
		case VerdictUnset:
			m.Unset++
		case VerdictError:
			m.Errors++
		}
	}
	m.Passed = !m.Cancelled && m.Failures == 0 && m.Errors == 0
}
