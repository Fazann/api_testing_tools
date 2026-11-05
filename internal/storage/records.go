package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"api-test-kits/internal/domain"
)

// ---- environments ----

func (s *Store) ListEnvironments(ctx context.Context) ([]domain.Environment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, variables_json, protected, created_at, updated_at FROM environments ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Environment{}
	for rows.Next() {
		e, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetEnvironment(ctx context.Context, id string) (domain.Environment, error) {
	e, err := scanEnvironment(s.db.QueryRowContext(ctx, `SELECT id, name, variables_json, protected, created_at, updated_at FROM environments WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

func scanEnvironment(r scanner) (domain.Environment, error) {
	var e domain.Environment
	var vars, created, updated string
	var protected int
	if err := r.Scan(&e.ID, &e.Name, &vars, &protected, &created, &updated); err != nil {
		return e, err
	}
	unmarshalLenient(vars, &e.Variables)
	if e.Variables == nil {
		e.Variables = []domain.Variable{}
	}
	e.Protected = protected == 1
	e.CreatedAt, e.UpdatedAt = parseTS(created), parseTS(updated)
	return e, nil
}

// SaveEnvironment inserts or updates an environment. Secret values are stored
// in the local database file, which is created with owner-only permissions;
// see docs/security.md for the limitations of this approach.
func (s *Store) SaveEnvironment(ctx context.Context, e domain.Environment) (domain.Environment, error) {
	// Drop fully empty rows the editor leaves behind.
	vars := make([]domain.Variable, 0, len(e.Variables))
	for _, v := range e.Variables {
		v.Key = strings.TrimSpace(v.Key)
		if v.Key == "" && v.Value == "" {
			continue
		}
		vars = append(vars, v)
	}
	e.Variables = vars
	if err := e.Validate(); err != nil {
		return e, err
	}
	now := time.Now().UTC()
	if e.ID == "" {
		e.ID = uuid.NewString()
		e.CreatedAt = now
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO environments (id, name, variables_json, protected, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, variables_json = excluded.variables_json,
			protected = excluded.protected, updated_at = excluded.updated_at`,
		e.ID, e.Name, mustJSON(e.Variables), boolInt(e.Protected), ts(e.CreatedAt), ts(e.UpdatedAt))
	return e, err
}

func (s *Store) DeleteEnvironment(ctx context.Context, id string) error {
	return s.execOne(ctx, `DELETE FROM environments WHERE id = ?`, id)
}

// ---- baselines ----

const baselineColumns = `id, request_id, IFNULL(environment_id, ''), mode, response_snapshot, selected_paths_json, ignored_paths_json, tolerance, created_at, updated_at`

// GetBaseline returns the baseline for a request in an environment
// (environmentID may be empty for "no environment").
func (s *Store) GetBaseline(ctx context.Context, requestID, environmentID string) (*domain.Baseline, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+baselineColumns+` FROM baselines WHERE request_id = ? AND IFNULL(environment_id, '') = ?`, requestID, environmentID)
	var b domain.Baseline
	var snap, selected, ignored, created, updated, mode string
	err := row.Scan(&b.ID, &b.RequestID, &b.EnvironmentID, &mode, &snap, &selected, &ignored, &b.Tolerance, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.Snapshot = json.RawMessage(snap)
	b.Mode = domain.BaselineMode(mode)
	unmarshalLenient(selected, &b.SelectedPaths)
	unmarshalLenient(ignored, &b.IgnoredPaths)
	b.SelectedPaths, b.IgnoredPaths = nonNil(b.SelectedPaths), nonNil(b.IgnoredPaths)
	b.CreatedAt, b.UpdatedAt = parseTS(created), parseTS(updated)
	return &b, nil
}

// SaveBaseline replaces the baseline for (request, environment). It must only
// be called after explicit user approval.
func (s *Store) SaveBaseline(ctx context.Context, b domain.Baseline) (domain.Baseline, error) {
	if !json.Valid(b.Snapshot) {
		return b, errors.New("baseline snapshot must be valid JSON")
	}
	now := time.Now().UTC()
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var existingID, created string
		err := tx.QueryRowContext(ctx, `SELECT id, created_at FROM baselines WHERE request_id = ? AND IFNULL(environment_id, '') = ?`,
			b.RequestID, b.EnvironmentID).Scan(&existingID, &created)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			b.ID = uuid.NewString()
			b.CreatedAt = now
		case err != nil:
			return err
		default:
			b.ID = existingID
			b.CreatedAt = parseTS(created)
		}
		b.UpdatedAt = now
		if b.Mode == "" {
			b.Mode = domain.BaselineValues
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO baselines (id, request_id, environment_id, mode, response_snapshot, selected_paths_json, ignored_paths_json, tolerance, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET mode = excluded.mode, response_snapshot = excluded.response_snapshot, selected_paths_json = excluded.selected_paths_json,
				ignored_paths_json = excluded.ignored_paths_json, tolerance = excluded.tolerance, updated_at = excluded.updated_at`,
			b.ID, b.RequestID, nullable(b.EnvironmentID), string(b.Mode), string(b.Snapshot), mustJSON(nonNil(b.SelectedPaths)), mustJSON(nonNil(b.IgnoredPaths)),
			b.Tolerance, ts(b.CreatedAt), ts(b.UpdatedAt))
		return err
	})
	return b, err
}

// UpdateBaselineRules changes ignored paths and tolerance without touching the snapshot.
func (s *Store) UpdateBaselineRules(ctx context.Context, id string, ignored []string, tolerance float64) error {
	return s.execOne(ctx, `UPDATE baselines SET ignored_paths_json = ?, tolerance = ?, updated_at = ? WHERE id = ?`,
		mustJSON(nonNil(ignored)), tolerance, ts(time.Now()), id)
}

func (s *Store) DeleteBaseline(ctx context.Context, id string) error {
	return s.execOne(ctx, `DELETE FROM baselines WHERE id = ?`, id)
}

// ---- runs ----

// SaveRun stores a run and its results in one transaction, truncating stored
// bodies to bodyLimit bytes, then prunes history beyond retain runs.
func (s *Store) SaveRun(ctx context.Context, run domain.Run, bodyLimit, retain int) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var finished any
		if run.FinishedAt != nil {
			finished = ts(*run.FinishedAt)
		}
		kind := run.Kind
		if kind == "" {
			kind = domain.RunKindNormal
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO runs (id, collection_id, collection_name, environment_name, environment_id, kind,
			status, started_at, finished_at, passed_count, failed_count, error_count, duration_ms)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			run.ID, nullable(run.CollectionID), run.CollectionName, run.EnvironmentName, run.EnvironmentID, kind,
			string(run.Status), ts(run.StartedAt), finished, run.PassedCount, run.FailedCount, run.ErrorCount, run.DurationMs)
		if err != nil {
			return err
		}
		for i, res := range run.Results {
			var resp any
			if res.Response != nil {
				r := *res.Response
				if bodyLimit >= 0 && len(r.Body) > bodyLimit {
					r.Body = r.Body[:bodyLimit]
					r.Truncated = true
				}
				resp = mustJSON(r)
			}
			var reg, perf any
			if res.Regression != nil {
				reg = mustJSON(res.Regression)
			}
			if res.Performance != nil {
				perf = mustJSON(res.Performance)
			}
			// The request may have been deleted while the run was in progress.
			var reqID any
			if res.RequestID != "" {
				var n int
				if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM requests WHERE id = ?`, res.RequestID).Scan(&n); err != nil {
					return err
				}
				if n > 0 {
					reqID = res.RequestID
				}
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO run_results (id, run_id, request_id, request_summary_json, response_summary_json,
				assertion_results_json, regression_json, performance_json, logs_json, error, duration_ms, status, sort_order, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				res.ID, run.ID, reqID, mustJSON(res.Request), resp, mustJSON(res.AssertionResults), reg, perf, mustJSON(nonNilLogs(res.Logs)),
				res.Error, res.DurationMs, string(res.Status), i, ts(res.CreatedAt))
			if err != nil {
				return err
			}
		}
		if retain > 0 {
			_, err = tx.ExecContext(ctx, `DELETE FROM runs WHERE id NOT IN (SELECT id FROM runs ORDER BY started_at DESC LIMIT ?)`, retain)
		}
		return err
	})
}

