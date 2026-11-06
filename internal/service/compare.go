package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/compare"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/regression"
	"api-test-kits/internal/runner"
)

// ChangingSamples is how many times Detect changing fields sends a request.
const ChangingSamples = 3

// ---- changing-field detection ----

// DetectChangingInput asks which fields of a request's response change
// between calls.
type DetectChangingInput struct {
	// Request is the saved request or the open draft with its edits.
	Request       domain.Request `json:"request"`
	EnvironmentID string         `json:"environmentId"`
	// IgnoredPaths are the patterns already ignored; matching fields are not reported.
	IgnoredPaths []string `json:"ignoredPaths"`
	// Confirmed allows repeating a request that may modify data, and running
	// against a protected environment.
	Confirmed bool `json:"confirmed"`
}

// ChangingReport lists the fields that changed between the calls.
type ChangingReport struct {
	Fields  []regression.ChangingField `json:"fields"`
	Samples int                        `json:"samples"`
	// StatusCodes are the status codes of the calls, in order.
	StatusCodes   []int `json:"statusCodes"`
	StatusChanged bool  `json:"statusChanged"`
}

// DetectChangingFields sends a request ChangingSamples times and reports the
// fields whose value changed. Script variable changes are not saved and the
// calls are not recorded in history.
func (s *Service) DetectChangingFields(ctx context.Context, opID string, in DetectChangingInput) (ChangingReport, error) {
	out := ChangingReport{Fields: []regression.ChangingField{}, StatusCodes: []int{}}
	req := in.Request
	req.Normalize()
	if err := req.Validate(); err != nil {
		return out, err
	}
	if req.Kind.IsSocket() {
		return out, errors.New("changing fields are detected in HTTP responses; WebSocket and Socket.IO requests are not supported")
	}
	if req.MayModifyData() && !in.Confirmed {
		return out, &ConfirmationRequiredError{Warnings: []Warning{{Level: LevelWarning, Code: "repeat_modifying",
			RequiresConfirmation: true,
			Message:              fmt.Sprintf("%s %s may modify data and would be sent %d more times", req.Method, req.Name, ChangingSamples)}}}
	}
	if err := s.requireConfirmation(ctx, []domain.Request{req}, nil, in.EnvironmentID, in.Confirmed); err != nil {
		return out, err
	}
	env, err := s.environment(ctx, in.EnvironmentID)
	if err != nil {
		return out, err
	}
	reqs, _, c, err := s.scope(ctx, []domain.Request{req}, env)
	if err != nil {
		return out, err
	}
	scope := runScope(env, c)
	client, _ := s.current()
	r := runner.New(client)

	opCtx, done := s.beginOp(ctx, opID)
	defer done()
	docs := make([]any, 0, ChangingSamples)
	for i := 1; i <= ChangingSamples; i++ {
		res := r.RunScoped(opCtx, reqs[0], scope, nil)
		doc, err := sampleDoc(res, i)
		if err != nil {
			return out, err
		}
		out.StatusCodes = append(out.StatusCodes, res.Response.StatusCode)
		if res.Response.StatusCode != out.StatusCodes[0] {
			out.StatusChanged = true
		}
		docs = append(docs, doc)
	}
	out.Samples = len(docs)
	out.Fields = regression.ChangingFields(docs, cleanPaths(in.IgnoredPaths), 0)
	return out, nil
}

// sampleDoc decodes the JSON body of call n, or explains why it cannot be used.
func sampleDoc(res domain.RunResult, n int) (any, error) {
	switch {
	case res.Status == domain.StatusCancelled:
		return nil, errors.New("detection was cancelled; nothing was suggested")
	case res.Response == nil:
		return nil, fmt.Errorf("call %d of %d failed: %s", n, ChangingSamples, res.Error)
	case res.Response.Truncated:
		return nil, fmt.Errorf("call %d of %d was truncated at the response size limit; raise the limit in Settings", n, ChangingSamples)
	}
	doc, err := assertions.DecodeJSON([]byte(res.Response.Body))
	if err != nil {
		return nil, fmt.Errorf("call %d of %d did not return JSON (status %d), so its fields cannot be compared", n, ChangingSamples, res.Response.StatusCode)
	}
	return doc, nil
}

