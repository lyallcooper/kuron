package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// SettingsData holds data for the settings template
type SettingsData struct {
	Title          string
	ActiveNav      string
	Paths          []*PathView
	RetentionDays  int
	Version        string
	FclonesVersion string
	DBPath         string
	Error          string
	Success        string
}

// PathView extends ScanPath
type PathView struct {
	ID        int64
	Path      string
	FromEnv   bool
	CreatedAt string
}

// Settings handles GET /settings
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	paths, err := h.db.ListScanPaths()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to views
	var pathViews []*PathView
	for _, p := range paths {
		pathViews = append(pathViews, &PathView{
			ID:        p.ID,
			Path:      p.Path,
			FromEnv:   p.FromEnv,
			CreatedAt: p.CreatedAt.Format("2006-01-02"),
		})
	}

	// Get fclones version
	fclonesVersion := "not found"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if version, err := h.executor.Version(ctx); err == nil {
		fclonesVersion = version
	}

	data := SettingsData{
		Title:          "Settings",
		ActiveNav:      "settings",
		Paths:          pathViews,
		RetentionDays:  h.cfg.RetentionDays,
		Version:        "0.1.0",
		FclonesVersion: fclonesVersion,
		DBPath:         h.cfg.DBPath,
		Error:          r.URL.Query().Get("error"),
		Success:        r.URL.Query().Get("success"),
	}

	h.render(w, "settings.html", data)
}

// AddPath handles POST /settings/paths
func (h *Handler) AddPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	path := strings.TrimSpace(r.FormValue("path"))
	if path == "" {
		http.Redirect(w, r, "/settings?error=Path cannot be empty", http.StatusSeeOther)
		return
	}

	// Validate path exists
	info, err := os.Stat(path)
	if err != nil {
		http.Redirect(w, r, "/settings?error=Path does not exist: "+path, http.StatusSeeOther)
		return
	}
	if !info.IsDir() {
		http.Redirect(w, r, "/settings?error=Path is not a directory: "+path, http.StatusSeeOther)
		return
	}

	// Add path
	_, err = h.db.CreateScanPath(path, false)
	if err != nil {
		http.Redirect(w, r, "/settings?error="+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings?success=Path added successfully", http.StatusSeeOther)
}

// AddPathInline handles POST /settings/paths/add-inline (HTMX)
func (h *Handler) AddPathInline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get currently checked paths to preserve them
	checkedPaths := make(map[int64]bool)
	for _, idStr := range r.Form["paths"] {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			checkedPaths[id] = true
		}
	}

	path := strings.TrimSpace(r.FormValue("new_path"))
	if path == "" {
		// Return current paths list preserving checked state
		h.renderPathsListWithCheckedMap(w, checkedPaths)
		return
	}

	// Validate path exists
	info, err := os.Stat(path)
	if err != nil {
		h.renderPathsListWithCheckedMap(w, checkedPaths)
		return
	}
	if !info.IsDir() {
		h.renderPathsListWithCheckedMap(w, checkedPaths)
		return
	}

	// Add path
	newPath, err := h.db.CreateScanPath(path, false)
	if err != nil {
		h.renderPathsListWithCheckedMap(w, checkedPaths)
		return
	}

	// Mark the new path as checked too
	checkedPaths[newPath.ID] = true

	// Return updated paths list with preserved + new path checked
	h.renderPathsListWithCheckedMap(w, checkedPaths)
}

// renderPathsListWithCheckedMap renders paths list with specified paths checked
func (h *Handler) renderPathsListWithCheckedMap(w http.ResponseWriter, checkedPaths map[int64]bool) {
	paths, _ := h.db.ListScanPaths()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if len(paths) == 0 {
		w.Write([]byte(`<p class="form-help" id="no-paths-msg">No paths configured yet.</p>`))
		return
	}

	for _, p := range paths {
		checked := ""
		if checkedPaths[p.ID] {
			checked = " checked"
		}
		locked := ""
		if p.FromEnv {
			locked = ` <span class="locked">(from env)</span>`
		}
		fmt.Fprintf(w, `<div class="form-checkbox">
    <input type="checkbox" name="paths" value="%d" id="path-%d"%s>
    <label for="path-%d">%s%s</label>
</div>
`, p.ID, p.ID, checked, p.ID, p.Path, locked)
	}
}

// DeletePath handles POST /settings/paths/{id}/delete
func (h *Handler) DeletePath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}

	idStr := parts[3]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Check if delete route
	if len(parts) >= 5 && parts[4] == "delete" && r.Method == http.MethodPost {
		if err := h.db.DeleteScanPath(id); err != nil {
			http.Redirect(w, r, "/settings?error="+err.Error(), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/settings?success=Path removed", http.StatusSeeOther)
		return
	}

	http.NotFound(w, r)
}
