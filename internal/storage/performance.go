package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"api-test-kits/internal/domain"
)

// GetPerformanceCheck returns the request's check, or the default one.
func (s *Store) GetPerformanceCheck(ctx context.Context, requestID string) (domain.PerformanceCheck, error) {
	c := domain.DefaultPerformanceCheck(requestID)
	var cfg, updated string
	err := s.db.QueryRowContext(ctx, `SELECT config_json, updated_at FROM performance_checks WHERE request_id = ?`, requestID).
		Scan(&cfg, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	unmarshalLenient(cfg, &c)
	c.RequestID = requestID
	c.UpdatedAt = parseTS(updated)
	c.Normalize()
	return c, nil
}

// SavePerformanceCheck stores the check for c.RequestID.
func (s *Store) SavePerformanceCheck(ctx context.Context, c domain.PerformanceCheck) (domain.PerformanceCheck, error) {
	c.Normalize()
	if err := c.Validate(); err != nil {
		return c, err
	}
	if _, err := s.GetRequest(ctx, c.RequestID); err != nil {
		return c, err
	}
	c.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO performance_checks (request_id, config_json, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(request_id) DO UPDATE SET config_json = excluded.config_json, updated_at = excluded.updated_at`,
		c.RequestID, mustJSON(c), ts(c.UpdatedAt))
	return c, err
}

// responseTimeFilter selects results that count for trends: ordinary runs
// (not race tests or permission matrices), in the given environment, that got
// a response. Runs saved before environment IDs were recorded match by name.
const responseTimeFilter = `r.kind = 'run' AND rr.status IN ('passed', 'failed') AND rr.response_summary_json IS NOT NULL
	AND (r.environment_id = ? OR (r.environment_id IS NULL AND r.environment_name = ?))`

// ResponseTimes returns up to limit of the request's latest responses in an
// environment, oldest first.
func (s *Store) ResponseTimes(ctx context.Context, requestID, envID, envName string, limit int) ([]domain.ResponseTimePoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT rr.run_id, rr.id, rr.created_at, rr.duration_ms, rr.status,
			IFNULL(json_extract(rr.response_summary_json, '$.statusCode'), 0)
		FROM run_results rr JOIN runs r ON r.id = rr.run_id
		WHERE rr.request_id = ? AND `+responseTimeFilter+`
		ORDER BY rr.created_at DESC, rr.id DESC LIMIT ?`, requestID, envID, envName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ResponseTimePoint{}
	for rows.Next() {
		p, err := scanPoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	reverse(out)
	return out, rows.Err()
}

// CollectionResponseTimes returns up to perRequest of the latest responses of
// every request in the collection, keyed by request ID, oldest first.
func (s *Store) CollectionResponseTimes(ctx context.Context, collectionID, envID, envName string, perRequest int) (map[string][]domain.ResponseTimePoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT request_id, run_id, id, created_at, duration_ms, status, status_code FROM (
			SELECT rr.request_id, rr.run_id, rr.id, rr.created_at, rr.duration_ms, rr.status,
				IFNULL(json_extract(rr.response_summary_json, '$.statusCode'), 0) AS status_code,
				ROW_NUMBER() OVER (PARTITION BY rr.request_id ORDER BY rr.created_at DESC, rr.id DESC) AS n
			FROM run_results rr JOIN runs r ON r.id = rr.run_id
			WHERE rr.request_id IN (SELECT id FROM requests WHERE collection_id = ?) AND `+responseTimeFilter+`
		) WHERE n <= ? ORDER BY request_id, created_at, id`, collectionID, envID, envName, perRequest)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]domain.ResponseTimePoint{}
	for rows.Next() {
		var reqID string
		p, err := scanPoint(rows, &reqID)
		if err != nil {
			return nil, err
		}
		out[reqID] = append(out[reqID], p)
	}
	return out, rows.Err()
}

// ListPerformanceChecks returns the stored checks of a collection's requests.
func (s *Store) ListPerformanceChecks(ctx context.Context, collectionID string) (map[string]domain.PerformanceCheck, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.request_id, p.config_json FROM performance_checks p
		JOIN requests q ON q.id = p.request_id WHERE q.collection_id = ?`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]domain.PerformanceCheck{}
	for rows.Next() {
		var id, cfg string
		if err := rows.Scan(&id, &cfg); err != nil {
			return nil, err
		}
		c := domain.DefaultPerformanceCheck(id)
		unmarshalLenient(cfg, &c)
		c.RequestID = id
		c.Normalize()
		out[id] = c
	}
	return out, rows.Err()
}

func scanPoint(r scanner, prefix ...any) (domain.ResponseTimePoint, error) {
	var p domain.ResponseTimePoint
	var at string
	dest := append(prefix, &p.RunID, &p.ResultID, &at, &p.DurationMs, &p.Status, &p.StatusCode)
	if err := r.Scan(dest...); err != nil {
		return p, err
	}
	p.At = parseTS(at)
	return p, nil
}

func reverse[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
