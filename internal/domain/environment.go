package domain

import (
	"fmt"
	"regexp"
	"time"
)

const MaxVariables = 500

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]{0,127}$`)

// IsValidVariableName reports whether name can be used inside {{...}}.
func IsValidVariableName(name string) bool { return variableNamePattern.MatchString(name) }

// Variable is a single environment entry. Secret values are masked in the UI,
// redacted from reports and stripped from exports unless explicitly included.
type Variable struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Secret  bool   `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// Environment is a named set of variables such as Local, Staging or Production.
type Environment struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Variables []Variable `json:"variables"`
	// Protected marks production-like environments. Destructive requests
	// against protected environments require explicit confirmation.
	Protected bool      `json:"protected"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (e *Environment) Validate() error {
	if err := ValidateName("environment name", e.Name); err != nil {
		return err
	}
	if len(e.Variables) > MaxVariables {
		return fmt.Errorf("at most %d variables are allowed", MaxVariables)
	}
	seen := map[string]bool{}
	for _, v := range e.Variables {
		if v.Key == "" && v.Value == "" {
			continue
		}
		if !IsValidVariableName(v.Key) {
			return fmt.Errorf("variable name %q is invalid: use letters, digits, '_', '.', or '-'", v.Key)
		}
		if seen[v.Key] {
			return fmt.Errorf("variable %q is defined more than once", v.Key)
		}
		seen[v.Key] = true
	}
	return nil
}

// SecretValues returns the non-empty values of enabled secret variables.
func (e *Environment) SecretValues() []string {
	if e == nil {
		return nil
	}
	var out []string
	for _, v := range e.Variables {
		if v.Secret && v.Value != "" {
			out = append(out, v.Value)
		}
	}
	return out
}

// HasSecrets reports whether any variable is marked secret and has a value.
func (e *Environment) HasSecrets() bool { return len(e.SecretValues()) > 0 }
