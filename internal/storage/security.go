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
