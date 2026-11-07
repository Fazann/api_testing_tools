package domain

import (
	"encoding/json"
	"time"
)

// Contract is the OpenAPI or Swagger document attached to a collection, used
// to check responses against it (docs/openapi-contract.md).
type Contract struct {
	CollectionID string `json:"collectionId"`
	// Spec is the document as JSON.
	Spec json.RawMessage `json:"spec"`
	// SourceName is the file name the spec was loaded from, for display.
	SourceName string `json:"sourceName"`
	// Enabled turns the check on; a disabled contract is kept but not used.
	Enabled bool `json:"enabled"`
	// AllowUndocumented accepts response fields the spec does not mention.
	AllowUndocumented bool      `json:"allowUndocumented"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
