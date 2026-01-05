package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SettingsData holds data for the settings template
type SettingsData struct {
	Title          string
	ActiveNav      string
	RetentionDays  int
	Version        string
	FclonesVersion string
	DBPath         string
	Error          string
	Success        string
}

// Settings handles GET /settings
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
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
		RetentionDays:  h.cfg.RetentionDays,
		Version:        "0.1.0",
		FclonesVersion: fclonesVersion,
		DBPath:         h.cfg.DBPath,
		Error:          r.URL.Query().Get("error"),
		Success:        r.URL.Query().Get("success"),
	}

	h.render(w, "settings.html", data)
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
			suggestions = append(suggestions, fullPath)
			if len(suggestions) >= 15 {
				break
			}
		}
	}

	// Return as JSON array
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("["))
	for i, s := range suggestions {
		if i > 0 {
			w.Write([]byte(","))
		}
		// Simple JSON string escaping
		escaped := strings.ReplaceAll(s, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		fmt.Fprintf(w, `"%s"`, escaped)
	}
	w.Write([]byte("]"))
}
