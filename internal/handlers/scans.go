package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/lyall/kuron/internal/db"
)

// ScansData holds data for the scans list template
type ScansData struct {
	Title     string
	ActiveNav string
	Configs   []*ScanConfigView
}

// ScanConfigView is a view model for scan configs
type ScanConfigView struct {
	*db.ScanConfig
	LastRun *db.ScanRun
}

// ScanFormData holds data for the scan form template
type ScanFormData struct {
	Title     string
	ActiveNav string
	Config    *db.ScanConfig
	Paths     []*db.ScanPath
}

// ScanResultsData holds data for the scan results template
type ScanResultsData struct {
	Title     string
	ActiveNav string
	Run       *db.ScanRun
	Groups    []*db.DuplicateGroup
}

// Scans handles GET /scans
func (h *Handler) Scans(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.CreateScanConfig(w, r)
		return
	}

	configs, err := h.db.ListScanConfigs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get last run for each config
	var views []*ScanConfigView
	for _, cfg := range configs {
		view := &ScanConfigView{ScanConfig: cfg}
		// TODO: Get last run for this config
		views = append(views, view)
	}

	data := ScansData{
		Title:     "Scans",
		ActiveNav: "scans",
		Configs:   views,
	}

	h.render(w, "scans.html", data)
}

// ScanForm handles GET/POST /scans/new and /scans/{id}/edit
func (h *Handler) ScanForm(w http.ResponseWriter, r *http.Request) {
	paths, err := h.db.ListScanPaths()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := ScanFormData{
		Title:     "New Scan Config",
		ActiveNav: "scans",
		Paths:     paths,
	}

	h.render(w, "scan_form.html", data)
}

// CreateScanConfig handles POST /scans
func (h *Handler) CreateScanConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	pathIDs := r.Form["paths"]
	minSizeStr := r.FormValue("min_size")
	maxSizeStr := r.FormValue("max_size")
	includeStr := r.FormValue("include_patterns")
	excludeStr := r.FormValue("exclude_patterns")

	// Parse path IDs
	var paths []int64
	for _, idStr := range pathIDs {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		paths = append(paths, id)
	}

	// Parse size
	minSize := parseSize(minSizeStr)
	var maxSize *int64
	if maxSizeStr != "" {
		ms := parseSize(maxSizeStr)
		if ms > 0 {
			maxSize = &ms
		}
	}

	// Parse patterns
	var includePatterns, excludePatterns []string
	if includeStr != "" {
		for _, p := range strings.Split(includeStr, ",") {
			if p = strings.TrimSpace(p); p != "" {
				includePatterns = append(includePatterns, p)
			}
		}
	}
	if excludeStr != "" {
		for _, p := range strings.Split(excludeStr, ",") {
			if p = strings.TrimSpace(p); p != "" {
				excludePatterns = append(excludePatterns, p)
			}
		}
	}

	cfg := &db.ScanConfig{
		Name:            name,
		Paths:           paths,
		MinSize:         minSize,
		MaxSize:         maxSize,
		IncludePatterns: includePatterns,
		ExcludePatterns: excludePatterns,
	}

	_, err := h.db.CreateScanConfig(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/scans", http.StatusSeeOther)
}

