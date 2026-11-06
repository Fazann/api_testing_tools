package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RuleKind says how a collection rule checks its values.
type RuleKind string

const (
	// RuleCompare compares the left value with the right side (one value, or
	// several added and subtracted).
	RuleCompare RuleKind = "compare"
	// RuleContains checks whether the left value appears in the list the
	// right side selects.
	RuleContains RuleKind = "contains"
)

// RuleSource says where a rule value comes from.
type RuleSource string

const (
	SourceResponse RuleSource = "response" // the response's JSON body
	SourceRequest  RuleSource = "request"  // the JSON body that was sent
	SourceStatus   RuleSource = "status"   // the response status code
	SourceHeader   RuleSource = "header"   // a response header
	SourceConstant RuleSource = "constant" // a fixed value
)

// Rule operators.
const (
	RuleEq    = "eq"
	RuleNe    = "ne"
	RuleGt    = "gt"
	RuleGte   = "gte"
	RuleLt    = "lt"
	RuleLte   = "lte"
	RuleIn    = "in"
	RuleNotIn = "not_in"
)

// Limits for rules.
const (
	MaxRules     = 50
	MaxRuleTerms = 10
)

// RuleValue is one value a rule reads: from a request's result in the run,
// or a constant.
type RuleValue struct {
	Source    RuleSource `json:"source"`
	RequestID string     `json:"requestId,omitempty"`
	// Path is a JSON path for bodies, or the header name.
	Path string `json:"path,omitempty"`
	// Aggregate reduces a body path: count, sum, min or max (see AggCount).
	Aggregate string `json:"aggregate,omitempty"`
	// Literal is a constant's value: JSON (1, true, null, "text") or plain text.
	Literal string `json:"literal,omitempty"`
}

// RuleTerm is a value on the right side of a rule with its sign.
type RuleTerm struct {
	// Sign is "+" or "-".
	Sign  string    `json:"sign"`
	Value RuleValue `json:"value"`
}

// Rule is a check across several requests of one collection run.
type Rule struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Enabled   bool       `json:"enabled"`
	Kind      RuleKind   `json:"kind"`
	Left      RuleValue  `json:"left"`
	Operator  string     `json:"operator"`
	Right     []RuleTerm `json:"right"`
	Tolerance float64    `json:"tolerance"`
}

// RequestIDs returns the requests a rule reads from, without duplicates.
func (r Rule) RequestIDs() []string {
	var out []string
	seen := map[string]bool{}
	add := func(v RuleValue) {
		if v.Source != SourceConstant && v.RequestID != "" && !seen[v.RequestID] {
			seen[v.RequestID] = true
			out = append(out, v.RequestID)
		}
	}
	add(r.Left)
	for _, t := range r.Right {
		add(t.Value)
	}
	return out
}

// RemapRuleRequests returns rules with request IDs replaced through ids
// (old → new); IDs missing from ids are kept, so the rule is skipped later.
func RemapRuleRequests(rules []Rule, ids map[string]string) []Rule {
	out := make([]Rule, len(rules))
	remap := func(v RuleValue) RuleValue {
		if n, ok := ids[v.RequestID]; ok {
			v.RequestID = n
		}
		return v
	}
	for i, r := range rules {
		r.Left = remap(r.Left)
		terms := make([]RuleTerm, len(r.Right))
		for j, t := range r.Right {
			t.Value = remap(t.Value)
			terms[j] = t
		}
		r.Right = terms
		out[i] = r
	}
	return out
}

// NormalizeRules trims names and paths and fills defaults.
func NormalizeRules(rules []Rule) []Rule {
	out := make([]Rule, 0, len(rules))
	norm := func(v RuleValue) RuleValue {
		v.Path = strings.TrimSpace(v.Path)
		v.Aggregate = strings.TrimSpace(v.Aggregate)
		if v.Source == SourceConstant {
			v.RequestID, v.Path, v.Aggregate = "", "", ""
		} else {
			v.Literal = ""
		}
		if v.Source == SourceStatus || v.Source == SourceHeader {
			v.Aggregate = ""
		}
		if v.Source == SourceStatus {
			v.Path = ""
		}
		return v
	}
	for _, r := range rules {
		r.Name = strings.TrimSpace(r.Name)
		r.Left = norm(r.Left)
		terms := make([]RuleTerm, 0, len(r.Right))
		for _, t := range r.Right {
			if t.Sign != "-" {
				t.Sign = "+"
			}
			t.Value = norm(t.Value)
			terms = append(terms, t)
		}
		r.Right = terms
		out = append(out, r)
	}
	return out
}

