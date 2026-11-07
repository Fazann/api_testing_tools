package contract

import (
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
)

// MaxProblems is how many problems are reported per response.
const MaxProblems = 20

// Checker checks responses against a spec with the collection's options.
type Checker struct {
	Spec *Spec
	// AllowUndocumented accepts fields the spec does not mention.
	AllowUndocumented bool
}

// Response is what the checker looks at. Body is the full (unredacted)
// body; results only quote short values and are redacted by the caller.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Truncated  bool
}

// Check returns the contract results for a request sent with method to
// rawURL, or nil when no operation of the spec matches it.
func (c *Checker) Check(method, rawURL string, resp Response) []domain.AssertionResult {
	if c == nil || c.Spec == nil {
		return nil
	}
	op := c.Spec.Match(method, rawURL)
	if op == nil {
		return nil
	}
	return op.Check(resp, c.AllowUndocumented)
}

type problem struct{ path, msg string }

// Check validates resp against the operation's documented responses.
func (o *Operation) Check(resp Response, allowUndocumented bool) []domain.AssertionResult {
	s := o.spec
	label := o.Label()
	result := func(passed bool, path, msg string) domain.AssertionResult {
		return domain.AssertionResult{Kind: domain.AssertContract, Path: path, Description: label + " → " + strconv.Itoa(resp.StatusCode),
			Passed: passed, Message: msg}
	}

	responses := s.resolve(o.op["responses"])
	key, documented := responseKey(responses, resp.StatusCode)
	if !documented {
		return []domain.AssertionResult{result(false, "", fmt.Sprintf("status %d is not documented for %s (documented: %s)",
			resp.StatusCode, label, documentedCodes(responses)))}
	}
	r := s.resolve(responses[key])
	var probs []problem

	// Required headers (OpenAPI 3 only; Swagger 2 headers have no "required").
	names := make([]string, 0)
	for name := range obj(r["headers"]) {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h := s.resolve(obj(r["headers"])[name])
		if req, _ := h["required"].(bool); req && resp.Header.Get(name) == "" && !strings.EqualFold(name, "content-type") {
			probs = append(probs, problem{msg: "required response header " + name + " is missing"})
		}
	}

	ct := mediaType(resp.Header.Get("Content-Type"))
	empty := len(strings.TrimSpace(string(resp.Body))) == 0
	var schema any
	hasSchema := false
	if s.v3 {
		content := obj(r["content"])
		if len(content) > 0 {
			mkey, ok := matchMedia(content, ct)
			switch {
			case ct == "" && empty:
				// No body at all: only a documented JSON schema makes that a problem.
				if sc, has := s.resolve(content[mkey])["schema"]; mkey != "" && has {
					schema, hasSchema = sc, true
				}
			case !ok && ct == "":
				probs = append(probs, problem{msg: "the response has no Content-Type (documented: " + mediaList(content) + ")"})
			case !ok:
				probs = append(probs, problem{msg: "Content-Type " + ct + " is not documented (documented: " + mediaList(content) + ")"})
			default:
				media := s.resolve(content[mkey])
				if sc, ok := media["schema"]; ok && (isJSON(ct) || (ct == "" && isJSON(mediaType(mkey)))) {
					schema, hasSchema = sc, true
				}
			}
		}
	} else {
		produces := strs(o.op["produces"])
		if len(produces) == 0 {
			produces = s.produces
		}
		if sc, ok := r["schema"]; ok && (isJSON(ct) || ct == "") {
			schema, hasSchema = sc, true
		} else if ok && ct != "" && !containsMedia(produces, ct) {
			probs = append(probs, problem{msg: "Content-Type " + ct + " is not documented (documented: " + strings.Join(produces, ", ") + ")"})
		}
	}

	note := ""
	if hasSchema && o.Method != "HEAD" {
		switch {
		case resp.Truncated:
			note = "body not checked: it was truncated by the response size limit"
		case empty:
			probs = append(probs, problem{path: "$", msg: "the body is empty, but the spec documents a JSON schema"})
		default:
			doc, err := assertions.DecodeJSON(resp.Body)
			if err != nil {
				probs = append(probs, problem{path: "$", msg: "the body is not valid JSON: " + err.Error()})
				break
			}
			v := newValidator(s, !allowUndocumented, MaxProblems-len(probs))
			v.validate(schema, doc, "", 0, true)
			probs = append(probs, v.problems...)
			if len(probs) > 0 && v.omitted > 0 {
				probs = append(probs, problem{msg: fmt.Sprintf("…and %d more problems", v.omitted)})
			}
			if v.exhausted {
				note = "validation stopped early: the schema is too complex for this response"
			}
		}
	}

	if len(probs) == 0 {
		msg := "Matches " + label + " → " + key
		if note != "" {
			msg += " (" + note + ")"
		}
		return []domain.AssertionResult{result(true, "", msg)}
	}
	out := make([]domain.AssertionResult, 0, len(probs))
	for _, p := range probs {
		out = append(out, result(false, p.path, p.msg))
	}
	return out
}

// responseKey finds the documented response for code: exact, range ("4XX"), default.
func responseKey(responses map[string]any, code int) (string, bool) {
	exact := strconv.Itoa(code)
	if _, ok := responses[exact]; ok {
		return exact, true
	}
	rng := exact[:1] + "XX"
	for k := range responses {
		if strings.EqualFold(k, rng) {
			return k, true
		}
	}
	if _, ok := responses["default"]; ok {
		return "default", true
	}
	return "", false
}

func documentedCodes(responses map[string]any) string {
	keys := make([]string, 0, len(responses))
	for k := range responses {
		if !strings.HasPrefix(k, "x-") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "none"
	}
	return strings.Join(keys, ", ")
}

func mediaType(ct string) string {
	ct = strings.TrimSpace(ct)
	if ct == "" {
		return ""
	}
	if mt, _, err := mime.ParseMediaType(ct); err == nil {
		return mt
	}
	mt, _, _ := strings.Cut(ct, ";")
	return strings.ToLower(strings.TrimSpace(mt))
}

func isJSON(mt string) bool {
	return mt == "application/json" || strings.HasSuffix(mt, "+json") || strings.Contains(mt, "/json")
}

// matchMedia finds the content key for media type ct: exact, "type/*", "*/*".
// When the response has no Content-Type, the first JSON key is used.
func matchMedia(content map[string]any, ct string) (string, bool) {
	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if ct == "" {
		for _, k := range keys {
			if isJSON(mediaType(k)) {
				return k, false
			}
		}
		return "", false
	}
	major, _, _ := strings.Cut(ct, "/")
	for _, want := range []string{ct, major + "/*", "*/*"} {
		for _, k := range keys {
			if mediaType(k) == want {
				return k, true
			}
		}
	}
	return "", false
}

func containsMedia(list []string, ct string) bool {
	if len(list) == 0 {
		return true // nothing documented
	}
	major, _, _ := strings.Cut(ct, "/")
	for _, l := range list {
		switch mediaType(l) {
		case ct, major + "/*", "*/*":
			return true
		}
	}
	return false
}

func mediaList(content map[string]any) string {
	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
