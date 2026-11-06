package domain

import (
	"errors"
	"strings"
	"time"
)

// EnvComparison holds a collection's environment-comparison settings: which
// two environments to compare and how. The comparison result is not stored.
type EnvComparison struct {
	CollectionID string `json:"collectionId"`
	// EnvA is the reference (for example Production or the old build); EnvB
	// is the environment being checked.
	EnvA string `json:"envA"`
	EnvB string `json:"envB"`
	// IgnoredPaths are body path patterns, or "header:Name" lines.
	IgnoredPaths []string `json:"ignoredPaths"`
	// IncludeModifying also compares requests that may modify data.
	IncludeModifying bool `json:"includeModifying"`
	// DetectChanging sends read-only requests twice per environment and leaves
	// out fields whose value differs between the two calls.
	DetectChanging bool      `json:"detectChanging"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// DefaultEnvComparison is used for a collection without saved settings.
func DefaultEnvComparison(collectionID string) EnvComparison {
	return EnvComparison{CollectionID: collectionID, IgnoredPaths: []string{}, DetectChanging: true}
}

// Normalize trims and de-duplicates ignored paths.
func (c *EnvComparison) Normalize() {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range c.IgnoredPaths {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	c.IgnoredPaths = out
}

// Validate checks the settings before they are saved or run.
func (c *EnvComparison) Validate() error {
	if c.CollectionID == "" {
		return errors.New("environment comparison needs a collection")
	}
	if len(c.IgnoredPaths) > 500 {
		return errors.New("at most 500 ignored paths")
	}
	return nil
}

// ValidateForRun also requires two different environments.
func (c *EnvComparison) ValidateForRun() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.EnvA == "" || c.EnvB == "" {
		return errors.New("choose the two environments to compare")
	}
	if c.EnvA == c.EnvB {
		return errors.New("choose two different environments to compare")
	}
	return nil
}
