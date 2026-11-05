// Package regression compares responses against approved baselines.
package regression

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
)

// Options control a comparison.
type Options struct {
	// IgnoredPaths are patterns whose values are not compared, such as
	// "meta.request_id", "data.items[*].updated_at" or "**.timestamp".
	IgnoredPaths []string
	// Tolerance is the absolute difference allowed between numbers.
	Tolerance float64
	// MaxDiffs caps the number of reported entries.
	MaxDiffs int
}

const defaultMaxDiffs = 500

// Diff compares two decoded JSON values independent of object key order.
// Arrays are compared by index.
func Diff(before, after any, opts Options) []domain.DiffEntry {
	d := newDiffer(opts)
	d.walk("", nil, before, after)
	return d.out
}

func newDiffer(opts Options) *differ {
	if opts.MaxDiffs <= 0 {
		opts.MaxDiffs = defaultMaxDiffs
	}
	// out starts empty, not nil, so a match is sent to the UI as "diffs": [] rather than null.
	return &differ{opts: opts, ignore: compilePatterns(opts.IgnoredPaths), out: []domain.DiffEntry{}}
}

// walkAt compares a value found at a selected path. A missing side (nil
// with present=false) is reported as an addition or removal.
func (d *differ) walkAt(path string, a any, aOK bool, b any, bOK bool) {
	segs := splitPattern(path)
	switch {
	case aOK && bOK:
		d.walk(path, segs, a, b)
	case aOK:
		if !d.ignored(segs) {
			d.add(domain.DiffEntry{Path: path, Kind: domain.DiffRemoved, Before: assertions.Format(a)})
		}
	case bOK:
		if !d.ignored(segs) {
			d.add(domain.DiffEntry{Path: path, Kind: domain.DiffAdded, After: assertions.Format(b)})
		}
	}
}

type differ struct {
	opts   Options
	ignore [][]string
	out    []domain.DiffEntry
}

func (d *differ) full() bool { return len(d.out) >= d.opts.MaxDiffs }

func (d *differ) add(e domain.DiffEntry) {
	if !d.full() {
		d.out = append(d.out, e)
	}
}

func (d *differ) walk(path string, segs []string, a, b any) {
	if d.full() || d.ignored(segs) {
		return
	}
	display := path
	if display == "" {
		display = "$"
	}
	ta, tb := kind(a), kind(b)
	if ta != tb {
		d.add(domain.DiffEntry{Path: display, Kind: domain.DiffTypeChanged,
			Before: describe(a), After: describe(b)})
		return
	}
	switch av := a.(type) {
	case map[string]any:
		bv := b.(map[string]any)
		for _, k := range unionKeys(av, bv) {
			child := assertions.JoinKey(path, k)
			childSegs := append(append([]string{}, segs...), k)
			va, okA := av[k]
			vb, okB := bv[k]
			switch {
			case okA && okB:
				d.walk(child, childSegs, va, vb)
			case okA:
				if !d.ignored(childSegs) {
					d.add(domain.DiffEntry{Path: child, Kind: domain.DiffRemoved, Before: assertions.Format(va)})
				}
			default:
				if !d.ignored(childSegs) {
					d.add(domain.DiffEntry{Path: child, Kind: domain.DiffAdded, After: assertions.Format(vb)})
				}
			}
		}
	case []any:
		bv := b.([]any)
		n := len(av)
		if len(bv) > n {
			n = len(bv)
		}
		for i := 0; i < n; i++ {
			child := assertions.JoinIndex(path, i)
			childSegs := append(append([]string{}, segs...), strconv.Itoa(i))
			switch {
			case i < len(av) && i < len(bv):
				d.walk(child, childSegs, av[i], bv[i])
			case i < len(av):
				if !d.ignored(childSegs) {
					d.add(domain.DiffEntry{Path: child, Kind: domain.DiffRemoved, Before: assertions.Format(av[i])})
				}
			default:
				if !d.ignored(childSegs) {
					d.add(domain.DiffEntry{Path: child, Kind: domain.DiffAdded, After: assertions.Format(bv[i])})
				}
			}
		}
	default:
		if ta == "number" {
			fa, _ := assertions.ToFloat(a)
			fb, _ := assertions.ToFloat(b)
			if fa == fb || math.Abs(fa-fb) <= d.opts.Tolerance {
				return
			}
		} else if assertions.ValuesEqual(a, b, 0) {
			return
		}
		d.add(domain.DiffEntry{Path: display, Kind: domain.DiffChanged,
			Before: assertions.Format(a), After: assertions.Format(b)})
	}
}

// kind groups integers and decimals together so 1 → 1.5 is a value change,
// not a type change.
func kind(v any) string {
	t := assertions.TypeName(v)
	if t == "integer" {
		return "number"
	}
	return t
}

func describe(v any) string { return assertions.TypeName(v) + " " + assertions.Format(v) }

func unionKeys[A, B any](a map[string]A, b map[string]B) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		set[k] = struct{}{}
	}
	for k := range b {
		set[k] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// compilePatterns turns ignore paths into segment patterns. "*" matches one
// segment (key or index) and "**" matches any number of segments.
func compilePatterns(paths []string) [][]string {
	var out [][]string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, splitPattern(p))
	}
	return out
}

// splitPattern splits on '.', '[' and ']' while keeping quoted keys intact.
func splitPattern(p string) []string {
	p = strings.TrimPrefix(strings.TrimPrefix(p, "$"), ".")
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch c {
		case '.', ']':
			flush()
		case '[':
			flush()
			if i+1 < len(p) && (p[i+1] == '"' || p[i+1] == '\'') {
				q := p[i+1]
				end := strings.IndexByte(p[i+2:], q)
				if end >= 0 {
					parts = append(parts, p[i+2:i+2+end])
					i = i + 2 + end
				}
			}
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return parts
}

func (d *differ) ignored(segs []string) bool {
	if len(segs) == 0 {
		return false
	}
	for _, p := range d.ignore {
		if matchSegs(p, segs) {
			return true
		}
	}
	return false
}

func matchSegs(pattern, segs []string) bool {
	if len(pattern) == 0 {
		return len(segs) == 0
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if matchSegs(pattern[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if pattern[0] != "*" && pattern[0] != segs[0] {
		return false
	}
	return matchSegs(pattern[1:], segs[1:])
}
