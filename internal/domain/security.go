package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ---- race-condition test ----

const (
	MinRaceCount = 2
	// MaxRaceCount bounds a burst so a race test cannot become a load test.
	MaxRaceCount = 50
)

// RaceTest fires one saved request many times at the same moment and checks
// invariants that a correct server keeps under concurrency, for example "only
// one withdrawal succeeds" or "the balance went down once per success".
// The request's own assertions and baseline are not part of the verdict,
// because losing requests are expected to fail (409, 422, …).
type RaceTest struct {
	RequestID string `json:"requestId"`
	// Count is how many copies are sent at once.
	Count int `json:"count"`
	// SuccessOp and SuccessCount limit how many responses may be 2xx:
	// "eq", "lte" or "gte". An empty SuccessOp means no limit.
	SuccessOp    Operator `json:"successOp"`
	SuccessCount int      `json:"successCount"`
	// UniquePath is a JSON path whose value must differ between successful
	// responses (for example a transaction ID). Empty means no check.
	UniquePath string `json:"uniquePath"`
	// AllowServerErrors accepts 5xx responses. By default a 5xx under
	// concurrency (deadlock, constraint violation) fails the test.
	AllowServerErrors bool `json:"allowServerErrors"`
	// Check, if set, reads a number before and after the burst and verifies
	// how much it changed.
	Check     *RaceCheck `json:"check,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// RaceCheck verifies a state change: after - before must equal
// successes × DeltaPerSuccess, within Tolerance. For a withdrawal of 10 the
// check reads the balance and DeltaPerSuccess is -10.
type RaceCheck struct {
	// RequestID is a saved request that returns the value, usually a GET.
	RequestID       string  `json:"requestId"`
	Path            string  `json:"path"`
	DeltaPerSuccess float64 `json:"deltaPerSuccess"`
	Tolerance       float64 `json:"tolerance"`
}

func (t *RaceTest) Normalize() {
	if t.Count == 0 {
		t.Count = 10
	}
	t.UniquePath = strings.TrimSpace(t.UniquePath)
	if t.Check != nil {
		t.Check.Path = strings.TrimSpace(t.Check.Path)
		if t.Check.RequestID == "" && t.Check.Path == "" {
			t.Check = nil
		}
	}
}

func (t *RaceTest) Validate() error {
	if t.RequestID == "" {
		return fmt.Errorf("save the request before running a race test")
	}
	if t.Count < MinRaceCount || t.Count > MaxRaceCount {
		return fmt.Errorf("parallel requests must be between %d and %d", MinRaceCount, MaxRaceCount)
	}
	switch t.SuccessOp {
	case "":
	case OpEq, OpLte, OpGte:
		if t.SuccessCount < 0 || t.SuccessCount > t.Count {
			return fmt.Errorf("expected successes must be between 0 and %d", t.Count)
		}
	default:
		return fmt.Errorf("success rule must be eq, lte or gte")
	}
	if len(t.UniquePath) > 1024 {
		return fmt.Errorf("unique path is too long")
	}
	if c := t.Check; c != nil {
		if c.RequestID == "" {
			return fmt.Errorf("choose the request that reads the value to check")
		}
		if c.RequestID == t.RequestID {
			return fmt.Errorf("the state check must use a different request than the one under test")
		}
		if c.Path == "" || len(c.Path) > 1024 {
			return fmt.Errorf("the state check needs a JSON path to a number")
		}
		if c.Tolerance < 0 || math.IsNaN(c.Tolerance) || math.IsNaN(c.DeltaPerSuccess) ||
			math.IsInf(c.Tolerance, 0) || math.IsInf(c.DeltaPerSuccess, 0) {
			return fmt.Errorf("the state check's change and tolerance must be finite, and tolerance not negative")
		}
	}
	return nil
}

// ---- permission matrix ----

// Access is the expected result of one role calling one endpoint.
type Access string

const (
	// AccessUnset means no expectation: the cell is shown but never fails.
	AccessUnset Access = ""
	AccessAllow Access = "allow"
	AccessDeny  Access = "deny"
)

const (
	MaxMatrixRoles    = 20
	MaxMatrixRequests = 500
)

// Role is one identity the matrix runs every endpoint as. Its variables
// override the environment's and collection's, so a collection whose auth is
// "Bearer {{token}}" switches user by setting token = {{user_b_token}}.
type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Anonymous sends requests without authentication.
	Anonymous bool       `json:"anonymous"`
	Variables []Variable `json:"variables"`
}

// PermissionMatrix runs selected requests of a collection as every role and
// compares who could call what with the approved expectations.
type PermissionMatrix struct {
	CollectionID string   `json:"collectionId"`
	Roles        []Role   `json:"roles"`
	RequestIDs   []string `json:"requestIds"`
	// Expected maps request ID → role ID → expected access.
	Expected  map[string]map[string]Access `json:"expected"`
	UpdatedAt time.Time                    `json:"updatedAt"`
}

func (m *PermissionMatrix) Normalize() {
	if m.Roles == nil {
		m.Roles = []Role{}
	}
	if m.RequestIDs == nil {
		m.RequestIDs = []string{}
	}
	if m.Expected == nil {
		m.Expected = map[string]map[string]Access{}
	}
	for i := range m.Roles {
		m.Roles[i].Name = strings.TrimSpace(m.Roles[i].Name)
		vars := m.Roles[i].Variables[:0:0]
		for _, v := range m.Roles[i].Variables {
			v.Key = strings.TrimSpace(v.Key)
			if v.Key == "" && v.Value == "" {
				continue
			}
			vars = append(vars, v)
		}
		m.Roles[i].Variables = vars
	}
	// Drop expectations for roles that no longer exist and unset cells.
	roles := map[string]bool{}
	for _, r := range m.Roles {
		roles[r.ID] = true
	}
	for req, cells := range m.Expected {
		for role, a := range cells {
			if !roles[role] || a == AccessUnset {
				delete(cells, role)
			}
		}
		if len(cells) == 0 {
			delete(m.Expected, req)
		}
	}
}

func (m *PermissionMatrix) Validate() error {
	if m.CollectionID == "" {
		return fmt.Errorf("permission matrix must belong to a collection")
	}
	if len(m.Roles) > MaxMatrixRoles {
		return fmt.Errorf("at most %d roles are allowed", MaxMatrixRoles)
	}
	if len(m.RequestIDs) > MaxMatrixRequests {
		return fmt.Errorf("at most %d requests are allowed", MaxMatrixRequests)
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, r := range m.Roles {
		if r.ID == "" {
			return fmt.Errorf("role %q has no id", r.Name)
		}
		if ids[r.ID] {
			return fmt.Errorf("duplicate role id %q", r.ID)
		}
		ids[r.ID] = true
		if err := ValidateName("role name", r.Name); err != nil {
			return err
		}
		if names[strings.ToLower(r.Name)] {
			return fmt.Errorf("two roles are named %q", r.Name)
		}
		names[strings.ToLower(r.Name)] = true
		if err := ValidateVariables(r.Variables); err != nil {
			return fmt.Errorf("role %q: %w", r.Name, err)
		}
	}
	for _, cells := range m.Expected {
		for _, a := range cells {
			if a != AccessAllow && a != AccessDeny && a != AccessUnset {
				return fmt.Errorf("expected access must be allow or deny, not %q", a)
			}
		}
	}
	return nil
}

// SecretValues returns the values of secret role variables.
func (m *PermissionMatrix) SecretValues() []string {
	var out []string
	for _, r := range m.Roles {
		for _, v := range r.Variables {
			if v.Secret && v.Value != "" {
				out = append(out, v.Value)
			}
		}
	}
	return out
}

// WithRoleVariables returns env with the role's enabled variables set,
// replacing variables of the same name. env may be nil.
func WithRoleVariables(env *Environment, role Role) *Environment {
	var out Environment
	if env != nil {
		out = *env
	}
	override := map[string]bool{}
	for _, v := range role.Variables {
		if v.Enabled && v.Key != "" {
			override[v.Key] = true
		}
	}
	vars := make([]Variable, 0, len(out.Variables)+len(role.Variables))
	for _, v := range out.Variables {
		if !override[v.Key] {
			vars = append(vars, v)
		}
	}
	for _, v := range role.Variables {
		if v.Enabled && v.Key != "" {
			vars = append(vars, v)
		}
	}
	out.Variables = vars
	return &out
}

// RemapRequests returns the race test with its request references replaced
// through ids (old ID → new ID). ok is false when a referenced request is
// not in ids.
func (t RaceTest) RemapRequests(ids map[string]string) (RaceTest, bool) {
	id, ok := ids[t.RequestID]
	if !ok {
		return t, false
	}
	t.RequestID = id
	if t.Check != nil {
		c := *t.Check
		if c.RequestID != "" {
			if c.RequestID, ok = ids[c.RequestID]; !ok {
				return t, false
			}
		}
		t.Check = &c
	}
	return t, true
}

// RemapRequests returns the matrix for collectionID with request references
// replaced through ids (old ID → new ID). Requests that are not in ids are
// dropped, with their expectations; missing lists how many.
func (m PermissionMatrix) RemapRequests(collectionID string, ids map[string]string) (out PermissionMatrix, missing int) {
	out = PermissionMatrix{CollectionID: collectionID, Roles: m.Roles, RequestIDs: []string{},
		Expected: map[string]map[string]Access{}}
	for _, id := range m.RequestIDs {
		if n, ok := ids[id]; ok {
			out.RequestIDs = append(out.RequestIDs, n)
		} else {
			missing++
		}
	}
	for id, cells := range m.Expected {
		if n, ok := ids[id]; ok {
			out.Expected[n] = cells
		}
	}
	out.Normalize()
	return out, missing
}

// IsEmpty reports whether the matrix has nothing worth keeping.
func (m PermissionMatrix) IsEmpty() bool { return len(m.Roles) == 0 && len(m.RequestIDs) == 0 }
