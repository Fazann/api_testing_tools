package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"api-test-kits/internal/domain"
)

// GetContract returns the collection's contract, or nil when none is attached.
func (s *Store) GetContract(ctx context.Context, collectionID string) (*domain.Contract, error) {
	c := domain.Contract{CollectionID: collectionID}
	var spec, updated string
	err := s.db.QueryRowContext(ctx, `SELECT spec_json, source_name, enabled, allow_undocumented, updated_at
		FROM collection_contracts WHERE collection_id = ?`, collectionID).
		Scan(&spec, &c.SourceName, &c.Enabled, &c.AllowUndocumented, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Spec = []byte(spec)
	c.UpdatedAt = parseTS(updated)
	return &c, nil
}

// SaveContract attaches or replaces the contract of c.CollectionID. The spec
// must already be validated.
func (s *Store) SaveContract(ctx context.Context, c domain.Contract) (domain.Contract, error) {
	if _, err := s.GetCollection(ctx, c.CollectionID); err != nil {
		return c, err
	}
	c.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO collection_contracts
		(collection_id, spec_json, source_name, enabled, allow_undocumented, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(collection_id) DO UPDATE SET spec_json = excluded.spec_json, source_name = excluded.source_name,
		enabled = excluded.enabled, allow_undocumented = excluded.allow_undocumented, updated_at = excluded.updated_at`,
		c.CollectionID, string(c.Spec), c.SourceName, c.Enabled, c.AllowUndocumented, ts(c.UpdatedAt))
	return c, err
}

// SaveContractOptions changes the options of an attached contract.
func (s *Store) SaveContractOptions(ctx context.Context, collectionID string, enabled, allowUndocumented bool) error {
	return s.execOne(ctx, `UPDATE collection_contracts SET enabled = ?, allow_undocumented = ?, updated_at = ?
		WHERE collection_id = ?`, enabled, allowUndocumented, ts(time.Now()), collectionID)
}

// DeleteContract detaches the collection's contract.
func (s *Store) DeleteContract(ctx context.Context, collectionID string) error {
	return s.execOne(ctx, `DELETE FROM collection_contracts WHERE collection_id = ?`, collectionID)
}

func (s *Store) copyContract(ctx context.Context, fromID, toID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO collection_contracts
		(collection_id, spec_json, source_name, enabled, allow_undocumented, updated_at)
		SELECT ?, spec_json, source_name, enabled, allow_undocumented, ? FROM collection_contracts WHERE collection_id = ?`,
		toID, ts(time.Now()), fromID)
	return err
}
