package regression

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
)

// Shape is the structure inferred from an approved response, stored as the
// snapshot of a schema-mode baseline. It is a small subset of JSON Schema
// (type, properties, required, items), so the stored JSON stays readable and
// can be checked by other tools.
//
// Integers and decimals share the type "number" so a value such as 10 → 10.5
// is not reported as a structural change.
type Shape struct {
	// Type lists the JSON types seen at this position, sorted.
	Type []string `json:"type"`
	// Properties describes object fields.
	Properties map[string]*Shape `json:"properties,omitempty"`
	// Required lists the object fields that were present every time.
	Required []string `json:"required,omitempty"`
	// Items describes array elements, merged over all elements. It is nil for
	// arrays that were empty when the baseline was approved.
	Items *Shape `json:"items,omitempty"`
}

// InferShape describes the structure of a decoded JSON value. Array elements
// are merged: a field missing from some elements becomes optional, and a
// field that is null in some elements becomes nullable.
func InferShape(v any) *Shape {
	s := &Shape{Type: []string{kind(v)}}
	switch t := v.(type) {
	case map[string]any:
		s.Properties = make(map[string]*Shape, len(t))
		for k, child := range t {
			s.Properties[k] = InferShape(child)
			s.Required = append(s.Required, k)
		}
		sort.Strings(s.Required)
	case []any:
		for _, el := range t {
			s.Items = mergeShapes(s.Items, InferShape(el))
		}
	}
	return s
}

// mergeShapes returns a shape that accepts everything a or b accepts.
func mergeShapes(a, b *Shape) *Shape {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := &Shape{Type: unionStrings(a.Type, b.Type)}
	if a.Properties != nil || b.Properties != nil {
		out.Properties = map[string]*Shape{}
		for k, v := range a.Properties {
			out.Properties[k] = v
		}
		for k, v := range b.Properties {
			out.Properties[k] = mergeShapes(out.Properties[k], v)
		}
		// A field is required only if every object that was seen had it.
		// When only one side was an object (the other was e.g. null), keep
		// that side's required list.
		switch {
		case a.Properties == nil:
			out.Required = b.Required
		case b.Properties == nil:
			out.Required = a.Required
		default:
			out.Required = intersectStrings(a.Required, b.Required)
		}
	}
	out.Items = mergeShapes(a.Items, b.Items)
	return out
}

// SchemaSnapshot builds the stored snapshot of a schema-mode baseline.
func SchemaSnapshot(body []byte) (json.RawMessage, error) {
	doc, err := assertions.DecodeJSON(body)
	if err != nil {
		return nil, fmt.Errorf("baseline requires a JSON response: %w", err)
	}
	return json.Marshal(InferShape(doc))
}

// CompareShape checks a decoded value against an approved shape and reports
// structural differences only: fields that appeared or disappeared, and
// values whose JSON type changed. Values themselves are never compared.
//
// Array elements are reported with a "[*]" path, once per distinct problem,
// so one changed field in a list of 500 items is a single entry.
//
// A position that was only ever null, or an array that was empty, when the
// baseline was approved has no known type and accepts any value.
func CompareShape(s *Shape, actual any, ignoredPaths []string, maxDiffs int) []domain.DiffEntry {
	if maxDiffs <= 0 {
		maxDiffs = defaultMaxDiffs
	}
	c := &shapeChecker{
		ignore: compilePatterns(ignoredPaths),
		max:    maxDiffs,
		seen:   map[domain.DiffEntry]bool{},
		out:    []domain.DiffEntry{},
	}
	c.check("", nil, s, actual)
	return c.out
}

type shapeChecker struct {
	ignore [][]string
	max    int
	seen   map[domain.DiffEntry]bool
	out    []domain.DiffEntry
}

func (c *shapeChecker) add(e domain.DiffEntry) {
	if len(c.out) >= c.max || c.seen[e] {
		return
	}
	c.seen[e] = true
	c.out = append(c.out, e)
}

func (c *shapeChecker) ignored(segs []string) bool {
	if len(segs) == 0 {
		return false
	}
	for _, p := range c.ignore {
		if matchSegs(p, segs) {
			return true
		}
	}
	return false
}

func (c *shapeChecker) check(path string, segs []string, s *Shape, v any) {
	if s == nil || len(c.out) >= c.max || c.ignored(segs) {
		return
	}
	display := path
	if display == "" {
		display = "$"
	}
	if onlyNull(s.Type) {
		return
	}
	k := kind(v)
	if !contains(s.Type, k) {
		c.add(domain.DiffEntry{Path: display, Kind: domain.DiffTypeChanged,
			Before: strings.Join(s.Type, " | "), After: k})
		return
	}
	switch t := v.(type) {
	case map[string]any:
		if s.Properties == nil {
			return
		}
		required := make(map[string]bool, len(s.Required))
		for _, r := range s.Required {
			required[r] = true
		}
		for _, key := range unionKeys(s.Properties, t) {
			child := assertions.JoinKey(path, key)
			childSegs := append(append([]string{}, segs...), key)
			ps, known := s.Properties[key]
			val, present := t[key]
			switch {
			case known && present:
				c.check(child, childSegs, ps, val)
			case known:
				if required[key] && !c.ignored(childSegs) {
					c.add(domain.DiffEntry{Path: child, Kind: domain.DiffRemoved, Before: describeShape(ps)})
				}
			default:
				if !c.ignored(childSegs) {
					c.add(domain.DiffEntry{Path: child, Kind: domain.DiffAdded, After: describeShape(InferShape(val))})
				}
			}
		}
	case []any:
		if s.Items == nil {
			return
		}
		child := path + "[*]"
		childSegs := append(append([]string{}, segs...), "*")
		for _, el := range t {
			c.check(child, childSegs, s.Items, el)
		}
	}
}

// describeShape renders a shape briefly for the diff table, e.g. "string",
// "null | object" or "array of number".
func describeShape(s *Shape) string {
	d := strings.Join(s.Type, " | ")
	if s.Items != nil && len(s.Type) == 1 && s.Type[0] == "array" {
		d += " of " + describeShape(s.Items)
	}
	return d
}

func onlyNull(types []string) bool { return len(types) == 1 && types[0] == "null" }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func unionStrings(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		set[x] = true
	}
	out := make([]string, 0, len(set))
	for x := range set {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

func intersectStrings(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range b {
		in[x] = true
	}
	out := []string{}
	for _, x := range a {
		if in[x] {
			out = append(out, x)
		}
	}
	return out
}
