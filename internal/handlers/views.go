package handlers

import "github.com/lyallcooper/kuron/internal/db"

// View model structs for templates.
// These are separate from db models to allow formatting and presentation logic.

// DashboardData holds data for the dashboard template
type DashboardData struct {
	Title       string
	ActiveNav   string
	Stats       DashboardStats
	RecentScans []*ScanRunView
	Jobs        []*JobView
	Error       string
}

// DashboardStats holds dashboard statistics
type DashboardStats struct {
	TotalSaved    int64
	PendingGroups int
	RecentScans   int
}

// ScanRunView is a view model for scan runs on the dashboard
type ScanRunView struct {
	ID              int64
	Status          string
	StartedAt       string
	DuplicateGroups int64
	WastedBytes     int64
}

// JobView is a view model for scheduled jobs
type JobView struct {
	ID             int64
	Name           string
	PathCount      int
	CronExpression string
	Action         string
	NextRunAt      string
	LastRunAt      string
	LastRunID      int64
	Enabled        bool
}

// toJobView converts a ScheduledJob to a JobView
func toJobView(job *db.ScheduledJob) *JobView {
	view := &JobView{
		ID:             job.ID,
		Name:           job.Name,
		PathCount:      len(job.Paths),
		CronExpression: job.CronExpression,
		Action:         job.Action,
		Enabled:        job.Enabled,
	}
	if job.NextRunAt != nil {
		view.NextRunAt = job.NextRunAt.Format("2006-01-02 15:04")
	}
	if job.LastRunAt != nil {
		view.LastRunAt = job.LastRunAt.Format("2006-01-02 15:04")
	}
	return view
}

// JobsData holds data for the jobs list template
type JobsData struct {
	Title     string
	ActiveNav string
	CSRFToken string
	Jobs      []*JobView
}

// JobFormData holds data for the job form template
type JobFormData struct {
	Title        string
	ActiveNav    string
	CSRFToken    string
	Job          *db.ScheduledJob
	Error        string
	AllowedPaths []string
}

// QuickScanData holds data for the quick scan template
type QuickScanData struct {
	Title           string
	ActiveNav       string
	CSRFToken       string
	Paths           []string
	MinSize         string
	MaxSize         string
	IncludePatterns []string
	ExcludePatterns []string
	Error           string
	AllowedPaths    []string

	// Advanced options
	IncludeHidden bool
	FollowLinks   bool
	OneFileSystem bool
	NoIgnore      bool
	IgnoreCase    bool
	MaxDepth      string
}

// ScanResultsData holds data for the scan results template
type ScanResultsData struct {
	Title       string
	ActiveNav   string
	CSRFToken   string
	Run         *db.ScanRun
	Job         *db.ScheduledJob // The job this scan was from, if any
	GroupsTable GroupsTableData
	Actions     []*db.Action // Actions taken from this scan
}

// HistoryData holds data for the history template
type HistoryData struct {
	Title        string
	ActiveNav    string
	Runs         []*ScanRunHistoryView
	Actions      []*db.Action
	StatusFilter string
	Page         int
	HasMore      bool
	NextPage     int
}

// ScanRunHistoryView extends ScanRun with duration for history display
type ScanRunHistoryView struct {
	*db.ScanRun
	Duration string
}

// SettingsData holds data for the settings template
type SettingsData struct {
	Title             string
	ActiveNav         string
	CSRFToken         string
	RetentionDays     int
	RetentionEditable bool
	Version           string
	FclonesVersion    string
	DBPath            string
	Port              int
	AllowedPaths      []string
	Error             string
	Success           string
}

// ActionDetailData holds data for the action detail template
type ActionDetailData struct {
	Title       string
	ActiveNav   string
	Action      *db.Action
	Run         *db.ScanRun // The scan run this action was from
	GroupsTable GroupsTableData
}

// GroupsTableData holds data for the shared groups table partial
type GroupsTableData struct {
	Groups       []*db.DuplicateGroup
	Interactive  bool // Show checkboxes, status column, file selection
	Page         int
	PageSize     int
	TotalCount   int
	TotalPages   int
	SortBy       string
	SortOrder    string
	BaseURL      string // Base URL for pagination/sorting links
	StatusFilter string // Optional status filter (only for interactive mode)
}

// ScanProgressData is sent via SSE during scans
type ScanProgressData struct {
	FilesScanned int64   `json:"files_scanned"`
	BytesScanned string  `json:"bytes_scanned"`
	GroupsFound  int64   `json:"groups_found"`
	WastedBytes  string  `json:"wasted_bytes"`
	Status       string  `json:"status"`
	PhaseNum     int     `json:"phase_num,omitempty"`
	PhaseTotal   int     `json:"phase_total,omitempty"`
	PhaseName    string  `json:"phase_name,omitempty"`
	PhasePercent float64 `json:"phase_percent,omitempty"`
}
