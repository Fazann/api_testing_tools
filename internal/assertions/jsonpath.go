// Package assertions evaluates assertions against HTTP responses.
package assertions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Segment is one step of a parsed JSON path.
type Segment struct {
	Key      string
	Index    int
	IsIndex  bool
	Wildcard bool
}

// Match is a value selected by a path, with its concrete location.
type Match struct {
	Path  string
	Value any
}

const maxPathSegments = 64

// ParsePath parses paths such as "data.items[0].id", "$.data.items[*].amount",
// `meta["request-id"]` and "items[-1]". A leading "$" is optional.
func ParsePath(path string) ([]Segment, error) {
	p := strings.TrimSpace(path)
	p = strings.TrimPrefix(p, "$")
	var segs []Segment
	i := 0
	for i < len(p) {
		if len(segs) > maxPathSegments {
			return nil, fmt.Errorf("path %q has too many segments", path)
		}
		switch p[i] {
		case '.':
			i++
			if i >= len(p) {
				return nil, fmt.Errorf("path %q ends with '.'", path)
			}
			if p[i] == '.' || p[i] == '[' {
				return nil, fmt.Errorf("path %q has an empty segment", path)
			}
		case '[':
			end := strings.IndexByte(p[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("path %q has an unclosed '['", path)
			}
			inner := p[i+1 : i+end]
			if q := len(inner); q >= 2 && (inner[0] == '"' && inner[q-1] == '"' || inner[0] == '\'' && inner[q-1] == '\'') {
				// A quoted key may itself contain ']', so find the matching quote.
				quote := p[i+1]
				closeQ := strings.IndexByte(p[i+2:], quote)
				if closeQ < 0 || i+2+closeQ+1 >= len(p) || p[i+2+closeQ+1] != ']' {
					return nil, fmt.Errorf("path %q has an unterminated quoted key", path)
				}
				segs = append(segs, Segment{Key: p[i+2 : i+2+closeQ]})
				i = i + 2 + closeQ + 2
				continue
			}
			if inner == "*" {
				segs = append(segs, Segment{Wildcard: true})
			} else {
				n, err := strconv.Atoi(strings.TrimSpace(inner))
				if err != nil {
					return nil, fmt.Errorf("path %q has an invalid index %q", path, inner)
				}
				segs = append(segs, Segment{Index: n, IsIndex: true})
			}
			i += end + 1
			continue
		}
		start := i
		for i < len(p) && p[i] != '.' && p[i] != '[' {
			i++
		}
		key := p[start:i]
		if key == "" {
			continue
		}
		if key == "*" {
			segs = append(segs, Segment{Wildcard: true})
		} else {
			segs = append(segs, Segment{Key: key})
		}
	}
	return segs, nil
}

// HasWildcard reports whether segs can select more than one value.
func HasWildcard(segs []Segment) bool {
	for _, s := range segs {
		if s.Wildcard {
			return true
		}
	}
	return false
}

// Select returns every value in doc matched by path. A missing path yields no
// matches rather than an error; only a malformed path is an error.
func Select(doc any, path string) ([]Match, error) {
	segs, err := ParsePath(path)
	if err != nil {
		return nil, err
	}
	return SelectSegments(doc, segs), nil
}

// SelectSegments is Select with a pre-parsed path.
func SelectSegments(doc any, segs []Segment) []Match {
	cur := []Match{{Path: "", Value: doc}}
	for _, s := range segs {
		var next []Match
		for _, m := range cur {
			switch v := m.Value.(type) {
			case map[string]any:
				if s.Wildcard {
					for _, k := range sortedKeys(v) {
						next = append(next, Match{Path: JoinKey(m.Path, k), Value: v[k]})
					}
				} else if !s.IsIndex {
					if child, ok := v[s.Key]; ok {
						next = append(next, Match{Path: JoinKey(m.Path, s.Key), Value: child})
					}
				}
			case []any:
				if s.Wildcard {
					for i, child := range v {
						next = append(next, Match{Path: JoinIndex(m.Path, i), Value: child})
					}
				} else if s.IsIndex {
					idx := s.Index
					if idx < 0 {
						idx += len(v)
					}
					if idx >= 0 && idx < len(v) {
						next = append(next, Match{Path: JoinIndex(m.Path, idx), Value: v[idx]})
					}
				}
			}
		}
		cur = next
		if len(cur) == 0 {
			return nil
		}
	}
	return cur
}

// JoinKey appends an object key to a display path.
func JoinKey(base, key string) string {
	if isSimpleKey(key) {
		if base == "" {
			return key
		}
		return base + "." + key
	}
	return base + "[" + strconv.Quote(key) + "]"
}

// JoinIndex appends an array index to a display path.
func JoinIndex(base string, i int) string { return base + "[" + strconv.Itoa(i) + "]" }

func isSimpleKey(k string) bool {
	if k == "" || k == "*" || k == "$" {
		return false
	}
	for _, c := range k {
		if !(c == '_' || c == '-' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// DecodeJSON parses data keeping numbers as json.Number so that large
// integers and decimals are compared without precision loss.
func DecodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected data after the JSON value")
	}
	return v, nil
}

// ToFloat converts JSON numbers to float64.
func ToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// TypeName returns the JSON type of v.
func TypeName(v any) string {
	switch n := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number, float64, int, int64:
		f, _ := ToFloat(n)
		if f == float64(int64(f)) && !strings.ContainsAny(fmt.Sprint(n), ".eE") {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

// Format renders a JSON value compactly for messages, truncated to a safe size.
func Format(v any) string {
	if s, ok := v.(string); ok {
		return truncate(strconv.Quote(s))
	}
	b, err := json.Marshal(v)
	if err != nil {
		return truncate(fmt.Sprint(v))
	}
	return truncate(string(b))
}

func truncate(s string) string {
	const max = 500
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
