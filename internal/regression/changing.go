package regression

import (
	"sort"
	"strconv"

	"api-test-kits/internal/assertions"
)

// Reasons a field is reported as changing between calls.
const (
	ChangedValue   = "value"   // the value differs between calls
	ChangedType    = "type"    // the JSON type differs between calls
	ChangedMissing = "missing" // present in some responses only
	ChangedLength  = "length"  // an array whose length differs between calls
)

const (
	defaultMaxChanging = 200
	maxExamples        = 3
)

// ChangingField is a path whose value is not the same in every response of
// the same request. Array indices are generalized to [*].
type ChangingField struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	// Examples are up to three distinct values seen at the path.
	Examples []string `json:"examples"`
	// Suggested is false for array length changes: ignoring the array would
	// ignore everything inside it.
	Suggested bool `json:"suggested"`
}

// ChangingFields compares decoded responses of the same request with each
// other and returns the fields that differ, in document order. Paths matched
// by ignored (baseline patterns) are left out. max <= 0 means 200.
func ChangingFields(docs []any, ignored []string, max int) []ChangingField {
	if max <= 0 {
		max = defaultMaxChanging
	}
	c := &changing{ignore: compilePatterns(ignored), max: max, index: map[string]int{}, out: []ChangingField{}}
	if len(docs) < 2 {
		return c.out
	}
	present := make([]bool, len(docs))
	for i := range present {
		present[i] = true
	}
	c.walk("", nil, docs, present)
	return c.out
}

// ChangingPaths returns the suggested paths of ChangingFields.
func ChangingPaths(fields []ChangingField) []string {
	out := []string{}
	for _, f := range fields {
		if f.Suggested {
			out = append(out, f.Path)
		}
	}
	return out
}

type changing struct {
	ignore [][]string
	max    int
	index  map[string]int
	out    []ChangingField
}

// walk compares the values found at one concrete path (segs) in every
// response. present[i] says whether response i has the path at all; display
// is the path with array indices generalized.
func (c *changing) walk(display string, segs []string, vals []any, present []bool) {
	if len(c.out) >= c.max || (len(segs) > 0 && c.ignored(segs)) {
		return
	}
	var seen []any
	for i, v := range vals {
		if present[i] {
			seen = append(seen, v)
		}
	}
	if len(seen) == 0 {
		return
	}
	if len(seen) < len(vals) {
		c.record(display, ChangedMissing, seen, true)
		return
	}
	t := kind(seen[0])
	for _, v := range seen[1:] {
		if kind(v) != t {
			c.record(display, ChangedType, seen, true)
			return
		}
	}
	switch first := seen[0].(type) {
	case map[string]any:
		keys := map[string]struct{}{}
		for _, v := range seen {
			for k := range v.(map[string]any) {
				keys[k] = struct{}{}
			}
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			child := make([]any, len(seen))
			has := make([]bool, len(seen))
			for i, v := range seen {
				child[i], has[i] = v.(map[string]any)[k]
			}
			c.walk(assertions.JoinKey(display, k), append(append([]string{}, segs...), k), child, has)
		}
	case []any:
		shortest := len(first)
		for _, v := range seen[1:] {
			if n := len(v.([]any)); n != len(first) {
				if n < shortest {
					shortest = n
				}
				c.recordLength(display, seen)
			}
		}
		for idx := 0; idx < shortest; idx++ {
			child := make([]any, len(seen))
			has := make([]bool, len(seen))
			for i, v := range seen {
				child[i], has[i] = v.([]any)[idx], true
			}
			c.walk(display+"[*]", append(append([]string{}, segs...), strconv.Itoa(idx)), child, has)
		}
	default:
		for _, v := range seen[1:] {
			if !sameLeaf(seen[0], v) {
				c.record(display, ChangedValue, seen, true)
				return
			}
		}
	}
}

// sameLeaf compares two values of the same kind; numbers by value, so 1 and 1.0 are equal.
func sameLeaf(a, b any) bool {
	if kind(a) == "number" {
		fa, _ := assertions.ToFloat(a)
		fb, _ := assertions.ToFloat(b)
		return fa == fb
	}
	return assertions.ValuesEqual(a, b, 0)
}

func (c *changing) ignored(segs []string) bool {
	for _, p := range c.ignore {
		if matchSegs(p, segs) {
			return true
		}
	}
	return false
}

func (c *changing) recordLength(display string, seen []any) {
	lengths := make([]any, len(seen))
	for i, v := range seen {
		lengths[i] = len(v.([]any))
	}
	if i, ok := c.index[display]; ok && c.out[i].Reason == ChangedLength {
		return
	}
	c.record(display, ChangedLength, lengths, false)
}

// record adds a field, or merges examples into it when the generalized path
// was already recorded (from another array item).
func (c *changing) record(display, reason string, vals []any, suggested bool) {
	if display == "" {
		display = "$"
	}
	i, ok := c.index[display]
	if !ok {
		if len(c.out) >= c.max {
			return
		}
		c.out = append(c.out, ChangingField{Path: display, Reason: reason, Examples: []string{}, Suggested: suggested})
		i = len(c.out) - 1
		c.index[display] = i
	}
	f := &c.out[i]
	for _, v := range vals {
		if len(f.Examples) >= maxExamples {
			break
		}
		ex := assertions.Format(v)
		if n, isLen := v.(int); isLen && reason == ChangedLength {
			ex = strconv.Itoa(n) + " items"
		}
		if !contains(f.Examples, ex) {
			f.Examples = append(f.Examples, ex)
		}
	}
}
