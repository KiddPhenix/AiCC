package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func OpenSQLite(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS solutions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	problem TEXT NOT NULL DEFAULT '',
	solution_text TEXT NOT NULL,
	raw_content TEXT NOT NULL DEFAULT '',
	efficacy_source TEXT NOT NULL,
	status TEXT NOT NULL,
	source_type TEXT NOT NULL DEFAULT '',
	source_name TEXT NOT NULL DEFAULT '',
	source_ref TEXT NOT NULL DEFAULT '',
	raw_content_ref TEXT NOT NULL DEFAULT '',
	dedup_key TEXT NOT NULL UNIQUE,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS solution_tags (
	solution_id INTEGER NOT NULL,
	tag TEXT NOT NULL,
	PRIMARY KEY (solution_id, tag),
	FOREIGN KEY (solution_id) REFERENCES solutions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS captures (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	content TEXT NOT NULL,
	content_hash TEXT NOT NULL UNIQUE,
	content_type TEXT NOT NULL DEFAULT '',
	char_count INTEGER NOT NULL DEFAULT 0,
	line_count INTEGER NOT NULL DEFAULT 0,
	source TEXT NOT NULL DEFAULT 'clipboard',
	captured_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS capture_insights (
	capture_id INTEGER PRIMARY KEY,
	title TEXT NOT NULL DEFAULT '',
	summary TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL DEFAULT '',
	problem TEXT NOT NULL DEFAULT '',
	solution_text TEXT NOT NULL DEFAULT '',
	occurred_at TEXT NOT NULL DEFAULT '',
	source_name TEXT NOT NULL DEFAULT '',
	source_ref TEXT NOT NULL DEFAULT '',
	suggested_status TEXT NOT NULL DEFAULT 'draft',
	confidence REAL NOT NULL DEFAULT 0,
	tags_json TEXT NOT NULL DEFAULT '[]',
	model TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL DEFAULT '',
	raw_json TEXT NOT NULL DEFAULT '',
	analyzed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (capture_id) REFERENCES captures(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_solutions_status ON solutions(status);
CREATE INDEX IF NOT EXISTS idx_solutions_efficacy_source ON solutions(efficacy_source);
CREATE INDEX IF NOT EXISTS idx_solution_tags_tag ON solution_tags(tag);
CREATE INDEX IF NOT EXISTS idx_captures_captured_at ON captures(captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_capture_insights_category ON capture_insights(category);
`

	if _, err := db.Exec(schema); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE solutions ADD COLUMN raw_content TEXT NOT NULL DEFAULT ''`); err != nil && !isDuplicateColumnError(err) {
		return err
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	return containsFold(err.Error(), "duplicate column name")
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
