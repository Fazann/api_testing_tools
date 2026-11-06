// Package rules evaluates collection rules: checks that combine values from
// several requests of one collection run, such as "the balance after equals
// the balance before minus the amount". It has no dependency on the desktop
// layer, so collection runs in the app and the CLI share it.
package rules

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
)

// ResultName is the name of the run result that holds the rule checks.
const ResultName = "Collection rules"

// ResultMethod is shown in the method column for that result.
const ResultMethod = "RULES"

// maxListed is how many list values a failed "appears in" message shows.
const maxListed = 5

// Outcome is the evaluation of one rule.
type Outcome struct {
	RuleID string `json:"ruleId"`
	// Skipped is set when the rule did not run; Reason says why.
	Skipped bool                   `json:"skipped"`
	Reason  string                 `json:"reason,omitempty"`
	Result  domain.AssertionResult `json:"result"`
}

// Check evaluates every rule against the results of one run, including
// disabled ones (reported as skipped), for the rule editor.
func Check(list []domain.Rule, results []domain.RunResult) []Outcome {
	e := newEval(results)
	out := make([]Outcome, 0, len(list))
	for _, r := range list {
		o := Outcome{RuleID: r.ID}
		switch reason := e.skipReason(r); {
		case !r.Enabled:
			o.Skipped, o.Reason = true, "the rule is disabled"
		case reason != "":
			o.Skipped, o.Reason = true, reason
		default:
			o.Result = e.rule(r)
		}
		out = append(out, o)
	}
	return out
}

// Evaluate runs the enabled rules whose requests were all part of the run and
// returns them as one run result, or nil when no rule applies.
func Evaluate(list []domain.Rule, results []domain.RunResult, now time.Time) *domain.RunResult {
	var checks []domain.AssertionResult
	for _, o := range Check(list, results) {
		if !o.Skipped {
			checks = append(checks, o.Result)
		}
	}
	if len(checks) == 0 {
		return nil
	}
	res := &domain.RunResult{
		ID:               uuid.NewString(),
		Request:          domain.RequestSummary{Name: ResultName, Method: ResultMethod},
		AssertionResults: checks,
		Status:           domain.StatusPassed,
		CreatedAt:        now.UTC(),
	}
	for _, c := range checks {
		if !c.Passed {
			res.Status = domain.StatusFailed
		}
	}
	return res
}

type eval struct {
	byID map[string]*domain.RunResult
	docs map[string]any
	errs map[string]error
}

func newEval(results []domain.RunResult) *eval {
	e := &eval{byID: map[string]*domain.RunResult{}, docs: map[string]any{}, errs: map[string]error{}}
	for i := range results {
		if id := results[i].RequestID; id != "" {
			e.byID[id] = &results[i]
		}
	}
	return e
}

