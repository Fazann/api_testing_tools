package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"api-test-kits/internal/domain"
)

// GetEnvComparison returns the collection's comparison settings, or the defaults.
func (s *Store) GetEnvComparison(ctx context.Context, collectionID string) (domain.EnvComparison, error) {
	c := domain.DefaultEnvComparison(collectionID)
	var cfg, updated string
	err := s.db.QueryRowContext(ctx, `SELECT config_json, updated_at FROM environment_comparisons WHERE collection_id = ?`,
		collectionID).Scan(&cfg, &updated)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return c, err
	}
	if err == nil {
		unmarshalLenient(cfg, &c)
		c.CollectionID = collectionID
		c.UpdatedAt = parseTS(updated)
	}
	c.Normalize()
	return c, nil
}

// SaveEnvComparison stores c for c.CollectionID.
func (s *Store) SaveEnvComparison(ctx context.Context, c domain.EnvComparison) (domain.EnvComparison, error) {
	c.Normalize()
	if err := c.Validate(); err != nil {
		return c, err
	}
	if _, err := s.GetCollection(ctx, c.CollectionID); err != nil {
		return c, err
	}
	c.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO environment_comparisons (collection_id, config_json, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(collection_id) DO UPDATE SET config_json = excluded.config_json, updated_at = excluded.updated_at`,
		c.CollectionID, mustJSON(c), ts(c.UpdatedAt))
	return c, err
}
