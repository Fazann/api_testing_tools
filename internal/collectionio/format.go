// Package collectionio implements the versioned collection export format
// documented in docs/collection-format.md. Imported files are untrusted.
package collectionio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/redact"
	"api-test-kits/internal/storage"
)

const (
	FormatName     = "api-regression-guard.collection"
	CurrentVersion = 1

	MaxFileBytes    = 20 << 20
	MaxRequests     = 5_000
	MaxFolders      = 1_000
	MaxEnvironments = 50
)

// Document is the on-disk export format.
type Document struct {
	Format       string               `json:"format"`
	Version      int                  `json:"version"`
	ExportedAt   time.Time            `json:"exportedAt"`
	AppVersion   string               `json:"appVersion,omitempty"`
	Collection   domain.Collection    `json:"collection"`
	Folders      []domain.Folder      `json:"folders"`
	Requests     []domain.Request     `json:"requests"`
	Environments []domain.Environment `json:"environments,omitempty"`
	// SecretsIncluded is true when the user chose to export secret values.
	SecretsIncluded bool `json:"secretsIncluded"`
}

// SecretFinding describes a secret value that an export would contain.
type SecretFinding struct {
	Location string `json:"location"`
	Detail   string `json:"detail"`
}

// ScanSecrets lists the secret values present in a tree and environments.
// Credential fields that only reference {{variables}} are not secrets.
func ScanSecrets(t domain.CollectionTree, envs []domain.Environment) []SecretFinding {
	var out []SecretFinding
	for _, c := range t.Collection.Auth.Credentials() {
		if isLiteral(c) {
			out = append(out, SecretFinding{Location: "collection " + t.Collection.Name,
				Detail: string(t.Collection.Auth.Type) + " credential is stored in the collection auth"})
			break
		}
	}
	for _, v := range t.Collection.Variables {
		if v.Secret && v.Value != "" {
			out = append(out, SecretFinding{Location: "collection " + t.Collection.Name, Detail: "secret variable " + v.Key})
		}
	}
	for _, r := range t.Requests {
		for _, c := range r.Auth.Credentials() {
			if isLiteral(c) {
				out = append(out, SecretFinding{Location: "request " + r.Name, Detail: string(r.Auth.Type) + " credential is stored in the request"})
				break
			}
		}
		for _, h := range r.Headers {
			if redact.New().IsSensitiveHeader(h.Key) && isLiteral(h.Value) {
				out = append(out, SecretFinding{Location: "request " + r.Name, Detail: "header " + h.Key + " has a literal value"})
			}
		}
	}
	for _, e := range envs {
		for _, v := range e.Variables {
			if v.Secret && v.Value != "" {
				out = append(out, SecretFinding{Location: "environment " + e.Name, Detail: "secret variable " + v.Key})
			}
		}
	}
	return out
}

// stripSecretValues returns a copy of vars with secret values blanked.
func stripSecretValues(vars []domain.Variable) []domain.Variable {
	if vars == nil {
		return nil
	}
	out := make([]domain.Variable, len(vars))
	for i, v := range vars {
		if v.Secret {
			v.Value = ""
		}
		out[i] = v
	}
	return out
}

func isLiteral(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && !strings.Contains(v, "{{")
}

// Export builds an export document. Unless includeSecrets is true, secret
// variable values and literal credentials are blanked.
func Export(t domain.CollectionTree, envs []domain.Environment, includeSecrets bool, appVersion string, now time.Time) Document {
	doc := Document{
		Format:          FormatName,
		Version:         CurrentVersion,
		ExportedAt:      now.UTC(),
		AppVersion:      appVersion,
		Collection:      t.Collection,
		Folders:         t.Folders,
		Requests:        make([]domain.Request, len(t.Requests)),
		SecretsIncluded: includeSecrets,
	}
	sensitive := redact.New()
	if !includeSecrets {
		doc.Collection.Auth = stripCredentials(doc.Collection.Auth)
		doc.Collection.Variables = stripSecretValues(doc.Collection.Variables)
	}
	for i, r := range t.Requests {
		if !includeSecrets {
			r.Auth = stripCredentials(r.Auth)
			headers := make([]domain.KeyValue, len(r.Headers))
			for j, h := range r.Headers {
				if sensitive.IsSensitiveHeader(h.Key) && isLiteral(h.Value) {
					h.Value = ""
				}
				headers[j] = h
			}
			r.Headers = headers
		}
		doc.Requests[i] = r
	}
	for _, e := range envs {
		if !includeSecrets {
			e.Variables = stripSecretValues(e.Variables)
		}
		doc.Environments = append(doc.Environments, e)
	}
	return doc
}

// stripCredentials blanks literal credentials; {{variable}} references stay.
func stripCredentials(a domain.Auth) domain.Auth {
	if isLiteral(a.Token) {
		a.Token = ""
	}
	if isLiteral(a.Password) {
		a.Password = ""
	}
	if isLiteral(a.KeyValue) {
		a.KeyValue = ""
	}
	return a
}

