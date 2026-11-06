package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/environments"
	"api-test-kits/internal/runner"
	"api-test-kits/internal/scripting"
	"api-test-kits/internal/security"
	"api-test-kits/internal/storage"
)

// reportBodyLimit caps each response body returned in a security report; the
// full (history-limited) body stays available in the run's history.
const reportBodyLimit = 64 << 10

// runScope builds the scope that requests of collection c (nil for none) see
// with env. Variable changes made by scripts in security tests are never
// saved: a burst or a role switch would overwrite real state.
func runScope(env *domain.Environment, c *domain.Collection) *runner.Scope {
	if c == nil {
		return runner.NewScope(env)
	}
	return &runner.Scope{Vars: scripting.NewVariables(env, c.Variables),
		PreRequestScript: c.PreRequestScript, PostResponseScript: c.PostResponseScript}
}

func (s *Service) newRun(c *domain.Collection, env *domain.Environment, kind string) domain.Run {
	// Race tests and matrices send requests concurrently, so their durations
	// are left out of response-time trends (which only count RunKindNormal).
	run := domain.Run{ID: uuid.NewString(), StartedAt: time.Now().UTC(), Kind: kind}
	if c != nil {
		run.CollectionID, run.CollectionName = c.ID, c.Name
	}
	run.EnvironmentID, run.EnvironmentName = envKey(env)
	return run
}

func (s *Service) finishRun(ctx context.Context, run *domain.Run) error {
	end := time.Now().UTC()
	run.FinishedAt = &end
	run.DurationMs = end.Sub(run.StartedAt).Milliseconds()
	for i := range run.Results {
		run.Results[i].RunID = run.ID
	}
	run.Tally()
	_, st := s.current()
	if err := s.store.SaveRun(context.WithoutCancel(ctx), *run, st.HistoryBodyLimitBytes, st.HistoryRetentionRuns); err != nil {
		return fmt.Errorf("the test finished but could not be saved to history: %w", err)
	}
	return nil
}

func trimBodies(results []domain.RunResult) []domain.RunResult {
	out := make([]domain.RunResult, len(results))
	for i, r := range results {
		if r.Response != nil && len(r.Response.Body) > reportBodyLimit {
			resp := *r.Response
			resp.Body = resp.Body[:reportBodyLimit]
			resp.Truncated = true
			r.Response = &resp
		}
		out[i] = r
	}
	return out
}

// ---- race-condition test ----

// GetRaceTest returns the saved race test for a request, or nil.
func (s *Service) GetRaceTest(ctx context.Context, requestID string) (*domain.RaceTest, error) {
	return s.store.GetRaceTest(ctx, requestID)
}

