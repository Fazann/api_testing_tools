// Package storage persists collections, environments, baselines and run
// history in a local SQLite database.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: no cgo, easy Windows/Linux builds
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// Store is the SQLite-backed repository.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (creating if needed) the database at path and applies migrations.
// Use ":memory:" for tests.
func Open(path string) (*Store, error) {
	dsn := "file::memory:"
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
		// Create the file with owner-only permissions before SQLite opens it.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create database file: %w", err)
		}
		f.Close()
		dsn = "file:" + filepath.ToSlash(path)
	}
	// WAL + synchronous=NORMAL survives application crashes without
	// corrupting the database; foreign keys enforce cascades.
	dsn += "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		// Every connection to :memory: is a separate database.
		db.SetMaxOpenConns(1)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the database file path.
func (s *Store) Path() string { return s.path }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		version, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: name must start with a number", e.Name())
		}
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		script, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		err = s.tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(script)); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, ts(time.Now()))
			return err
		})
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", e.Name(), err)
		}
	}
	return nil
}

// tx runs fn in a transaction, rolling back on error or panic.
func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// DataInfo describes local storage for the data-management screen.
type DataInfo struct {
	Path         string `json:"path"`
	SizeBytes    int64  `json:"sizeBytes"`
	Collections  int    `json:"collections"`
	Requests     int    `json:"requests"`
	Environments int    `json:"environments"`
	Runs         int    `json:"runs"`
	Baselines    int    `json:"baselines"`
}

func (s *Store) DataInfo(ctx context.Context) (DataInfo, error) {
	info := DataInfo{Path: s.path}
	for table, dst := range map[string]*int{
		"collections": &info.Collections, "requests": &info.Requests,
		"environments": &info.Environments, "runs": &info.Runs, "baselines": &info.Baselines,
	} {
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(dst); err != nil {
			return info, err
		}
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if st, err := os.Stat(s.path + suffix); err == nil {
			info.SizeBytes += st.Size()
		}
	}
	return info, nil
}

// Vacuum reclaims space after large deletions.
func (s *Store) Vacuum(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `VACUUM`)
	return err
}

// DeleteAllData removes every user record but keeps the schema.
func (s *Store) DeleteAllData(ctx context.Context) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, t := range []string{"run_results", "runs", "race_tests", "permission_matrices", "environment_comparisons", "baselines", "assertions", "requests", "folders", "collections", "environments", "settings"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+t); err != nil {
				return err
			}
		}
		return nil
	})
}

const timeLayout = time.RFC3339Nano

func ts(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(timeLayout, s)
	return t
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// unmarshalLenient decodes stored JSON, leaving dst at its zero value if the
// stored text is damaged so that one bad row cannot crash the app.
func unmarshalLenient(s string, dst any) {
	if s == "" {
		return
	}
	_ = json.Unmarshal([]byte(s), dst)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
