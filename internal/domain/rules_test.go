package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func validRule() Rule {
	return Rule{ID: "r1", Name: "Rule", Enabled: true, Kind: RuleCompare, Operator: RuleEq,
		Left:  RuleValue{Source: SourceResponse, RequestID: "a", Path: "x"},
		Right: []RuleTerm{{Sign: "+", Value: RuleValue{Source: SourceConstant, Literal: "1"}}}}
}

func TestValidateRules(t *testing.T) {
	if err := ValidateRules([]Rule{validRule()}); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(r *Rule){
		"name":             func(r *Rule) { r.Name = "" },
		"kind":             func(r *Rule) { r.Kind = "magic" },
		"operator":         func(r *Rule) { r.Operator = "in" },
		"no right":         func(r *Rule) { r.Right = nil },
		"request":          func(r *Rule) { r.Left.RequestID = "" },
		"path":             func(r *Rule) { r.Left.Path = "" },
		"aggregate":        func(r *Rule) { r.Left.Aggregate = "avg" },
		"header name":      func(r *Rule) { r.Left = RuleValue{Source: SourceHeader, RequestID: "a", Path: "bad header"} },
		"negative tol":     func(r *Rule) { r.Tolerance = -1 },
		"contains two":     func(r *Rule) { r.Kind, r.Operator = RuleContains, RuleIn; r.Right = append(r.Right, r.Right[0]) },
		"contains literal": func(r *Rule) { r.Kind, r.Operator = RuleContains, RuleIn },
		"too many terms": func(r *Rule) {
			for len(r.Right) <= MaxRuleTerms {
				r.Right = append(r.Right, r.Right[0])
			}
		},
	}
	for name, mutate := range cases {
		r := validRule()
		mutate(&r)
		if err := ValidateRules(NormalizeRules([]Rule{r})); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	dup := []Rule{validRule(), validRule()}
	if err := ValidateRules(dup); err == nil || !strings.Contains(err.Error(), "unique id") {
		t.Errorf("duplicate ids: %v", err)
	}
}

func TestNormalizeRules(t *testing.T) {
	r := validRule()
	r.Name = "  Rule  "
	r.Right = []RuleTerm{{Sign: "", Value: RuleValue{Source: SourceConstant, Literal: "1", RequestID: "x", Path: "p"}},
		{Sign: "-", Value: RuleValue{Source: SourceStatus, RequestID: "a", Path: "p", Aggregate: "sum"}}}
	got := NormalizeRules([]Rule{r})[0]
	if got.Name != "Rule" || got.Right[0].Sign != "+" || got.Right[0].Value.RequestID != "" || got.Right[0].Value.Path != "" ||
		got.Right[1].Sign != "-" || got.Right[1].Value.Path != "" || got.Right[1].Value.Aggregate != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestRuleRequestIDsAndRemap(t *testing.T) {
	r := validRule()
	r.Right = append(r.Right, RuleTerm{Sign: "-", Value: RuleValue{Source: SourceRequest, RequestID: "b", Path: "y"}},
		RuleTerm{Sign: "+", Value: RuleValue{Source: SourceResponse, RequestID: "a", Path: "z"}})
	if ids := r.RequestIDs(); strings.Join(ids, ",") != "a,b" {
		t.Fatalf("ids %v", ids)
	}
	got := RemapRuleRequests([]Rule{r}, map[string]string{"a": "A"})[0]
	if got.Left.RequestID != "A" || got.Right[1].Value.RequestID != "b" || got.Right[2].Value.RequestID != "A" {
		t.Fatalf("remap %+v", got)
	}
	if r.Left.RequestID != "a" {
		t.Fatal("remap must not change the input")
	}
}

func TestConstantValue(t *testing.T) {
	cases := map[string]any{`1.5`: json.Number("1.5"), `"x"`: "x", `paid`: "paid", `true`: true, `null`: nil, `1 2`: "1 2"}
	for literal, want := range cases {
		if got := (RuleValue{Source: SourceConstant, Literal: literal}).ConstantValue(); got != want {
			t.Errorf("%s → %#v, want %#v", literal, got, want)
		}
	}
}
