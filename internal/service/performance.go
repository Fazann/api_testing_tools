package service

import (
	"context"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/perf"
)

// trendPoints is how many responses a request's trend chart shows.
const trendPoints = 100

// overviewPoints is how many responses per request the collection overview
// uses; the latest overviewRecent of them are compared with the rest.
const (
	overviewPoints = 30
	overviewRecent = 5
)

// perfInput is what checking one request's response time needs, loaded
// before the run so the run's own response is not part of its reference.
type perfInput struct {
	check   domain.PerformanceCheck
	earlier []int64
	enabled bool
}

func envKey(env *domain.Environment) (id, name string) {
	if env == nil {
		return "", ""
	}
	return env.ID, env.Name
}

func (s *Service) perfInputs(ctx context.Context, reqs []domain.Request, env *domain.Environment) ([]perfInput, error) {
	envID, envName := envKey(env)
	out := make([]perfInput, len(reqs))
	for i, r := range reqs {
		if r.ID == "" {
			continue // unsaved draft: no history
		}
		c, err := s.store.GetPerformanceCheck(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		if c.Mode == domain.PerformanceOff {
			continue
		}
		pts, err := s.store.ResponseTimes(ctx, r.ID, envID, envName, c.Window)
		if err != nil {
			return nil, err
		}
		out[i] = perfInput{check: c, earlier: durations(pts), enabled: true}
	}
	return out, nil
}

// applyPerformance checks a result's duration. Only responses count: errors
// and cancellations say nothing about how fast the API is.
func applyPerformance(res *domain.RunResult, in perfInput) {
	if !in.enabled || res.Response == nil || (res.Status != domain.StatusPassed && res.Status != domain.StatusFailed) {
		return
	}
	p := perf.Evaluate(in.check, in.earlier, res.DurationMs)
	res.Performance = &p
	if p.Slower && in.check.Mode == domain.PerformanceFail {
		res.Status = domain.StatusFailed
	}
}

func durations(pts []domain.ResponseTimePoint) []int64 {
	d := make([]int64, len(pts))
	for i, p := range pts {
		d[i] = p.DurationMs
	}
	return d
}

// GetPerformanceCheck returns the request's check (the default if none is saved).
func (s *Service) GetPerformanceCheck(ctx context.Context, requestID string) (domain.PerformanceCheck, error) {
	return s.store.GetPerformanceCheck(ctx, requestID)
}

func (s *Service) SavePerformanceCheck(ctx context.Context, c domain.PerformanceCheck) (domain.PerformanceCheck, error) {
	return s.store.SavePerformanceCheck(ctx, c)
}

// GetResponseTimeTrend returns a request's recent response times in an
// environment, with past slowdowns flagged and the reference for the next run.
func (s *Service) GetResponseTimeTrend(ctx context.Context, requestID, envID string) (domain.ResponseTimeTrend, error) {
	env, err := s.environment(ctx, envID)
	if err != nil {
		return domain.ResponseTimeTrend{}, err
	}
	c, err := s.store.GetPerformanceCheck(ctx, requestID)
	if err != nil {
		return domain.ResponseTimeTrend{}, err
	}
	id, name := envKey(env)
	pts, err := s.store.ResponseTimes(ctx, requestID, id, name, trendPoints)
	if err != nil {
		return domain.ResponseTimeTrend{}, err
	}
	perf.Flag(c, pts)
	d := durations(pts)
	return domain.ResponseTimeTrend{RequestID: requestID, Points: pts, Stats: perf.Stats(d), Check: c,
		Next: perf.Reference(c, d)}, nil
}

// GetCollectionResponseTimes summarises response times of every request in a
// collection for one environment, in tree order.
func (s *Service) GetCollectionResponseTimes(ctx context.Context, collectionID, envID string) ([]domain.ResponseTimeRow, error) {
	env, err := s.environment(ctx, envID)
	if err != nil {
		return nil, err
	}
	tree, err := s.store.GetTree(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	id, name := envKey(env)
	byReq, err := s.store.CollectionResponseTimes(ctx, collectionID, id, name, overviewPoints)
	if err != nil {
		return nil, err
	}
	checks, err := s.store.ListPerformanceChecks(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	reqs := OrderedRequests(tree, "")
	rows := make([]domain.ResponseTimeRow, 0, len(reqs))
	for _, r := range reqs {
		c, ok := checks[r.ID]
		if !ok {
			c = domain.DefaultPerformanceCheck(r.ID)
		}
		pts := byReq[r.ID]
		d := durations(pts)
		row := domain.ResponseTimeRow{RequestID: r.ID, Name: r.Name, Method: r.Method, ParentID: r.ParentID,
			Stats: perf.Stats(d), Recent: d, Mode: c.Mode}
		if len(pts) > 0 {
			row.LastMs = d[len(d)-1]
			perf.Flag(c, pts)
			row.LastSlower = pts[len(pts)-1].Slower
		}
		if pct, ok := perf.Change(d, overviewRecent); ok {
			row.ChangePct = &pct
		}
		rows = append(rows, row)
	}
	return rows, nil
}