// Write encodes doc as indented JSON.
func Write(w io.Writer, doc Document) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// ValidationError lists every problem found in an import file.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	const show = 10
	p := e.Problems
	more := ""
	if len(p) > show {
		more = fmt.Sprintf(" (and %d more)", len(p)-show)
		p = p[:show]
	}
	return "import file is invalid: " + strings.Join(p, "; ") + more
}

// Imported is a validated import with fresh IDs, ready to store.
type Imported struct {
	Tree         domain.CollectionTree
	Environments []domain.Environment
}

// Read parses and validates an import file. All IDs are regenerated so an
// import can never overwrite existing records.
func Read(r io.Reader) (*Imported, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("import file is larger than %d MB", MaxFileBytes>>20)
	}
	if looksLikeYAML(data) {
		if data, err = yamlToJSON(data); err != nil {
			return nil, fmt.Errorf("import file is not valid JSON or YAML: %w", err)
		}
	}
	var probe struct {
		Format  string `json:"format"`
		Swagger any    `json:"swagger"`
		OpenAPI any    `json:"openapi"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("import file is not valid JSON: %w", err)
	}
	switch {
	case probe.Format == FormatName:
		var doc Document
		if err := json.NewDecoder(bytes.NewReader(data)).Decode(&doc); err != nil {
			return nil, fmt.Errorf("import file is not a valid collection: %w", err)
		}
		return validate(doc)
	case probe.Swagger != nil || probe.OpenAPI != nil:
		doc, err := FromOpenAPI(data)
		if err != nil {
			return nil, err
		}
		return validate(doc)
	case probe.Format != "":
		return nil, fmt.Errorf("unrecognized file format %q (expected %q or an OpenAPI/Swagger document)", probe.Format, FormatName)
	}
	return nil, errors.New("unrecognized file: expected an API Regression Guard collection export " +
		"or an OpenAPI 3 / Swagger 2 document (swagger.json, openapi.json or .yaml)")
}

// validate checks a decoded document and assigns fresh IDs.
func validate(doc Document) (*Imported, error) {
	if doc.Version < 1 || doc.Version > CurrentVersion {
		return nil, fmt.Errorf("unsupported format version %d; this app supports version %d", doc.Version, CurrentVersion)
	}
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if len(doc.Requests) > MaxRequests {
		add("too many requests (%d, max %d)", len(doc.Requests), MaxRequests)
	}
	if len(doc.Folders) > MaxFolders {
		add("too many folders (%d, max %d)", len(doc.Folders), MaxFolders)
	}
	if len(doc.Environments) > MaxEnvironments {
		add("too many environments (%d, max %d)", len(doc.Environments), MaxEnvironments)
	}
	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}

	c := domain.Collection{
		Name: strings.TrimSpace(doc.Collection.Name), Description: doc.Collection.Description,
		RequiredVariables:  doc.Collection.RequiredVariables,
		Variables:          doc.Collection.Variables,
		Auth:               doc.Collection.Auth,
		PreRequestScript:   doc.Collection.PreRequestScript,
		PostResponseScript: doc.Collection.PostResponseScript,
	}
	if err := c.Validate(); err != nil {
		add("collection: %v", err)
	}
	folderIDs := map[string]bool{}
	for i, f := range doc.Folders {
		if f.ID == "" || folderIDs[f.ID] {
			add("folder %d: missing or duplicate id", i+1)
		}
		folderIDs[f.ID] = true
		if err := domain.ValidateName("folder name", f.Name); err != nil {
			add("folder %d: %v", i+1, err)
		}
	}
	reqs := make([]domain.Request, 0, len(doc.Requests))
	for i, r := range doc.Requests {
		r.Normalize()
		if r.ParentID != "" && !folderIDs[r.ParentID] {
			r.ParentID = ""
		}
		if err := r.Validate(); err != nil {
			label := r.Name
			if label == "" {
				label = fmt.Sprintf("#%d", i+1)
			}
			add("request %s: %v", label, err)
		}
		r.CreatedAt, r.UpdatedAt = time.Time{}, time.Time{}
		reqs = append(reqs, r)
	}
	envs := make([]domain.Environment, 0, len(doc.Environments))
	for i, e := range doc.Environments {
		e.ID = ""
		e.CreatedAt, e.UpdatedAt = time.Time{}, time.Time{}
		if e.Variables == nil {
			e.Variables = []domain.Variable{}
		}
		if err := e.Validate(); err != nil {
			add("environment %d: %v", i+1, err)
		}
		envs = append(envs, e)
	}
	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	tree := storage.ReassignIDs(domain.CollectionTree{Collection: c, Folders: doc.Folders, Requests: reqs})
	return &Imported{Tree: tree, Environments: envs}, nil
}
