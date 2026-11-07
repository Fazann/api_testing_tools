// Package contract checks real responses against the responses documented in
// an OpenAPI 3.x or Swagger 2.0 document (docs/openapi-contract.md).
//
// The document is untrusted: $ref chains, nesting and the amount of work per
// response are bounded, and external references are never fetched.
package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Limits for an attached spec.
const (
	MaxSpecBytes   = 10 << 20
	MaxOperations  = 5_000
	maxRefHops     = 20
	maxPatternSize = 512
)

var methods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// Spec is a parsed OpenAPI or Swagger document. It is safe for concurrent use.
type Spec struct {
	root    map[string]any
	v3      bool
	Title   string
	Version string
	// basePaths are the server path prefixes ("/v1"), without a trailing slash.
	basePaths []string
	produces  []string
	ops       []*Operation
	patterns  sync.Map // pattern source → *regexp.Regexp, or nil when it does not compile
}

// Operation is one method on one path of the spec.
type Operation struct {
	Method  string // upper case
	Path    string // the path template as written, e.g. /pets/{petId}
	Summary string
	segs    []segment
	literal int
	op      map[string]any
	spec    *Spec
}

type segment struct {
	literal string
	re      *regexp.Regexp // set for segments with a {parameter}
}

// Parse reads an OpenAPI 3.x or Swagger 2.0 document in JSON.
func Parse(data []byte) (*Spec, error) {
	if len(data) > MaxSpecBytes {
		return nil, fmt.Errorf("the spec is larger than %d MB", MaxSpecBytes>>20)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("the spec is not valid JSON: %w", err)
	}
	s := &Spec{root: root}
	switch {
	case strings.HasPrefix(str(root["openapi"]), "3."):
		s.v3 = true
	case str(root["swagger"]) == "2.0":
	default:
		return nil, fmt.Errorf("unsupported OpenAPI/Swagger version (openapi %q, swagger %q); supported: 3.x and 2.0",
			str(root["openapi"]), str(root["swagger"]))
	}
	info := obj(root["info"])
	s.Title, s.Version = strings.TrimSpace(str(info["title"])), strings.TrimSpace(str(info["version"]))
	s.basePaths = s.serverPaths()
	s.produces = strs(root["produces"])

	paths := obj(root["paths"])
	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, p := range keys {
		item := s.resolve(paths[p])
		for _, m := range methods {
			op := obj(item[m])
			if op == nil {
				continue
			}
			if len(s.ops) >= MaxOperations {
				return nil, fmt.Errorf("the spec has more than %d operations", MaxOperations)
			}
			segs, literal := compilePath(p)
			summary := strings.TrimSpace(str(op["summary"]))
			if summary == "" {
				summary = strings.TrimSpace(str(op["operationId"]))
			}
			s.ops = append(s.ops, &Operation{Method: strings.ToUpper(m), Path: p, Summary: summary,
				segs: segs, literal: literal, op: op, spec: s})
		}
	}
	if len(s.ops) == 0 {
		return nil, errors.New("the spec has no operations")
	}
	return s, nil
}

// Operations lists the spec's operations in path order.
func (s *Spec) Operations() []*Operation { return s.ops }

