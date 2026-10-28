package assertions

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	"api-test-kits/internal/domain"
)

// Response is the data assertions are evaluated against.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Duration   time.Duration
	// Truncated responses cannot be reliably parsed as JSON.
	Truncated bool
}

// Evaluator evaluates assertions for one response, parsing the body at most once.
type Evaluator struct {
	resp     Response
	parsed   bool
	doc      any
	parseErr error
}

func NewEvaluator(resp Response) *Evaluator { return &Evaluator{resp: resp} }

// Evaluate runs all enabled assertions. It never panics on malformed input.
func Evaluate(resp Response, list []domain.Assertion) []domain.AssertionResult {
	e := NewEvaluator(resp)
	out := make([]domain.AssertionResult, 0, len(list))
	for _, a := range list {
		if !a.Enabled {
			continue
		}
		out = append(out, e.Evaluate(a))
	}
	return out
}

// JSON returns the parsed body, or an error if it is not valid JSON.
func (e *Evaluator) JSON() (any, error) {
	if !e.parsed {
		e.parsed = true
		if e.resp.Truncated {
			e.parseErr = fmt.Errorf("response body was truncated at the size limit")
		} else if len(strings.TrimSpace(string(e.resp.Body))) == 0 {
			e.parseErr = fmt.Errorf("response body is empty")
		} else {
			e.doc, e.parseErr = DecodeJSON(e.resp.Body)
		}
	}
	return e.doc, e.parseErr
}

// Evaluate runs a single assertion.
func (e *Evaluator) Evaluate(a domain.Assertion) (res domain.AssertionResult) {
	res = domain.AssertionResult{
		AssertionID: a.ID, Kind: a.Kind, Path: a.Path, Operator: a.Operator, Description: a.Description,
	}
	defer func() {
		if r := recover(); r != nil {
			res.Passed = false
			res.Message = fmt.Sprintf("internal error evaluating assertion: %v", r)
		}
	}()
	if err := a.Validate(); err != nil {
		res.Message = "invalid assertion: " + err.Error()
		return res
	}
	var expected any
	if len(a.Expected) > 0 {
		var err error
		if expected, err = DecodeJSON(a.Expected); err != nil {
			res.Message = "invalid expected value: " + err.Error()
			return res
		}
	}
	if expected != nil || len(a.Expected) > 0 {
		res.Expected = Format(expected)
	}

	switch a.Kind {
	case domain.AssertStatus:
		e.evalStatus(a, expected, &res)
	case domain.AssertHeader:
		e.evalHeader(a, expected, &res)
	case domain.AssertJSONPath:
		e.evalJSONPath(a, expected, &res)
	case domain.AssertResponseTime:
		ms := float64(e.resp.Duration.Milliseconds())
		res.Actual = fmt.Sprintf("%d ms", int64(ms))
		res.Expected = fmt.Sprintf("%s %s ms", opSymbol(a.Operator), res.Expected)
		ok, msg := compare(a.Operator, json.Number(fmt.Sprint(int64(ms))), expected, 0)
		res.Passed, res.Message = ok, orDefault(msg, "response time", ok)
	case domain.AssertValidJSON:
		res.Expected = "valid JSON"
		if _, err := e.JSON(); err != nil {
			res.Actual = "invalid"
			res.Message = "response body is not valid JSON: " + err.Error()
		} else {
			res.Actual = "valid JSON"
			res.Passed = true
			res.Message = "response body is valid JSON"
		}
	case domain.AssertRequiredFields:
		e.evalRequired(expected, &res)
	case domain.AssertBody:
		body := string(e.resp.Body)
		res.Actual = truncate(body)
		ok, msg := compare(a.Operator, body, expected, 0)
		res.Passed, res.Message = ok, orDefault(msg, "response body", ok)
	case domain.AssertAggregate:
		e.evalAggregate(a, expected, &res)
	}
	return res
}

