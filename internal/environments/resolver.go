// Package environments resolves {{variable}} references against an environment.
package environments

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"api-test-kits/internal/domain"
)

var referencePattern = regexp.MustCompile(`\{\{\s*(\$?[A-Za-z_][A-Za-z0-9_.\-]*)\s*\}\}`)

// Resolver substitutes variables and remembers which were missing.
type Resolver struct {
	values  map[string]string
	secrets map[string]bool
	now     func() time.Time
	missing map[string]bool
}

// NewResolver builds a resolver from the enabled variables of env. env may be nil.
func NewResolver(env *domain.Environment) *Resolver {
	r := &Resolver{
		values:  map[string]string{},
		secrets: map[string]bool{},
		now:     time.Now,
		missing: map[string]bool{},
	}
	if env != nil {
		for _, v := range env.Variables {
			if !v.Enabled || v.Key == "" {
				continue
			}
			r.values[v.Key] = v.Value
			if v.Secret {
				r.secrets[v.Key] = true
			}
		}
	}
	return r
}

// Resolve replaces every {{name}} in s. Unknown names are left untouched and
// recorded so callers can report them instead of sending a broken request.
func (r *Resolver) Resolve(s string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	return referencePattern.ReplaceAllStringFunc(s, func(m string) string {
		name := referencePattern.FindStringSubmatch(m)[1]
		if v, ok := r.builtin(name); ok {
			return v
		}
		if v, ok := r.values[name]; ok {
			return v
		}
		r.missing[name] = true
		return m
	})
}

func (r *Resolver) builtin(name string) (string, bool) {
	switch name {
	case "$uuid":
		return uuid.NewString(), true
	case "$timestamp":
		return strconv.FormatInt(r.now().Unix(), 10), true
	case "$isoTimestamp":
		return r.now().UTC().Format(time.RFC3339), true
	}
	return "", false
}

// Missing returns the sorted names that could not be resolved so far.
func (r *Resolver) Missing() []string {
	out := make([]string, 0, len(r.missing))
	for k := range r.missing {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IsSecret reports whether the named variable is marked secret.
func (r *Resolver) IsSecret(name string) bool { return r.secrets[name] }

// ReplaceFunc replaces each {{name}} in s with fn(name); references for which
// fn returns false are left unchanged.
func ReplaceFunc(s string, fn func(name string) (string, bool)) string {
	return referencePattern.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := fn(referencePattern.FindStringSubmatch(m)[1]); ok {
			return v
		}
		return m
	})
}

// References returns the variable names referenced in s, excluding built-ins.
func References(s string) []string {
	var out []string
	for _, m := range referencePattern.FindAllStringSubmatch(s, -1) {
		if !strings.HasPrefix(m[1], "$") {
			out = append(out, m[1])
		}
	}
	return out
}

// RequestReferences returns every variable referenced anywhere in req.
func RequestReferences(req *domain.Request) []string {
	set := map[string]bool{}
	add := func(s string) {
		for _, n := range References(s) {
			set[n] = true
		}
	}
	add(req.URLTemplate)
	add(req.Body)
	add(req.BodyFile)
	for _, f := range req.Form {
		if f.Enabled {
			add(f.Key)
			add(f.Value)
			add(f.File)
			add(f.ContentType)
		}
	}
	for _, kv := range append(append([]domain.KeyValue{}, req.Headers...), req.Query...) {
		if kv.Enabled {
			add(kv.Key)
			add(kv.Value)
		}
	}
	a := req.Auth
	for _, s := range []string{a.Token, a.Username, a.Password, a.KeyName, a.KeyValue} {
		add(s)
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MissingRequired returns required names that are absent, disabled or empty in env.
func MissingRequired(env *domain.Environment, required []string) []string {
	present := map[string]bool{}
	if env != nil {
		for _, v := range env.Variables {
			if v.Enabled && v.Value != "" {
				present[v.Key] = true
			}
		}
	}
	var out []string
	for _, name := range required {
		if !present[name] {
			out = append(out, name)
		}
	}
	return out
}