// ---- environment comparison ----

// GetEnvComparison returns a collection's comparison settings.
func (s *Service) GetEnvComparison(ctx context.Context, collectionID string) (domain.EnvComparison, error) {
	return s.store.GetEnvComparison(ctx, collectionID)
}

// SaveEnvComparison stores a collection's comparison settings.
func (s *Service) SaveEnvComparison(ctx context.Context, c domain.EnvComparison) (domain.EnvComparison, error) {
	return s.store.SaveEnvComparison(ctx, c)
}

// envSide is one environment's part of a comparison.
type envSide struct {
	env    *domain.Environment
	reqs   []domain.Request
	scope  *runner.Scope
	pass1  []domain.RunResult
	pass2  []domain.RunResult // detection calls, by index into reqs (nil when not sent)
	runRec domain.Run
}

// CompareEnvironments runs a collection's requests against two environments
// at the same time and reports the differences. The settings are saved; each
// environment's first pass is recorded in history; script variable changes
// are not saved.
func (s *Service) CompareEnvironments(ctx context.Context, opID string, cfg domain.EnvComparison, confirmed bool,
	progress func(done, total int, res domain.RunResult)) (compare.Report, error) {
	report := compare.Report{Requests: []compare.Result{}}
	cfg.Normalize()
	if err := cfg.ValidateForRun(); err != nil {
		return report, err
	}
	tree, err := s.store.GetTree(ctx, cfg.CollectionID)
	if err != nil {
		return report, err
	}

	// Decide per request whether it is compared, keeping collection order.
	all := OrderedRequests(tree, "")
	var included []domain.Request
	skipped := map[int]compare.Result{}
	for i, r := range all {
		r.Normalize()
		switch {
		case r.Kind.IsSocket():
			skipped[i] = compare.SkippedResult(r, "socket requests have no single response to compare")
		case r.MayModifyData() && !cfg.IncludeModifying:
			skipped[i] = compare.SkippedResult(r, "may modify data; tick Include requests that may modify data to compare it")
		default:
			included = append(included, r)
		}
	}
	if len(included) == 0 {
		return report, errors.New("there are no requests to compare: the collection has no read-only HTTP requests " +
			"(tick Include requests that may modify data to compare the others)")
	}

	sides := [2]*envSide{{}, {}}
	for i, envID := range []string{cfg.EnvA, cfg.EnvB} {
		env, err := s.environment(ctx, envID)
		if err != nil {
			return report, err
		}
		if env == nil {
			return report, errors.New("choose the two environments to compare")
		}
		if err := s.requireConfirmation(ctx, included, tree.Collection.RequiredVariables, envID, confirmed); err != nil {
			return report, err
		}
		if missing := s.missingRequired(ctx, envID, tree.Collection); len(missing) > 0 {
			return report, fmt.Errorf("%s: required variables are missing or empty: %s", env.Name, strings.Join(missing, ", "))
		}
		reqs, _, c, err := s.scope(ctx, included, env)
		if err != nil {
			return report, err
		}
		sides[i] = &envSide{env: env, reqs: reqs, scope: runScope(env, c), runRec: s.newRun(c, env, domain.RunKindNormal)}
		// The first pass is an ordinary run; only the detection calls are extra.
	}
	report.A = compare.EnvRef{ID: sides[0].env.ID, Name: sides[0].env.Name}
	report.B = compare.EnvRef{ID: sides[1].env.ID, Name: sides[1].env.Name}
	if _, err := s.store.SaveEnvComparison(ctx, cfg); err != nil {
		return report, err
	}

	ignoredPaths, ignoredHeaders := compare.SplitIgnored(cfg.IgnoredPaths)
	baselineIgnored := make([][]string, len(included))
	for i, r := range included {
		for _, side := range sides {
			b, err := s.store.GetBaseline(ctx, r.ID, side.env.ID)
			if err != nil {
				return report, err
			}
			if b != nil {
				baselineIgnored[i] = append(baselineIgnored[i], b.IgnoredPaths...)
			}
		}
	}

	var readOnly []int
	if cfg.DetectChanging {
		for i, r := range included {
			if !r.MayModifyData() {
				readOnly = append(readOnly, i)
			}
		}
	}

	client, st := s.current()
	opCtx, done := s.beginOp(ctx, opID)
	defer done()

	total := 2 * (len(included) + len(readOnly))
	var mu sync.Mutex
	finished := 0
	tick := func(_ int, res domain.RunResult) {
		mu.Lock()
		defer mu.Unlock()
		finished++
		if progress != nil {
			progress(finished, total, res)
		}
	}
	var wg sync.WaitGroup
	for _, side := range sides {
		wg.Add(1)
		go func(side *envSide) {
			defer wg.Done()
			r := runner.New(client)
			items := make([]runner.Item, len(side.reqs))
			for i, req := range side.reqs {
				items[i] = runner.Item{Request: req}
			}
			side.pass1 = r.RunAll(opCtx, items, side.scope, st.Concurrency, tick)
			if len(readOnly) == 0 {
				return
			}
			again := make([]runner.Item, len(readOnly))
			for j, i := range readOnly {
				again[j] = items[i]
			}
			second := r.RunAll(opCtx, again, side.scope, st.Concurrency, tick)
			side.pass2 = make([]domain.RunResult, len(side.reqs))
			for j, i := range readOnly {
				side.pass2[i] = second[j]
			}
		}(side)
	}
	wg.Wait()

	compared := make([]compare.Result, len(included))
	for i, req := range included {
		changing := changingPaths(sides, i)
		ignored := append(append(append([]string{}, ignoredPaths...), baselineIgnored[i]...), changing...)
		res := compare.Request(sides[0].pass1[i], sides[1].pass1[i], compare.Options{IgnoredPaths: ignored, IgnoredHeaders: ignoredHeaders})
		res.RequestID, res.Name, res.Method = req.ID, req.Name, req.Kind.Badge(req.Method)
		res.Changing = changing
		compared[i] = res
	}
	next := 0
	for i := range all {
		if sk, ok := skipped[i]; ok {
			report.Requests = append(report.Requests, sk)
			continue
		}
		report.Requests = append(report.Requests, compared[next])
		next++
	}
	report.Tally()

	var saveErr error
	for i, side := range sides {
		side.runRec.Results = side.pass1
		if err := s.finishRun(ctx, &side.runRec); err != nil && saveErr == nil {
			saveErr = err
		}
		if i == 0 {
			report.RunIDA = side.runRec.ID
		} else {
			report.RunIDB = side.runRec.ID
		}
	}
	return report, saveErr
}

// changingPaths returns the fields of request i whose value differed between
// the two calls in either environment, sorted.
func changingPaths(sides [2]*envSide, i int) []string {
	set := map[string]bool{}
	for _, side := range sides {
		if side.pass2 == nil || side.pass2[i].Response == nil || side.pass1[i].Response == nil {
			continue
		}
		var docs []any
		for _, res := range []domain.RunResult{side.pass1[i], side.pass2[i]} {
			if res.Response.Truncated {
				break
			}
			doc, err := assertions.DecodeJSON([]byte(res.Response.Body))
			if err != nil {
				break
			}
			docs = append(docs, doc)
		}
		if len(docs) == 2 {
			for _, p := range regression.ChangingPaths(regression.ChangingFields(docs, nil, 0)) {
				set[p] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
