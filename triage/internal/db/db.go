package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the single SQLite connection used by every CLI invocation.
type DB struct {
	*sql.DB
	Path string
}

// Open opens (and creates) the database, applies the schema, and returns it.
// DefaultPath is /data/triage.sqlite inside the container.
func Open(path string) (*DB, error) {
	if strings.TrimSpace(path) == "" {
		path = os.Getenv("TRIAGE_DB")
	}
	if strings.TrimSpace(path) == "" {
		path = "/data/triage.sqlite"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// modernc's driver is pure Go; one connection keeps SQLite's write model
	// simple and avoids two connections fighting over the write lock.
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=10000",
		"PRAGMA temp_store=MEMORY",
	} {
		if _, err := sqlDB.Exec(pragma); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}

	var jsonOK int
	if err := sqlDB.QueryRow(`SELECT json_valid('{}')`).Scan(&jsonOK); err != nil || jsonOK != 1 {
		sqlDB.Close()
		return nil, fmt.Errorf("SQLite JSON1 is not available: %v", err)
	}
	if _, err := sqlDB.Exec(schemaSQL); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO meta(key,value) VALUES('schema_version',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, schemaVersion); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("record schema version: %w", err)
	}
	if _, err := sqlDB.Exec(fmt.Sprintf("PRAGMA user_version=%s", schemaVersion)); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set user_version: %w", err)
	}

	return &DB{DB: sqlDB, Path: path}, nil
}

// EnsureRepo returns the id for slug, inserting the row if needed.
func (d *DB) EnsureRepo(slug string) (int64, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	owner, name, ok := strings.Cut(slug, "/")
	if !ok || owner == "" || name == "" {
		return 0, fmt.Errorf("repository must be owner/name, got %q", slug)
	}
	if _, err := d.Exec(`INSERT INTO repos(owner,name,slug) VALUES(?,?,?)
		ON CONFLICT(slug) DO NOTHING`, owner, name, slug); err != nil {
		return 0, err
	}
	var id int64
	if err := d.QueryRow(`SELECT id FROM repos WHERE slug=?`, slug).Scan(&id); err != nil {
		return 0, err
	}
	if _, err := d.Exec(`INSERT INTO sync_state(repo_id,phase) VALUES(?, 'done')
		ON CONFLICT(repo_id) DO NOTHING`, id); err != nil {
		return 0, err
	}
	return id, nil
}

// Refs returns every configured repository slug.
func (d *DB) Repos() ([]string, error) {
	rows, err := d.Query(`SELECT slug FROM repos ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RefreshIssueFTS rebuilds the FTS row for one issue.
func (d *DB) RefreshIssueFTS(issueID int64) error {
	_, err := d.Exec(`DELETE FROM issue_fts WHERE rowid=?`, issueID)
	if err != nil {
		return err
	}
	_, err = d.Exec(`
		INSERT INTO issue_fts(rowid,title,body,comments,repo_slug,number)
		SELECT i.id, i.title, i.body,
		       COALESCE((SELECT group_concat(c.body, char(10)) FROM comments c WHERE c.issue_id=i.id), ''),
		       r.slug, i.number
		FROM issues i JOIN repos r ON r.id=i.repo_id
		WHERE i.id=?`, issueID)
	return err
}

// RefreshAllFTS rebuilds the whole index. Used after a full sync or repair.
func (d *DB) RefreshAllFTS() error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM issue_fts`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO issue_fts(rowid,title,body,comments,repo_slug,number)
		SELECT i.id, i.title, i.body,
		       COALESCE((SELECT group_concat(c.body, char(10)) FROM comments c WHERE c.issue_id=i.id), ''),
		       r.slug, i.number
		FROM issues i JOIN repos r ON r.id=i.repo_id`); err != nil {
		return err
	}
	return tx.Commit()
}

// Now returns the UTC timestamp format used throughout the database.
func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// Bool converts a bool to SQLite's 0/1.
func Bool(b bool) int {
	if b {
		return 1
	}
	return 0
}