// ValidateRules checks rules before they are saved or imported.
func ValidateRules(rules []Rule) error {
	if len(rules) > MaxRules {
		return fmt.Errorf("at most %d rules per collection", MaxRules)
	}
	ids := map[string]bool{}
	for i, r := range rules {
		label := r.Name
		if label == "" {
			label = fmt.Sprintf("rule %d", i+1)
		}
		if err := r.validate(); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		if r.ID == "" || ids[r.ID] {
			return fmt.Errorf("%s: every rule needs a unique id", label)
		}
		ids[r.ID] = true
	}
	return nil
}

func (r Rule) validate() error {
	if err := ValidateName("rule name", r.Name); err != nil {
		return err
	}
	switch r.Kind {
	case RuleCompare:
		switch r.Operator {
		case RuleEq, RuleNe, RuleGt, RuleGte, RuleLt, RuleLte:
		default:
			return fmt.Errorf("unknown operator %q for comparing values", r.Operator)
		}
		if len(r.Right) < 1 || len(r.Right) > MaxRuleTerms {
			return fmt.Errorf("the right side needs 1 to %d values", MaxRuleTerms)
		}
	case RuleContains:
		if r.Operator != RuleIn && r.Operator != RuleNotIn {
			return fmt.Errorf("unknown operator %q for a list check", r.Operator)
		}
		if len(r.Right) != 1 {
			return fmt.Errorf("the list to look in must be exactly one value")
		}
		if r.Right[0].Value.Source == SourceConstant {
			return fmt.Errorf("the list to look in must come from a request")
		}
	default:
		return fmt.Errorf("unknown rule kind %q", r.Kind)
	}
	if r.Tolerance < 0 {
		return fmt.Errorf("tolerance must not be negative")
	}
	if err := r.Left.validate("left value"); err != nil {
		return err
	}
	for i, t := range r.Right {
		if err := t.Value.validate(fmt.Sprintf("right value %d", i+1)); err != nil {
			return err
		}
		if t.Sign != "+" && t.Sign != "-" {
			return fmt.Errorf("right value %d: sign must be + or -", i+1)
		}
	}
	return nil
}

func (v RuleValue) validate(label string) error {
	switch v.Source {
	case SourceConstant:
		if len(v.Literal) > 10_000 {
			return fmt.Errorf("%s: the constant is too long", label)
		}
		return nil
	case SourceResponse, SourceRequest, SourceStatus, SourceHeader:
	default:
		return fmt.Errorf("%s: unknown source %q", label, v.Source)
	}
	if v.RequestID == "" {
		return fmt.Errorf("%s: choose a request", label)
	}
	if v.Source == SourceHeader && !IsValidHeaderName(v.Path) {
		return fmt.Errorf("%s: %q is not a valid header name", label, v.Path)
	}
	if (v.Source == SourceResponse || v.Source == SourceRequest) && v.Path == "" && v.Aggregate == "" {
		return fmt.Errorf("%s: enter a JSON path", label)
	}
	switch v.Aggregate {
	case "", AggCount, AggSum, AggMin, AggMax:
	default:
		return fmt.Errorf("%s: unknown function %q", label, v.Aggregate)
	}
	return nil
}

// ConstantValue decodes a constant: JSON when it parses, otherwise text.
func (v RuleValue) ConstantValue() any {
	var out any
	dec := json.NewDecoder(strings.NewReader(v.Literal))
	dec.UseNumber()
	if err := dec.Decode(&out); err == nil && !dec.More() {
		return out
	}
	return v.Literal
}
