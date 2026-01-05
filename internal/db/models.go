package db

import (
	"database/sql"
	"time"
)

// ScanPath represents a configured path for scanning
type ScanPath struct {
	ID        int64
	Path      string
	FromEnv   bool // If true, path is locked (configured via env var)
	CreatedAt time.Time
}

// ScanConfig represents a reusable scan configuration
type ScanConfig struct {
	ID              int64
	Name            string
	Paths           []int64  // JSON array of path IDs
	MinSize         int64    // Minimum file size in bytes
	MaxSize         *int64   // Maximum file size (nil = no limit)
	IncludePatterns []string // Glob patterns to include
	ExcludePatterns []string // Glob patterns to exclude
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ScheduledJob represents a cron job for automatic scans
type ScheduledJob struct {
	ID             int64
	Name           string
	ScanConfigID   int64
	CronExpression string
	Action         string // 'scan', 'scan_hardlink', 'scan_reflink'
	Enabled        bool
	LastRunAt      *time.Time
	NextRunAt      *time.Time
	CreatedAt      time.Time
}

// ScanRunStatus represents the status of a scan run
type ScanRunStatus string

const (
	ScanRunStatusRunning   ScanRunStatus = "running"
	ScanRunStatusCompleted ScanRunStatus = "completed"
	ScanRunStatusFailed    ScanRunStatus = "failed"
	ScanRunStatusCancelled ScanRunStatus = "cancelled"
)

// ScanRun represents a single execution of a scan
type ScanRun struct {
	ID              int64
	ScanConfigID    *int64
	ScheduledJobID  *int64
	Status          ScanRunStatus
	StartedAt       time.Time
	CompletedAt     *time.Time
	FilesScanned    int64
	BytesScanned    int64
	DuplicateGroups int64
	DuplicateFiles  int64
	WastedBytes     int64
	ErrorMessage    *string
}

// DuplicateGroupStatus represents the status of a duplicate group
type DuplicateGroupStatus string

const (
	DuplicateGroupStatusPending   DuplicateGroupStatus = "pending"
	DuplicateGroupStatusProcessed DuplicateGroupStatus = "processed"
	DuplicateGroupStatusIgnored   DuplicateGroupStatus = "ignored"
)

// DuplicateGroup represents a group of duplicate files
type DuplicateGroup struct {
	ID          int64
	ScanRunID   int64
	FileHash    string
	FileSize    int64
	FileCount   int
	WastedBytes int64 // (count-1) * size
	Status      DuplicateGroupStatus
	Files       []string // Paths of duplicate files
}

// ActionStatus represents the status of an action
type ActionStatus string

const (
	ActionStatusRunning   ActionStatus = "running"
	ActionStatusCompleted ActionStatus = "completed"
	ActionStatusFailed    ActionStatus = "failed"
)

// ActionType represents the type of deduplication action
type ActionType string

const (
	ActionTypeHardlink ActionType = "hardlink"
	ActionTypeReflink  ActionType = "reflink"
)

// Action represents a deduplication action taken
type Action struct {
	ID              int64
	ScanRunID       int64
	ActionType      ActionType
	GroupsProcessed int
	FilesProcessed  int
	BytesSaved      int64
	DryRun          bool
	StartedAt       time.Time
	CompletedAt     *time.Time
	Status          ActionStatus
	ErrorMessage    *string
}

// DailyStats represents aggregated daily statistics
type DailyStats struct {
	Date        time.Time
	ScansRun    int
	GroupsFound int
	FilesFound  int
	BytesWasted int64
	BytesSaved  int64
}

// NullInt64 is a helper for nullable int64 fields
type NullInt64 struct {
	sql.NullInt64
}

// NullString is a helper for nullable string fields
type NullString struct {
	sql.NullString
}

// NullTime is a helper for nullable time fields
type NullTime struct {
	sql.NullTime
}
