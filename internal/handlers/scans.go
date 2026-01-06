package handlers

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/lyallcooper/kuron/internal/config"
	"github.com/lyallcooper/kuron/internal/db"
	"github.com/lyallcooper/kuron/internal/services"
)

// ScanResultsData holds data for the scan results template
type ScanResultsData struct {
	Title     string
	ActiveNav string
	Run       *db.ScanRun
	Job       *db.ScheduledJob // The job this scan was from, if any
	Groups    []*db.DuplicateGroup
	// Pagination
	Page       int
	PageSize   int
	TotalCount int
	TotalPages int
	// Sorting
	SortBy    string
	SortOrder string
	// Filter
	Status string
}

// QuickScanData holds data for the quick scan template
type QuickScanData struct {
	Title           string
	ActiveNav       string
	Paths           []string
	MinSize         string
	MaxSize         string
	IncludePatterns []string
	ExcludePatterns []string
	Error           string

	// Advanced options
	IncludeHidden bool
	FollowLinks   bool
	OneFileSystem bool
	NoIgnore      bool
	IgnoreCase    bool
	MaxDepth      string
}

// QuickScan handles GET/POST /scans/quick
func (h *Handler) QuickScan(w http.ResponseWriter, r *http.Request) {
	// GET - show form
	if r.Method == http.MethodGet {
		data := QuickScanData{
			Title:     "Quick Scan",
			ActiveNav: "jobs",
		}

		h.render(w, "quick_scan.html", data)
		return
	}

	// POST - run scan
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	minSizeStr := r.FormValue("min_size")
	maxSizeStr := r.FormValue("max_size")

	// Get paths from form array
	var paths []string
	for _, p := range r.Form["paths"] {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, config.ExpandPath(p))
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

	// Parse advanced options
	includeHidden := r.FormValue("include_hidden") == "1"
	followLinks := r.FormValue("follow_links") == "1"
	oneFileSystem := r.FormValue("one_file_system") == "1"
	noIgnore := r.FormValue("no_ignore") == "1"
	ignoreCase := r.FormValue("ignore_case") == "1"
	maxDepthStr := r.FormValue("max_depth")

	// Helper to render form with error
	renderError := func(errMsg string) {
		data := QuickScanData{
			Title:           "Quick Scan",
			ActiveNav:       "jobs",
			Paths:           paths,
			MinSize:         minSizeStr,
			MaxSize:         maxSizeStr,
			IncludePatterns: includePatterns,
			ExcludePatterns: excludePatterns,
			IncludeHidden:   includeHidden,
			FollowLinks:     followLinks,
			OneFileSystem:   oneFileSystem,
			NoIgnore:        noIgnore,
			IgnoreCase:      ignoreCase,
			MaxDepth:        maxDepthStr,
			Error:           errMsg,
		}
		h.render(w, "quick_scan.html", data)
	}

	if len(paths) == 0 {
		renderError("At least one path is required")
		return
	}

	// Validate paths against allowlist
	if len(h.cfg.AllowedPaths) > 0 {
		for _, p := range paths {
			if !h.cfg.IsPathAllowed(p) {
				renderError(fmt.Sprintf("Path not allowed: %s", p))
				return
			}
		}
	}

	// Parse sizes
	minSize, err := parseSizeWithError(minSizeStr)
	if err != nil {
		renderError(fmt.Sprintf("Invalid min size: %s", minSizeStr))
		return
	}

	var maxSize *int64
	if maxSizeStr != "" {
		ms, err := parseSizeWithError(maxSizeStr)
		if err != nil {
			renderError(fmt.Sprintf("Invalid max size: %s", maxSizeStr))
			return
		}
		if ms > 0 {
			maxSize = &ms
		}
	}

	// Validate max >= min
	if maxSize != nil && minSize > 0 && *maxSize < minSize {
		renderError("Max size must be greater than or equal to min size")
		return
	}

	// Parse max depth
	var maxDepth *int
	if maxDepthStr != "" {
		if d, err := strconv.Atoi(maxDepthStr); err == nil && d >= 0 {
			maxDepth = &d
		}
	}

	// Build scan config
	cfg := &services.ScanConfig{
		Paths:           paths,
		MinSize:         minSize,
		MaxSize:         maxSize,
		IncludePatterns: includePatterns,
		ExcludePatterns: excludePatterns,
		IncludeHidden:   includeHidden,
		FollowLinks:     followLinks,
		OneFileSystem:   oneFileSystem,
		NoIgnore:        noIgnore,
		IgnoreCase:      ignoreCase,
		MaxDepth:        maxDepth,
	}

	// Start scan
	run, err := h.scanner.StartScan(r.Context(), cfg, nil)
	if err != nil {
		renderError("Failed to start scan: " + err.Error())
		return
	}

	http.Redirect(w, r, "/scans/runs/"+strconv.FormatInt(run.ID, 10), http.StatusSeeOther)
}

