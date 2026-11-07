package contract

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"api-test-kits/internal/assertions"
)

const (
	maxDepth  = 64
	maxSteps  = 100_000
	maxQuoted = 40
)

// validator checks a decoded JSON value against a schema of the spec.
type validator struct {
	spec *Spec
	// strict reports fields the schema does not document.
	strict   bool
	max      int
	problems []problem
	seen     map[problem]bool
	omitted  int
	// trial validators only need to know whether a value matches.
	trial     bool
	steps     *int
	exhausted bool
}

func newValidator(s *Spec, strict bool, max int) *validator {
	steps := 0
	return &validator{spec: s, strict: strict, max: max, seen: map[problem]bool{}, steps: &steps}
}

// child returns a trial validator sharing the step budget.
func (v *validator) child(max int) *validator {
	return &validator{spec: v.spec, strict: v.strict, max: max, seen: map[problem]bool{}, trial: true, steps: v.steps}
}

func (v *validator) done() bool {
	if *v.steps > maxSteps {
		v.exhausted = true
		return true
	}
	return v.trial && len(v.problems) >= v.max
}

func (v *validator) add(path, format string, args ...any) {
	if path == "" {
		path = "$"
	}
	p := problem{path: path, msg: fmt.Sprintf(format, args...)}
	if v.seen[p] {
		return
	}
	v.seen[p] = true
	if len(v.problems) >= v.max {
		v.omitted++
		return
	}
	v.problems = append(v.problems, p)
}

// validate checks value against raw (a schema or a $ref to one). extra is
// false for allOf parts and oneOf/anyOf alternatives: the undocumented-field
// check at this level is done once, by the schema that combines them.
func (v *validator) validate(raw any, value any, path string, depth int, extra bool) {
	*v.steps++
	if v.done() {
		return
	}
	if b, ok := raw.(bool); ok { // OpenAPI 3.1 boolean schemas
		if !b {
			v.add(path, "no value is allowed here")
		}
		return
	}
	s := v.spec.resolve(raw)
	if s == nil || depth > maxDepth {
		return // external $ref, cycle or too deep: anything goes
	}

	kind := jsonKind(value)
	types, nullable := schemaTypes(s)
	if value == nil && nullable {
		return
	}
	if len(types) > 0 && !typeAllows(types, value) {
		v.add(path, "expected %s, got %s%s", strings.Join(types, " or "), kind, quoted(value))
		return
	}

	if enum, ok := s["enum"].([]any); ok && len(enum) > 0 {
		found := false
		for _, e := range enum {
			if jsonEqual(e, value) {
				found = true
				break
			}
		}
		if !found && !(value == nil && nullable) {
			v.add(path, "%s is not one of %s", short(value), listValues(enum))
		}
	}
	if c, ok := s["const"]; ok && !jsonEqual(c, value) {
		v.add(path, "expected %s, got %s", short(c), short(value))
	}

	for _, part := range arr(s["allOf"]) {
		v.validate(part, value, path, depth+1, false)
	}
	if alts := arr(s["anyOf"]); len(alts) > 0 {
		v.alternatives("anyOf", alts, value, path, depth, false)
	}
	if alts := arr(s["oneOf"]); len(alts) > 0 {
		v.alternatives("oneOf", alts, value, path, depth, true)
	}
	if not, ok := s["not"]; ok {
		t := v.child(1)
		t.validate(not, value, path, depth+1, false)
		if len(t.problems) == 0 && !t.exhausted {
			v.add(path, "the value must not match the schema under \"not\"")
		}
	}

	switch t := value.(type) {
	case string:
		v.checkString(s, t, path)
	case json.Number:
		v.checkNumber(s, t, path)
	case []any:
		v.checkArray(s, t, path, depth)
	case map[string]any:
		v.checkObject(s, t, path, depth, extra)
	}
}

