package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"api-test-kits/internal/domain"
)

// GetRaceTest returns the saved race test for a request, or nil.
func (s *Store) GetRaceTest(ctx context.Context, requestID string) (*domain.RaceTest, error) {
	var cfg, updated string
	err := s.db.QueryRowContext(ctx, `SELECT config_json, updated_at FROM race_tests WHERE request_id = ?`, requestID).Scan(&cfg, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t domain.RaceTest
	unmarshalLenient(cfg, &t)
	t.RequestID = requestID
	t.UpdatedAt = parseTS(updated)
	return &t, nil
}

// SaveRaceTest stores the race test for t.RequestID.
func (s *Store) SaveRaceTest(ctx context.Context, t domain.RaceTest) (domain.RaceTest, error) {
	t.Normalize()
	if err := t.Validate(); err != nil {
		return t, err
	}
	t.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO race_tests (request_id, config_json, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(request_id) DO UPDATE SET config_json = excluded.config_json, updated_at = excluded.updated_at`,
		t.RequestID, mustJSON(t), ts(t.UpdatedAt))
	return t, err
}

// GetPermissionMatrix returns the collection's matrix, or an empty one.
func (s *Store) GetPermissionMatrix(ctx context.Context, collectionID string) (domain.PermissionMatrix, error) {
	m := domain.PermissionMatrix{CollectionID: collectionID}
	var cfg, updated string
	err := s.db.QueryRowContext(ctx, `SELECT config_json, updated_at FROM permission_matrices WHERE collection_id = ?`, collectionID).Scan(&cfg, &updated)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return m, err
	}
	if err == nil {
		unmarshalLenient(cfg, &m)
		m.CollectionID = collectionID
		m.UpdatedAt = parseTS(updated)
	}
	m.Normalize()
	return m, nil
}

// SavePermissionMatrix stores m for m.CollectionID.
func (s *Store) SavePermissionMatrix(ctx context.Context, m domain.PermissionMatrix) (domain.PermissionMatrix, error) {
	m.Normalize()
	if err := m.Validate(); err != nil {
		return m, err
	}
	if _, err := s.GetCollection(ctx, m.CollectionID); err != nil {
		return m, err
	}
	m.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO permission_matrices (collection_id, config_json, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(collection_id) DO UPDATE SET config_json = excluded.config_json, updated_at = excluded.updated_at`,
		m.CollectionID, mustJSON(m), ts(m.UpdatedAt))
	return m, err
}

// ListCollectionRaceTests returns the saved race tests of a collection's requests.
func (s *Store) ListCollectionRaceTests(ctx context.Context, collectionID string) ([]domain.RaceTest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.request_id, t.config_json, t.updated_at FROM race_tests t
		JOIN requests r ON r.id = t.request_id WHERE r.collection_id = ? ORDER BY r.sort_order, r.name`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.RaceTest{}
	for rows.Next() {
		var id, cfg, updated string
		if err := rows.Scan(&id, &cfg, &updated); err != nil {
			return nil, err
		}
		var t domain.RaceTest
		unmarshalLenient(cfg, &t)
		t.RequestID, t.UpdatedAt = id, parseTS(updated)
		out = append(out, t)
	}
	return out, rows.Err()
}

// copySecurity copies the race tests and the permission matrix of one
// collection to its duplicate; ids maps old request IDs to new ones.
func (s *Store) copySecurity(ctx context.Context, fromID, toID string, ids map[string]string) error {
	tests, err := s.ListCollectionRaceTests(ctx, fromID)
	if err != nil {
		return err
	}
	for _, t := range tests {
		if nt, ok := t.RemapRequests(ids); ok {
			if _, err := s.SaveRaceTest(ctx, nt); err != nil {
				return err
			}
		}
	}
	m, err := s.GetPermissionMatrix(ctx, fromID)
	if err != nil || m.IsEmpty() {
		return err
	}
	nm, _ := m.RemapRequests(toID, ids)
	_, err = s.SavePermissionMatrix(ctx, nm)
	return err
}
