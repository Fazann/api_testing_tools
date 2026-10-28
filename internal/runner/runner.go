// Package runner executes saved requests: it resolves variables, sends the
// request, evaluates assertions and compares against a baseline. It has no
// dependency on the desktop layer so that a CLI can reuse it.
package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/environments"
	"api-test-kits/internal/httpclient"
	"api-test-kits/internal/redact"
	"api-test-kits/internal/regression"
)

// Sender is the subset of httpclient.Client used by the runner.
type Sender interface {
	Do(ctx context.Context, req httpclient.Request) (*httpclient.Response, error)
}

type Runner struct {
	client Sender
	now    func() time.Time
}

func New(client Sender) *Runner { return &Runner{client: client, now: time.Now} }

// Prepared is a resolved request plus everything needed to redact it.
type Prepared struct {
	HTTP     httpclient.Request
	Redactor *redact.Redactor
	Summary  domain.RequestSummary
}

// MissingVariablesError lists variables referenced but not defined.
type MissingVariablesError struct{ Names []string }

func (e *MissingVariablesError) Error() string {
	return "undefined variables: " + strings.Join(e.Names, ", ") + " (define them in the selected environment)"
}

// Prepare resolves variables and authentication for req.
func Prepare(req domain.Request, env *domain.Environment) (*Prepared, error) {
	req.Normalize()
	res := environments.NewResolver(env)

	auth := req.Auth
	auth.Token = res.Resolve(auth.Token)
	auth.Username = res.Resolve(auth.Username)
	auth.Password = res.Resolve(auth.Password)
	auth.KeyName = res.Resolve(auth.KeyName)
	auth.KeyValue = res.Resolve(auth.KeyValue)

	red := redact.New(append(env.SecretValues(), auth.Credentials()...)...)
	if auth.Type == domain.AuthAPIKey && auth.KeyIn == domain.APIKeyHeader {
		red.AddSensitiveHeader(auth.KeyName)
	}

	rawURL := res.Resolve(req.URLTemplate)
	header := http.Header{}
	var credentialHeaders []string
	for _, h := range req.Headers {
		if !h.Enabled || strings.TrimSpace(h.Key) == "" {
			continue
		}
		name := strings.TrimSpace(res.Resolve(h.Key))
		value := res.Resolve(h.Value)
		if !domain.IsValidHeaderName(name) {
			return nil, fmt.Errorf("header name %q is invalid after variable substitution", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("header %s contains a line break after variable substitution", name)
		}
		header.Add(name, value)
		if headerUsesSecret(h, res) {
			red.AddSensitiveHeader(name)
			credentialHeaders = append(credentialHeaders, name)
		}
	}
	var body []byte
	if req.BodyType != domain.BodyNone && req.Method != http.MethodGet && req.Method != http.MethodHead {
		body = []byte(res.Resolve(req.Body))
		if header.Get("Content-Type") == "" {
			if req.BodyType == domain.BodyJSON {
				header.Set("Content-Type", "application/json")
			} else {
				header.Set("Content-Type", "text/plain; charset=utf-8")
			}
		}
	}

	query := make([][2]string, 0, len(req.Query))
	for _, q := range req.Query {
		if q.Enabled && strings.TrimSpace(q.Key) != "" {
			query = append(query, [2]string{res.Resolve(q.Key), res.Resolve(q.Value)})
		}
	}

	switch auth.Type {
	case domain.AuthBearer:
		header.Set("Authorization", "Bearer "+auth.Token)
	case domain.AuthBasic:
		header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth.Username+":"+auth.Password)))
	case domain.AuthAPIKey:
		if auth.KeyIn == domain.APIKeyQuery {
			query = append(query, [2]string{auth.KeyName, auth.KeyValue})
		} else {
			header.Set(auth.KeyName, auth.KeyValue)
			credentialHeaders = append(credentialHeaders, auth.KeyName)
		}
	}

	if missing := res.Missing(); len(missing) > 0 {
		return nil, &MissingVariablesError{Names: missing}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", red.String(rawURL), err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("URL %q must start with http:// or https://", red.String(rawURL))
	}
	if len(query) > 0 {
		values := u.Query()
		for _, kv := range query {
			values.Add(kv[0], kv[1])
		}
		u.RawQuery = values.Encode()
	}
	finalURL := u.String()

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	p := &Prepared{
		HTTP: httpclient.Request{
			Method:            req.Method,
			URL:               finalURL,
			Header:            header,
			Body:              body,
			Timeout:           timeout,
			FollowRedirects:   req.FollowRedirects,
			CredentialHeaders: credentialHeaders,
		},
		Redactor: red,
	}
	p.Summary = domain.RequestSummary{
		Name:    req.Name,
		Method:  req.Method,
		URL:     red.URL(finalURL),
		Headers: red.HTTPHeader(header),
		Body:    red.String(truncateString(string(body), 64<<10)),
	}
	return p, nil
}