// RunRaceTest saves t and runs it: the state check (if any) reads its value,
// t.Count copies of the request are released at the same moment, the state
// check reads again, and the invariants are evaluated. The run is recorded in
// history with one extra "verdict" result that lists the invariants.
func (s *Service) RunRaceTest(ctx context.Context, opID string, t domain.RaceTest, envID string, confirmed bool) (security.RaceReport, error) {
	t.Normalize()
	if err := t.Validate(); err != nil {
		return security.RaceReport{}, err
	}
	req, err := s.store.GetRequest(ctx, t.RequestID)
	if errors.Is(err, storage.ErrNotFound) {
		return security.RaceReport{}, errors.New("save the request before running a race test")
	}
	if err != nil {
		return security.RaceReport{}, err
	}
	if req.Kind.IsSocket() {
		return security.RaceReport{}, errors.New("race tests send HTTP requests; WebSocket and Socket.IO requests are not supported")
	}
	reqs := []domain.Request{req}
	if t.Check != nil {
		check, err := s.store.GetRequest(ctx, t.Check.RequestID)
		if err != nil {
			return security.RaceReport{}, errors.New("the state check's request no longer exists")
		}
		if check.Kind.IsSocket() {
			return security.RaceReport{}, errors.New("the state check must be an HTTP request")
		}
		reqs = append(reqs, check)
	}
	if err := s.requireConfirmation(ctx, reqs, nil, envID, confirmed); err != nil {
		return security.RaceReport{}, err
	}
	if t, err = s.store.SaveRaceTest(ctx, t); err != nil {
		return security.RaceReport{}, err
	}

	env, err := s.environment(ctx, envID)
	if err != nil {
		return security.RaceReport{}, err
	}
	reqs, _, c, err := s.scope(ctx, reqs, env)
	if err != nil {
		return security.RaceReport{}, err
	}
	scope := runScope(env, c)
	client, _ := s.current()
	r := runner.New(client)

	opCtx, done := s.beginOp(ctx, opID)
	defer done()
	run := s.newRun(c, env, domain.RunKindRace)

	var before, after *security.StateValue
	readState := func(label string) *security.StateValue {
		res := r.RunScoped(opCtx, reqs[1], scope, nil)
		res.Request.Name = label + ": " + res.Request.Name
		res.AssertionResults = []domain.AssertionResult{}
		v := security.ReadNumber(res, t.Check.Path)
		if v.Err == "" && res.Status != domain.StatusCancelled {
			res.Status, res.Error = domain.StatusPassed, ""
		} else if res.Status != domain.StatusCancelled {
			res.Status = domain.StatusError
			if res.Error == "" {
				res.Error = v.Err
			}
		}
		run.Results = append(run.Results, res)
		return &v
	}
	if t.Check != nil {
		before = readState("State before")
	}
	burst := r.RunBurst(opCtx, reqs[0], scope, t.Count)
	if t.Check != nil {
		after = readState("State after")
	}

	report := security.EvaluateRace(t, burst, before, after)
	for i := range burst {
		b := &burst[i]
		b.Request.Name = fmt.Sprintf("#%d %s", i+1, b.Request.Name)
		// Individual responses are evidence, not verdicts: losing requests
		// are expected to fail, so only transport errors keep their status.
		if b.Response != nil {
			b.Status = domain.StatusPassed
		}
	}
	verdict := domain.RunResult{
		ID:               uuid.NewString(),
		RequestID:        req.ID,
		Request:          domain.RequestSummary{Name: fmt.Sprintf("Race test verdict: %d × %s", t.Count, req.Name), Method: req.Method},
		AssertionResults: report.Checks,
		Status:           domain.StatusPassed,
		CreatedAt:        time.Now().UTC(),
	}
	if len(burst) > 0 {
		verdict.Request.URL = burst[0].Request.URL
	}
	switch {
	case report.Cancelled:
		verdict.Status = domain.StatusCancelled
	case !report.Passed:
		verdict.Status = domain.StatusFailed
	}
	// Order: state before, the burst, state after, verdict.
	results := make([]domain.RunResult, 0, len(run.Results)+len(burst)+1)
	if before != nil {
		results = append(results, run.Results[0])
	}
	results = append(results, burst...)
	if after != nil {
		results = append(results, run.Results[len(run.Results)-1])
	}
	run.Results = append(results, verdict)
	saveErr := s.finishRun(ctx, &run)

	report.RunID = run.ID
	report.Results = trimBodies(burst)
	return report, saveErr
}

// ---- permission matrix ----

func (s *Service) GetPermissionMatrix(ctx context.Context, collectionID string) (domain.PermissionMatrix, error) {
	return s.store.GetPermissionMatrix(ctx, collectionID)
}

func (s *Service) SavePermissionMatrix(ctx context.Context, m domain.PermissionMatrix) (domain.PermissionMatrix, error) {
	return s.store.SavePermissionMatrix(ctx, m)
}

// anonymous returns req without any credentials: no auth and no
// Authorization or Cookie header rows.
func anonymous(req domain.Request) domain.Request {
	req.Auth = domain.Auth{Type: domain.AuthNone}
	headers := make([]domain.KeyValue, 0, len(req.Headers))
	for _, h := range req.Headers {
		name := strings.TrimSpace(h.Key)
		if strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Cookie") {
			continue
		}
		headers = append(headers, h)
	}
	req.Headers = headers
	return req
}

// resolveRole substitutes references in the role's variable values with the
// environment and collection variables, so token = {{bob_token}} works even
// though variables are resolved in a single pass. A value that uses a secret
// variable becomes secret, so it is redacted like the original.
func resolveRole(role domain.Role, env *domain.Environment, c *domain.Collection) domain.Role {
	base := env
	if c != nil {
		base = domain.WithCollectionVariables(env, c.Variables)
	}
	res := environments.NewResolver(base)
	vars := make([]domain.Variable, len(role.Variables))
	for i, v := range role.Variables {
		for _, name := range environments.References(v.Value) {
			if res.IsSecret(name) {
				v.Secret = true
			}
		}
		v.Value = res.Resolve(v.Value)
		vars[i] = v
	}
	role.Variables = vars
	return role
}