// alternatives checks anyOf (at least one) or oneOf (exactly one). When no
// alternative matches, the problems of the closest one are reported.
func (v *validator) alternatives(name string, alts []any, value any, path string, depth int, exactlyOne bool) {
	matched := 0
	var closest *validator
	for _, alt := range alts {
		t := v.child(v.max)
		t.validate(alt, value, path, depth+1, false)
		if t.exhausted {
			v.exhausted = true
			return
		}
		if len(t.problems) == 0 {
			matched++
			continue
		}
		if closest == nil || len(t.problems)+t.omitted < len(closest.problems)+closest.omitted {
			closest = t
		}
	}
	switch {
	case matched == 0:
		if closest != nil && len(alts) == 1 {
			for _, p := range closest.problems {
				v.add(p.path, "%s", p.msg)
			}
			return
		}
		v.add(path, "matches none of the %d alternatives of %s", len(alts), name)
		if closest != nil {
			for _, p := range closest.problems {
				v.add(p.path, "%s (closest %s alternative)", p.msg, name)
			}
		}
	case exactlyOne && matched > 1:
		v.add(path, "matches %d alternatives of oneOf, expected exactly one", matched)
	}
}

func (v *validator) checkString(s map[string]any, t, path string) {
	n := utf8.RuneCountInString(t)
	if min, ok := num(s["minLength"]); ok && float64(n) < min {
		v.add(path, "%s is shorter than %s characters", short(t), fmtNum(min))
	}
	if max, ok := num(s["maxLength"]); ok && float64(n) > max {
		v.add(path, "%s is longer than %s characters", short(t), fmtNum(max))
	}
	if p := str(s["pattern"]); p != "" {
		if re := v.spec.pattern(p); re != nil && !re.MatchString(t) {
			v.add(path, "%s does not match the pattern %s", short(t), p)
		}
	}
	if f := str(s["format"]); f != "" && !validFormat(f, t) {
		v.add(path, "%s is not a valid %s", short(t), f)
	}
}

func (v *validator) checkNumber(s map[string]any, n json.Number, path string) {
	f, err := n.Float64()
	if err != nil {
		return
	}
	if min, ok := num(s["minimum"]); ok {
		if ex, _ := s["exclusiveMinimum"].(bool); ex && f <= min {
			v.add(path, "%s must be greater than %s", n, fmtNum(min))
		} else if f < min {
			v.add(path, "%s is less than the minimum %s", n, fmtNum(min))
		}
	}
	if min, ok := num(s["exclusiveMinimum"]); ok && f <= min {
		v.add(path, "%s must be greater than %s", n, fmtNum(min))
	}
	if max, ok := num(s["maximum"]); ok {
		if ex, _ := s["exclusiveMaximum"].(bool); ex && f >= max {
			v.add(path, "%s must be less than %s", n, fmtNum(max))
		} else if f > max {
			v.add(path, "%s is greater than the maximum %s", n, fmtNum(max))
		}
	}
	if max, ok := num(s["exclusiveMaximum"]); ok && f >= max {
		v.add(path, "%s must be less than %s", n, fmtNum(max))
	}
	if m, ok := num(s["multipleOf"]); ok && m > 0 {
		q := f / m
		if math.Abs(q-math.Round(q)) > 1e-9 {
			v.add(path, "%s is not a multiple of %s", n, fmtNum(m))
		}
	}
}

