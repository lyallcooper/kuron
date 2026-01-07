package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// ScheduledJob queries

// ScanRun queries

// ScanRunOptions contains scan options to record with a scan run
type ScanRunOptions struct {
	MinSize         int64
	MaxSize         *int64
	IncludePatterns []string
	ExcludePatterns []string
	IncludeHidden   bool
	FollowLinks     bool
	OneFileSystem   bool
	NoIgnore        bool
	IgnoreCase      bool
	MaxDepth        *int
}

// CreateScanRun creates a new scan run
func (db *DB) CreateScanRun(configID *int64, jobID *int64, paths []string, opts *ScanRunOptions) (*ScanRun, error) {
	pathsJSON, err := json.Marshal(paths)
	if err != nil {
		return nil, err
	}

	// Default options if not provided
	if opts == nil {
		opts = &ScanRunOptions{}
	}

	includePatternsJSON, err := json.Marshal(opts.IncludePatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal include patterns: %w", err)
	}
	excludePatternsJSON, err := json.Marshal(opts.ExcludePatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal exclude patterns: %w", err)
	}

	result, err := db.Exec(`
		INSERT INTO scan_runs (scan_config_id, scheduled_job_id, paths, status, started_at,
			min_size, max_size, include_patterns, exclude_patterns,
			include_hidden, follow_links, one_file_system, no_ignore, ignore_case, max_depth)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		configID, jobID, string(pathsJSON), ScanRunStatusRunning, time.Now(),
		opts.MinSize, opts.MaxSize, string(includePatternsJSON), string(excludePatternsJSON),
		opts.IncludeHidden, opts.FollowLinks, opts.OneFileSystem, opts.NoIgnore, opts.IgnoreCase, opts.MaxDepth,
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
		SELECT id, scan_config_id, scheduled_job_id, paths, status, started_at, completed_at,
			files_scanned, bytes_scanned, duplicate_groups, duplicate_files, wasted_bytes, error_message,
			min_size, max_size, include_patterns, exclude_patterns,
			include_hidden, follow_links, one_file_system, no_ignore, ignore_case, max_depth
		FROM scan_runs WHERE id = ?`, id)
	return scanScanRun(row)
}

