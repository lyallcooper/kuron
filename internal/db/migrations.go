package db

import (
	"fmt"
)

// Migrate runs all database migrations
func (db *DB) Migrate() error {
	// Create migrations table if not exists
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	row := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	// Run migrations
	migrations := []struct {
		version int
		sql     string
	}{
		{1, migration001},
		{2, migration002},
		{3, migration003},
		{4, migration004},
	}

	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to run migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", m.version, err)
		}
	}

	return nil
}

const migration001 = `
-- Paths available for scanning (env + user-added)
CREATE TABLE scan_paths (
    id INTEGER PRIMARY KEY,
    path TEXT UNIQUE NOT NULL,
    from_env BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Scan configurations
CREATE TABLE scan_configs (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    paths TEXT NOT NULL DEFAULT '[]',
    min_size INTEGER DEFAULT 1,
    max_size INTEGER,
    include_patterns TEXT DEFAULT '[]',
    exclude_patterns TEXT DEFAULT '[]',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Scheduled jobs
CREATE TABLE scheduled_jobs (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    scan_config_id INTEGER,
    cron_expression TEXT NOT NULL,
    action TEXT NOT NULL DEFAULT 'scan',
    enabled BOOLEAN DEFAULT 1,
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Scan runs (history)
CREATE TABLE scan_runs (
    id INTEGER PRIMARY KEY,
    scan_config_id INTEGER,
    scheduled_job_id INTEGER,
    status TEXT NOT NULL DEFAULT 'running',
    started_at DATETIME NOT NULL,
    completed_at DATETIME,
    files_scanned INTEGER DEFAULT 0,
    bytes_scanned INTEGER DEFAULT 0,
    duplicate_groups INTEGER DEFAULT 0,
    duplicate_files INTEGER DEFAULT 0,
    wasted_bytes INTEGER DEFAULT 0,
    error_message TEXT
);

CREATE INDEX idx_scan_runs_status ON scan_runs(status);
CREATE INDEX idx_scan_runs_started_at ON scan_runs(started_at);

-- Duplicate groups (stored for review before action)
CREATE TABLE duplicate_groups (
    id INTEGER PRIMARY KEY,
    scan_run_id INTEGER,
    file_hash TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    file_count INTEGER NOT NULL,
    wasted_bytes INTEGER NOT NULL,
    status TEXT DEFAULT 'pending',
    files TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX idx_duplicate_groups_scan_run_id ON duplicate_groups(scan_run_id);
CREATE INDEX idx_duplicate_groups_status ON duplicate_groups(status);

-- Actions taken (audit log)
CREATE TABLE actions (
    id INTEGER PRIMARY KEY,
    scan_run_id INTEGER,
    action_type TEXT NOT NULL,
    groups_processed INTEGER DEFAULT 0,
    files_processed INTEGER DEFAULT 0,
    bytes_saved INTEGER DEFAULT 0,
    dry_run BOOLEAN DEFAULT 0,
    started_at DATETIME NOT NULL,
    completed_at DATETIME,
    status TEXT NOT NULL DEFAULT 'running',
    error_message TEXT
);

CREATE INDEX idx_actions_scan_run_id ON actions(scan_run_id);
CREATE INDEX idx_actions_started_at ON actions(started_at);

-- Daily aggregate stats
CREATE TABLE daily_stats (
    date DATE PRIMARY KEY,
    scans_run INTEGER DEFAULT 0,
    groups_found INTEGER DEFAULT 0,
    files_found INTEGER DEFAULT 0,
    bytes_wasted INTEGER DEFAULT 0,
    bytes_saved INTEGER DEFAULT 0
);
`

const migration002 = `
-- Remove name column from scheduled_jobs (redundant with scan_config name)
ALTER TABLE scheduled_jobs DROP COLUMN name;
`

const migration003 = `
-- Simplify architecture: merge scan configs into jobs, remove global paths
-- This is a breaking change - all existing jobs/configs will be lost

-- Drop old tables
DROP TABLE IF EXISTS scheduled_jobs;
DROP TABLE IF EXISTS scan_configs;
DROP TABLE IF EXISTS scan_paths;

-- Create new self-contained scheduled_jobs table
CREATE TABLE scheduled_jobs (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    paths TEXT NOT NULL DEFAULT '[]',
    min_size INTEGER DEFAULT 0,
    max_size INTEGER,
    include_patterns TEXT DEFAULT '[]',
    exclude_patterns TEXT DEFAULT '[]',
    cron_expression TEXT NOT NULL,
    action TEXT NOT NULL DEFAULT 'scan',
    enabled BOOLEAN DEFAULT 1,
    last_run_at DATETIME,
    next_run_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- App settings (key-value store)
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Default retention days
INSERT INTO settings (key, value) VALUES ('retention_days', '30');
`

const migration004 = `
-- Add paths column to scan_runs to store scanned paths for all scans (including quick scans)
ALTER TABLE scan_runs ADD COLUMN paths TEXT NOT NULL DEFAULT '[]';
`
