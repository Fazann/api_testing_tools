package scripting

import (
	"fmt"
	"sort"
	"sync"

	"api-test-kits/internal/domain"
)

// Scope names a set of variables that scripts can change.
type Scope string

const (
	ScopeEnvironment Scope = "environment"
	ScopeCollection  Scope = "collection"
)

// Variables is the variable state of one run. Scripts read and change it
// through pm.environment and pm.collectionVariables, and every request in the
// run sees the changes made by the requests before it. It is safe for
// concurrent use.
type Variables struct {
	mu         sync.Mutex
	env        *domain.Environment // nil until something is set when no environment is selected
	collection []domain.Variable
	changed    map[Scope]bool
}

// NewVariables copies env (which may be nil) and the collection variables.
func NewVariables(env *domain.Environment, collection []domain.Variable) *Variables {
	return &Variables{env: copyEnv(env), collection: append([]domain.Variable(nil), collection...), changed: map[Scope]bool{}}
}

func copyEnv(env *domain.Environment) *domain.Environment {
	if env == nil {
		return nil
	}
	c := *env
	c.Variables = append([]domain.Variable(nil), env.Variables...)
	return &c
}

// Resolved returns the variables a request sees: collection variables, then
// environment variables, then local ones (pm.variables), each overriding the
// previous. A local value that replaces a secret variable stays secret.
func (v *Variables) Resolved(local map[string]string) *domain.Environment {
	v.mu.Lock()
	merged := domain.WithCollectionVariables(copyEnv(v.env), append([]domain.Variable(nil), v.collection...))
	v.mu.Unlock()
	if len(local) == 0 {
		return merged
	}
	if merged == nil {
		merged = &domain.Environment{}
	}
	secret := map[string]bool{}
	kept := merged.Variables[:0:0]
	for _, mv := range merged.Variables {
		if _, ok := local[mv.Key]; ok {
			secret[mv.Key] = secret[mv.Key] || (mv.Secret && mv.Enabled)
			continue
		}
		kept = append(kept, mv)
	}
	keys := make([]string, 0, len(local))
	for k := range local {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kept = append(kept, domain.Variable{Key: k, Value: local[k], Secret: secret[k], Enabled: true})
	}
	merged.Variables = kept
	return merged
}

func (v *Variables) list(scope Scope) *[]domain.Variable {
	if scope == ScopeCollection {
		return &v.collection
	}
	if v.env == nil {
		v.env = &domain.Environment{}
	}
	return &v.env.Variables
}

// Get returns an enabled variable's value.
func (v *Variables) Get(scope Scope, name string) (string, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if scope == ScopeEnvironment && v.env == nil {
		return "", false
	}
	for _, x := range *v.list(scope) {
		if x.Key == name && x.Enabled {
			return x.Value, true
		}
	}
	return "", false
}

// Set creates or updates a variable. Updating keeps its secret flag and
// enables it; new variables are not secret.
func (v *Variables) Set(scope Scope, name, value string) error {
	if !domain.IsValidVariableName(name) {
		return fmt.Errorf("variable name %q is invalid: use letters, digits, '_', '.', or '-'", name)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	list := v.list(scope)
	for i := range *list {
		if (*list)[i].Key == name {
			(*list)[i].Value, (*list)[i].Enabled = value, true
			v.changed[scope] = true
			return nil
		}
	}
	if len(*list) >= domain.MaxVariables {
		return fmt.Errorf("at most %d variables are allowed", domain.MaxVariables)
	}
	*list = append(*list, domain.Variable{Key: name, Value: value, Enabled: true})
	v.changed[scope] = true
	return nil
}

// Unset removes a variable.
func (v *Variables) Unset(scope Scope, name string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if scope == ScopeEnvironment && v.env == nil {
		return
	}
	list := v.list(scope)
	out := (*list)[:0]
	for _, x := range *list {
		if x.Key != name {
			out = append(out, x)
		}
	}
	if len(out) != len(*list) {
		v.changed[scope] = true
	}
	*list = out
}

// All returns the enabled variables of scope as a map.
func (v *Variables) All(scope Scope) map[string]string {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := map[string]string{}
	if scope == ScopeEnvironment && v.env == nil {
		return out
	}
	for _, x := range *v.list(scope) {
		if x.Enabled && x.Key != "" {
			if _, dup := out[x.Key]; !dup {
				out[x.Key] = x.Value
			}
		}
	}
	return out
}

// Environment returns the current environment variables and whether a
// script changed them. The environment is nil if none was selected and
// nothing was set.
func (v *Variables) Environment() (*domain.Environment, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return copyEnv(v.env), v.changed[ScopeEnvironment]
}

// Collection returns the current collection variables and whether a script
// changed them.
func (v *Variables) Collection() ([]domain.Variable, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]domain.Variable(nil), v.collection...), v.changed[ScopeCollection]
}