// ListScanRuns returns scan runs with pagination
func (db *DB) ListScanRuns(limit, offset int) ([]*ScanRun, error) {
	rows, err := db.Query(`
		SELECT id, scan_config_id, scheduled_job_id, paths, status, started_at, completed_at,
			files_scanned, bytes_scanned, duplicate_groups, duplicate_files, wasted_bytes, error_message,
			min_size, max_size, include_patterns, exclude_patterns,
			include_hidden, follow_links, one_file_system, no_ignore, ignore_case, max_depth
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

// GetLastRunForJob returns the most recent scan run for a scheduled job
func (db *DB) GetLastRunForJob(jobID int64) (*ScanRun, error) {
	row := db.QueryRow(`
		SELECT id, scan_config_id, scheduled_job_id, paths, status, started_at, completed_at,
			files_scanned, bytes_scanned, duplicate_groups, duplicate_files, wasted_bytes, error_message,
			min_size, max_size, include_patterns, exclude_patterns,
			include_hidden, follow_links, one_file_system, no_ignore, ignore_case, max_depth
		FROM scan_runs WHERE scheduled_job_id = ? ORDER BY started_at DESC LIMIT 1`, jobID)
	return scanScanRun(row)
}

// GetLastRunIDsForJobs returns a map of job IDs to their most recent scan run IDs.
// This is more efficient than calling GetLastRunForJob for each job.
func (db *DB) GetLastRunIDsForJobs(jobIDs []int64) (map[int64]int64, error) {
	if len(jobIDs) == 0 {
		return make(map[int64]int64), nil
	}

	// Build query with placeholders
	placeholders := strings.Repeat("?,", len(jobIDs))
	placeholders = placeholders[:len(placeholders)-1] // remove trailing comma

	query := `
		SELECT scheduled_job_id, id FROM scan_runs
		WHERE scheduled_job_id IN (` + placeholders + `)
		AND id IN (
			SELECT MAX(id) FROM scan_runs
			WHERE scheduled_job_id IN (` + placeholders + `)
			GROUP BY scheduled_job_id
		)`

	// Build args slice (need to pass jobIDs twice for both IN clauses)
	args := make([]interface{}, 0, len(jobIDs)*2)
	for _, id := range jobIDs {
		args = append(args, id)
	}
	for _, id := range jobIDs {
		args = append(args, id)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]int64)
	for rows.Next() {
		var jobID, runID int64
		if err := rows.Scan(&jobID, &runID); err != nil {
			return nil, err
		}
		result[jobID] = runID
	}

	return result, rows.Err()
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
	var pathsJSON string
	var completedAt sql.NullTime
	var errorMsg sql.NullString
	var maxSize sql.NullInt64
	var includePatternsJSON, excludePatternsJSON string
	var maxDepth sql.NullInt64

	err := row.Scan(&r.ID, &configID, &jobID, &pathsJSON, &r.Status, &r.StartedAt, &completedAt,
		&r.FilesScanned, &r.BytesScanned, &r.DuplicateGroups, &r.DuplicateFiles,
		&r.WastedBytes, &errorMsg,
		&r.MinSize, &maxSize, &includePatternsJSON, &excludePatternsJSON,
		&r.IncludeHidden, &r.FollowLinks, &r.OneFileSystem, &r.NoIgnore, &r.IgnoreCase, &maxDepth)
	if err != nil {
		return nil, err
	}

	if configID.Valid {
		r.ScanConfigID = &configID.Int64
	}
	if jobID.Valid {
		r.ScheduledJobID = &jobID.Int64
	}
	if err := json.Unmarshal([]byte(pathsJSON), &r.Paths); err != nil {
		log.Printf("db: failed to unmarshal paths JSON for scan run %d: %v", r.ID, err)
	}
	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		r.ErrorMessage = &errorMsg.String
	}
	if maxSize.Valid {
		r.MaxSize = &maxSize.Int64
	}
	if err := json.Unmarshal([]byte(includePatternsJSON), &r.IncludePatterns); err != nil {
		log.Printf("db: failed to unmarshal include_patterns JSON for scan run %d: %v", r.ID, err)
	}
	if err := json.Unmarshal([]byte(excludePatternsJSON), &r.ExcludePatterns); err != nil {
		log.Printf("db: failed to unmarshal exclude_patterns JSON for scan run %d: %v", r.ID, err)
	}
	if maxDepth.Valid {
		d := int(maxDepth.Int64)
		r.MaxDepth = &d
	}

	return &r, nil
}

func scanScanRunRow(rows *sql.Rows) (*ScanRun, error) {
	var r ScanRun
	var configID, jobID sql.NullInt64
	var pathsJSON string
	var completedAt sql.NullTime
	var errorMsg sql.NullString
	var maxSize sql.NullInt64
	var includePatternsJSON, excludePatternsJSON string
	var maxDepth sql.NullInt64

	err := rows.Scan(&r.ID, &configID, &jobID, &pathsJSON, &r.Status, &r.StartedAt, &completedAt,
		&r.FilesScanned, &r.BytesScanned, &r.DuplicateGroups, &r.DuplicateFiles,
		&r.WastedBytes, &errorMsg,
		&r.MinSize, &maxSize, &includePatternsJSON, &excludePatternsJSON,
		&r.IncludeHidden, &r.FollowLinks, &r.OneFileSystem, &r.NoIgnore, &r.IgnoreCase, &maxDepth)
	if err != nil {
		return nil, err
	}

	if configID.Valid {
		r.ScanConfigID = &configID.Int64
	}
	if jobID.Valid {
		r.ScheduledJobID = &jobID.Int64
	}
	if err := json.Unmarshal([]byte(pathsJSON), &r.Paths); err != nil {
		log.Printf("db: failed to unmarshal paths JSON for scan run %d: %v", r.ID, err)
	}
	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		r.ErrorMessage = &errorMsg.String
	}
	if maxSize.Valid {
		r.MaxSize = &maxSize.Int64
	}
	if err := json.Unmarshal([]byte(includePatternsJSON), &r.IncludePatterns); err != nil {
		log.Printf("db: failed to unmarshal include_patterns JSON for scan run %d: %v", r.ID, err)
	}
	if err := json.Unmarshal([]byte(excludePatternsJSON), &r.ExcludePatterns); err != nil {
		log.Printf("db: failed to unmarshal exclude_patterns JSON for scan run %d: %v", r.ID, err)
	}
	if maxDepth.Valid {
		d := int(maxDepth.Int64)
		r.MaxDepth = &d
	}

	return &r, nil
}

// DuplicateGroup queries

// CreateDuplicateGroup creates a new duplicate group
func (db *DB) CreateDuplicateGroup(g *DuplicateGroup) (*DuplicateGroup, error) {
	// Validate required fields
	if g.ScanRunID == 0 {
		return nil, errors.New("db: scan_run_id is required")
	}
	if g.FileHash == "" {
		return nil, errors.New("db: file_hash is required")
	}
	if len(g.Files) == 0 {
		return nil, errors.New("db: files list is required")
	}

	filesJSON, err := json.Marshal(g.Files)
	if err != nil {
		return nil, err
	}

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

// DuplicateGroupQuery holds query parameters for listing duplicate groups
type DuplicateGroupQuery struct {
	ScanRunID int64
	Status    string // filter by status (empty = all)
	SortBy    string // "wasted", "size", "count", "hash"
	SortOrder string // "asc" or "desc"
	Limit     int
	Offset    int
}

// ListDuplicateGroups returns duplicate groups for a scan run (unpaginated, for backwards compat)
func (db *DB) ListDuplicateGroups(scanRunID int64, status string) ([]*DuplicateGroup, error) {
	return db.ListDuplicateGroupsPaginated(DuplicateGroupQuery{
		ScanRunID: scanRunID,
		Status:    status,
		SortBy:    "wasted",
		SortOrder: "desc",
		Limit:     0, // no limit
	})
}

// ListDuplicateGroupsPaginated returns duplicate groups with sorting and pagination
func (db *DB) ListDuplicateGroupsPaginated(q DuplicateGroupQuery) ([]*DuplicateGroup, error) {
	query := `
		SELECT id, scan_run_id, file_hash, file_size, file_count, wasted_bytes, status, files
		FROM duplicate_groups WHERE scan_run_id = ?`
	args := []any{q.ScanRunID}

	if q.Status != "" {
		query += " AND status = ?"
		args = append(args, q.Status)
	}

	// Determine sort column
	sortCol := "wasted_bytes"
	switch q.SortBy {
	case "size":
		sortCol = "file_size"
	case "count":
		sortCol = "file_count"
	case "hash":
		sortCol = "file_hash"
	case "status":
		sortCol = "status"
	}

	// Determine sort order
	sortOrder := "DESC"
	if q.SortOrder == "asc" {
		sortOrder = "ASC"
	}

	query += " ORDER BY " + sortCol + " " + sortOrder

	// Add pagination
	if q.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, q.Limit, q.Offset)
	}

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

// CountDuplicateGroups returns the total count of duplicate groups for a scan run
func (db *DB) CountDuplicateGroups(scanRunID int64, status string) (int, error) {
	query := "SELECT COUNT(*) FROM duplicate_groups WHERE scan_run_id = ?"
	args := []any{scanRunID}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	var count int
	err := db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// GetDuplicateGroupIDs returns all group IDs for a scan run (for bulk operations)
func (db *DB) GetDuplicateGroupIDs(scanRunID int64, status string) ([]int64, error) {
	query := "SELECT id FROM duplicate_groups WHERE scan_run_id = ?"
	args := []any{scanRunID}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpdateDuplicateGroupStatus updates the status of duplicate groups
func (db *DB) UpdateDuplicateGroupStatus(ids []int64, status DuplicateGroupStatus) error {
	if len(ids) == 0 {
		return nil
	}

	query := "UPDATE duplicate_groups SET status = ? WHERE id IN (?" + strings.Repeat(",?", len(ids)-1) + ")"
	args := make([]any, len(ids)+1)
	args[0] = status
	for i, id := range ids {
		args[i+1] = id
	}

	_, err := db.Exec(query, args...)
	return err
}

func scanDuplicateGroup(row *sql.Row) (*DuplicateGroup, error) {
	var g DuplicateGroup
	var filesJSON string

	err := row.Scan(&g.ID, &g.ScanRunID, &g.FileHash, &g.FileSize, &g.FileCount,
		&g.WastedBytes, &g.Status, &filesJSON)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(filesJSON), &g.Files); err != nil {
		log.Printf("db: failed to unmarshal files JSON for group %d: %v", g.ID, err)
	}
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

	if err := json.Unmarshal([]byte(filesJSON), &g.Files); err != nil {
		log.Printf("db: failed to unmarshal files JSON for group %d: %v", g.ID, err)
	}
	return &g, nil
}

// CreateScheduledJob creates a new scheduled job
func (db *DB) CreateScheduledJob(job *ScheduledJob) (*ScheduledJob, error) {
	pathsJSON, err := json.Marshal(job.Paths)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal paths: %w", err)
	}
	includeJSON, err := json.Marshal(job.IncludePatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal include patterns: %w", err)
	}
	excludeJSON, err := json.Marshal(job.ExcludePatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal exclude patterns: %w", err)
	}

	result, err := db.Exec(`
		INSERT INTO scheduled_jobs (name, paths, min_size, max_size, include_patterns, exclude_patterns,
			cron_expression, action, enabled, next_run_at,
			include_hidden, follow_links, one_file_system, no_ignore, ignore_case, max_depth)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.Name, string(pathsJSON), job.MinSize, job.MaxSize, string(includeJSON), string(excludeJSON),
		job.CronExpression, job.Action, job.Enabled, job.NextRunAt,
		job.IncludeHidden, job.FollowLinks, job.OneFileSystem, job.NoIgnore, job.IgnoreCase, job.MaxDepth,
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
		SELECT id, name, paths, min_size, max_size, include_patterns, exclude_patterns,
			cron_expression, action, enabled, last_run_at, next_run_at, created_at,
			include_hidden, follow_links, one_file_system, no_ignore, ignore_case, max_depth
		FROM scheduled_jobs WHERE id = ?`, id)
	return scanScheduledJob(row)
}

// ListScheduledJobs returns all scheduled jobs
func (db *DB) ListScheduledJobs() ([]*ScheduledJob, error) {
	rows, err := db.Query(`
		SELECT id, name, paths, min_size, max_size, include_patterns, exclude_patterns,
			cron_expression, action, enabled, last_run_at, next_run_at, created_at,
			include_hidden, follow_links, one_file_system, no_ignore, ignore_case, max_depth
		FROM scheduled_jobs ORDER BY name`)
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
		SELECT id, name, paths, min_size, max_size, include_patterns, exclude_patterns,
			cron_expression, action, enabled, last_run_at, next_run_at, created_at,
			include_hidden, follow_links, one_file_system, no_ignore, ignore_case, max_depth
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
	pathsJSON, err := json.Marshal(job.Paths)
	if err != nil {
		return fmt.Errorf("failed to marshal paths: %w", err)
	}
	includeJSON, err := json.Marshal(job.IncludePatterns)
	if err != nil {
		return fmt.Errorf("failed to marshal include patterns: %w", err)
	}
	excludeJSON, err := json.Marshal(job.ExcludePatterns)
	if err != nil {
		return fmt.Errorf("failed to marshal exclude patterns: %w", err)
	}

	_, err = db.Exec(`
		UPDATE scheduled_jobs SET
			name = ?, paths = ?, min_size = ?, max_size = ?, include_patterns = ?, exclude_patterns = ?,
			cron_expression = ?, action = ?, enabled = ?, next_run_at = ?,
			include_hidden = ?, follow_links = ?, one_file_system = ?, no_ignore = ?, ignore_case = ?, max_depth = ?
		WHERE id = ?`,
		job.Name, string(pathsJSON), job.MinSize, job.MaxSize, string(includeJSON), string(excludeJSON),
		job.CronExpression, job.Action, job.Enabled, job.NextRunAt,
		job.IncludeHidden, job.FollowLinks, job.OneFileSystem, job.NoIgnore, job.IgnoreCase, job.MaxDepth,
		job.ID,
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
	var pathsJSON, includeJSON, excludeJSON string
	var maxSize, maxDepth sql.NullInt64
	var lastRun, nextRun sql.NullTime

	err := row.Scan(&j.ID, &j.Name, &pathsJSON, &j.MinSize, &maxSize, &includeJSON, &excludeJSON,
		&j.CronExpression, &j.Action, &j.Enabled, &lastRun, &nextRun, &j.CreatedAt,
		&j.IncludeHidden, &j.FollowLinks, &j.OneFileSystem, &j.NoIgnore, &j.IgnoreCase, &maxDepth)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(pathsJSON), &j.Paths); err != nil {
		log.Printf("db: failed to unmarshal paths JSON for job %d: %v", j.ID, err)
	}
	if err := json.Unmarshal([]byte(includeJSON), &j.IncludePatterns); err != nil {
		log.Printf("db: failed to unmarshal include patterns JSON for job %d: %v", j.ID, err)
	}
	if err := json.Unmarshal([]byte(excludeJSON), &j.ExcludePatterns); err != nil {
		log.Printf("db: failed to unmarshal exclude patterns JSON for job %d: %v", j.ID, err)
	}
	if maxSize.Valid {
		j.MaxSize = &maxSize.Int64
	}
	if lastRun.Valid {
		j.LastRunAt = &lastRun.Time
	}
	if nextRun.Valid {
		j.NextRunAt = &nextRun.Time
	}
	if maxDepth.Valid {
		depth := int(maxDepth.Int64)
		j.MaxDepth = &depth
	}

	return &j, nil
}

func scanScheduledJobRow(rows *sql.Rows) (*ScheduledJob, error) {
	var j ScheduledJob
	var pathsJSON, includeJSON, excludeJSON string
	var maxSize, maxDepth sql.NullInt64
	var lastRun, nextRun sql.NullTime

	err := rows.Scan(&j.ID, &j.Name, &pathsJSON, &j.MinSize, &maxSize, &includeJSON, &excludeJSON,
		&j.CronExpression, &j.Action, &j.Enabled, &lastRun, &nextRun, &j.CreatedAt,
		&j.IncludeHidden, &j.FollowLinks, &j.OneFileSystem, &j.NoIgnore, &j.IgnoreCase, &maxDepth)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(pathsJSON), &j.Paths); err != nil {
		log.Printf("db: failed to unmarshal paths JSON for job %d: %v", j.ID, err)
	}
	if err := json.Unmarshal([]byte(includeJSON), &j.IncludePatterns); err != nil {
		log.Printf("db: failed to unmarshal include patterns JSON for job %d: %v", j.ID, err)
	}
	if err := json.Unmarshal([]byte(excludeJSON), &j.ExcludePatterns); err != nil {
		log.Printf("db: failed to unmarshal exclude patterns JSON for job %d: %v", j.ID, err)
	}
	if maxSize.Valid {
		j.MaxSize = &maxSize.Int64
	}
	if lastRun.Valid {
		j.LastRunAt = &lastRun.Time
	}
	if nextRun.Valid {
		j.NextRunAt = &nextRun.Time
	}
	if maxDepth.Valid {
		depth := int(maxDepth.Int64)
		j.MaxDepth = &depth
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
	row := db.QueryRow("SELECT COALESCE(SUM(bytes_saved), 0) FROM actions WHERE status = ?", ActionStatusCompleted)
	if err = row.Scan(&totalSaved); err != nil {
		return
	}

	// Pending duplicate groups
	row = db.QueryRow("SELECT COUNT(*) FROM duplicate_groups WHERE status = ?", DuplicateGroupStatusPending)
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
	_, err := db.Exec("DELETE FROM scan_runs WHERE completed_at < ? AND status != ?", cutoff, ScanRunStatusRunning)
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

// Settings queries

// GetSetting retrieves a setting value by key
func (db *DB) GetSetting(key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting updates or inserts a setting value
func (db *DB) SetSetting(key, value string) error {
	_, err := db.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}
