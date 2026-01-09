package handlers

import (
	"fmt"
	"html"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/lyallcooper/kuron/internal/config"
	"github.com/lyallcooper/kuron/internal/db"
	"github.com/lyallcooper/kuron/internal/services"
)

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

// QuickScan handles GET/POST /scans/quick
func (h *Handler) QuickScan(w http.ResponseWriter, r *http.Request) {
	// GET - show form
	if r.Method == http.MethodGet {
		data := QuickScanData{
			Title:        "Quick Scan",
			ActiveNav:    "jobs",
			CSRFToken:    h.getOrCreateCSRFToken(w, r),
			AllowedPaths: h.cfg.AllowedPaths,
		}

		h.render(w, "quick_scan.html", data)
		return
	}

	// POST - run scan
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
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
			CSRFToken:       h.getOrCreateCSRFToken(w, r),
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

	h.redirect(w, r, "/scans/runs/"+strconv.FormatInt(run.ID, 10))
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

	// Handle delete-files POST
	if len(parts) >= 5 && parts[4] == "delete-files" && r.Method == http.MethodPost {
		h.HandleDeleteFiles(w, r, parts[3])
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

	// Fetch actions taken from this scan
	actions, _ := h.db.ListActionsByScanRun(id)

	data := ScanResultsData{
		Title:     "Scan Results",
		ActiveNav: "history",
		CSRFToken: h.getOrCreateCSRFToken(w, r),
		Run:       run,
		Job:       job,
		GroupsTable: GroupsTableData{
			Groups:       groups,
			Interactive:  true,
			Page:         page,
			PageSize:     pageSize,
			TotalCount:   totalCount,
			TotalPages:   totalPages,
			SortBy:       sortBy,
			SortOrder:    sortOrder,
			BaseURL:      fmt.Sprintf("/scans/runs/%d", id),
			StatusFilter: status,
		},
		Actions: actions,
	}

	h.render(w, "scan_results.html", data)
}

// HandleAction handles POST /scans/runs/{id}/action
func (h *Handler) HandleAction(w http.ResponseWriter, r *http.Request, runIDStr string) {
	if !h.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Action comes from query param (from button hx-post URL) or form
	action := r.URL.Query().Get("action")
	if action == "" {
		action = r.FormValue("action")
	}

	// Always preview first unless explicitly confirming
	confirm := r.FormValue("confirm") == "1"
	dryRun := !confirm

	selectAll := r.FormValue("select_all") == "1"
	statusFilter := r.FormValue("status_filter")
	groupIDsStr := r.FormValue("group_ids")
	priority := r.FormValue("remove-priority") // For remove action

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
			h.redirect(w, r, "/scans/runs/"+runIDStr)
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
		h.redirect(w, r, "/scans/runs/"+runIDStr)
		return
	}

	// Determine action type
	var actionType db.ActionType
	switch action {
	case "hardlink":
		actionType = db.ActionTypeHardlink
	case "reflink":
		actionType = db.ActionTypeReflink
	case "remove":
		actionType = db.ActionTypeRemove
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	result, err := h.scanner.ExecuteAction(r.Context(), runID, groupIDs, actionType, dryRun, priority)

	// For HTMX requests, show modal with results (or error)
	if r.Header.Get("HX-Request") == "true" {
		// Get CSRF token from cookie for the confirm form
		var csrfToken string
		if cookie, err := r.Cookie(csrfCookieName); err == nil {
			csrfToken = cookie.Value
		}

		var errorMsg string
		var output string
		if err != nil {
			errorMsg = err.Error()
			if result != nil {
				output = result.Output
			}
		} else {
			output = result.Output
		}

		h.renderActionResultModal(w, renderActionModalParams{
			Action:       action,
			Output:       output,
			DryRun:       dryRun,
			RedirectURL:  "/scans/runs/" + runIDStr,
			RunID:        runIDStr,
			GroupIDs:     groupIDsStr,
			SelectAll:    selectAll,
			StatusFilter: statusFilter,
			CSRFToken:    csrfToken,
			Priority:     priority,
			Error:        errorMsg,
		})
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.redirect(w, r, "/scans/runs/"+runIDStr)
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
	CSRFToken    string
	Priority     string // For remove action
	Error        string // Error message if action failed
}

// renderActionResultModal renders the action results modal
func (h *Handler) renderActionResultModal(w http.ResponseWriter, p renderActionModalParams) {
	var actionName, confirmBtnText, confirmBtnClass string
	switch p.Action {
	case "hardlink":
		actionName = "Hardlink"
		confirmBtnText = "Apply Hardlinks"
		confirmBtnClass = "btn btn-primary"
	case "reflink":
		actionName = "Reflink"
		confirmBtnText = "Apply Reflinks"
		confirmBtnClass = "btn btn-primary"
	case "remove":
		actionName = "Remove"
		confirmBtnText = "Remove Files"
		confirmBtnClass = "btn btn-danger"
	default:
		actionName = "Action"
		confirmBtnText = "Apply"
		confirmBtnClass = "btn btn-primary"
	}

	var title, description string
	if p.Error != "" {
		title = actionName + " Failed"
		description = "The operation failed:"
	} else if p.DryRun {
		title = actionName + " Preview"
		description = "The following operations would be performed:"
	} else {
		title = actionName + " Complete"
		description = "The following operations were performed:"
	}

	// Escape output to prevent XSS
	escapedOutput := html.EscapeString(p.Output)

	// Build the confirm form for previews (not shown on error)
	var confirmForm string
	if p.DryRun && p.Error == "" {
		selectAllValue := ""
		if p.SelectAll {
			selectAllValue = "1"
		}
		confirmForm = `<form method="POST" action="/scans/runs/` + p.RunID + `/action" style="display:inline;"
			hx-post="/scans/runs/` + p.RunID + `/action"
			hx-target="#modal-backdrop"
			hx-swap="outerHTML">
			<input type="hidden" name="` + csrfFormField + `" value="` + p.CSRFToken + `">
			<input type="hidden" name="action" value="` + p.Action + `">
			<input type="hidden" name="group_ids" value="` + html.EscapeString(p.GroupIDs) + `">
			<input type="hidden" name="select_all" value="` + selectAllValue + `">
			<input type="hidden" name="status_filter" value="` + html.EscapeString(p.StatusFilter) + `">
			<input type="hidden" name="remove-priority" value="` + html.EscapeString(p.Priority) + `">
			<input type="hidden" name="confirm" value="1">
			<button type="submit" class="` + confirmBtnClass + `">
				<span class="btn-text">` + confirmBtnText + `</span>
				<span class="btn-spinner"><span class="spinner"></span></span>
			</button>
		</form>`
	}

	// Build footer buttons and warning
	var footerButtons, warningHTML string
	if p.Error != "" {
		footerButtons = `<button class="btn" onclick="closeModal()">Close</button>`
	} else if p.DryRun {
		footerButtons = `<button class="btn" onclick="closeModal()">Cancel</button>` + confirmForm
		if p.Action == "remove" {
			warningHTML = `<p class="muted" style="margin:0.75rem 0 0;">Warning: this cannot be undone</p>`
		}
	} else {
		footerButtons = `<button class="btn" onclick="window.location.href='` + p.RedirectURL + `'">Done</button>`
	}

	// Build error banner if there's an error
	var errorBanner string
	if p.Error != "" {
		errorBanner = `<div class="error-banner" style="margin-bottom:1rem;">` + html.EscapeString(p.Error) + `</div>`
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	modalHTML := `<div id="modal-backdrop" class="modal-backdrop" onclick="closeModal()">
	<div class="modal" onclick="event.stopPropagation()">
		<div class="modal-header">
			<h3>` + title + `</h3>
			<button class="modal-close" onclick="closeModal()">&times;</button>
		</div>
		<div class="modal-body">
			<p>` + description + `</p>` + errorBanner + `
			<pre class="output">` + escapedOutput + `</pre>` + warningHTML + `
		</div>
		<div class="modal-footer">
			` + footerButtons + `
		</div>
	</div>
</div>
<script>
// Hide remove options modal if it was open
var removeModal = document.getElementById('remove-modal-backdrop');
if (removeModal) removeModal.style.display = 'none';
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
	if !h.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	h.scanner.CancelScan(runID)
	h.redirect(w, r, "/scans/runs/"+runIDStr)
}

// HandleDeleteFiles handles POST /scans/runs/{id}/delete-files for manual file deletion
func (h *Handler) HandleDeleteFiles(w http.ResponseWriter, r *http.Request, runIDStr string) {
	if !h.validateCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	// Always preview first unless explicitly confirming
	confirm := r.FormValue("confirm") == "1"
	dryRun := !confirm

	filePathsStr := r.FormValue("file_paths")
	filePaths := strings.Split(filePathsStr, "\n")

	var results []string
	var deletedCount, errorCount int

	for _, path := range filePaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}

		// Validate path is within allowed directories
		if len(h.cfg.AllowedPaths) > 0 && !h.cfg.IsPathAllowed(path) {
			results = append(results, fmt.Sprintf("Not allowed: %s", path))
			errorCount++
			continue
		}

		if dryRun {
			results = append(results, path)
		} else {
			if err := os.Remove(path); err != nil {
				results = append(results, fmt.Sprintf("Error: %s - %v", path, err))
				errorCount++
			} else {
				results = append(results, fmt.Sprintf("Deleted: %s", path))
				deletedCount++
			}
		}
	}

	// Get CSRF token for confirm form
	var csrfToken string
	if cookie, err := r.Cookie(csrfCookieName); err == nil {
		csrfToken = cookie.Value
	}

	h.renderDeleteFilesModal(w, deleteFilesModalParams{
		Output:      strings.Join(results, "\n"),
		DryRun:      dryRun,
		FilePaths:   filePathsStr,
		RunID:       runIDStr,
		CSRFToken:   csrfToken,
		DeleteCount: deletedCount,
		ErrorCount:  errorCount,
	})
}

// deleteFilesModalParams holds parameters for rendering the delete files modal
type deleteFilesModalParams struct {
	Output      string
	DryRun      bool
	FilePaths   string
	RunID       string
	CSRFToken   string
	DeleteCount int
	ErrorCount  int
}

// renderDeleteFilesModal renders the delete files result modal
func (h *Handler) renderDeleteFilesModal(w http.ResponseWriter, p deleteFilesModalParams) {
	var title, description string
	if p.DryRun {
		title = "Delete Files Preview"
		description = "The following files will be deleted:"
	} else {
		title = "Delete Files Complete"
		if p.ErrorCount > 0 {
			description = fmt.Sprintf("Deleted %d files with %d errors:", p.DeleteCount, p.ErrorCount)
		} else {
			description = fmt.Sprintf("Successfully deleted %d files:", p.DeleteCount)
		}
	}

	escapedOutput := html.EscapeString(p.Output)

	var confirmForm string
	if p.DryRun {
		confirmForm = `<form method="POST" action="/scans/runs/` + p.RunID + `/delete-files" style="display:inline;"
			hx-post="/scans/runs/` + p.RunID + `/delete-files"
			hx-target="#modal-backdrop"
			hx-swap="outerHTML">
			<input type="hidden" name="` + csrfFormField + `" value="` + p.CSRFToken + `">
			<input type="hidden" name="file_paths" value="` + html.EscapeString(p.FilePaths) + `">
			<input type="hidden" name="confirm" value="1">
			<button type="submit" class="btn btn-danger">
				<span class="btn-text">Delete Files</span>
				<span class="btn-spinner"><span class="spinner"></span></span>
			</button>
		</form>`
	}

	var footerButtons, warningHTML string
	if p.DryRun {
		footerButtons = `<button class="btn" onclick="closeModal()">Cancel</button>` + confirmForm
		warningHTML = `<p class="muted" style="margin:0.75rem 0 0;">Warning: this cannot be undone</p>`
	} else {
		footerButtons = `<button class="btn" onclick="window.location.reload()">Done</button>`
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
			<pre class="output">` + escapedOutput + `</pre>` + warningHTML + `
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

// parseSizeWithError parses a human-readable size string to bytes, returning an error if invalid
// Supports both decimal (MB, GB) and binary (MiB, GiB) units
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
		// Binary units (IEC) - check these first as they're longer
		{"PIB", 1 << 50},
		{"TIB", 1 << 40},
		{"GIB", 1 << 30},
		{"MIB", 1 << 20},
		{"KIB", 1 << 10},
		// Decimal units (SI)
		{"PB", 1e15},
		{"TB", 1e12},
		{"GB", 1e9},
		{"MB", 1e6},
		{"KB", 1e3},
		// Short forms (treat as decimal)
		{"P", 1e15},
		{"T", 1e12},
		{"G", 1e9},
		{"M", 1e6},
		{"K", 1e3},
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

	if n < 0 {
		return 0, fmt.Errorf("size cannot be negative")
	}

	return int64(n * float64(multiplier)), nil
}
