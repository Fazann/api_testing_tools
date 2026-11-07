package service

import (
	"context"
	"time"

	"api-test-kits/internal/collectionio"
	"api-test-kits/internal/contract"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/runner"
)

// ContractInfo describes a collection's attached OpenAPI spec, without the
// spec itself.
type ContractInfo struct {
	Attached          bool      `json:"attached"`
	SourceName        string    `json:"sourceName"`
	Title             string    `json:"title"`
	Version           string    `json:"version"`
	Operations        int       `json:"operations"`
	Enabled           bool      `json:"enabled"`
	AllowUndocumented bool      `json:"allowUndocumented"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func contractInfo(c *domain.Contract, spec *contract.Spec) ContractInfo {
	if c == nil {
		return ContractInfo{}
	}
	info := ContractInfo{Attached: true, SourceName: c.SourceName, Enabled: c.Enabled,
		AllowUndocumented: c.AllowUndocumented, UpdatedAt: c.UpdatedAt}
	if spec != nil {
		info.Title, info.Version, info.Operations = spec.Title, spec.Version, len(spec.Operations())
	}
	return info
}

// GetContract describes the collection's contract; Attached is false when
// there is none.
func (s *Service) GetContract(ctx context.Context, collectionID string) (ContractInfo, error) {
	c, spec, err := s.loadContract(ctx, collectionID)
	if err != nil {
		return ContractInfo{}, err
	}
	return contractInfo(c, spec), nil
}

// AttachContract attaches (or replaces) the collection's spec from an
// OpenAPI or Swagger file. Replacing keeps the options; a new contract is on.
func (s *Service) AttachContract(ctx context.Context, collectionID string, data []byte, sourceName string) (ContractInfo, error) {
	specJSON, err := collectionio.ReadSpec(data)
	if err != nil {
		return ContractInfo{}, err
	}
	c := domain.Contract{CollectionID: collectionID, Spec: specJSON, SourceName: sourceName, Enabled: true}
	if old, err := s.store.GetContract(ctx, collectionID); err != nil {
		return ContractInfo{}, err
	} else if old != nil {
		c.Enabled, c.AllowUndocumented = old.Enabled, old.AllowUndocumented
	}
	if _, err := s.store.SaveContract(ctx, c); err != nil {
		return ContractInfo{}, err
	}
	return s.GetContract(ctx, collectionID)
}

// SetContractOptions turns the check on or off and sets whether
// undocumented fields are accepted.
func (s *Service) SetContractOptions(ctx context.Context, collectionID string, enabled, allowUndocumented bool) (ContractInfo, error) {
	if err := s.store.SaveContractOptions(ctx, collectionID, enabled, allowUndocumented); err != nil {
		return ContractInfo{}, err
	}
	return s.GetContract(ctx, collectionID)
}

// RemoveContract detaches the collection's spec. Requests are not changed.
func (s *Service) RemoveContract(ctx context.Context, collectionID string) error {
	return s.store.DeleteContract(ctx, collectionID)
}

// CoverageRow is one request of the collection and the operation it matches.
type CoverageRow struct {
	RequestID string `json:"requestId"`
	Name      string `json:"name"`
	Method    string `json:"method"`
	// Operation is "GET /pets/{petId}", or "" when the request matches none.
	Operation string `json:"operation"`
	// Problem explains why the request could not be matched (for example a
	// variable that is not defined in the environment).
	Problem string `json:"problem,omitempty"`
}

// OperationRow is an operation of the spec.
type OperationRow struct {
	Operation string `json:"operation"`
	Summary   string `json:"summary"`
	// Requests is how many requests match it.
	Requests int `json:"requests"`
}

// ContractCoverage relates the collection's requests to the spec's operations.
type ContractCoverage struct {
	Requests   []CoverageRow  `json:"requests"`
	Operations []OperationRow `json:"operations"`
}

// GetContractCoverage matches every HTTP request of the collection, with
// its URL resolved in envID, against the attached spec. Nothing is sent and
// pre-request scripts do not run.
func (s *Service) GetContractCoverage(ctx context.Context, collectionID, envID string) (ContractCoverage, error) {
	out := ContractCoverage{Requests: []CoverageRow{}, Operations: []OperationRow{}}
	_, spec, err := s.loadContract(ctx, collectionID)
	if err != nil || spec == nil {
		return out, err
	}
	tree, err := s.store.GetTree(ctx, collectionID)
	if err != nil {
		return out, err
	}
	env, err := s.environment(ctx, envID)
	if err != nil {
		return out, err
	}
	reqs, env, _, err := s.scope(ctx, OrderedRequests(tree, ""), env)
	if err != nil {
		return out, err
	}
	counts := map[*contract.Operation]int{}
	for _, r := range reqs {
		if r.Kind.IsSocket() {
			continue
		}
		row := CoverageRow{RequestID: r.ID, Name: r.Name, Method: r.Method}
		p, err := runner.Prepare(r, env)
		if err != nil {
			row.Problem = err.Error()
		} else if op := spec.Match(p.HTTP.Method, p.HTTP.URL); op != nil {
			row.Operation = op.Label()
			counts[op]++
		}
		out.Requests = append(out.Requests, row)
	}
	for _, op := range spec.Operations() {
		out.Operations = append(out.Operations, OperationRow{Operation: op.Label(), Summary: op.Summary, Requests: counts[op]})
	}
	return out, nil
}

// contractChecker returns the checker for runs of collection c, or nil when
// it has no contract or the check is off.
func (s *Service) contractChecker(ctx context.Context, c *domain.Collection) (*contract.Checker, error) {
	if c == nil {
		return nil, nil
	}
	ct, spec, err := s.loadContract(ctx, c.ID)
	if err != nil || ct == nil || !ct.Enabled || spec == nil {
		return nil, err
	}
	return &contract.Checker{Spec: spec, AllowUndocumented: ct.AllowUndocumented}, nil
}

type cachedSpec struct {
	updated time.Time
	size    int
	spec    *contract.Spec
}

// loadContract returns the stored contract and its parsed spec. Parsed
// specs are cached until the contract changes. A stored spec that no longer
// parses is returned with a nil spec.
func (s *Service) loadContract(ctx context.Context, collectionID string) (*domain.Contract, *contract.Spec, error) {
	c, err := s.store.GetContract(ctx, collectionID)
	if err != nil || c == nil {
		return nil, nil, err
	}
	s.mu.Lock()
	cached, ok := s.specs[collectionID]
	s.mu.Unlock()
	if ok && cached.updated.Equal(c.UpdatedAt) && cached.size == len(c.Spec) {
		return c, cached.spec, nil
	}
	spec, err := contract.Parse(c.Spec)
	if err != nil {
		return c, nil, nil
	}
	s.mu.Lock()
	if s.specs == nil {
		s.specs = map[string]cachedSpec{}
	}
	s.specs[collectionID] = cachedSpec{updated: c.UpdatedAt, size: len(c.Spec), spec: spec}
	s.mu.Unlock()
	return c, spec, nil
}