func (v *validator) checkArray(s map[string]any, items []any, path string, depth int) {
	if min, ok := num(s["minItems"]); ok && float64(len(items)) < min {
		v.add(path, "has %d items, expected at least %s", len(items), fmtNum(min))
	}
	if max, ok := num(s["maxItems"]); ok && float64(len(items)) > max {
		v.add(path, "has %d items, expected at most %s", len(items), fmtNum(max))
	}
	if u, _ := s["uniqueItems"].(bool); u && len(items) <= 1000 {
	outer:
		for i := range items {
			for j := i + 1; j < len(items); j++ {
				if jsonEqual(items[i], items[j]) {
					v.add(path, "items %d and %d are equal, but items must be unique", i, j)
					break outer
				}
			}
		}
	}
	// OpenAPI 3.1 prefixItems, then items for the rest.
	prefix := arr(s["prefixItems"])
	itemSchema, hasItems := s["items"]
	for i, el := range items {
		if v.done() {
			return
		}
		child := assertions.JoinIndex(path, i)
		switch {
		case i < len(prefix):
			v.validate(prefix[i], el, child, depth+1, true)
		case hasItems:
			v.validate(itemSchema, el, child, depth+1, true)
		}
	}
}

func (v *validator) checkObject(s map[string]any, o map[string]any, path string, depth int, extra bool) {
	props := obj(s["properties"])
	for _, r := range arr(s["required"]) {
		name := str(r)
		if _, ok := o[name]; name != "" && !ok {
			v.add(assertions.JoinKey(path, name), "required field is missing")
		}
	}
	if min, ok := num(s["minProperties"]); ok && float64(len(o)) < min {
		v.add(path, "has %d fields, expected at least %s", len(o), fmtNum(min))
	}
	if max, ok := num(s["maxProperties"]); ok && float64(len(o)) > max {
		v.add(path, "has %d fields, expected at most %s", len(o), fmtNum(max))
	}

	var docs *documentation
	if extra && v.strict {
		docs = v.documented(s, o, depth)
	}
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	addl, hasAddl := s["additionalProperties"]
	for _, k := range keys {
		if v.done() {
			return
		}
		child := assertions.JoinKey(path, k)
		if ps, ok := props[k]; ok {
			v.validate(ps, o[k], child, depth+1, true)
			continue
		}
		if ps := v.patternSchema(s, k); ps != nil {
			v.validate(ps, o[k], child, depth+1, true)
			continue
		}
		if hasAddl {
			if b, ok := addl.(bool); ok {
				if !b {
					v.add(child, "field is not allowed (additionalProperties is false)")
				}
				continue
			}
			v.validate(addl, o[k], child, depth+1, true)
			continue
		}
		if docs != nil && !docs.open && !docs.names[k] {
			v.add(child, "field is not documented")
		}
	}
}

func (v *validator) patternSchema(s map[string]any, key string) any {
	for p, ps := range obj(s["patternProperties"]) {
		if re := v.spec.pattern(p); re != nil && re.MatchString(key) {
			return ps
		}
	}
	return nil
}

// documentation is the set of field names a schema documents for an object.
type documentation struct {
	names map[string]bool
	// open means any field is documented (free-form object or
	// additionalProperties other than false).
	open bool
}

// documented collects the fields documented for o by s, its allOf parts and
// the anyOf/oneOf alternatives that o matches (all of them if none does).
func (v *validator) documented(s map[string]any, o map[string]any, depth int) *documentation {
	d := &documentation{names: map[string]bool{}}
	v.collect(s, o, depth, d)
	return d
}

func (v *validator) collect(s map[string]any, o map[string]any, depth int, d *documentation) {
	if s == nil || depth > maxDepth || d.open {
		if s == nil {
			d.open = true
		}
		return
	}
	*v.steps++
	if addl, ok := s["additionalProperties"]; ok {
		if b, isBool := addl.(bool); !isBool || b {
			d.open = true
			return
		}
	}
	if s["patternProperties"] != nil {
		d.open = true // matching keys are validated against their pattern schema
		return
	}
	props := obj(s["properties"])
	allOf, anyOf, oneOf := arr(s["allOf"]), arr(s["anyOf"]), arr(s["oneOf"])
	if len(props) == 0 && len(allOf) == 0 && len(anyOf) == 0 && len(oneOf) == 0 {
		if _, closed := s["additionalProperties"]; !closed {
			d.open = true // a free-form object
		}
		return
	}
	for k := range props {
		d.names[k] = true
	}
	for _, part := range allOf {
		v.collect(v.spec.resolve(part), o, depth+1, d)
	}
	for _, alts := range [][]any{anyOf, oneOf} {
		if len(alts) == 0 {
			continue
		}
		var matching []map[string]any
		for _, alt := range alts {
			t := v.child(1)
			t.strict = false
			t.validate(alt, o, "", depth+1, false)
			if len(t.problems) == 0 {
				matching = append(matching, v.spec.resolve(alt))
			}
		}
		if len(matching) == 0 {
			for _, alt := range alts {
				matching = append(matching, v.spec.resolve(alt))
			}
		}
		for _, m := range matching {
			v.collect(m, o, depth+1, d)
		}
	}
}