func (e *Evaluator) evalStatus(a domain.Assertion, expected any, res *domain.AssertionResult) {
	res.Actual = fmt.Sprint(e.resp.StatusCode)
	ok, msg := compare(a.Operator, json.Number(res.Actual), expected, 0)
	res.Passed = ok
	if ok {
		res.Message = fmt.Sprintf("status %d %s %s", e.resp.StatusCode, opSymbol(a.Operator), res.Expected)
	} else {
		res.Message = fmt.Sprintf("expected status %s %s but got %d", opSymbol(a.Operator), res.Expected, e.resp.StatusCode)
		if msg != "" && !strings.HasPrefix(msg, "expected") {
			res.Message += ": " + msg
		}
	}
}

func (e *Evaluator) evalHeader(a domain.Assertion, expected any, res *domain.AssertionResult) {
	vals := e.resp.Header.Values(a.Path)
	present := len(vals) > 0
	joined := strings.Join(vals, ", ")
	if present {
		res.Actual = truncate(joined)
	} else {
		res.Actual = "(missing)"
	}
	switch a.Operator {
	case domain.OpExists:
		res.Passed = present
		res.Expected = "present"
	case domain.OpNotExists:
		res.Passed = !present
		res.Expected = "absent"
	default:
		if !present {
			res.Message = fmt.Sprintf("header %s is missing", a.Path)
			return
		}
		ok, msg := compare(a.Operator, joined, expected, 0)
		res.Passed, res.Message = ok, msg
	}
	if res.Message == "" {
		res.Message = orDefault("", "header "+a.Path, res.Passed)
	}
}

func (e *Evaluator) evalJSONPath(a domain.Assertion, expected any, res *domain.AssertionResult) {
	doc, err := e.JSON()
	if err != nil {
		res.Actual = "(no JSON)"
		res.Message = "cannot evaluate JSON path: " + err.Error()
		return
	}
	segs, err := ParsePath(a.Path)
	if err != nil {
		res.Message = err.Error()
		return
	}
	matches := SelectSegments(doc, segs)
	switch a.Operator {
	case domain.OpExists:
		res.Expected = "exists"
		res.Passed = len(matches) > 0
		res.Actual = describeMatches(matches)
		if res.Passed {
			res.Message = fmt.Sprintf("%s exists", a.Path)
		} else {
			res.Message = fmt.Sprintf("%s does not exist", a.Path)
		}
		return
	case domain.OpNotExists:
		res.Expected = "does not exist"
		res.Passed = len(matches) == 0
		res.Actual = describeMatches(matches)
		if res.Passed {
			res.Message = fmt.Sprintf("%s is absent", a.Path)
		} else {
			res.Message = fmt.Sprintf("%s should not exist but is %s", a.Path, res.Actual)
		}
		return
	}
	if len(matches) == 0 {
		res.Actual = "(missing)"
		res.Message = fmt.Sprintf("%s does not exist", a.Path)
		return
	}
	expected, ok := e.resolveExpected(expected, res)
	if !ok {
		return
	}
	res.Actual = describeMatches(matches)
	for _, m := range matches {
		var pass bool
		var msg string
		if a.Operator == domain.OpType {
			want, _ := expected.(string)
			got := TypeName(m.Value)
			pass = got == want || want == "number" && got == "integer"
			if !pass {
				msg = fmt.Sprintf("%s has type %s, expected %s", m.Path, got, want)
			}
		} else {
			pass, msg = compare(a.Operator, m.Value, expected, a.Tolerance)
			if !pass && msg != "" {
				msg = m.Path + ": " + msg
			}
		}
		if !pass {
			res.Message = msg
			return
		}
	}
	res.Passed = true
	if len(matches) > 1 {
		res.Message = fmt.Sprintf("all %d values at %s satisfy %s %s", len(matches), a.Path, opSymbol(a.Operator), res.Expected)
	} else {
		res.Message = fmt.Sprintf("%s %s %s", a.Path, opSymbol(a.Operator), res.Expected)
	}
}

