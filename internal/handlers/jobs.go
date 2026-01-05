package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lyall/kuron/internal/db"
	"github.com/lyall/kuron/internal/services"
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
	Error     string
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

	// Build view models
	var views []*JobView
	for _, job := range jobs {
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
	data := JobFormData{
		Title:     "New Job",
		ActiveNav: "jobs",
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

// parseJobForm parses the job form and returns a ScheduledJob
func (h *Handler) parseJobForm(r *http.Request) (*db.ScheduledJob, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(r.FormValue("name"))
	minSizeStr := r.FormValue("min_size")
	maxSizeStr := r.FormValue("max_size")
	cronExpr := strings.TrimSpace(r.FormValue("cron_expression"))
	action := r.FormValue("action")
	enabled := r.FormValue("enabled") == "1"

	// Get paths from form array
	var paths []string
	for _, p := range r.Form["paths"] {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, p)
		}
	}

	// Parse sizes
	var minSize int64 = 0
	if minSizeStr != "" {
		if v, err := strconv.ParseInt(minSizeStr, 10, 64); err == nil {
			minSize = v
		}
	}

	var maxSize *int64
	if maxSizeStr != "" {
		if v, err := strconv.ParseInt(maxSizeStr, 10, 64); err == nil && v > 0 {
			maxSize = &v
		}
	}

	// Get patterns from form arrays
	var includePatterns []string
	for _, p := range r.Form["include_patterns"] {
		p = strings.TrimSpace(p)
		if p != "" {
			includePatterns = append(includePatterns, p)
		}
	}

	var excludePatterns []string
	for _, p := range r.Form["exclude_patterns"] {
		p = strings.TrimSpace(p)
		if p != "" {
			excludePatterns = append(excludePatterns, p)
		}
	}

	return &db.ScheduledJob{
		Name:            name,
		Paths:           paths,
		MinSize:         minSize,
		MaxSize:         maxSize,
		IncludePatterns: includePatterns,
		ExcludePatterns: excludePatterns,
		CronExpression:  cronExpr,
		Action:          action,
		Enabled:         enabled,
	}, nil
}

// CreateJob handles POST /jobs
func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.parseJobForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Helper to render form with error
	renderError := func(errMsg string) {
		data := JobFormData{
			Title:     "New Job",
			ActiveNav: "jobs",
			Job:       job,
			Error:     errMsg,
		}
		h.render(w, "job_form.html", data)
	}

	// Validate name
	if job.Name == "" {
		renderError("Name is required")
		return
	}

	// Validate paths
	if len(job.Paths) == 0 {
		renderError("At least one path is required")
		return
	}

	// Validate cron expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(job.CronExpression)
	if err != nil {
		renderError("Invalid cron expression: " + err.Error())
		return
	}

	nextRun := schedule.Next(time.Now())
	job.NextRunAt = &nextRun

	_, err = h.db.CreateScheduledJob(job)
	if err != nil {
		renderError("Failed to create job: " + err.Error())
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

	data := JobFormData{
		Title:     "Edit Job",
		ActiveNav: "jobs",
		Job:       job,
	}

	h.render(w, "job_form.html", data)
}

// UpdateJob handles POST /jobs/{id}
func (h *Handler) UpdateJob(w http.ResponseWriter, r *http.Request, id int64) {
	job, err := h.parseJobForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	job.ID = id

	// Helper to render form with error
	renderError := func(errMsg string) {
		data := JobFormData{
			Title:     "Edit Job",
			ActiveNav: "jobs",
			Job:       job,
			Error:     errMsg,
		}
		h.render(w, "job_form.html", data)
	}

	// Validate name
	if job.Name == "" {
		renderError("Name is required")
		return
	}

	// Validate paths
	if len(job.Paths) == 0 {
		renderError("At least one path is required")
		return
	}

	// Validate and calculate next run
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(job.CronExpression)
	if err != nil {
		renderError("Invalid cron expression: " + err.Error())
		return
	}

	nextRun := schedule.Next(time.Now())
	job.NextRunAt = &nextRun

	if err := h.db.UpdateScheduledJob(job); err != nil {
		renderError("Failed to update job: " + err.Error())
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

	if len(job.Paths) == 0 {
		http.Error(w, "No paths configured for this job", http.StatusBadRequest)
		return
	}

	// Build scan config from job
	cfg := &services.ScanConfig{
		Paths:           job.Paths,
		MinSize:         job.MinSize,
		MaxSize:         job.MaxSize,
		IncludePatterns: job.IncludePatterns,
		ExcludePatterns: job.ExcludePatterns,
	}

	// Start scan
	run, err := h.scanner.StartScan(r.Context(), cfg, &job.ID)
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