// headerUsesSecret reports whether a header row references a secret variable.
func headerUsesSecret(h domain.KeyValue, res *environments.Resolver) bool {
	for _, name := range environments.References(h.Value) {
		if res.IsSecret(name) {
			return true
		}
	}
	return false
}

// Run executes one request. baseline may be nil. The returned result is
// already redacted and safe to display, store and export.
func (r *Runner) Run(ctx context.Context, req domain.Request, env *domain.Environment, baseline *domain.Baseline) domain.RunResult {
	result := domain.RunResult{
		ID:               uuid.NewString(),
		RequestID:        req.ID,
		Request:          domain.RequestSummary{Name: req.Name, Method: strings.ToUpper(req.Method)},
		AssertionResults: []domain.AssertionResult{},
		CreatedAt:        r.now().UTC(),
	}
	if err := ctx.Err(); err != nil {
		result.Status = domain.StatusCancelled
		result.Error = "run was cancelled before this request started"
		return result
	}

	prepared, err := Prepare(req, env)
	if err != nil {
		result.Status = domain.StatusError
		// Prepare errors never contain unredacted secrets, but be defensive.
		result.Error = redact.New(env.SecretValues()...).String(err.Error())
		return result
	}
	red := prepared.Redactor
	result.Request = prepared.Summary

	start := r.now()
	resp, err := r.client.Do(ctx, prepared.HTTP)
	if err != nil {
		result.DurationMs = r.now().Sub(start).Milliseconds()
		result.Status = domain.StatusError
		var he *httpclient.Error
		if errors.As(err, &he) && he.Kind == httpclient.ErrCancelled {
			result.Status = domain.StatusCancelled
		}
		result.Error = red.String(err.Error())
		return result
	}
	result.DurationMs = resp.Duration.Milliseconds()

	redactedBody := red.String(string(resp.Body))
	redirects := make([]string, len(resp.Redirects))
	for i, u := range resp.Redirects {
		redirects[i] = red.URL(u)
	}
	result.Response = &domain.ResponseSummary{
		StatusCode:  resp.StatusCode,
		Status:      resp.Status,
		Headers:     red.HTTPHeader(resp.Header),
		Body:        redactedBody,
		BodyBytes:   resp.BodyBytes,
		Truncated:   resp.Truncated,
		ContentType: resp.Header.Get("Content-Type"),
		Redirects:   redirects,
	}

	results := assertions.Evaluate(assertions.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       resp.Body,
		Duration:   resp.Duration,
		Truncated:  resp.Truncated,
	}, resolveAssertions(req.Assertions, env))
	for i := range results {
		results[i].Expected = red.String(results[i].Expected)
		results[i].Actual = red.String(results[i].Actual)
		results[i].Message = red.String(results[i].Message)
	}
	result.AssertionResults = results

	if baseline != nil {
		// Baselines are approved from redacted bodies, so compare redacted to redacted.
		reg := regression.Compare(*baseline, []byte(redactedBody), resp.Truncated)
		result.Regression = &reg
	}

	result.Status = domain.StatusPassed
	for _, a := range results {
		if !a.Passed {
			result.Status = domain.StatusFailed
		}
	}
	if result.Regression != nil && !result.Regression.Passed {
		result.Status = domain.StatusFailed
	}
	return result
}

// resolveAssertions substitutes {{variables}} inside expected values so that
// assertions can compare against environment-specific data. Values are
// JSON-escaped so a variable cannot break out of a JSON string.
func resolveAssertions(list []domain.Assertion, env *domain.Environment) []domain.Assertion {
	res := environments.NewResolver(env)
	out := make([]domain.Assertion, len(list))
	for i, a := range list {
		if len(a.Expected) > 0 && strings.Contains(string(a.Expected), "{{") {
			resolved := environments.ReplaceFunc(string(a.Expected), func(name string) (string, bool) {
				v := res.Resolve("{{" + name + "}}")
				if v == "{{"+name+"}}" {
					return "", false
				}
				enc, _ := json.Marshal(v)
				return string(enc[1 : len(enc)-1]), true
			})
			a.Expected = json.RawMessage(resolved)
		}
		out[i] = a
	}
	return out
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n… (truncated)"
}
