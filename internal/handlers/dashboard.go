package handlers

import (
	"net/http"
)

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
		data.Jobs = append(data.Jobs, toJobView(job))
	}

	h.render(w, "dashboard.html", data)
}
