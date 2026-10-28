// Package domain holds the typed models shared by the engine, storage and UI
// layers, plus explicit validation for each of them.
package domain

import (
	"fmt"
	"strings"
	"time"
)

const (
	MaxNameLength        = 200
	MaxDescriptionLength = 10_000
)

// Collection groups folders and requests that are run together.
type Collection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// RequiredVariables lists environment variables that must be present and
	// non-empty before the collection runs.
	RequiredVariables []string `json:"requiredVariables"`
	// Auth applies to every request whose auth type is "inherit".
	Auth      Auth      `json:"auth"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Folder organizes requests inside a collection. Folders may nest.
type Folder struct {
	ID           string    `json:"id"`
	CollectionID string    `json:"collectionId"`
	ParentID     string    `json:"parentId"`
	Name         string    `json:"name"`
	SortOrder    int       `json:"sortOrder"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// CollectionTree is a collection with all of its folders and requests.
type CollectionTree struct {
	Collection Collection `json:"collection"`
	Folders    []Folder   `json:"folders"`
	Requests   []Request  `json:"requests"`
}

func (c *Collection) Validate() error {
	if err := ValidateName("collection name", c.Name); err != nil {
		return err
	}
	if len(c.Description) > MaxDescriptionLength {
		return fmt.Errorf("collection description must be at most %d characters", MaxDescriptionLength)
	}
	if c.Auth.Type == "" {
		c.Auth.Type = AuthNone
	}
	if c.Auth.Type == AuthInherit {
		return fmt.Errorf("a collection has no parent to inherit auth from")
	}
	if err := c.Auth.Validate(); err != nil {
		return fmt.Errorf("collection auth: %w", err)
	}
	for _, v := range c.RequiredVariables {
		if !IsValidVariableName(v) {
			return fmt.Errorf("required variable %q is not a valid variable name", v)
		}
	}
	return nil
}

func (f *Folder) Validate() error {
	if f.CollectionID == "" {
		return fmt.Errorf("folder must belong to a collection")
	}
	if f.ParentID != "" && f.ParentID == f.ID {
		return fmt.Errorf("folder cannot be its own parent")
	}
	return ValidateName("folder name", f.Name)
}

// ValidateName checks a user-provided display name.
func ValidateName(field, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("%s must be at most %d characters", field, MaxNameLength)
	}
	if strings.ContainsAny(name, "\x00\r\n") {
		return fmt.Errorf("%s must not contain control characters", field)
	}
	return nil
}
