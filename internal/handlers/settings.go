package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SettingsData holds data for the settings template
type SettingsData struct {
	Title             string
	ActiveNav         string
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

// Settings handles GET/POST /settings
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.UpdateSettings(w, r)
		return
	}

	// Get fclones version
	fclonesVersion := "not found"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if version, err := h.executor.Version(ctx); err == nil {
		fclonesVersion = version
	}

	data := SettingsData{
		Title:             "Settings",
		ActiveNav:         "settings",
		RetentionDays:     h.cfg.RetentionDays,
		RetentionEditable: !h.cfg.RetentionDaysFromEnv,
		Version:           "0.1.0",
		FclonesVersion:    fclonesVersion,
		DBPath:            h.cfg.DBPath,
		Port:              h.cfg.Port,
		AllowedPaths:      h.cfg.AllowedPaths,
		Error:             r.URL.Query().Get("error"),
		Success:           r.URL.Query().Get("success"),
	}

	h.render(w, "settings.html", data)
}

// UpdateSettings handles POST /settings
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	// Only allow updating if not set via env var
	if h.cfg.RetentionDaysFromEnv {
		http.Redirect(w, r, "/settings?error=Retention+is+set+via+environment+variable", http.StatusSeeOther)
		return
	}

	retentionStr := r.FormValue("retention_days")
	retention, err := strconv.Atoi(retentionStr)
	if err != nil || retention < 1 || retention > 9999 {
		http.Redirect(w, r, "/settings?error=Retention+must+be+between+1+and+9999+days", http.StatusSeeOther)
		return
	}

	// Save to database
	if err := h.db.SetSetting("retention_days", retentionStr); err != nil {
		http.Redirect(w, r, "/settings?error=Failed+to+save+setting", http.StatusSeeOther)
		return
	}

	// Update in-memory config
	h.cfg.RetentionDays = retention

	http.Redirect(w, r, "/settings?success=Settings+saved", http.StatusSeeOther)
}

// SuggestPaths handles GET /api/paths/suggest?prefix=...
// Used for path autocomplete in job and quick scan forms
func (h *Handler) SuggestPaths(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	if prefix == "" || !strings.HasPrefix(prefix, "/") {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	// Clean the prefix
	prefix = filepath.Clean(prefix)

	// Check if the prefix is within allowed paths
	if !h.cfg.IsPathAllowed(prefix) && !h.isAllowedPathPrefix(prefix) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	// Determine the directory to list and the partial name to match
	var dir, partial string
	if strings.HasSuffix(r.URL.Query().Get("prefix"), "/") {
		// User typed a trailing slash, list contents of that directory
		dir = prefix
		partial = ""
	} else {
		// User is typing a name, list parent directory and filter
		dir = filepath.Dir(prefix)
		partial = filepath.Base(prefix)
	}

	// Read directory contents
	entries, err := os.ReadDir(dir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	// Filter to directories that match the partial name
	var suggestions []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden directories
		if strings.HasPrefix(name, ".") {
			continue
		}
		// Match prefix (case-insensitive)
		if partial == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(partial)) {
			fullPath := filepath.Join(dir, name)
			// Only suggest paths within allowed paths
			if h.cfg.IsPathAllowed(fullPath) || h.isAllowedPathPrefix(fullPath) {
				suggestions = append(suggestions, fullPath)
				if len(suggestions) >= 15 {
					break
				}
			}
		}
	}

	// Return as JSON array
	w.Header().Set("Content-Type", "application/json")
	data, _ := json.Marshal(suggestions)
	w.Write(data)
}

// isAllowedPathPrefix checks if a path is a prefix of any allowed path.
// This allows browsing directories that lead to allowed paths.
func (h *Handler) isAllowedPathPrefix(path string) bool {
	if len(h.cfg.AllowedPaths) == 0 {
		return true
	}

	for _, allowed := range h.cfg.AllowedPaths {
		if strings.HasPrefix(allowed, path) || strings.HasPrefix(allowed+"/", path) {
			return true
		}
	}
	return false
}
