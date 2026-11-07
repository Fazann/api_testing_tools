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
	"api-test-kits/internal/contract"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/environments"
	"api-test-kits/internal/httpclient"
	"api-test-kits/internal/redact"
	"api-test-kits/internal/regression"
	"api-test-kits/internal/scripting"
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
	// Socket is set for WebSocket and Socket.IO requests: their settings with
	// every {{variable}} resolved. HTTP.URL and HTTP.Header are the handshake.
	Socket *domain.SocketConfig
	// BodyText is the body as text for history and scripts: the body itself,
	// or for form-data and binary bodies a list of fields with files described
	// by name, size and type. Not redacted.
	BodyText string
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
	var bodyText string
	if req.BodyType != domain.BodyNone && req.Method != http.MethodGet && req.Method != http.MethodHead && !req.Kind.IsSocket() {
		built, err := buildBody(req, res)
		if err != nil {
			return nil, errors.New(red.String(err.Error()))
		}
		if built != nil {
			body, bodyText = built.data, built.summary
			// A multipart body only parses with the boundary it was written with.
			if header.Get("Content-Type") == "" || req.BodyType == domain.BodyFormData {
				header.Set("Content-Type", built.contentType)
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

	var sock *domain.SocketConfig
	if req.Kind.IsSocket() {
		sock = resolveSocket(*req.Socket, res, env)
	}

	if missing := res.Missing(); len(missing) > 0 {
		return nil, &MissingVariablesError{Names: missing}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", red.String(rawURL), err)
	}
	if u.Scheme == "" {
		if req.Kind.IsSocket() {
			return nil, fmt.Errorf("URL %q must start with ws://, wss://, http:// or https://", red.String(rawURL))
		}
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
		Socket:   sock,
	}
	p.Summary = domain.RequestSummary{
		Name:    req.Name,
		Method:  req.Kind.Badge(req.Method),
		URL:     red.URL(finalURL),
		Headers: red.HTTPHeader(header),
		Body:    red.String(truncateString(bodyText, 64<<10)),
	}
	p.BodyText = bodyText
	return p, nil
}

// resolveSocket substitutes variables in everything a socket request sends:
// subprotocols, the Socket.IO auth payload, saved messages and test steps
// (including the expected values of their checks).
func resolveSocket(c domain.SocketConfig, res *environments.Resolver, env *domain.Environment) *domain.SocketConfig {
	out := c
	out.Subprotocols = make([]string, len(c.Subprotocols))
	for i, p := range c.Subprotocols {
		out.Subprotocols[i] = res.Resolve(p)
	}
	out.AuthPayload = res.Resolve(c.AuthPayload)
	out.Messages = make([]domain.SocketMessage, len(c.Messages))
	for i, m := range c.Messages {
		out.Messages[i] = ResolveSocketMessage(m, res)
	}
	out.Steps = make([]domain.SocketStep, len(c.Steps))
	for i, s := range c.Steps {
		if s.Enabled {
			s.Message = ResolveSocketMessage(s.Message, res)
			s.Event = res.Resolve(s.Event)
			s.Assertions = resolveAssertions(s.Assertions, env)
		}
		out.Steps[i] = s
	}
	return &out
}

// ResolveSocketMessage substitutes variables in a message's event and body.
func ResolveSocketMessage(m domain.SocketMessage, res *environments.Resolver) domain.SocketMessage {
	m.Event = res.Resolve(m.Event)
	m.Body = res.Resolve(m.Body)
	return m
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

// Scope is the state shared by every request in a run: the variables that
// scripts may change, and the collection's scripts that wrap each request's own.
type Scope struct {
	Vars               *scripting.Variables
	PreRequestScript   string
	PostResponseScript string
	// Contract checks responses against the collection's OpenAPI spec; nil
	// when none is attached or the check is off.
	Contract *contract.Checker
}

// NewScope returns a scope over env (which may be nil) with no collection.
func NewScope(env *domain.Environment) *Scope {
	return &Scope{Vars: scripting.NewVariables(env, nil)}
}

type script struct{ name, source string }

func scripts(list ...script) []script {
	var out []script
	for _, s := range list {
		if strings.TrimSpace(s.source) != "" {
			out = append(out, s)
		}
	}
	return out
}

// Run executes one request. baseline may be nil. The returned result is
// already redacted and safe to display, store and export.
func (r *Runner) Run(ctx context.Context, req domain.Request, env *domain.Environment, baseline *domain.Baseline) domain.RunResult {
	return r.RunScoped(ctx, req, NewScope(env), baseline)
}

// RunScoped executes one request within a run's scope: collection and
// request pre-request scripts, then the request, then the post-response
// scripts. Variable changes made by scripts are visible to later requests.
func (r *Runner) RunScoped(ctx context.Context, req domain.Request, scope *Scope, baseline *domain.Baseline) domain.RunResult {
	if scope == nil {
		scope = NewScope(nil)
	}
	req.Normalize()
	result := domain.RunResult{
		ID:               uuid.NewString(),
		RequestID:        req.ID,
		Request:          domain.RequestSummary{Name: req.Name, Method: req.Kind.Badge(req.Method)},
		AssertionResults: []domain.AssertionResult{},
		CreatedAt:        r.now().UTC(),
	}
	if err := ctx.Err(); err != nil {
		result.Status = domain.StatusCancelled
		result.Error = "run was cancelled before this request started"
		return result
	}

	locals := map[string]string{}
	var logs []domain.ScriptLog
	// Logs are redacted with every secret known at the end, including ones
	// that scripts set.
	finish := func(red *redact.Redactor) domain.RunResult {
		if red == nil {
			red = redact.New(scope.Vars.Resolved(locals).SecretValues()...)
		}
		for _, l := range logs {
			l.Message = red.String(l.Message)
			result.Logs = append(result.Logs, l)
		}
		return result
	}
	collect := func(phase scripting.Phase, out scripting.Output) { logs = append(logs, scriptLogs(phase, out)...) }

	req, env, prepared, preLogs, err := PreRequest(ctx, req, scope, locals)
	logs = append(logs, preLogs...)
	if err != nil {
		result.Status = domain.StatusError
		if ctx.Err() != nil {
			result.Status = domain.StatusCancelled
		}
		result.Error = err.Error()
		return finish(nil)
	}
	red := prepared.Redactor
	result.Request = prepared.Summary
	if req.Kind.IsSocket() {
		// Socket tests have steps instead of a response: no post-response
		// scripts, assertions or baseline.
		r.runSocket(ctx, req, env, prepared, &result)
		return finish(red)
	}

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
		return finish(red)
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
	// The contract sees the full body; its messages are redacted below with the assertions'.
	results = append(results, scope.Contract.Check(prepared.HTTP.Method, prepared.HTTP.URL, contract.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       resp.Body,
		Truncated:  resp.Truncated,
	})...)
	for i := range results {
		results[i].Expected = red.String(results[i].Expected)
		results[i].Actual = red.String(results[i].Actual)
		results[i].Message = red.String(results[i].Message)
	}

	var scriptErr string
	sent := scripting.Request{Method: prepared.HTTP.Method, URL: prepared.HTTP.URL, Headers: scripting.HeaderRows(prepared.HTTP.Header),
		Query: []domain.KeyValue{}, Body: prepared.BodyText, BodyType: req.BodyType}
	scriptResp := &scripting.Response{Code: resp.StatusCode, Status: resp.Status, Header: resp.Header, Body: resp.Body,
		DurationMs: resp.Duration.Milliseconds(), Size: resp.BodyBytes}
	for _, s := range scripts(script{"collection post-response script", scope.PostResponseScript}, script{"post-response script", req.PostResponseScript}) {
		in := scriptInput(scripting.PostResponse, s, sent, req, scope, locals)
		in.Response = scriptResp
		out, err := scripting.Run(ctx, in)
		collect(scripting.PostResponse, out)
		for _, t := range out.Tests {
			results = append(results, domain.AssertionResult{Kind: domain.AssertScript, Description: red.String(t.Name),
				Passed: t.Passed, Message: red.String(t.Error)})
		}
		if err != nil {
			scriptErr = s.name + " failed: " + red.String(err.Error())
			break
		}
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
	if scriptErr != "" {
		result.Status = domain.StatusError
		result.Error = scriptErr
	}
	return finish(red)
}

// PreRequest runs the collection's and the request's pre-request scripts on
// req, validates the result and prepares it against the variables in scope.
// It returns the request as the scripts left it, the resolved environment, and
// the script logs (not yet redacted). Variable changes stay in scope.Vars.
func PreRequest(ctx context.Context, req domain.Request, scope *Scope, locals map[string]string) (domain.Request, *domain.Environment, *Prepared, []domain.ScriptLog, error) {
	if scope == nil {
		scope = NewScope(nil)
	}
	var logs []domain.ScriptLog
	req.Normalize()
	for _, s := range scripts(script{"collection pre-request script", scope.PreRequestScript}, script{"pre-request script", req.PreRequestScript}) {
		out, err := scripting.Run(ctx, scriptInput(scripting.PreRequest, s, scriptRequest(req), req, scope, locals))
		logs = append(logs, scriptLogs(scripting.PreRequest, out)...)
		if err != nil {
			return req, nil, nil, logs, errors.New(s.name + " failed: " + err.Error())
		}
		applyScriptRequest(&req, out.Request)
	}
	if err := req.Validate(); err != nil {
		return req, nil, nil, logs, errors.New("the request is invalid after the pre-request script ran: " + err.Error())
	}
	env := scope.Vars.Resolved(locals)
	prepared, err := Prepare(req, env)
	if err != nil {
		// Prepare errors never contain unredacted secrets, but be defensive.
		return req, env, nil, logs, errors.New(redact.New(env.SecretValues()...).String(err.Error()))
	}
	return req, env, prepared, logs, nil
}

func scriptInput(phase scripting.Phase, s script, sreq scripting.Request, req domain.Request, scope *Scope, locals map[string]string) scripting.Input {
	in := scripting.Input{Phase: phase, Script: s.source, Name: s.name, RequestName: req.Name, RequestID: req.ID,
		Request: sreq, Vars: scope.Vars, Locals: locals}
	if e, _ := scope.Vars.Environment(); e != nil {
		in.EnvironmentName = e.Name
	}
	return in
}

func scriptLogs(phase scripting.Phase, out scripting.Output) []domain.ScriptLog {
	logs := make([]domain.ScriptLog, 0, len(out.Logs))
	for _, l := range out.Logs {
		logs = append(logs, domain.ScriptLog{Phase: string(phase), Level: l.Level, Message: l.Message})
	}
	return logs
}

func scriptRequest(req domain.Request) scripting.Request {
	return scripting.Request{Method: req.Method, URL: req.URLTemplate,
		Headers: append([]domain.KeyValue{}, req.Headers...), Query: append([]domain.KeyValue{}, req.Query...),
		Body: req.Body, BodyType: req.BodyType}
}

func applyScriptRequest(req *domain.Request, s scripting.Request) {
	req.Method, req.URLTemplate, req.Headers, req.Query = s.Method, s.URL, s.Headers, s.Query
	req.Body, req.BodyType = s.Body, s.BodyType
	req.Normalize()
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
