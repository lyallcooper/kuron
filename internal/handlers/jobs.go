package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lyall/kuron/internal/db"
	"github.com/robfig/cron/v3"
)

// JobsData holds data for the jobs list template
type JobsData struct {
	Title     string
	ActiveNav string
	Jobs      []*JobView
}

// JobFormData holds data for the job form template
type JobFormData struct {
	Title     string
	ActiveNav string
	Job       *db.ScheduledJob
	Configs   []*db.ScanConfig
}

// Jobs handles GET /jobs
func (h *Handler) Jobs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.CreateJob(w, r)
		return
	}

	jobs, err := h.db.ListScheduledJobs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build view models with config names
	var views []*JobView
	for _, job := range jobs {
		view := &JobView{
			ID:             job.ID,
			ConfigID:       job.ScanConfigID,
			CronExpression: job.CronExpression,
			Action:         job.Action,
			Enabled:        job.Enabled,
		}
		if cfg, err := h.db.GetScanConfig(job.ScanConfigID); err == nil {
			view.ConfigName = cfg.Name
		}
		if job.NextRunAt != nil {
			view.NextRunAt = job.NextRunAt.Format("2006-01-02 15:04")
		}
		if job.LastRunAt != nil {
			view.LastRunAt = job.LastRunAt.Format("2006-01-02 15:04")
		}
		if lastRun, err := h.db.GetLastRunForJob(job.ID); err == nil {
			view.LastRunID = lastRun.ID
		}
		views = append(views, view)
	}

	data := JobsData{
		Title:     "Jobs",
		ActiveNav: "jobs",
		Jobs:      views,
	}

	h.render(w, "jobs.html", data)
}

// JobForm handles GET /jobs/new
func (h *Handler) JobForm(w http.ResponseWriter, r *http.Request) {
	configs, err := h.db.ListScanConfigs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := JobFormData{
		Title:     "New Job",
		ActiveNav: "jobs",
		Configs:   configs,
	}

	h.render(w, "job_form.html", data)
}

// JobRoutes handles routes under /jobs/{id}
func (h *Handler) JobRoutes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}

	idStr := parts[2]
	if idStr == "new" {
		h.JobForm(w, r)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Handle sub-routes
	if len(parts) >= 4 {
		switch parts[3] {
		case "edit":
			h.EditJobForm(w, r, id)
			return
		case "toggle":
			if r.Method == http.MethodPost {
				h.ToggleJob(w, r, id)
				return
			}
		case "run":
			if r.Method == http.MethodPost {
				h.RunJob(w, r, id)
				return
			}
		case "delete":
			if r.Method == http.MethodPost || r.Method == http.MethodDelete {
				h.DeleteJob(w, r, id)
				return
			}
		}
	}

	// POST to /jobs/{id} = update
	if r.Method == http.MethodPost {
		h.UpdateJob(w, r, id)
		return
	}

	// DELETE to /jobs/{id}
	if r.Method == http.MethodDelete {
		h.DeleteJob(w, r, id)
		return
	}

	// GET /jobs/{id} = view (redirect to edit for now)
	http.Redirect(w, r, "/jobs/"+idStr+"/edit", http.StatusSeeOther)
}

// CreateJob handles POST /jobs
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	configIDStr := r.FormValue("scan_config_id")
	cronExpr := r.FormValue("cron_expression")
	action := r.FormValue("action")
	enabled := r.FormValue("enabled") == "1"

	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid scan config", http.StatusBadRequest)
		return
	}

	// Validate cron expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		http.Error(w, "Invalid cron expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	nextRun := schedule.Next(time.Now())

	job := &db.ScheduledJob{
		ScanConfigID:   configID,
		CronExpression: cronExpr,
		Action:         action,
		Enabled:        enabled,
		NextRunAt:      &nextRun,
	}

	_, err = h.db.CreateScheduledJob(job)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

// EditJobForm handles GET /jobs/{id}/edit
func (h *Handler) EditJobForm(w http.ResponseWriter, r *http.Request, id int64) {
	job, err := h.db.GetScheduledJob(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	configs, err := h.db.ListScanConfigs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := JobFormData{
		Title:     "Edit Job",
		ActiveNav: "jobs",
		Job:       job,
		Configs:   configs,
	}

	h.render(w, "job_form.html", data)
}

// UpdateJob handles POST /jobs/{id}
func (h *Handler) UpdateJob(w http.ResponseWriter, r *http.Request, id int64) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	job, err := h.db.GetScheduledJob(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	configIDStr := r.FormValue("scan_config_id")
	job.CronExpression = r.FormValue("cron_expression")
	job.Action = r.FormValue("action")
	job.Enabled = r.FormValue("enabled") == "1"

	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid scan config", http.StatusBadRequest)
		return
	}
	job.ScanConfigID = configID

	// Validate and calculate next run
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(job.CronExpression)
	if err != nil {
		http.Error(w, "Invalid cron expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	nextRun := schedule.Next(time.Now())
	job.NextRunAt = &nextRun

	if err := h.db.UpdateScheduledJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

// ToggleJob handles POST /jobs/{id}/toggle
func (h *Handler) ToggleJob(w http.ResponseWriter, r *http.Request, id int64) {
	job, err := h.db.GetScheduledJob(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.db.SetJobEnabled(id, !job.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

// RunJob handles POST /jobs/{id}/run
func (h *Handler) RunJob(w http.ResponseWriter, r *http.Request, id int64) {
	job, err := h.db.GetScheduledJob(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get scan config
	cfg, err := h.db.GetScanConfig(job.ScanConfigID)
	if err != nil {
		http.Error(w, "Scan config not found", http.StatusInternalServerError)
		return
	}

	// Get paths
	var paths []string
	for _, pathID := range cfg.Paths {
		path, err := h.db.GetScanPath(pathID)
		if err != nil {
			continue
		}
		paths = append(paths, path.Path)
	}

	if len(paths) == 0 {
		http.Error(w, "No valid paths in scan config", http.StatusBadRequest)
		return
	}

	// Start scan
	run, err := h.scanner.StartScan(r.Context(), paths, &job.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update last run time
	now := time.Now()
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, _ := parser.Parse(job.CronExpression)
	nextRun := schedule.Next(now)
	h.db.UpdateJobLastRun(id, now, nextRun)

	http.Redirect(w, r, "/scans/runs/"+strconv.FormatInt(run.ID, 10), http.StatusSeeOther)
}

// DeleteJob handles DELETE /jobs/{id}
func (h *Handler) DeleteJob(w http.ResponseWriter, r *http.Request, id int64) {
	if err := h.db.DeleteScheduledJob(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// For HTMX requests, return empty response
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/jobs")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}
