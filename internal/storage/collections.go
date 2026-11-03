package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"api-test-kits/internal/domain"
)

// ---- collections ----

func (s *Store) ListCollections(ctx context.Context) ([]domain.Collection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, required_vars_json, variables_json, auth_json, pre_request_script, post_response_script, created_at, updated_at FROM collections ORDER BY name COLLATE NOCASE, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Collection{}
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCollection(ctx context.Context, id string) (domain.Collection, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, description, required_vars_json, variables_json, auth_json, pre_request_script, post_response_script, created_at, updated_at FROM collections WHERE id = ?`, id)
	c, err := scanCollection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

type scanner interface{ Scan(...any) error }

func scanCollection(r scanner) (domain.Collection, error) {
	var c domain.Collection
	var reqVars, vars, auth, created, updated string
	if err := r.Scan(&c.ID, &c.Name, &c.Description, &reqVars, &vars, &auth, &c.PreRequestScript, &c.PostResponseScript, &created, &updated); err != nil {
		return c, err
	}
	unmarshalLenient(auth, &c.Auth)
	if c.Auth.Type == "" {
		c.Auth.Type = domain.AuthNone
	}
	unmarshalLenient(reqVars, &c.RequiredVariables)
	if c.RequiredVariables == nil {
		c.RequiredVariables = []string{}
	}
	unmarshalLenient(vars, &c.Variables)
	if c.Variables == nil {
		c.Variables = []domain.Variable{}
	}
	c.CreatedAt, c.UpdatedAt = parseTS(created), parseTS(updated)
	return c, nil
}

// SaveCollection inserts or updates c and returns the stored record.
func (s *Store) SaveCollection(ctx context.Context, c domain.Collection) (domain.Collection, error) {
	if err := c.Validate(); err != nil {
		return c, err
	}
	now := time.Now().UTC()
	if c.RequiredVariables == nil {
		c.RequiredVariables = []string{}
	}
	if c.Variables == nil {
		c.Variables = []domain.Variable{}
	}
	if c.ID == "" {
		c.ID = uuid.NewString()
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO collections (id, name, description, required_vars_json, variables_json, auth_json,
			pre_request_script, post_response_script, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, description = excluded.description,
			required_vars_json = excluded.required_vars_json, variables_json = excluded.variables_json,
			auth_json = excluded.auth_json, pre_request_script = excluded.pre_request_script,
			post_response_script = excluded.post_response_script, updated_at = excluded.updated_at`,
		c.ID, c.Name, c.Description, mustJSON(c.RequiredVariables), mustJSON(c.Variables), mustJSON(c.Auth),
		c.PreRequestScript, c.PostResponseScript, ts(c.CreatedAt), ts(c.UpdatedAt))
	return c, err
}