func (e *eval) skipReason(r domain.Rule) string {
	var missing []string
	for _, id := range r.RequestIDs() {
		if _, ok := e.byID[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("%d of its requests were not part of the run", len(missing))
}

// rule evaluates one rule; a value that cannot be read fails it.
func (e *eval) rule(r domain.Rule) domain.AssertionResult {
	res := domain.AssertionResult{AssertionID: r.ID, Kind: domain.AssertRule, Description: r.Name}
	fail := func(err error) domain.AssertionResult {
		res.Passed = false
		res.Message = err.Error()
		return res
	}
	left, err := e.scalar(r.Left)
	if err != nil {
		return fail(err)
	}
	if r.Kind == domain.RuleContains {
		list, err := e.list(r.Right[0].Value)
		if err != nil {
			return fail(err)
		}
		return contains(res, r, left, list, e.describe(r.Right[0].Value))
	}
	terms := make([]term, len(r.Right))
	for i, t := range r.Right {
		v, err := e.scalar(t.Value)
		if err != nil {
			return fail(err)
		}
		terms[i] = term{sign: t.Sign, val: v}
	}
	return compare(res, r, left, terms)
}

type term struct {
	sign string
	val  any
}

var opSymbol = map[string]string{
	domain.RuleEq: "=", domain.RuleNe: "≠", domain.RuleGt: ">", domain.RuleGte: "≥", domain.RuleLt: "<", domain.RuleLte: "≤",
}

// negated is the symbol shown when an operator does not hold.
var negated = map[string]string{
	domain.RuleEq: "≠", domain.RuleNe: "=", domain.RuleGt: "is not >", domain.RuleGte: "is not ≥",
	domain.RuleLt: "is not <", domain.RuleLte: "is not ≤",
}

func compare(res domain.AssertionResult, r domain.Rule, left any, terms []term) domain.AssertionResult {
	numeric := len(terms) > 1 || terms[0].sign == "-" || (r.Operator != domain.RuleEq && r.Operator != domain.RuleNe)
	if !numeric {
		equal := scalarEqual(left, terms[0].val, r.Tolerance)
		res.Passed = equal == (r.Operator == domain.RuleEq)
		res.Actual, res.Expected = show(left), opSymbol[r.Operator]+" "+show(terms[0].val)
		sym := opSymbol[r.Operator]
		if !res.Passed {
			sym = negated[r.Operator]
		}
		res.Message = fmt.Sprintf("%s %s %s", show(left), sym, show(terms[0].val))
		return res
	}
	lf, ok := assertions.ToFloat(left)
	if !ok {
		res.Actual = show(left)
		res.Message = fmt.Sprintf("the left value is %s %s, not a number", assertions.TypeName(left), show(left))
		return res
	}
	var sum float64
	var expr strings.Builder
	for i, t := range terms {
		f, ok := assertions.ToFloat(t.val)
		if !ok {
			res.Actual = assertions.FormatFloat(lf)
			res.Message = fmt.Sprintf("right value %d is %s %s, not a number", i+1, assertions.TypeName(t.val), show(t.val))
			return res
		}
		switch {
		case t.sign == "-":
			sum -= f
			if i == 0 {
				expr.WriteString("−")
			} else {
				expr.WriteString(" − ")
			}
		case i > 0:
			sum += f
			expr.WriteString(" + ")
		default:
			sum += f
		}
		expr.WriteString(assertions.FormatFloat(f))
	}
	tol := r.Tolerance
	switch r.Operator {
	case domain.RuleEq:
		res.Passed = math.Abs(lf-sum) <= tol
	case domain.RuleNe:
		res.Passed = math.Abs(lf-sum) > tol
	case domain.RuleGt:
		res.Passed = lf > sum
	case domain.RuleGte:
		res.Passed = lf >= sum-tol
	case domain.RuleLt:
		res.Passed = lf < sum
	case domain.RuleLte:
		res.Passed = lf <= sum+tol
	}
	rhs := expr.String()
	if len(terms) > 1 || terms[0].sign == "-" {
		rhs += " = " + assertions.FormatFloat(sum)
		if res.Passed {
			rhs = expr.String() // "90 = 100 − 10" reads better without repeating the result
		}
	}
	res.Actual = assertions.FormatFloat(lf)
	res.Expected = opSymbol[r.Operator] + " " + assertions.FormatFloat(sum)
	sym := opSymbol[r.Operator]
	if !res.Passed {
		sym = negated[r.Operator]
	}
	res.Message = fmt.Sprintf("%s %s %s", assertions.FormatFloat(lf), sym, rhs)
	if tol > 0 {
		res.Message += fmt.Sprintf(" (tolerance %s)", assertions.FormatFloat(tol))
	}
	return res
}

func contains(res domain.AssertionResult, r domain.Rule, left any, list []any, where string) domain.AssertionResult {
	found := false
	for _, v := range list {
		if scalarEqual(left, v, 0) {
			found = true
			break
		}
	}
	res.Passed = found == (r.Operator == domain.RuleIn)
	res.Actual = show(left)
	if r.Operator == domain.RuleIn {
		res.Expected = "appears in " + where
	} else {
		res.Expected = "does not appear in " + where
	}
	verb := "appears in"
	if !found {
		verb = "does not appear in"
	}
	res.Message = fmt.Sprintf("%s %s %s", show(left), verb, where)
	if !res.Passed && !found {
		shown := make([]string, 0, maxListed)
		for i, v := range list {
			if i == maxListed {
				shown = append(shown, "…")
				break
			}
			shown = append(shown, show(v))
		}
		res.Message += fmt.Sprintf(" (%d values: %s)", len(list), strings.Join(shown, ", "))
	}
	return res
}

// scalarEqual compares JSON values; numbers by value (within tol), and a
// number equals a string holding the same number ("42" and 42).
func scalarEqual(a, b any, tol float64) bool {
	fa, okA := number(a)
	fb, okB := number(b)
	if okA && okB {
		return math.Abs(fa-fb) <= tol
	}
	return assertions.ValuesEqual(a, b, 0)
}

// number reads a JSON number, or a string that holds exactly one ("42", "-1.5").
func number(v any) (float64, bool) {
	if f, ok := assertions.ToFloat(v); ok {
		return f, true
	}
	if s, ok := v.(string); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return f, err == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
	}
	return 0, false
}

func show(v any) string {
	if f, ok := assertions.ToFloat(v); ok {
		return assertions.FormatFloat(f)
	}
	return assertions.Format(v)
}

// describe names a value for messages, e.g. "List orders → orders[*].id".
func (e *eval) describe(v domain.RuleValue) string {
	if v.Source == domain.SourceConstant {
		return show(v.ConstantValue())
	}
	name := v.RequestID
	if res, ok := e.byID[v.RequestID]; ok {
		name = res.Request.Name
	}
	switch v.Source {
	case domain.SourceStatus:
		return name + " → status"
	case domain.SourceHeader:
		return name + " → header " + v.Path
	}
	where := v.Path
	if where == "" {
		where = "$"
	}
	if v.Source == domain.SourceRequest {
		where = "request " + where
	}
	if v.Aggregate != "" {
		where = v.Aggregate + "(" + where + ")"
	}
	return name + " → " + where
}

// scalar reads one value; a path that matches several values needs an aggregate.
func (e *eval) scalar(v domain.RuleValue) (any, error) {
	if v.Source == domain.SourceConstant {
		return v.ConstantValue(), nil
	}
	res := e.byID[v.RequestID]
	name := res.Request.Name
	switch v.Source {
	case domain.SourceStatus:
		if res.Response == nil {
			return nil, noResponse(res)
		}
		return res.Response.StatusCode, nil
	case domain.SourceHeader:
		if res.Response == nil {
			return nil, noResponse(res)
		}
		var vals []string
		for _, h := range res.Response.Headers {
			if strings.EqualFold(h.Key, v.Path) {
				vals = append(vals, h.Value)
			}
		}
		if len(vals) == 0 {
			return nil, fmt.Errorf("%s: the response has no %s header", name, v.Path)
		}
		return strings.Join(vals, ", "), nil
	}
	values, wildcard, err := e.matches(v)
	if err != nil {
		return nil, err
	}
	if v.Aggregate != "" {
		return aggregate(v, name, values, wildcard)
	}
	switch len(values) {
	case 0:
		return nil, fmt.Errorf("%s: %s not found in the %s body", name, v.Path, bodyName(v))
	case 1:
		return values[0], nil
	}
	return nil, fmt.Errorf("%s: %s matches %d values; choose count, sum, min or max", name, v.Path, len(values))
}

// list reads the values a path selects; one array is expanded to its items.
func (e *eval) list(v domain.RuleValue) ([]any, error) {
	if v.Source == domain.SourceStatus || v.Source == domain.SourceHeader || v.Aggregate != "" {
		x, err := e.scalar(v)
		if err != nil {
			return nil, err
		}
		return []any{x}, nil
	}
	values, wildcard, err := e.matches(v)
	if err != nil {
		return nil, err
	}
	if !wildcard && len(values) == 1 {
		if arr, ok := values[0].([]any); ok {
			return arr, nil
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s: %s not found in the %s body", e.byID[v.RequestID].Request.Name, v.Path, bodyName(v))
	}
	return values, nil
}

// matches returns the values a body path matches and whether it has a wildcard.
func (e *eval) matches(v domain.RuleValue) ([]any, bool, error) {
	res := e.byID[v.RequestID]
	doc, err := e.doc(res, v.Source)
	if err != nil {
		return nil, false, err
	}
	if v.Path == "" || v.Path == "$" {
		return []any{doc}, false, nil
	}
	segs, err := assertions.ParsePath(v.Path)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", res.Request.Name, err)
	}
	matches := assertions.SelectSegments(doc, segs)
	values := make([]any, len(matches))
	for i, m := range matches {
		values[i] = m.Value
	}
	return values, assertions.HasWildcard(segs), nil
}

// doc decodes a result's response or request body once.
func (e *eval) doc(res *domain.RunResult, src domain.RuleSource) (any, error) {
	key := res.ID + "/" + string(src)
	if d, ok := e.docs[key]; ok {
		return d, nil
	}
	if err, ok := e.errs[key]; ok {
		return nil, err
	}
	var body string
	var err error
	switch {
	case src == domain.SourceRequest:
		body = res.Request.Body
		if strings.HasSuffix(body, "… (truncated)") {
			err = fmt.Errorf("%s: the request body was too large to keep", res.Request.Name)
		}
	case res.Response == nil:
		err = noResponse(res)
	case res.Response.Truncated:
		err = fmt.Errorf("%s: the response body was truncated at the response size limit", res.Request.Name)
	default:
		body = res.Response.Body
	}
	var d any
	if err == nil {
		d, err = assertions.DecodeJSON([]byte(body))
		if err != nil {
			err = fmt.Errorf("%s: the %s body is not JSON", res.Request.Name, bodyName(domain.RuleValue{Source: src}))
		}
	}
	if err != nil {
		e.errs[key] = err
		return nil, err
	}
	e.docs[key] = d
	return d, nil
}

func aggregate(v domain.RuleValue, name string, values []any, wildcard bool) (any, error) {
	// count(items) counts the array's elements rather than one match.
	if !wildcard && len(values) == 1 {
		if arr, ok := values[0].([]any); ok {
			values = arr
		}
	}
	if v.Aggregate == domain.AggCount {
		return float64(len(values)), nil
	}
	if len(values) == 0 {
		if v.Aggregate == domain.AggSum {
			return 0.0, nil
		}
		return nil, fmt.Errorf("%s: %s(%s) has no values", name, v.Aggregate, v.Path)
	}
	var out float64
	for i, x := range values {
		f, ok := assertions.ToFloat(x)
		if !ok {
			return nil, fmt.Errorf("%s: %s(%s): value %d is %s, not a number", name, v.Aggregate, v.Path, i+1, assertions.TypeName(x))
		}
		switch {
		case v.Aggregate == domain.AggSum:
			out += f
		case i == 0:
			out = f
		case v.Aggregate == domain.AggMin:
			out = math.Min(out, f)
		default:
			out = math.Max(out, f)
		}
	}
	return out, nil
}

func noResponse(res *domain.RunResult) error {
	msg := res.Error
	if msg == "" {
		msg = string(res.Status)
	}
	return fmt.Errorf("%s had no response (%s)", res.Request.Name, msg)
}

func bodyName(v domain.RuleValue) string {
	if v.Source == domain.SourceRequest {
		return "request"
	}
	return "response"
}