func (s *Spec) serverPaths() []string {
	var raw []string
	if s.v3 {
		for _, sv := range arr(s.root["servers"]) {
			m := obj(sv)
			u := str(m["url"])
			for name, v := range obj(m["variables"]) {
				u = strings.ReplaceAll(u, "{"+name+"}", str(obj(v)["default"]))
			}
			raw = append(raw, u)
		}
	} else {
		raw = append(raw, str(s.root["basePath"]))
	}
	var out []string
	seen := map[string]bool{}
	for _, u := range raw {
		p := u
		if parsed, err := url.Parse(u); err == nil {
			p = parsed.Path
		}
		p = strings.TrimRight(p, "/")
		if p != "" && !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

var paramPattern = regexp.MustCompile(`\{[^{}/]+\}`)

func compilePath(p string) ([]segment, int) {
	var segs []segment
	literal := 0
	for _, part := range splitPath(p) {
		if !strings.Contains(part, "{") {
			segs = append(segs, segment{literal: part})
			literal++
			continue
		}
		var b strings.Builder
		b.WriteString("^")
		last := 0
		for _, loc := range paramPattern.FindAllStringIndex(part, -1) {
			b.WriteString(regexp.QuoteMeta(part[last:loc[0]]))
			b.WriteString(".+?")
			last = loc[1]
		}
		b.WriteString(regexp.QuoteMeta(part[last:]))
		b.WriteString("$")
		re, err := regexp.Compile(b.String())
		if err != nil {
			segs = append(segs, segment{literal: part})
			continue
		}
		segs = append(segs, segment{re: re})
	}
	return segs, literal
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func (o *Operation) matches(segs []string) bool {
	if len(segs) != len(o.segs) {
		return false
	}
	for i, s := range o.segs {
		if s.re != nil {
			if segs[i] == "" || !s.re.MatchString(segs[i]) {
				return false
			}
		} else if s.literal != segs[i] {
			return false
		}
	}
	return true
}

// Match returns the operation for method and the path of rawURL, or nil.
// The spec's server path may precede the operation path; when nothing matches
// that way, an operation whose path ends the URL's path is accepted (for
// gateways that add a prefix). The most specific operation wins.
func (s *Spec) Match(method, rawURL string) *Operation {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	var segs []string
	for _, part := range splitPath(u.EscapedPath()) {
		if p, err := url.PathUnescape(part); err == nil {
			part = p
		}
		segs = append(segs, part)
	}
	method = strings.ToUpper(method)
	candidates := [][]string{segs}
	for _, bp := range s.basePaths {
		prefix := splitPath(bp)
		if len(prefix) <= len(segs) && equalStrings(prefix, segs[:len(prefix)]) {
			candidates = append(candidates, segs[len(prefix):])
		}
	}
	if op := s.best(method, candidates); op != nil {
		return op
	}
	var suffixes [][]string
	for i := 1; i < len(segs); i++ {
		suffixes = append(suffixes, segs[i:])
	}
	return s.best(method, suffixes)
}

func (s *Spec) best(method string, candidates [][]string) *Operation {
	var best *Operation
	for _, segs := range candidates {
		for _, op := range s.ops {
			if op.Method != method || !op.matches(segs) {
				continue
			}
			if best == nil || op.literal > best.literal || (op.literal == best.literal && len(op.segs) > len(best.segs)) {
				best = op
			}
		}
	}
	return best
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Label is "GET /pets/{petId}".
func (o *Operation) Label() string { return o.Method + " " + o.Path }

// ---- $ref resolution ----

// resolve follows local "$ref" pointers. External references resolve to nil.
func (s *Spec) resolve(v any) map[string]any {
	m := obj(v)
	for hops := 0; m != nil && hops < maxRefHops; hops++ {
		ref, ok := m["$ref"].(string)
		if !ok {
			return m
		}
		m = obj(s.pointer(ref))
	}
	if m != nil {
		if _, ok := m["$ref"]; ok {
			return nil // too many hops or a cycle
		}
	}
	return m
}

func (s *Spec) pointer(ref string) any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	var cur any = s.root
	for _, part := range strings.Split(ref[2:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		if p, err := url.PathUnescape(part); err == nil {
			part = p
		}
		switch node := cur.(type) {
		case map[string]any:
			cur = node[part]
		case []any:
			i := 0
			if _, err := fmt.Sscanf(part, "%d", &i); err != nil || i < 0 || i >= len(node) {
				return nil
			}
			cur = node[i]
		default:
			return nil
		}
	}
	return cur
}

func (s *Spec) pattern(src string) *regexp.Regexp {
	if len(src) > maxPatternSize {
		return nil
	}
	if v, ok := s.patterns.Load(src); ok {
		re, _ := v.(*regexp.Regexp)
		return re
	}
	re, err := regexp.Compile(src)
	if err != nil {
		re = nil
	}
	s.patterns.Store(src, re)
	return re
}

// ---- small helpers ----

func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }
func arr(v any) []any          { a, _ := v.([]any); return a }
func str(v any) string         { s, _ := v.(string); return s }

func strs(v any) []string {
	var out []string
	for _, x := range arr(v) {
		if s := str(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}