// InheritCollectionAuth switches every request in the collection to inherit
// the collection's auth and returns how many requests changed.
func (s *Store) InheritCollectionAuth(ctx context.Context, collectionID string) (int, error) {
	if _, err := s.GetCollection(ctx, collectionID); err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE requests SET auth_json = ?, updated_at = ? WHERE collection_id = ?
			AND (CASE WHEN json_valid(auth_json) THEN json_extract(auth_json, '$.type') END) IS NOT 'inherit'`,
		mustJSON(domain.Auth{Type: domain.AuthInherit}), ts(time.Now().UTC()), collectionID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteCollection removes a collection with its folders, requests,
// assertions and baselines. Run history is kept.
func (s *Store) DeleteCollection(ctx context.Context, id string) error {
	return s.execOne(ctx, `DELETE FROM collections WHERE id = ?`, id)
}

func (s *Store) execOne(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetTree loads a collection with all folders and requests.
func (s *Store) GetTree(ctx context.Context, id string) (domain.CollectionTree, error) {
	var t domain.CollectionTree
	c, err := s.GetCollection(ctx, id)
	if err != nil {
		return t, err
	}
	t.Collection = c
	if t.Folders, err = s.listFolders(ctx, s.db, id); err != nil {
		return t, err
	}
	if t.Requests, err = s.listRequests(ctx, s.db, `WHERE r.collection_id = ?`, id); err != nil {
		return t, err
	}
	return t, nil
}

// ImportTree stores a complete tree in one transaction. IDs must already be
// unique (callers generate fresh IDs for imported data).
func (s *Store) ImportTree(ctx context.Context, t domain.CollectionTree) error {
	if err := t.Collection.Validate(); err != nil {
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		c := t.Collection
		if _, err := tx.ExecContext(ctx, `INSERT INTO collections (id, name, description, required_vars_json, variables_json, auth_json,
			pre_request_script, post_response_script, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, c.Name, c.Description, mustJSON(nonNil(c.RequiredVariables)), mustJSON(nonNilVars(c.Variables)), mustJSON(c.Auth),
			c.PreRequestScript, c.PostResponseScript, ts(now), ts(now)); err != nil {
			return err
		}
		for _, f := range orderFolders(t.Folders) {
			f.CollectionID = c.ID
			if err := insertFolder(ctx, tx, f, now); err != nil {
				return err
			}
		}
		for _, r := range t.Requests {
			r.CollectionID = c.ID
			if err := writeRequest(ctx, tx, &r, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// DuplicateCollection copies a collection, its folders, requests and
// assertions (baselines and history are not copied).
func (s *Store) DuplicateCollection(ctx context.Context, id, newName string) (domain.Collection, error) {
	t, err := s.GetTree(ctx, id)
	if err != nil {
		return domain.Collection{}, err
	}
	t = ReassignIDs(t)
	t.Collection.Name = newName
	if err := s.ImportTree(ctx, t); err != nil {
		return domain.Collection{}, err
	}
	return s.GetCollection(ctx, t.Collection.ID)
}

// ReassignIDs gives every entity in t a fresh ID while keeping relationships.
func ReassignIDs(t domain.CollectionTree) domain.CollectionTree {
	t.Collection.ID = uuid.NewString()
	folderIDs := map[string]string{}
	for _, f := range t.Folders {
		folderIDs[f.ID] = uuid.NewString()
	}
	folders := make([]domain.Folder, len(t.Folders))
	for i, f := range t.Folders {
		f.ID = folderIDs[f.ID]
		f.ParentID = folderIDs[f.ParentID] // unknown parents become top-level
		f.CollectionID = t.Collection.ID
		folders[i] = f
	}
	reqs := make([]domain.Request, len(t.Requests))
	for i, r := range t.Requests {
		r.ID = uuid.NewString()
		r.ParentID = folderIDs[r.ParentID]
		r.CollectionID = t.Collection.ID
		as := make([]domain.Assertion, len(r.Assertions))
		for j, a := range r.Assertions {
			a.ID = uuid.NewString()
			a.RequestID = r.ID
			as[j] = a
		}
		r.Assertions = as
		reqs[i] = r
	}
	t.Folders, t.Requests = folders, reqs
	return t
}

// orderFolders returns parents before children so foreign keys hold.
// Folders with unknown parents or in a cycle are moved to the top level.
func orderFolders(folders []domain.Folder) []domain.Folder {
	known := map[string]bool{}
	for _, f := range folders {
		known[f.ID] = true
	}
	placed := map[string]bool{}
	out := make([]domain.Folder, 0, len(folders))
	remaining := folders
	for len(remaining) > 0 {
		var next []domain.Folder
		for _, f := range remaining {
			if !known[f.ParentID] {
				f.ParentID = ""
			}
			if f.ParentID == "" || placed[f.ParentID] {
				placed[f.ID] = true
				out = append(out, f)
			} else {
				next = append(next, f)
			}
		}
		if len(next) == len(remaining) {
			// No progress: the rest form a cycle.
			for _, f := range next {
				f.ParentID = ""
				placed[f.ID] = true
				out = append(out, f)
			}
			break
		}
		remaining = next
	}
	return out
}

func nonNilVars(v []domain.Variable) []domain.Variable {
	if v == nil {
		return []domain.Variable{}
	}
	return v
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ---- folders ----

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *Store) listFolders(ctx context.Context, q querier, collectionID string) ([]domain.Folder, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, collection_id, IFNULL(parent_id, ''), name, sort_order, created_at, updated_at FROM folders WHERE collection_id = ? ORDER BY sort_order, name COLLATE NOCASE`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Folder{}
	for rows.Next() {
		var f domain.Folder
		var created, updated string
		if err := rows.Scan(&f.ID, &f.CollectionID, &f.ParentID, &f.Name, &f.SortOrder, &created, &updated); err != nil {
			return nil, err
		}
		f.CreatedAt, f.UpdatedAt = parseTS(created), parseTS(updated)
		out = append(out, f)
	}
	return out, rows.Err()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertFolder(ctx context.Context, x execer, f domain.Folder, now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	_, err := x.ExecContext(ctx, `INSERT INTO folders (id, collection_id, parent_id, name, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.CollectionID, nullable(f.ParentID), f.Name, f.SortOrder, ts(now), ts(now))
	return err
}

// SaveFolder creates or renames/moves a folder.
func (s *Store) SaveFolder(ctx context.Context, f domain.Folder) (domain.Folder, error) {
	now := time.Now().UTC()
	if f.ID == "" {
		f.ID = uuid.NewString()
		f.CreatedAt, f.UpdatedAt = now, now
		return f, insertFolder(ctx, s.db, f, now)
	}
	if err := f.Validate(); err != nil {
		return f, err
	}
	if f.ParentID != "" {
		// Refuse moves that would create a cycle.
		cur := f.ParentID
		for i := 0; cur != "" && i < 1000; i++ {
			if cur == f.ID {
				return f, errors.New("a folder cannot be moved into itself")
			}
			if err := s.db.QueryRowContext(ctx, `SELECT IFNULL(parent_id, '') FROM folders WHERE id = ?`, cur).Scan(&cur); err != nil {
				return f, err
			}
		}
	}
	f.UpdatedAt = now
	err := s.execOne(ctx, `UPDATE folders SET name = ?, parent_id = ?, sort_order = ?, updated_at = ? WHERE id = ?`,
		f.Name, nullable(f.ParentID), f.SortOrder, ts(now), f.ID)
	return f, err
}

// DeleteFolder removes a folder and everything inside it.
func (s *Store) DeleteFolder(ctx context.Context, id string) error {
	return s.execOne(ctx, `DELETE FROM folders WHERE id = ?`, id)
}

// ---- requests ----

const requestColumns = `r.id, r.collection_id, IFNULL(r.parent_id, ''), r.name, r.method, r.url_template, r.headers_json, r.query_json,
	r.body, r.body_type, r.auth_json, r.timeout_ms, r.follow_redirects, r.sort_order, r.pre_request_script, r.post_response_script,
	r.created_at, r.updated_at`

func (s *Store) listRequests(ctx context.Context, q querier, where string, args ...any) ([]domain.Request, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+requestColumns+` FROM requests r `+where+` ORDER BY r.sort_order, r.name COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	var out []domain.Request
	idx := map[string]int{}
	for rows.Next() {
		var r domain.Request
		var headers, query, auth, created, updated string
		var follow int
		if err := rows.Scan(&r.ID, &r.CollectionID, &r.ParentID, &r.Name, &r.Method, &r.URLTemplate, &headers, &query,
			&r.Body, &r.BodyType, &auth, &r.TimeoutMs, &follow, &r.SortOrder, &r.PreRequestScript, &r.PostResponseScript,
			&created, &updated); err != nil {
			rows.Close()
			return nil, err
		}
		unmarshalLenient(headers, &r.Headers)
		unmarshalLenient(query, &r.Query)
		unmarshalLenient(auth, &r.Auth)
		r.FollowRedirects = follow == 1
		r.CreatedAt, r.UpdatedAt = parseTS(created), parseTS(updated)
		r.Normalize()
		idx[r.ID] = len(out)
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		return []domain.Request{}, nil
	}
	// Load assertions for all requests in one query.
	arows, err := q.QueryContext(ctx, `SELECT a.id, a.request_id, a.kind, a.path, a.operator, IFNULL(a.expected_json, ''), a.options_json, a.enabled, a.created_at
		FROM assertions a JOIN requests r ON r.id = a.request_id `+where+` ORDER BY a.sort_order`, args...)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var a domain.Assertion
		var expected, options, created string
		var enabled int
		if err := arows.Scan(&a.ID, &a.RequestID, &a.Kind, &a.Path, &a.Operator, &expected, &options, &enabled, &created); err != nil {
			return nil, err
		}
		if expected != "" {
			a.Expected = json.RawMessage(expected)
		}
		var opt assertionOptions
		unmarshalLenient(options, &opt)
		a.Tolerance, a.Function, a.Description = opt.Tolerance, opt.Function, opt.Description
		a.Enabled = enabled == 1
		a.CreatedAt = parseTS(created)
		if i, ok := idx[a.RequestID]; ok {
			out[i].Assertions = append(out[i].Assertions, a)
		}
	}
	return out, arows.Err()
}

type assertionOptions struct {
	Tolerance   float64 `json:"tolerance,omitempty"`
	Function    string  `json:"function,omitempty"`
	Description string  `json:"description,omitempty"`
}

func (s *Store) GetRequest(ctx context.Context, id string) (domain.Request, error) {
	list, err := s.listRequests(ctx, s.db, `WHERE r.id = ?`, id)
	if err != nil {
		return domain.Request{}, err
	}
	if len(list) == 0 {
		return domain.Request{}, ErrNotFound
	}
	return list[0], nil
}

// SaveRequest inserts or updates a request and replaces its assertions in a
// single transaction.
func (s *Store) SaveRequest(ctx context.Context, r domain.Request) (domain.Request, error) {
	r.Normalize()
	if err := r.Validate(); err != nil {
		return r, err
	}
	if r.CollectionID == "" {
		return r, errors.New("request must belong to a collection")
	}
	err := s.tx(ctx, func(tx *sql.Tx) error { return writeRequest(ctx, tx, &r, time.Now().UTC()) })
	if err != nil {
		return r, err
	}
	return s.GetRequest(ctx, r.ID)
}

func writeRequest(ctx context.Context, tx *sql.Tx, r *domain.Request, now time.Time) error {
	r.Normalize()
	if err := r.Validate(); err != nil {
		return err
	}
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	_, err := tx.ExecContext(ctx, `
		INSERT INTO requests (id, collection_id, parent_id, name, method, url_template, headers_json, query_json, body, body_type,
			auth_json, timeout_ms, follow_redirects, sort_order, pre_request_script, post_response_script, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET parent_id = excluded.parent_id, name = excluded.name, method = excluded.method,
			url_template = excluded.url_template, headers_json = excluded.headers_json, query_json = excluded.query_json,
			body = excluded.body, body_type = excluded.body_type, auth_json = excluded.auth_json, timeout_ms = excluded.timeout_ms,
			follow_redirects = excluded.follow_redirects, sort_order = excluded.sort_order,
			pre_request_script = excluded.pre_request_script, post_response_script = excluded.post_response_script,
			updated_at = excluded.updated_at`,
		r.ID, r.CollectionID, nullable(r.ParentID), r.Name, r.Method, r.URLTemplate, mustJSON(r.Headers), mustJSON(r.Query),
		r.Body, string(r.BodyType), mustJSON(r.Auth), r.TimeoutMs, boolInt(r.FollowRedirects), r.SortOrder,
		r.PreRequestScript, r.PostResponseScript, ts(r.CreatedAt), ts(r.UpdatedAt))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM assertions WHERE request_id = ?`, r.ID); err != nil {
		return err
	}
	for i := range r.Assertions {
		a := &r.Assertions[i]
		if a.ID == "" {
			a.ID = uuid.NewString()
		}
		a.RequestID = r.ID
		if a.CreatedAt.IsZero() {
			a.CreatedAt = now
		}
		var expected any
		if len(a.Expected) > 0 {
			expected = string(a.Expected)
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO assertions (id, request_id, kind, path, operator, expected_json, options_json, enabled, sort_order, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, r.ID, string(a.Kind), a.Path, string(a.Operator), expected,
			mustJSON(assertionOptions{Tolerance: a.Tolerance, Function: a.Function, Description: a.Description}),
			boolInt(a.Enabled), i, ts(a.CreatedAt))
		if err != nil {
			return err
		}
	}
	return nil
}

// DuplicateRequest copies a request (with assertions) next to the original.
func (s *Store) DuplicateRequest(ctx context.Context, id string) (domain.Request, error) {
	r, err := s.GetRequest(ctx, id)
	if err != nil {
		return r, err
	}
	r.ID = ""
	r.Name = copyName(r.Name)
	r.CreatedAt = time.Time{}
	for i := range r.Assertions {
		r.Assertions[i].ID = ""
	}
	return s.SaveRequest(ctx, r)
}

func copyName(name string) string {
	n := name + " (copy)"
	if len(n) > domain.MaxNameLength {
		n = n[:domain.MaxNameLength]
	}
	return n
}

func (s *Store) DeleteRequest(ctx context.Context, id string) error {
	return s.execOne(ctx, `DELETE FROM requests WHERE id = ?`, id)
}