const defaultPageSize = 50

// ScanResults handles GET /scans/runs/{id}
func (h *Handler) ScanResults(w http.ResponseWriter, r *http.Request) {
	// Parse ID from path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}

	// Handle action POST
	if len(parts) >= 5 && parts[4] == "action" && r.Method == http.MethodPost {
		h.HandleAction(w, r, parts[3])
		return
	}

	// Handle cancel POST
	if len(parts) >= 5 && parts[4] == "cancel" && r.Method == http.MethodPost {
		h.CancelScan(w, r, parts[3])
		return
	}

	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	run, err := h.db.GetScanRun(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	// Pagination
	page := 1
	if p, err := strconv.Atoi(query.Get("page")); err == nil && p > 0 {
		page = p
	}
	pageSize := defaultPageSize
	if ps, err := strconv.Atoi(query.Get("page_size")); err == nil && ps > 0 && ps <= 200 {
		pageSize = ps
	}

	// Sorting
	sortBy := query.Get("sort")
	if sortBy == "" {
		sortBy = "wasted"
	}
	sortOrder := query.Get("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Filter
	status := query.Get("status")

	// Get total count
	totalCount, err := h.db.CountDuplicateGroups(id, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Calculate total pages
	totalPages := (totalCount + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	// Get groups with pagination
	groups, err := h.db.ListDuplicateGroupsPaginated(db.DuplicateGroupQuery{
		ScanRunID: id,
		Status:    status,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Limit:     pageSize,
		Offset:    (page - 1) * pageSize,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch the job if this scan was from a scheduled job
	var job *db.ScheduledJob
	if run.ScheduledJobID != nil {
		job, _ = h.db.GetScheduledJob(*run.ScheduledJobID)
	}

	data := ScanResultsData{
		Title:      "Scan Results",
		ActiveNav:  "history",
		Run:        run,
		Job:        job,
		Groups:     groups,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		TotalPages: totalPages,
		SortBy:     sortBy,
		SortOrder:  sortOrder,
		Status:     status,
	}

	h.render(w, "scan_results.html", data)
}

// HandleAction handles POST /scans/runs/{id}/action
func (h *Handler) HandleAction(w http.ResponseWriter, r *http.Request, runIDStr string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	action := r.FormValue("action")
	dryRun := r.FormValue("dry_run") == "1"
	selectAll := r.FormValue("select_all") == "1"
	statusFilter := r.FormValue("status_filter")
	groupIDsStr := r.FormValue("group_ids")

	var groupIDs []int64

	if selectAll {
		// Fetch all group IDs matching the current filter
		groupIDs, err = h.db.GetDuplicateGroupIDs(runID, statusFilter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Parse individual group IDs
		if groupIDsStr == "" {
			http.Redirect(w, r, "/scans/runs/"+runIDStr, http.StatusSeeOther)
			return
		}

		for _, idStr := range strings.Split(groupIDsStr, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				continue
			}
			groupIDs = append(groupIDs, id)
		}
	}

	if len(groupIDs) == 0 {
		http.Redirect(w, r, "/scans/runs/"+runIDStr, http.StatusSeeOther)
		return
	}

	// Execute action
	var actionType db.ActionType
	if action == "hardlink" {
		actionType = db.ActionTypeHardlink
	} else {
		actionType = db.ActionTypeReflink
	}

	result, err := h.scanner.ExecuteAction(r.Context(), runID, groupIDs, actionType, dryRun)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// For HTMX requests, show modal with results
	if r.Header.Get("HX-Request") == "true" {
		h.renderActionResultModal(w, renderActionModalParams{
			Action:       action,
			Output:       result.Output,
			DryRun:       dryRun,
			RedirectURL:  "/scans/runs/" + runIDStr,
			RunID:        runIDStr,
			GroupIDs:     groupIDsStr,
			SelectAll:    selectAll,
			StatusFilter: statusFilter,
		})
		return
	}

	http.Redirect(w, r, "/scans/runs/"+runIDStr, http.StatusSeeOther)
}

// renderActionModalParams holds parameters for rendering the action result modal
type renderActionModalParams struct {
	Action       string
	Output       string
	DryRun       bool
	RedirectURL  string
	RunID        string
	GroupIDs     string
	SelectAll    bool
	StatusFilter string
}

// renderActionResultModal renders the action results modal
func (h *Handler) renderActionResultModal(w http.ResponseWriter, p renderActionModalParams) {
	actionName := "Hardlink"
	if p.Action == "reflink" {
		actionName = "Reflink"
	}

	var title, description string
	if p.DryRun {
		title = actionName + " Preview (Dry Run)"
		description = "The following operations would be performed:"
	} else {
		title = actionName + " Complete"
		description = "The following operations were performed:"
	}

	// Escape output to prevent XSS
	escapedOutput := html.EscapeString(p.Output)

	// Build the "Run for Real" form for dry runs
	var runForRealForm string
	if p.DryRun {
		selectAllValue := ""
		if p.SelectAll {
			selectAllValue = "1"
		}
		runForRealForm = `<form method="POST" action="/scans/runs/` + p.RunID + `/action" style="display:inline;"
			hx-post="/scans/runs/` + p.RunID + `/action"
			hx-target="#modal-backdrop"
			hx-swap="outerHTML">
			<input type="hidden" name="action" value="` + p.Action + `">
			<input type="hidden" name="group_ids" value="` + html.EscapeString(p.GroupIDs) + `">
			<input type="hidden" name="select_all" value="` + selectAllValue + `">
			<input type="hidden" name="status_filter" value="` + html.EscapeString(p.StatusFilter) + `">
			<button type="submit" class="btn btn-primary">Run for Real</button>
		</form>`
	}

	// Build footer buttons
	var footerButtons string
	if p.DryRun {
		footerButtons = `<button class="btn" onclick="closeModal()">Cancel</button>` + runForRealForm
	} else {
		footerButtons = `<button class="btn" onclick="window.location.href='` + p.RedirectURL + `'">Done</button>`
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	modalHTML := `<div id="modal-backdrop" class="modal-backdrop" onclick="closeModal()">
	<div class="modal" onclick="event.stopPropagation()">
		<div class="modal-header">
			<h3>` + title + `</h3>
			<button class="modal-close" onclick="closeModal()">&times;</button>
		</div>
		<div class="modal-body">
			<p>` + description + `</p>
			<pre class="output">` + escapedOutput + `</pre>
		</div>
		<div class="modal-footer">
			` + footerButtons + `
		</div>
	</div>
</div>
<script>
document.body.classList.add('modal-open');
function closeModal() {
	document.getElementById('modal-backdrop').remove();
	document.body.classList.remove('modal-open');
}
document.addEventListener('keydown', function(e) {
	if (e.key === 'Escape') closeModal();
});
</script>`
	w.Write([]byte(modalHTML))
}

// CancelScan handles POST /scans/runs/{id}/cancel
func (h *Handler) CancelScan(w http.ResponseWriter, r *http.Request, runIDStr string) {
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	h.scanner.CancelScan(runID)
	http.Redirect(w, r, "/scans/runs/"+runIDStr, http.StatusSeeOther)
}

// parseSizeWithError parses a human-readable size string to bytes, returning an error if invalid
func parseSizeWithError(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return 0, nil
	}

	multiplier := int64(1)
	for _, suffix := range []struct {
		s string
		m int64
	}{
		{"PB", 1 << 50},
		{"TB", 1 << 40},
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
		{"P", 1 << 50},
		{"T", 1 << 40},
		{"G", 1 << 30},
		{"M", 1 << 20},
		{"K", 1 << 10},
		{"B", 1},
	} {
		if strings.HasSuffix(s, suffix.s) {
			s = strings.TrimSuffix(s, suffix.s)
			multiplier = suffix.m
			break
		}
	}

	s = strings.TrimSpace(s)
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size format")
	}

	return int64(n * float64(multiplier)), nil
}