func (e *Evaluator) evalRequired(expected any, res *domain.AssertionResult) {
	doc, err := e.JSON()
	if err != nil {
		res.Actual = "(no JSON)"
		res.Message = "cannot check required fields: " + err.Error()
		return
	}
	arr, _ := expected.([]any)
	var missing []string
	for _, item := range arr {
		p, _ := item.(string)
		m, err := Select(doc, p)
		if err != nil || len(m) == 0 {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		res.Passed = true
		res.Actual = "all present"
		res.Message = fmt.Sprintf("all %d required fields are present", len(arr))
		return
	}
	res.Actual = "missing: " + strings.Join(missing, ", ")
	res.Message = fmt.Sprintf("%d of %d required fields are missing: %s", len(missing), len(arr), strings.Join(missing, ", "))
}

func (e *Evaluator) evalAggregate(a domain.Assertion, expected any, res *domain.AssertionResult) {
	doc, err := e.JSON()
	if err != nil {
		res.Actual = "(no JSON)"
		res.Message = "cannot evaluate aggregate: " + err.Error()
		return
	}
	segs, err := ParsePath(a.Path)
	if err != nil {
		res.Message = err.Error()
		return
	}
	matches := SelectSegments(doc, segs)
	values := make([]any, 0, len(matches))
	for _, m := range matches {
		values = append(values, m.Value)
	}
	// count(data.items) counts the array's elements rather than one match.
	if !HasWildcard(segs) && len(values) == 1 {
		if arr, ok := values[0].([]any); ok {
			values = arr
		}
	}
	var result float64
	switch a.Function {
	case domain.AggCount:
		result = float64(len(values))
	default:
		if len(values) == 0 {
			if a.Function == domain.AggSum {
				break
			}
			res.Actual = "(no values)"
			res.Message = fmt.Sprintf("%s(%s) has no values", a.Function, a.Path)
			return
		}
		for i, v := range values {
			f, ok := ToFloat(v)
			if !ok {
				res.Actual = Format(v)
				res.Message = fmt.Sprintf("%s(%s): value %d is %s, not a number", a.Function, a.Path, i, TypeName(v))
				return
			}
			switch {
			case a.Function == domain.AggSum:
				result += f
			case i == 0:
				result = f
			case a.Function == domain.AggMin:
				result = math.Min(result, f)
			case a.Function == domain.AggMax:
				result = math.Max(result, f)
			}
		}
	}
	expected, ok := e.resolveExpected(expected, res)
	if !ok {
		return
	}
	res.Actual = formatFloat(result)
	label := fmt.Sprintf("%s(%s)", a.Function, a.Path)
	pass, msg := compare(a.Operator, result, expected, a.Tolerance)
	res.Passed = pass
	if pass {
		res.Message = fmt.Sprintf("%s = %s %s %s", label, res.Actual, opSymbol(a.Operator), res.Expected)
		if a.Tolerance > 0 {
			res.Message += fmt.Sprintf(" (tolerance %s)", formatFloat(a.Tolerance))
		}
	} else {
		res.Message = label + ": " + msg
	}
}

// resolveExpected follows a {"$path": "..."} reference into the response.
func (e *Evaluator) resolveExpected(expected any, res *domain.AssertionResult) (any, bool) {
	ref, ok := domain.ExpectedPathRef(expected)
	if !ok {
		return expected, true
	}
	doc, err := e.JSON()
	if err != nil {
		res.Message = "cannot resolve $path reference: " + err.Error()
		return nil, false
	}
	m, err := Select(doc, ref)
	if err != nil {
		res.Message = err.Error()
		return nil, false
	}
	if len(m) != 1 {
		res.Message = fmt.Sprintf("reference %s matched %d values, expected exactly 1", ref, len(m))
		return nil, false
	}
	res.Expected = fmt.Sprintf("%s (from %s)", Format(m[0].Value), ref)
	return m[0].Value, true
}

// compare applies op to actual and expected. It returns a failure message
// when the comparison does not hold.
func compare(op domain.Operator, actual, expected any, tolerance float64) (bool, string) {
	switch op {
	case domain.OpEq, domain.OpNeq:
		eq := valuesEqual(actual, expected, tolerance)
		if op == domain.OpEq {
			if eq {
				return true, ""
			}
			if TypeName(actual) != TypeName(expected) && !(isNumber(actual) && isNumber(expected)) {
				return false, fmt.Sprintf("expected %s (%s) but got %s (%s)", Format(expected), TypeName(expected), Format(actual), TypeName(actual))
			}
			return false, fmt.Sprintf("expected %s but got %s", Format(expected), Format(actual))
		}
		if !eq {
			return true, ""
		}
		return false, fmt.Sprintf("expected a value other than %s", Format(expected))
	case domain.OpGt, domain.OpGte, domain.OpLt, domain.OpLte:
		a, okA := ToFloat(actual)
		b, okB := ToFloat(expected)
		if !okA {
			return false, fmt.Sprintf("actual value %s is %s, not a number", Format(actual), TypeName(actual))
		}
		if !okB {
			return false, fmt.Sprintf("expected value %s is not a number", Format(expected))
		}
		var ok bool
		switch op {
		case domain.OpGt:
			ok = a > b
		case domain.OpGte:
			ok = a >= b
		case domain.OpLt:
			ok = a < b
		case domain.OpLte:
			ok = a <= b
		}
		if ok {
			return true, ""
		}
		return false, fmt.Sprintf("expected %s %s %s", formatFloat(a), opSymbol(op), formatFloat(b))
	case domain.OpContains, domain.OpNotContains:
		found, err := contains(actual, expected)
		if err != nil {
			return false, err.Error()
		}
		if op == domain.OpContains {
			if found {
				return true, ""
			}
			return false, fmt.Sprintf("%s does not contain %s", Format(actual), Format(expected))
		}
		if !found {
			return true, ""
		}
		return false, fmt.Sprintf("%s should not contain %s", Format(actual), Format(expected))
	case domain.OpMatches:
		s, ok := actual.(string)
		if !ok {
			return false, fmt.Sprintf("value %s is %s, not a string", Format(actual), TypeName(actual))
		}
		pattern, _ := expected.(string)
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, "invalid pattern: " + err.Error()
		}
		if re.MatchString(s) {
			return true, ""
		}
		return false, fmt.Sprintf("%s does not match /%s/", Format(s), pattern)
	}
	return false, fmt.Sprintf("unsupported operator %q", op)
}

