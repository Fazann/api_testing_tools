package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

type AssertionKind string

const (
	// AssertStatus compares the HTTP status code.
	AssertStatus AssertionKind = "status"
	// AssertHeader checks a response header; Path is the header name.
	AssertHeader AssertionKind = "header"
	// AssertJSONPath checks a value selected by a JSON path.
	AssertJSONPath AssertionKind = "json_path"
	// AssertResponseTime checks total duration in milliseconds.
	AssertResponseTime AssertionKind = "response_time"
	// AssertValidJSON checks that the body parses as JSON.
	AssertValidJSON AssertionKind = "valid_json"
	// AssertRequiredFields checks that every path in Expected (a string array) exists.
	AssertRequiredFields AssertionKind = "required_fields"
	// AssertBody checks the raw response body text.
	AssertBody AssertionKind = "body"
	// AssertAggregate applies Function (count, sum, min, max) to the values
	// selected by Path and compares the result.
	AssertAggregate AssertionKind = "aggregate"
	// AssertScript is a pm.test() result from a post-response script. It only
	// appears in results, never in a request's saved assertions.
	AssertScript AssertionKind = "script"
	// AssertSocketStep is the outcome of one socket test step. It only
	// appears in results.
	AssertSocketStep AssertionKind = "socket_step"
)

type Operator string

const (
	OpExists      Operator = "exists"
	OpNotExists   Operator = "not_exists"
	OpEq          Operator = "eq"
	OpNeq         Operator = "neq"
	OpGt          Operator = "gt"
	OpGte         Operator = "gte"
	OpLt          Operator = "lt"
	OpLte         Operator = "lte"
	OpContains    Operator = "contains"
	OpNotContains Operator = "not_contains"
	OpMatches     Operator = "matches"
	OpType        Operator = "type"
)

// Aggregate functions for AssertAggregate.
const (
	AggCount = "count"
	AggSum   = "sum"
	AggMin   = "min"
	AggMax   = "max"
)

// JSON type names accepted by OpType.
var JSONTypeNames = []string{"string", "number", "integer", "boolean", "object", "array", "null"}

const MaxPatternLength = 512

// Assertion is one check evaluated against a response.
//
// Expected holds a JSON value. For comparisons against another field of the
// same response it may be an object of the form {"$path": "data.total"}.
type Assertion struct {
	ID        string          `json:"id"`
	RequestID string          `json:"requestId"`
	Kind      AssertionKind   `json:"kind"`
	Path      string          `json:"path"`
	Operator  Operator        `json:"operator"`
	Expected  json.RawMessage `json:"expected"`
	// Tolerance is an absolute tolerance for numeric equality.
	Tolerance float64 `json:"tolerance"`
	// Function is the aggregate function for AssertAggregate.
	Function    string    `json:"function,omitempty"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
}

var operatorsByKind = map[AssertionKind][]Operator{
	AssertStatus:         {OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte},
	AssertHeader:         {OpExists, OpNotExists, OpEq, OpNeq, OpContains, OpNotContains, OpMatches},
	AssertJSONPath:       {OpExists, OpNotExists, OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpContains, OpNotContains, OpMatches, OpType},
	AssertResponseTime:   {OpLt, OpLte},
	AssertValidJSON:      {""},
	AssertRequiredFields: {""},
	AssertBody:           {OpContains, OpNotContains, OpMatches, OpEq},
	AssertAggregate:      {OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte},
}

// OperatorsFor returns the operators allowed for an assertion kind.
func OperatorsFor(kind AssertionKind) []Operator { return operatorsByKind[kind] }

func (a *Assertion) Validate() error {
	ops, ok := operatorsByKind[a.Kind]
	if !ok {
		return fmt.Errorf("unknown assertion kind %q", a.Kind)
	}
	if !containsOp(ops, a.Operator) {
		return fmt.Errorf("operator %q is not valid for %s assertions", a.Operator, a.Kind)
	}
	if len(a.Path) > 1024 {
		return fmt.Errorf("path is too long")
	}
	switch a.Kind {
	case AssertHeader:
		if !IsValidHeaderName(a.Path) {
			return fmt.Errorf("header name %q is invalid", a.Path)
		}
	case AssertJSONPath, AssertAggregate:
		if a.Path == "" {
			return fmt.Errorf("JSON path is required")
		}
	}
	if a.Kind == AssertAggregate {
		switch a.Function {
		case AggCount, AggSum, AggMin, AggMax:
		default:
			return fmt.Errorf("aggregate function must be count, sum, min or max")
		}
	}
	if a.Tolerance < 0 {
		return fmt.Errorf("tolerance must not be negative")
	}
	needsExpected := a.Operator != OpExists && a.Operator != OpNotExists && a.Kind != AssertValidJSON
	if needsExpected {
		if len(a.Expected) == 0 {
			return fmt.Errorf("expected value is required")
		}
		var v any
		if err := json.Unmarshal(a.Expected, &v); err != nil {
			return fmt.Errorf("expected value is not valid JSON: %v", err)
		}
		if err := a.validateExpected(v); err != nil {
			return err
		}
	}
	return nil
}

func (a *Assertion) validateExpected(v any) error {
	if p, ok := ExpectedPathRef(v); ok {
		if p == "" {
			return fmt.Errorf("$path reference must not be empty")
		}
		return nil
	}
	switch a.Operator {
	case OpMatches:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("pattern must be a string")
		}
		if len(s) > MaxPatternLength {
			return fmt.Errorf("pattern must be at most %d characters", MaxPatternLength)
		}
		// Go's regexp is RE2-based and runs in linear time, so user
		// patterns cannot cause catastrophic backtracking.
		if _, err := regexp.Compile(s); err != nil {
			return fmt.Errorf("invalid pattern: %v", err)
		}
	case OpType:
		s, _ := v.(string)
		for _, t := range JSONTypeNames {
			if s == t {
				return nil
			}
		}
		return fmt.Errorf("type must be one of %v", JSONTypeNames)
	case OpGt, OpGte, OpLt, OpLte:
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("expected value must be a number for %s", a.Operator)
		}
	}
	switch a.Kind {
	case AssertStatus, AssertResponseTime:
		if _, ok := v.(float64); !ok {
			return fmt.Errorf("expected value must be a number")
		}
	case AssertRequiredFields:
		arr, ok := v.([]any)
		if !ok || len(arr) == 0 {
			return fmt.Errorf("required fields must be a non-empty array of paths")
		}
		for _, item := range arr {
			if s, ok := item.(string); !ok || s == "" {
				return fmt.Errorf("required fields must be strings")
			}
		}
	}
	return nil
}

// ExpectedPathRef reports whether v is a {"$path": "..."} reference.
func ExpectedPathRef(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return "", false
	}
	p, ok := m["$path"].(string)
	return p, ok
}

func containsOp(ops []Operator, op Operator) bool {
	for _, o := range ops {
		if o == op {
			return true
		}
	}
	return false
}

// AssertionResult records the outcome of one assertion.
type AssertionResult struct {
	AssertionID string        `json:"assertionId"`
	Kind        AssertionKind `json:"kind"`
	Path        string        `json:"path,omitempty"`
	Operator    Operator      `json:"operator,omitempty"`
	Description string        `json:"description,omitempty"`
	Passed      bool          `json:"passed"`
	Expected    string        `json:"expected"`
	Actual      string        `json:"actual"`
	Message     string        `json:"message"`
}
