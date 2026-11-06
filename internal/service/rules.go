package service

import (
	"context"
	"errors"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/rules"
)

// SaveCollectionRules replaces a collection's rules.
func (s *Service) SaveCollectionRules(ctx context.Context, collectionID string, list []domain.Rule) ([]domain.Rule, error) {
	return s.store.SaveCollectionRules(ctx, collectionID, list)
}

// RulesCheck is the outcome of rules evaluated against a stored run.
type RulesCheck struct {
	RunID           string          `json:"runId"`
	StartedAt       time.Time       `json:"startedAt"`
	EnvironmentName string          `json:"environmentName"`
	Outcomes        []rules.Outcome `json:"outcomes"`
}

// CheckCollectionRules evaluates rules (typically unsaved edits) against the
// newest collection run in history, without sending any request.
func (s *Service) CheckCollectionRules(ctx context.Context, collectionID string, list []domain.Rule) (RulesCheck, error) {
	out := RulesCheck{Outcomes: []rules.Outcome{}}
	list = domain.NormalizeRules(list)
	if err := domain.ValidateRules(list); err != nil {
		return out, err
	}
	// Race tests and permission matrices are stored with the collection too;
	// only an ordinary run has one result per request.
	runs, err := s.store.ListRuns(ctx, domain.RunFilter{CollectionID: collectionID, Limit: 50})
	if err != nil {
		return out, err
	}
	for _, r := range runs {
		if r.Kind != domain.RunKindNormal {
			continue
		}
		run, err := s.store.GetRun(ctx, r.ID)
		if err != nil {
			return out, err
		}
		out.RunID, out.StartedAt, out.EnvironmentName = run.ID, run.StartedAt, run.EnvironmentName
		out.Outcomes = rules.Check(list, run.Results)
		return out, nil
	}
	return out, errors.New("there is no run of this collection in history yet; run the collection once first")
}

// applyRules adds the "Collection rules" result to a finished collection run
// and returns it, or nil when no rule applies.
func applyRules(run *domain.Run, c *domain.Collection) *domain.RunResult {
	if c == nil || run.CollectionID == "" {
		return nil
	}
	res := rules.Evaluate(c.Rules, run.Results, time.Now())
	if res != nil {
		run.Results = append(run.Results, *res)
	}
	return res
}