// QuickScan handles POST /scans/quick
func (h *Handler) QuickScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pathIDs := r.Form["paths"]
	if len(pathIDs) == 0 {
		http.Error(w, "No paths selected", http.StatusBadRequest)
		return
	}

	// Get actual paths
	var paths []string
	for _, idStr := range pathIDs {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		path, err := h.db.GetScanPath(id)
		if err != nil {
			continue
		}
		paths = append(paths, path.Path)
	}

	if len(paths) == 0 {
		http.Error(w, "No valid paths", http.StatusBadRequest)
		return
	}

	// Start scan
	run, err := h.scanner.StartScan(r.Context(), paths, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/scans/runs/"+strconv.FormatInt(run.ID, 10), http.StatusSeeOther)
}

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

	groups, err := h.db.ListDuplicateGroups(id, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := ScanResultsData{
		Title:     "Scan Results",
		ActiveNav: "scans",
		Run:       run,
		Groups:    groups,
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
	groupIDsStr := r.FormValue("group_ids")

	if groupIDsStr == "" {
		http.Redirect(w, r, "/scans/runs/"+runIDStr, http.StatusSeeOther)
		return
	}

	// Parse group IDs
	var groupIDs []int64
	for _, idStr := range strings.Split(groupIDsStr, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			continue
		}
		groupIDs = append(groupIDs, id)
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

	// For HTMX requests
	if r.Header.Get("HX-Request") == "true" {
		if dryRun {
			// Return modal content for dry runs
			h.renderDryRunModal(w, action, result.Output)
			return
		}
		// For actual runs, tell HTMX to redirect
		w.Header().Set("HX-Redirect", "/scans/runs/"+runIDStr)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/scans/runs/"+runIDStr, http.StatusSeeOther)
}

// renderDryRunModal renders the dry run results modal
func (h *Handler) renderDryRunModal(w http.ResponseWriter, action, output string) {
	actionName := "Hardlink"
	if action == "reflink" {
		actionName = "Reflink"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<div id="modal-backdrop" class="modal-backdrop" onclick="closeModal()">
	<div class="modal" onclick="event.stopPropagation()">
		<div class="modal-header">
			<h3>` + actionName + ` Preview (Dry Run)</h3>
			<button class="modal-close" onclick="closeModal()">&times;</button>
		</div>
		<div class="modal-body">
			<p>The following operations would be performed:</p>
			<pre class="output">` + output + `</pre>
		</div>
		<div class="modal-footer">
			<button class="btn" onclick="closeModal()">Close</button>
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
	w.Write([]byte(html))
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

// parseSize parses a human-readable size string to bytes
func parseSize(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return 0
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
		return 0
	}

	return int64(n * float64(multiplier))
}

// ScanConfigRoutes handles routes under /scans/{id}
func (h *Handler) ScanConfigRoutes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}

	idStr := parts[2]

	// Skip routes handled elsewhere
	if idStr == "new" || idStr == "quick" || idStr == "runs" {
		http.NotFound(w, r)
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
		case "run":
			if r.Method == http.MethodPost {
				h.RunScanConfig(w, r, id)
				return
			}
		case "edit":
			h.EditScanConfigForm(w, r, id)
			return
		case "delete":
			if r.Method == http.MethodPost || r.Method == http.MethodDelete {
				h.DeleteScanConfig(w, r, id)
				return
			}
		}
	}

	// GET /scans/{id} = view config (redirect to edit for now)
	http.Redirect(w, r, "/scans/"+idStr+"/edit", http.StatusSeeOther)
}

// RunScanConfig handles POST /scans/{id}/run
func (h *Handler) RunScanConfig(w http.ResponseWriter, r *http.Request, id int64) {
	config, err := h.db.GetScanConfig(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get actual paths from path IDs
	var paths []string
	for _, pathID := range config.Paths {
		path, err := h.db.GetScanPath(pathID)
		if err != nil {
			continue
		}
		paths = append(paths, path.Path)
	}

	if len(paths) == 0 {
		http.Error(w, "No valid paths in scan configuration", http.StatusBadRequest)
		return
	}

	// Start scan
	run, err := h.scanner.StartScan(r.Context(), paths, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/scans/runs/"+strconv.FormatInt(run.ID, 10), http.StatusSeeOther)
}

// EditScanConfigForm handles GET /scans/{id}/edit
func (h *Handler) EditScanConfigForm(w http.ResponseWriter, r *http.Request, id int64) {
	config, err := h.db.GetScanConfig(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	paths, err := h.db.ListScanPaths()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := ScanFormData{
		Title:     "Edit Scan Config",
		ActiveNav: "scans",
		Config:    config,
		Paths:     paths,
	}

	h.render(w, "scan_form.html", data)
}

// DeleteScanConfig handles DELETE /scans/{id}
func (h *Handler) DeleteScanConfig(w http.ResponseWriter, r *http.Request, id int64) {
	err := h.db.DeleteScanConfig(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/scans", http.StatusSeeOther)
}