// ---- types and values ----

func jsonKind(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		if isInteger(t) {
			return "integer"
		}
		return "number"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}

func isInteger(n json.Number) bool {
	if !strings.ContainsAny(n.String(), ".eE") {
		return true
	}
	f, err := n.Float64()
	return err == nil && f == math.Trunc(f) && !math.IsInf(f, 0)
}

// schemaTypes returns the allowed types and whether null is allowed.
func schemaTypes(s map[string]any) ([]string, bool) {
	var types []string
	if t := str(s["type"]); t != "" {
		types = []string{t}
	} else {
		types = strs(s["type"])
	}
	nullable, _ := s["nullable"].(bool)
	if xn, _ := s["x-nullable"].(bool); xn {
		nullable = true
	}
	out := types[:0:0]
	for _, t := range types {
		if t == "null" {
			nullable = true
			continue
		}
		out = append(out, t)
	}
	if len(types) > 0 && len(out) == 0 {
		return []string{"null"}, true
	}
	return out, nullable
}

func typeAllows(types []string, v any) bool {
	k := jsonKind(v)
	for _, t := range types {
		switch {
		case t == k, t == "number" && k == "integer":
			return true
		}
	}
	return false
}

func jsonEqual(a, b any) bool {
	na, aok := toFloat(a)
	nb, bok := toFloat(b)
	if aok && bok {
		return na == nb
	}
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ja) == string(jb)
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	}
	return 0, false
}

func num(v any) (float64, bool) {
	f, ok := toFloat(v)
	return f, ok
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// short renders a value for a message; long values are not quoted.
func short(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "the value"
	}
	if utf8.RuneCount(b) > maxQuoted {
		return "the value"
	}
	return string(b)
}

// quoted is " (value)" for short scalar values, else "".
func quoted(v any) string {
	switch v.(type) {
	case nil, map[string]any, []any:
		return ""
	}
	if s := short(v); s != "the value" {
		return " (" + s + ")"
	}
	return ""
}

func listValues(vals []any) string {
	parts := make([]string, 0, len(vals))
	for i, x := range vals {
		if i == 5 {
			parts = append(parts, fmt.Sprintf("… (%d values)", len(vals)))
			break
		}
		parts = append(parts, short(x))
	}
	return strings.Join(parts, ", ")
}

var (
	uuidPattern  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// validFormat checks the formats listed in the guide; others always pass.
func validFormat(format, s string) bool {
	switch format {
	case "date-time":
		_, err := time.Parse(time.RFC3339, strings.ToUpper(s))
		return err == nil
	case "date":
		_, err := time.Parse("2006-01-02", s)
		return err == nil
	case "email":
		return emailPattern.MatchString(s)
	case "uuid":
		return uuidPattern.MatchString(s)
	case "uri":
		u, err := url.Parse(s)
		return err == nil && u.IsAbs()
	case "ipv4":
		ip := net.ParseIP(s)
		return ip != nil && ip.To4() != nil && !strings.Contains(s, ":")
	case "ipv6":
		return net.ParseIP(s) != nil && strings.Contains(s, ":")
	}
	return true
}