// RunPermissionMatrix runs every selected request of the collection as every
// role and judges each cell against the saved expectations. The run is
// recorded in history with one result per cell.
func (s *Service) RunPermissionMatrix(ctx context.Context, opID, collectionID, envID string, confirmed bool, progress func(done, total int, res domain.RunResult)) (security.MatrixReport, error) {
	tree, err := s.store.GetTree(ctx, collectionID)
	if err != nil {
		return security.MatrixReport{}, err
	}
	m, err := s.store.GetPermissionMatrix(ctx, collectionID)
	if err != nil {
		return security.MatrixReport{}, err
	}
	if len(m.Roles) == 0 {
		return security.MatrixReport{}, errors.New("add at least one role to the permission matrix")
	}
	selected := map[string]bool{}
	for _, id := range m.RequestIDs {
		selected[id] = true
	}
	var reqs []domain.Request
	for _, r := range OrderedRequests(tree, "") {
		// Socket requests have no status code to judge access by.
		if selected[r.ID] && !r.Kind.IsSocket() {
			reqs = append(reqs, r)
		}
	}
	if len(reqs) == 0 {
		return security.MatrixReport{}, errors.New("choose at least one request for the permission matrix")
	}
	if err := s.requireConfirmation(ctx, reqs, nil, envID, confirmed); err != nil {
		return security.MatrixReport{}, err
	}
	env, err := s.environment(ctx, envID)
	if err != nil {
		return security.MatrixReport{}, err
	}
	reqs, _, c, err := s.scope(ctx, reqs, env)
	if err != nil {
		return security.MatrixReport{}, err
	}
	client, st := s.current()
	r := runner.New(client)

	opCtx, done := s.beginOp(ctx, opID)
	defer done()
	run := s.newRun(c, env, domain.RunKindMatrix)

	report := security.MatrixReport{Roles: []security.MatrixRole{}, Requests: []security.MatrixRequest{}, Cells: []security.MatrixCell{}}
	for _, req := range reqs {
		report.Requests = append(report.Requests, security.MatrixRequest{ID: req.ID, Name: req.Name, Method: req.Method})
	}
	total, finished := len(reqs)*len(m.Roles), 0
	for _, role := range m.Roles {
		report.Roles = append(report.Roles, security.MatrixRole{ID: role.ID, Name: role.Name, Anonymous: role.Anonymous})
		items := make([]runner.Item, len(reqs))
		for i, req := range reqs {
			if role.Anonymous {
				req = anonymous(req)
			}
			items[i] = runner.Item{Request: req}
		}
		// Each role gets a fresh scope so one role's script changes never leak into another's.
		scope := runScope(domain.WithRoleVariables(env, resolveRole(role, env, c)), c)
		results := r.RunAll(opCtx, items, scope, st.Concurrency, func(_ int, res domain.RunResult) {
			finished++
			if progress != nil {
				progress(finished, total, res)
			}
		})
		for i, res := range results {
			cell := judgeCell(m, reqs[i], role, &res)
			report.Cells = append(report.Cells, cell)
			if res.Status == domain.StatusCancelled {
				report.Cancelled = true
			}
			run.Results = append(run.Results, res)
		}
	}
	report.Tally()
	saveErr := s.finishRun(ctx, &run)
	report.RunID = run.ID
	return report, saveErr
}

// judgeCell classifies res, rewrites it as the matrix's evidence for history
// (its own assertions are replaced by the access check) and returns the cell.
func judgeCell(m domain.PermissionMatrix, req domain.Request, role domain.Role, res *domain.RunResult) security.MatrixCell {
	expected := m.Expected[req.ID][role.ID]
	outcome := security.Classify(*res)
	status := 0
	if res.Response != nil {
		status = res.Response.StatusCode
	}
	verdict, msg := security.Judge(expected, outcome, status, role.Name)
	if res.Status == domain.StatusCancelled {
		verdict, msg = security.VerdictError, "cancelled"
	}
	cell := security.MatrixCell{RequestID: req.ID, RoleID: role.ID, Expected: expected, Outcome: outcome,
		StatusCode: status, DurationMs: res.DurationMs, Verdict: verdict, Message: msg, ResultID: res.ID,
		Suggested: security.SuggestedAccess(outcome)}
	if verdict == security.VerdictError && msg == "no response" && res.Error != "" {
		cell.Message = res.Error
	}

	res.Request.Name = "[" + role.Name + "] " + res.Request.Name
	actual := string(outcome)
	if status > 0 {
		actual = fmt.Sprintf("%d %s", status, http.StatusText(status))
	}
	want := "any (no expectation)"
	if expected != domain.AccessUnset {
		want = string(expected)
	}
	res.AssertionResults = []domain.AssertionResult{{Kind: domain.AssertScript,
		Description: fmt.Sprintf("Access for %s", role.Name),
		Passed:      verdict != security.VerdictFail && verdict != security.VerdictError,
		Expected:    want, Actual: actual, Message: msg}}
	switch verdict {
	case security.VerdictFail:
		res.Status = domain.StatusFailed
	case security.VerdictError:
		if res.Status != domain.StatusCancelled {
			res.Status = domain.StatusError
		}
	default:
		res.Status = domain.StatusPassed
	}
	return cell
}
