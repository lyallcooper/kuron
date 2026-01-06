package db

import (
	"database/sql"
	"time"
)

// ScheduledJob represents a scheduled scan job with all configuration
type ScheduledJob struct {
	ID              int64
	Name            string
	Paths           []string // Paths to scan
	MinSize         int64    // Minimum file size in bytes
	MaxSize         *int64   // Maximum file size (nil = no limit)
	IncludePatterns []string // Glob patterns to include
	ExcludePatterns []string // Glob patterns to exclude
	CronExpression  string
	Action          string // 'scan', 'scan_hardlink', 'scan_reflink'
	Enabled         bool
	LastRunAt       *time.Time
	NextRunAt       *time.Time
	CreatedAt       time.Time

	// Advanced options
	IncludeHidden bool // Include hidden files
	FollowLinks   bool // Follow symbolic links
	OneFileSystem bool // Stay on same filesystem
	NoIgnore      bool // Don't respect .gitignore/.fdignore
	IgnoreCase    bool // Case-insensitive pattern matching
	MaxDepth      *int // Recursion depth limit (nil = unlimited)
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
	Paths           []string // Paths that were scanned
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
