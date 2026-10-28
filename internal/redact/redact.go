// Package redact removes secret values from anything that is displayed,
// stored in history or exported.
package redact

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"api-test-kits/internal/domain"
)

const Mask = "[REDACTED]"

// minSubstringLength is the shortest secret that is replaced wherever it
// appears. Shorter secrets would mangle unrelated text, so they are only
// redacted where they form a whole value (a header, query value or variable).
const minSubstringLength = 4

// sensitiveHeaders are always masked regardless of their value.
var sensitiveHeaders = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
	"Cookie":              true,
	"Set-Cookie":          true,
	"X-Api-Key":           true,
	"X-Auth-Token":        true,
	"X-Csrf-Token":        true,
}

// Redactor replaces known secret values.
type Redactor struct {
	secrets      []string // sorted longest first so overlapping secrets are fully masked
	short        map[string]bool
	extraHeaders map[string]bool
}

// New returns a redactor for the given secret values. Empty values are ignored.
func New(secrets ...string) *Redactor {
	r := &Redactor{short: map[string]bool{}, extraHeaders: map[string]bool{}}
	seen := map[string]bool{}
	for _, s := range secrets {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if len(s) < minSubstringLength {
			r.short[s] = true
			continue
		}
		r.secrets = append(r.secrets, s)
	}
	sort.Slice(r.secrets, func(i, j int) bool { return len(r.secrets[i]) > len(r.secrets[j]) })
	return r
}

// AddSensitiveHeader masks the named header in addition to the defaults,
// for example a custom API key header.
func (r *Redactor) AddSensitiveHeader(name string) {
	if name != "" {
		r.extraHeaders[http.CanonicalHeaderKey(name)] = true
	}
}

// IsSensitiveHeader reports whether a header value must always be masked.
func (r *Redactor) IsSensitiveHeader(name string) bool {
	k := http.CanonicalHeaderKey(name)
	return sensitiveHeaders[k] || r.extraHeaders[k]
}

// String replaces every secret occurrence in s.
func (r *Redactor) String(s string) string {
	if s == "" {
		return s
	}
	if r.short[s] {
		return Mask
	}
	for _, secret := range r.secrets {
		if strings.Contains(s, secret) {
			s = strings.ReplaceAll(s, secret, Mask)
		}
	}
	// Secrets may appear percent-encoded in URLs and form bodies.
	for _, secret := range r.secrets {
		enc := url.QueryEscape(secret)
		if enc != secret && strings.Contains(s, enc) {
			s = strings.ReplaceAll(s, enc, Mask)
		}
		pathEnc := url.PathEscape(secret)
		if pathEnc != secret && pathEnc != enc && strings.Contains(s, pathEnc) {
			s = strings.ReplaceAll(s, pathEnc, Mask)
		}
	}
	return s
}

// Headers returns a redacted copy of headers.
func (r *Redactor) Headers(headers []domain.KeyValue) []domain.KeyValue {
	out := make([]domain.KeyValue, len(headers))
	for i, h := range headers {
		out[i] = h
		if r.IsSensitiveHeader(h.Key) {
			out[i].Value = Mask
		} else {
			out[i].Value = r.String(h.Value)
		}
	}
	return out
}

// URL redacts secret values from a URL, including short secrets used as whole
// query values.
func (r *Redactor) URL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return r.String(raw)
	}
	if u.User != nil {
		u.User = url.User("redacted")
	}
	if len(r.short) > 0 && u.RawQuery != "" {
		q := u.Query()
		changed := false
		for k, vals := range q {
			for i, v := range vals {
				if r.short[v] {
					vals[i] = Mask
					changed = true
				}
			}
			q[k] = vals
		}
		if changed {
			u.RawQuery = q.Encode()
		}
	}
	return r.String(u.String())
}

// HTTPHeader converts an http.Header into sorted, redacted rows.
func (r *Redactor) HTTPHeader(h http.Header) []domain.KeyValue {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []domain.KeyValue
	for _, k := range keys {
		for _, v := range h[k] {
			out = append(out, domain.KeyValue{Key: k, Value: v, Enabled: true})
		}
	}
	return r.Headers(out)
}