func contains(actual, expected any) (bool, error) {
	switch a := actual.(type) {
	case string:
		s, ok := expected.(string)
		if !ok {
			s = Format(expected)
		}
		return strings.Contains(a, s), nil
	case []any:
		for _, item := range a {
			if valuesEqual(item, expected, 0) {
				return true, nil
			}
		}
		return false, nil
	case map[string]any:
		key, ok := expected.(string)
		if !ok {
			return false, fmt.Errorf("object contains-check needs a key name")
		}
		_, found := a[key]
		return found, nil
	}
	return false, fmt.Errorf("contains is not supported for %s values", TypeName(actual))
}

// valuesEqual compares JSON values deeply. Numbers compare numerically
// (with optional absolute tolerance); object key order is irrelevant.
func valuesEqual(a, b any, tolerance float64) bool {
	if fa, ok := ToFloat(a); ok {
		fb, ok := ToFloat(b)
		if !ok {
			return false
		}
		if na, okA := a.(json.Number); okA {
			if nb, okB := b.(json.Number); okB && na == nb {
				return true
			}
		}
		return math.Abs(fa-fb) <= tolerance
	}
	switch av := a.(type) {
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !valuesEqual(av[i], bv[i], tolerance) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			w, ok := bv[k]
			if !ok || !valuesEqual(v, w, tolerance) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

// ValuesEqual is exported for the regression comparator.
func ValuesEqual(a, b any, tolerance float64) bool { return valuesEqual(a, b, tolerance) }

func isNumber(v any) bool { _, ok := ToFloat(v); return ok }

func describeMatches(m []Match) string {
	switch len(m) {
	case 0:
		return "(missing)"
	case 1:
		return Format(m[0].Value)
	}
	vals := make([]any, len(m))
	for i := range m {
		vals[i] = m[i].Value
	}
	return Format(vals)
}

func formatFloat(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

func opSymbol(op domain.Operator) string {
	switch op {
	case domain.OpEq:
		return "="
	case domain.OpNeq:
		return "≠"
	case domain.OpGt:
		return ">"
	case domain.OpGte:
		return "≥"
	case domain.OpLt:
		return "<"
	case domain.OpLte:
		return "≤"
	}
	return strings.ReplaceAll(string(op), "_", " ")
}

func orDefault(msg, subject string, passed bool) string {
	if msg != "" {
		return msg
	}
	if passed {
		return subject + " check passed"
	}
	return subject + " check failed"
}