// ListRuns returns run summaries (without results), newest first.
func (s *Store) ListRuns(ctx context.Context, f domain.RunFilter) ([]domain.Run, error) {
	var where []string
	var args []any
	if f.CollectionID != "" {
		where = append(where, "collection_id = ?")
		args = append(args, f.CollectionID)
	}
	if f.RequestID != "" {
		where = append(where, "id IN (SELECT run_id FROM run_results WHERE request_id = ?)")
		args = append(args, f.RequestID)
	}
	if f.From != nil {
		where = append(where, "started_at >= ?")
		args = append(args, ts(*f.From))
	}
	if f.To != nil {
		where = append(where, "started_at <= ?")
		args = append(args, ts(*f.To))
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	q := `SELECT id, IFNULL(collection_id, ''), collection_name, environment_name, status, started_at, IFNULL(finished_at, ''),
		passed_count, failed_count, error_count, duration_ms FROM runs`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q += " ORDER BY started_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, max(f.Offset, 0))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Run{}
	for rows.Next() {
		var r domain.Run
		var started, finished string
		if err := rows.Scan(&r.ID, &r.CollectionID, &r.CollectionName, &r.EnvironmentName, &r.Status, &started, &finished,
			&r.PassedCount, &r.FailedCount, &r.ErrorCount, &r.DurationMs); err != nil {
			return nil, err
		}
		r.StartedAt = parseTS(started)
		if finished != "" {
			t := parseTS(finished)
			r.FinishedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRun returns a run with all of its results.
func (s *Store) GetRun(ctx context.Context, id string) (domain.Run, error) {
	runs, err := s.listRunsByID(ctx, id)
	if err != nil {
		return domain.Run{}, err
	}
	if len(runs) == 0 {
		return domain.Run{}, ErrNotFound
	}
	run := runs[0]
	rows, err := s.db.QueryContext(ctx, `SELECT id, IFNULL(request_id, ''), request_summary_json, IFNULL(response_summary_json, ''),
		assertion_results_json, IFNULL(regression_json, ''), IFNULL(performance_json, ''), logs_json, error, duration_ms, status, created_at
		FROM run_results WHERE run_id = ? ORDER BY sort_order`, id)
	if err != nil {
		return run, err
	}
	defer rows.Close()
	run.Results = []domain.RunResult{}
	for rows.Next() {
		var r domain.RunResult
		var reqJSON, respJSON, assertJSON, regJSON, perfJSON, logsJSON, created string
		if err := rows.Scan(&r.ID, &r.RequestID, &reqJSON, &respJSON, &assertJSON, &regJSON, &perfJSON, &logsJSON, &r.Error, &r.DurationMs, &r.Status, &created); err != nil {
			return run, err
		}
		r.RunID = id
		unmarshalLenient(reqJSON, &r.Request)
		if respJSON != "" {
			r.Response = &domain.ResponseSummary{}
			unmarshalLenient(respJSON, r.Response)
		}
		unmarshalLenient(assertJSON, &r.AssertionResults)
		if r.AssertionResults == nil {
			r.AssertionResults = []domain.AssertionResult{}
		}
		unmarshalLenient(logsJSON, &r.Logs)
		if regJSON != "" {
			r.Regression = &domain.RegressionResult{}
			unmarshalLenient(regJSON, r.Regression)
		}
		if perfJSON != "" {
			r.Performance = &domain.PerformanceResult{}
			unmarshalLenient(perfJSON, r.Performance)
		}
		r.CreatedAt = parseTS(created)
		run.Results = append(run.Results, r)
	}
	return run, rows.Err()
}

func (s *Store) listRunsByID(ctx context.Context, id string) ([]domain.Run, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, IFNULL(collection_id, ''), collection_name, environment_name, IFNULL(environment_id, ''),
		kind, status, started_at, IFNULL(finished_at, ''), passed_count, failed_count, error_count, duration_ms FROM runs WHERE id = ?`, id)
	var r domain.Run
	var started, finished string
	err := row.Scan(&r.ID, &r.CollectionID, &r.CollectionName, &r.EnvironmentName, &r.EnvironmentID, &r.Kind, &r.Status, &started, &finished,
		&r.PassedCount, &r.FailedCount, &r.ErrorCount, &r.DurationMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.StartedAt = parseTS(started)
	if finished != "" {
		t := parseTS(finished)
		r.FinishedAt = &t
	}
	return []domain.Run{r}, nil
}

// PreviousResult returns the most recent stored result for a request before
// the given run, used for "compare with previous run".
func (s *Store) PreviousResult(ctx context.Context, requestID, beforeRunID string) (*domain.RunResult, error) {
	var runID string
	err := s.db.QueryRowContext(ctx, `SELECT rr.run_id FROM run_results rr JOIN runs r ON r.id = rr.run_id
		WHERE rr.request_id = ? AND rr.run_id <> ? ORDER BY r.started_at DESC LIMIT 1`, requestID, beforeRunID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	for i := range run.Results {
		if run.Results[i].RequestID == requestID {
			return &run.Results[i], nil
		}
	}
	return nil, nil
}

func (s *Store) DeleteRun(ctx context.Context, id string) error {
	return s.execOne(ctx, `DELETE FROM runs WHERE id = ?`, id)
}

// ClearHistory deletes all runs, optionally only those older than before.
func (s *Store) ClearHistory(ctx context.Context, before *time.Time) (int64, error) {
	var res sql.Result
	var err error
	if before != nil {
		res, err = s.db.ExecContext(ctx, `DELETE FROM runs WHERE started_at < ?`, ts(*before))
	} else {
		res, err = s.db.ExecContext(ctx, `DELETE FROM runs`)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---- settings ----

const settingsKey = "app"

// GetSettings returns stored settings merged over defaults.
func (s *Store) GetSettings(ctx context.Context) (domain.Settings, error) {
	st := domain.DefaultSettings()
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, settingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	unmarshalLenient(raw, &st)
	if st.Validate() != nil {
		return domain.DefaultSettings(), nil
	}
	return st, nil
}

func (s *Store) SaveSettings(ctx context.Context, st domain.Settings) error {
	if err := st.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		settingsKey, mustJSON(st))
	return err
}

func nonNilLogs(l []domain.ScriptLog) []domain.ScriptLog {
	if l == nil {
		return []domain.ScriptLog{}
	}
	return l
}

// NamedBaseline is a baseline with the name of its environment ("" for none).
type NamedBaseline struct {
	Baseline        domain.Baseline
	EnvironmentName string
}

// ListCollectionBaselines returns every baseline of the collection's requests.
func (s *Store) ListCollectionBaselines(ctx context.Context, collectionID string) ([]NamedBaseline, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.id, b.request_id, IFNULL(b.environment_id, ''), b.mode, b.response_snapshot,
		b.selected_paths_json, b.ignored_paths_json, b.tolerance, b.created_at, b.updated_at, IFNULL(e.name, '')
		FROM baselines b JOIN requests r ON r.id = b.request_id LEFT JOIN environments e ON e.id = b.environment_id
		WHERE r.collection_id = ? ORDER BY r.sort_order, r.name, IFNULL(e.name, '')`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NamedBaseline
	for rows.Next() {
		var nb NamedBaseline
		b := &nb.Baseline
		var snap, selected, ignored, created, updated, mode string
		if err := rows.Scan(&b.ID, &b.RequestID, &b.EnvironmentID, &mode, &snap, &selected, &ignored, &b.Tolerance,
			&created, &updated, &nb.EnvironmentName); err != nil {
			return nil, err
		}
		b.Snapshot = json.RawMessage(snap)
		b.Mode = domain.BaselineMode(mode)
		unmarshalLenient(selected, &b.SelectedPaths)
		unmarshalLenient(ignored, &b.IgnoredPaths)
		b.SelectedPaths, b.IgnoredPaths = nonNil(b.SelectedPaths), nonNil(b.IgnoredPaths)
		b.CreatedAt, b.UpdatedAt = parseTS(created), parseTS(updated)
		out = append(out, nb)
	}
	return out, rows.Err()
}
