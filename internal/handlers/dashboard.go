package handlers

import (
	"net/http"
)

// DashboardData holds data for the dashboard template
type DashboardData struct {
	Title       string
	ActiveNav   string
	Stats       DashboardStats
	RecentScans []*ScanRunView
	Jobs        []*JobView
}

// DashboardStats holds dashboard statistics
type DashboardStats struct {
	TotalSaved    int64
	PendingGroups int
	RecentScans   int
}

// ScanRunView is a view model for scan runs
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
	ConfigName     string
	CronExpression string
	Action         string
	NextRunAt      string
	LastRunAt      string
	LastRunID      int64
	Enabled        bool
}

// Dashboard handles GET /
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Get stats
	totalSaved, pendingGroups, recentScans, err := h.db.GetDashboardStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get recent scan runs
	runs, err := h.db.GetRecentScanRuns(5)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get scheduled jobs
	jobs, err := h.db.ListScheduledJobs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := DashboardData{
		Title:     "",
		ActiveNav: "dashboard",
		Stats: DashboardStats{
			TotalSaved:    totalSaved,
			PendingGroups: pendingGroups,
			RecentScans:   recentScans,
		},
	}

	// Convert to view models
	for _, run := range runs {
		data.RecentScans = append(data.RecentScans, &ScanRunView{
			ID:              run.ID,
			Status:          string(run.Status),
			StartedAt:       run.StartedAt.Format("2006-01-02 15:04"),
			DuplicateGroups: run.DuplicateGroups,
			WastedBytes:     run.WastedBytes,
		})
	}

	for _, job := range jobs {
		jv := &JobView{
			ID:             job.ID,
			CronExpression: job.CronExpression,
			Action:         job.Action,
			Enabled:        job.Enabled,
		}
		if cfg, err := h.db.GetScanConfig(job.ScanConfigID); err == nil {
			jv.ConfigName = cfg.Name
		}
		if job.NextRunAt != nil {
			jv.NextRunAt = job.NextRunAt.Format("2006-01-02 15:04")
		}
		if job.LastRunAt != nil {
			jv.LastRunAt = job.LastRunAt.Format("2006-01-02 15:04")
		}
		data.Jobs = append(data.Jobs, jv)
	}

	h.render(w, "dashboard.html", data)
}
