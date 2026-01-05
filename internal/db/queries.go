package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ScanPath queries

// CreateScanPath creates a new scan path
func (db *DB) CreateScanPath(path string, fromEnv bool) (*ScanPath, error) {
	result, err := db.Exec(
		"INSERT INTO scan_paths (path, from_env) VALUES (?, ?) ON CONFLICT(path) DO UPDATE SET from_env = ?",
		path, fromEnv, fromEnv,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return db.GetScanPath(id)
}

// GetScanPath retrieves a scan path by ID
func (db *DB) GetScanPath(id int64) (*ScanPath, error) {
	row := db.QueryRow("SELECT id, path, from_env, created_at FROM scan_paths WHERE id = ?", id)
	return scanScanPath(row)
}

// GetScanPathByPath retrieves a scan path by its path
func (db *DB) GetScanPathByPath(path string) (*ScanPath, error) {
	row := db.QueryRow("SELECT id, path, from_env, created_at FROM scan_paths WHERE path = ?", path)
	return scanScanPath(row)
}

// ListScanPaths returns all scan paths
func (db *DB) ListScanPaths() ([]*ScanPath, error) {
	rows, err := db.Query("SELECT id, path, from_env, created_at FROM scan_paths ORDER BY from_env DESC, path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []*ScanPath
	for rows.Next() {
		p, err := scanScanPathRow(rows)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// DeleteScanPath deletes a scan path (only if not from env)
func (db *DB) DeleteScanPath(id int64) error {
	result, err := db.Exec("DELETE FROM scan_paths WHERE id = ? AND from_env = 0", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("path not found or is locked (from env)")
	}
	return nil
}

func scanScanPath(row *sql.Row) (*ScanPath, error) {
	var p ScanPath
	err := row.Scan(&p.ID, &p.Path, &p.FromEnv, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func scanScanPathRow(rows *sql.Rows) (*ScanPath, error) {
	var p ScanPath
	err := rows.Scan(&p.ID, &p.Path, &p.FromEnv, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ScanConfig queries

// CreateScanConfig creates a new scan configuration
func (db *DB) CreateScanConfig(cfg *ScanConfig) (*ScanConfig, error) {
	pathsJSON, _ := json.Marshal(cfg.Paths)
	includeJSON, _ := json.Marshal(cfg.IncludePatterns)
	excludeJSON, _ := json.Marshal(cfg.ExcludePatterns)

	result, err := db.Exec(`
		INSERT INTO scan_configs (name, paths, min_size, max_size, include_patterns, exclude_patterns)
		VALUES (?, ?, ?, ?, ?, ?)`,
		cfg.Name, string(pathsJSON), cfg.MinSize, cfg.MaxSize, string(includeJSON), string(excludeJSON),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return db.GetScanConfig(id)
}

// GetScanConfig retrieves a scan config by ID
func (db *DB) GetScanConfig(id int64) (*ScanConfig, error) {
	row := db.QueryRow(`
		SELECT id, name, paths, min_size, max_size, include_patterns, exclude_patterns, created_at, updated_at
		FROM scan_configs WHERE id = ?`, id)
	return scanScanConfig(row)
}

// ListScanConfigs returns all scan configurations
func (db *DB) ListScanConfigs() ([]*ScanConfig, error) {
	rows, err := db.Query(`
		SELECT id, name, paths, min_size, max_size, include_patterns, exclude_patterns, created_at, updated_at
		FROM scan_configs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*ScanConfig
	for rows.Next() {
		c, err := scanScanConfigRow(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// UpdateScanConfig updates a scan configuration
func (db *DB) UpdateScanConfig(cfg *ScanConfig) error {
	pathsJSON, _ := json.Marshal(cfg.Paths)
	includeJSON, _ := json.Marshal(cfg.IncludePatterns)
	excludeJSON, _ := json.Marshal(cfg.ExcludePatterns)

	_, err := db.Exec(`
		UPDATE scan_configs SET
			name = ?, paths = ?, min_size = ?, max_size = ?,
			include_patterns = ?, exclude_patterns = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		cfg.Name, string(pathsJSON), cfg.MinSize, cfg.MaxSize,
		string(includeJSON), string(excludeJSON), cfg.ID,
	)
	return err
}

// DeleteScanConfig deletes a scan configuration
func (db *DB) DeleteScanConfig(id int64) error {
	_, err := db.Exec("DELETE FROM scan_configs WHERE id = ?", id)
	return err
}

func scanScanConfig(row *sql.Row) (*ScanConfig, error) {
	var c ScanConfig
	var pathsJSON, includeJSON, excludeJSON string
	var maxSize sql.NullInt64

	err := row.Scan(&c.ID, &c.Name, &pathsJSON, &c.MinSize, &maxSize,
		&includeJSON, &excludeJSON, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(pathsJSON), &c.Paths)
	json.Unmarshal([]byte(includeJSON), &c.IncludePatterns)
	json.Unmarshal([]byte(excludeJSON), &c.ExcludePatterns)
	if maxSize.Valid {
		c.MaxSize = &maxSize.Int64
	}

	return &c, nil
}

func scanScanConfigRow(rows *sql.Rows) (*ScanConfig, error) {
	var c ScanConfig
	var pathsJSON, includeJSON, excludeJSON string
	var maxSize sql.NullInt64

	err := rows.Scan(&c.ID, &c.Name, &pathsJSON, &c.MinSize, &maxSize,
		&includeJSON, &excludeJSON, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(pathsJSON), &c.Paths)
	json.Unmarshal([]byte(includeJSON), &c.IncludePatterns)
	json.Unmarshal([]byte(excludeJSON), &c.ExcludePatterns)
	if maxSize.Valid {
		c.MaxSize = &maxSize.Int64
	}

	return &c, nil
}

// ScanRun queries

// CreateScanRun creates a new scan run
func (db *DB) CreateScanRun(configID *int64, jobID *int64) (*ScanRun, error) {
	result, err := db.Exec(`
		INSERT INTO scan_runs (scan_config_id, scheduled_job_id, status, started_at)
		VALUES (?, ?, ?, ?)`,
		configID, jobID, ScanRunStatusRunning, time.Now(),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return db.GetScanRun(id)
}

// GetScanRun retrieves a scan run by ID
func (db *DB) GetScanRun(id int64) (*ScanRun, error) {
	row := db.QueryRow(`
		SELECT id, scan_config_id, scheduled_job_id, status, started_at, completed_at,
			files_scanned, bytes_scanned, duplicate_groups, duplicate_files, wasted_bytes, error_message
		FROM scan_runs WHERE id = ?`, id)
	return scanScanRun(row)
}

// ListScanRuns returns scan runs with pagination
func (db *DB) ListScanRuns(limit, offset int) ([]*ScanRun, error) {
	rows, err := db.Query(`
		SELECT id, scan_config_id, scheduled_job_id, status, started_at, completed_at,
			files_scanned, bytes_scanned, duplicate_groups, duplicate_files, wasted_bytes, error_message
		FROM scan_runs ORDER BY started_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*ScanRun
	for rows.Next() {
		r, err := scanScanRunRow(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetRecentScanRuns returns the most recent scan runs
func (db *DB) GetRecentScanRuns(limit int) ([]*ScanRun, error) {
	return db.ListScanRuns(limit, 0)
}

// UpdateScanRunProgress updates scan progress
func (db *DB) UpdateScanRunProgress(id int64, filesScanned, bytesScanned, groups, files, wasted int64) error {
	_, err := db.Exec(`
		UPDATE scan_runs SET
			files_scanned = ?, bytes_scanned = ?, duplicate_groups = ?,
			duplicate_files = ?, wasted_bytes = ?
		WHERE id = ?`,
		filesScanned, bytesScanned, groups, files, wasted, id,
	)
	return err
}

// CompleteScanRun marks a scan run as completed
func (db *DB) CompleteScanRun(id int64, status ScanRunStatus, errorMsg *string) error {
	_, err := db.Exec(`
		UPDATE scan_runs SET status = ?, completed_at = ?, error_message = ?
		WHERE id = ?`,
		status, time.Now(), errorMsg, id,
	)
	return err
}

func scanScanRun(row *sql.Row) (*ScanRun, error) {
	var r ScanRun
	var configID, jobID sql.NullInt64
	var completedAt sql.NullTime
	var errorMsg sql.NullString

	err := row.Scan(&r.ID, &configID, &jobID, &r.Status, &r.StartedAt, &completedAt,
		&r.FilesScanned, &r.BytesScanned, &r.DuplicateGroups, &r.DuplicateFiles,
		&r.WastedBytes, &errorMsg)
	if err != nil {
		return nil, err
	}

	if configID.Valid {
		r.ScanConfigID = &configID.Int64
	}
	if jobID.Valid {
		r.ScheduledJobID = &jobID.Int64
	}
	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		r.ErrorMessage = &errorMsg.String
	}

	return &r, nil
}

func scanScanRunRow(rows *sql.Rows) (*ScanRun, error) {
	var r ScanRun
	var configID, jobID sql.NullInt64
	var completedAt sql.NullTime
	var errorMsg sql.NullString

	err := rows.Scan(&r.ID, &configID, &jobID, &r.Status, &r.StartedAt, &completedAt,
		&r.FilesScanned, &r.BytesScanned, &r.DuplicateGroups, &r.DuplicateFiles,
		&r.WastedBytes, &errorMsg)
	if err != nil {
		return nil, err
	}

	if configID.Valid {
		r.ScanConfigID = &configID.Int64
	}
	if jobID.Valid {
		r.ScheduledJobID = &jobID.Int64
	}
	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		r.ErrorMessage = &errorMsg.String
	}

	return &r, nil
}

// DuplicateGroup queries

// CreateDuplicateGroup creates a new duplicate group
func (db *DB) CreateDuplicateGroup(g *DuplicateGroup) (*DuplicateGroup, error) {
	filesJSON, _ := json.Marshal(g.Files)

	result, err := db.Exec(`
		INSERT INTO duplicate_groups (scan_run_id, file_hash, file_size, file_count, wasted_bytes, status, files)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		g.ScanRunID, g.FileHash, g.FileSize, g.FileCount, g.WastedBytes, g.Status, string(filesJSON),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	g.ID = id
	return g, nil
}

// GetDuplicateGroup retrieves a duplicate group by ID
func (db *DB) GetDuplicateGroup(id int64) (*DuplicateGroup, error) {
	row := db.QueryRow(`
		SELECT id, scan_run_id, file_hash, file_size, file_count, wasted_bytes, status, files
		FROM duplicate_groups WHERE id = ?`, id)
	return scanDuplicateGroup(row)
}

// ListDuplicateGroups returns duplicate groups for a scan run
func (db *DB) ListDuplicateGroups(scanRunID int64, status string) ([]*DuplicateGroup, error) {
	query := `
		SELECT id, scan_run_id, file_hash, file_size, file_count, wasted_bytes, status, files
		FROM duplicate_groups WHERE scan_run_id = ?`
	args := []any{scanRunID}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY wasted_bytes DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*DuplicateGroup
	for rows.Next() {
		g, err := scanDuplicateGroupRow(rows)
		if err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// UpdateDuplicateGroupStatus updates the status of duplicate groups
func (db *DB) UpdateDuplicateGroupStatus(ids []int64, status DuplicateGroupStatus) error {
	if len(ids) == 0 {
		return nil
	}

	query := "UPDATE duplicate_groups SET status = ? WHERE id IN (?" + repeatString(",?", len(ids)-1) + ")"
	args := make([]any, len(ids)+1)
	args[0] = status
	for i, id := range ids {
		args[i+1] = id
	}

	_, err := db.Exec(query, args...)
	return err
}

func repeatString(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func scanDuplicateGroup(row *sql.Row) (*DuplicateGroup, error) {
	var g DuplicateGroup
	var filesJSON string

	err := row.Scan(&g.ID, &g.ScanRunID, &g.FileHash, &g.FileSize, &g.FileCount,
		&g.WastedBytes, &g.Status, &filesJSON)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(filesJSON), &g.Files)
	return &g, nil
}

func scanDuplicateGroupRow(rows *sql.Rows) (*DuplicateGroup, error) {
	var g DuplicateGroup
	var filesJSON string

	err := rows.Scan(&g.ID, &g.ScanRunID, &g.FileHash, &g.FileSize, &g.FileCount,
		&g.WastedBytes, &g.Status, &filesJSON)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(filesJSON), &g.Files)
	return &g, nil
}

// ScheduledJob queries

// CreateScheduledJob creates a new scheduled job
func (db *DB) CreateScheduledJob(job *ScheduledJob) (*ScheduledJob, error) {
	result, err := db.Exec(`
		INSERT INTO scheduled_jobs (scan_config_id, cron_expression, action, enabled, next_run_at)
		VALUES (?, ?, ?, ?, ?)`,
		job.ScanConfigID, job.CronExpression, job.Action, job.Enabled, job.NextRunAt,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return db.GetScheduledJob(id)
}

// GetScheduledJob retrieves a scheduled job by ID
func (db *DB) GetScheduledJob(id int64) (*ScheduledJob, error) {
	row := db.QueryRow(`
		SELECT id, scan_config_id, cron_expression, action, enabled, last_run_at, next_run_at, created_at
		FROM scheduled_jobs WHERE id = ?`, id)
	return scanScheduledJob(row)
}

// ListScheduledJobs returns all scheduled jobs
func (db *DB) ListScheduledJobs() ([]*ScheduledJob, error) {
	rows, err := db.Query(`
		SELECT id, scan_config_id, cron_expression, action, enabled, last_run_at, next_run_at, created_at
		FROM scheduled_jobs ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*ScheduledJob
	for rows.Next() {
		j, err := scanScheduledJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// GetEnabledJobs returns all enabled scheduled jobs
func (db *DB) GetEnabledJobs() ([]*ScheduledJob, error) {
	rows, err := db.Query(`
		SELECT id, scan_config_id, cron_expression, action, enabled, last_run_at, next_run_at, created_at
		FROM scheduled_jobs WHERE enabled = 1 ORDER BY next_run_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*ScheduledJob
	for rows.Next() {
		j, err := scanScheduledJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateScheduledJob updates a scheduled job
func (db *DB) UpdateScheduledJob(job *ScheduledJob) error {
	_, err := db.Exec(`
		UPDATE scheduled_jobs SET
			scan_config_id = ?, cron_expression = ?, action = ?, enabled = ?, next_run_at = ?
		WHERE id = ?`,
		job.ScanConfigID, job.CronExpression, job.Action, job.Enabled, job.NextRunAt, job.ID,
	)
	return err
}

// UpdateJobLastRun updates the last run time and next run time
func (db *DB) UpdateJobLastRun(id int64, lastRun, nextRun time.Time) error {
	_, err := db.Exec(`
		UPDATE scheduled_jobs SET last_run_at = ?, next_run_at = ?
		WHERE id = ?`,
		lastRun, nextRun, id,
	)
	return err
}

// SetJobEnabled enables or disables a job
func (db *DB) SetJobEnabled(id int64, enabled bool) error {
	_, err := db.Exec("UPDATE scheduled_jobs SET enabled = ? WHERE id = ?", enabled, id)
	return err
}

// DeleteScheduledJob deletes a scheduled job
func (db *DB) DeleteScheduledJob(id int64) error {
	_, err := db.Exec("DELETE FROM scheduled_jobs WHERE id = ?", id)
	return err
}

func scanScheduledJob(row *sql.Row) (*ScheduledJob, error) {
	var j ScheduledJob
	var lastRun, nextRun sql.NullTime

	err := row.Scan(&j.ID, &j.ScanConfigID, &j.CronExpression, &j.Action,
		&j.Enabled, &lastRun, &nextRun, &j.CreatedAt)
	if err != nil {
		return nil, err
	}

	if lastRun.Valid {
		j.LastRunAt = &lastRun.Time
	}
	if nextRun.Valid {
		j.NextRunAt = &nextRun.Time
	}

	return &j, nil
}

func scanScheduledJobRow(rows *sql.Rows) (*ScheduledJob, error) {
	var j ScheduledJob
	var lastRun, nextRun sql.NullTime

	err := rows.Scan(&j.ID, &j.ScanConfigID, &j.CronExpression, &j.Action,
		&j.Enabled, &lastRun, &nextRun, &j.CreatedAt)
	if err != nil {
		return nil, err
	}

	if lastRun.Valid {
		j.LastRunAt = &lastRun.Time
	}
	if nextRun.Valid {
		j.NextRunAt = &nextRun.Time
	}

	return &j, nil
}

// Action queries

// CreateAction creates a new action record
func (db *DB) CreateAction(a *Action) (*Action, error) {
	result, err := db.Exec(`
		INSERT INTO actions (scan_run_id, action_type, dry_run, started_at, status)
		VALUES (?, ?, ?, ?, ?)`,
		a.ScanRunID, a.ActionType, a.DryRun, time.Now(), ActionStatusRunning,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return db.GetAction(id)
}

// GetAction retrieves an action by ID
func (db *DB) GetAction(id int64) (*Action, error) {
	row := db.QueryRow(`
		SELECT id, scan_run_id, action_type, groups_processed, files_processed, bytes_saved,
			dry_run, started_at, completed_at, status, error_message
		FROM actions WHERE id = ?`, id)
	return scanAction(row)
}

// ListActions returns actions with pagination
func (db *DB) ListActions(limit, offset int) ([]*Action, error) {
	rows, err := db.Query(`
		SELECT id, scan_run_id, action_type, groups_processed, files_processed, bytes_saved,
			dry_run, started_at, completed_at, status, error_message
		FROM actions ORDER BY started_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []*Action
	for rows.Next() {
		a, err := scanActionRow(rows)
		if err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

// CompleteAction marks an action as completed
func (db *DB) CompleteAction(id int64, groups, files int, bytesSaved int64, status ActionStatus, errorMsg *string) error {
	_, err := db.Exec(`
		UPDATE actions SET
			groups_processed = ?, files_processed = ?, bytes_saved = ?,
			completed_at = ?, status = ?, error_message = ?
		WHERE id = ?`,
		groups, files, bytesSaved, time.Now(), status, errorMsg, id,
	)
	return err
}

func scanAction(row *sql.Row) (*Action, error) {
	var a Action
	var completedAt sql.NullTime
	var errorMsg sql.NullString

	err := row.Scan(&a.ID, &a.ScanRunID, &a.ActionType, &a.GroupsProcessed, &a.FilesProcessed,
		&a.BytesSaved, &a.DryRun, &a.StartedAt, &completedAt, &a.Status, &errorMsg)
	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		a.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		a.ErrorMessage = &errorMsg.String
	}

	return &a, nil
}

func scanActionRow(rows *sql.Rows) (*Action, error) {
	var a Action
	var completedAt sql.NullTime
	var errorMsg sql.NullString

	err := rows.Scan(&a.ID, &a.ScanRunID, &a.ActionType, &a.GroupsProcessed, &a.FilesProcessed,
		&a.BytesSaved, &a.DryRun, &a.StartedAt, &completedAt, &a.Status, &errorMsg)
	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		a.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		a.ErrorMessage = &errorMsg.String
	}

	return &a, nil
}

// Stats queries

// GetDashboardStats returns aggregate statistics
func (db *DB) GetDashboardStats() (totalSaved int64, pendingGroups int, recentScans int, err error) {
	// Total bytes saved
	row := db.QueryRow("SELECT COALESCE(SUM(bytes_saved), 0) FROM actions WHERE status = 'completed'")
	if err = row.Scan(&totalSaved); err != nil {
		return
	}

	// Pending duplicate groups
	row = db.QueryRow("SELECT COUNT(*) FROM duplicate_groups WHERE status = 'pending'")
	if err = row.Scan(&pendingGroups); err != nil {
		return
	}

	// Scans in last 24 hours
	row = db.QueryRow("SELECT COUNT(*) FROM scan_runs WHERE started_at > datetime('now', '-1 day')")
	err = row.Scan(&recentScans)
	return
}

// UpdateDailyStats updates or inserts daily statistics
func (db *DB) UpdateDailyStats(date time.Time, scans, groups, files int, wasted, saved int64) error {
	dateStr := date.Format("2006-01-02")
	_, err := db.Exec(`
		INSERT INTO daily_stats (date, scans_run, groups_found, files_found, bytes_wasted, bytes_saved)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			scans_run = scans_run + excluded.scans_run,
			groups_found = groups_found + excluded.groups_found,
			files_found = files_found + excluded.files_found,
			bytes_wasted = bytes_wasted + excluded.bytes_wasted,
			bytes_saved = bytes_saved + excluded.bytes_saved`,
		dateStr, scans, groups, files, wasted, saved,
	)
	return err
}

// CleanupOldData removes data older than the retention period
func (db *DB) CleanupOldData(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	// Delete old scan runs (cascades to duplicate_groups)
	_, err := db.Exec("DELETE FROM scan_runs WHERE completed_at < ? AND status != 'running'", cutoff)
	if err != nil {
		return err
	}

	// Delete old actions
	_, err = db.Exec("DELETE FROM actions WHERE completed_at < ?", cutoff)
	if err != nil {
		return err
	}

	// Delete old daily stats
	_, err = db.Exec("DELETE FROM daily_stats WHERE date < ?", cutoff.Format("2006-01-02"))
	return err
}
