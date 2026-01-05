package handlers

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/lyall/kuron/internal/config"
	"github.com/lyall/kuron/internal/db"
	"github.com/lyall/kuron/internal/fclones"
	"github.com/lyall/kuron/internal/services"
)

// Handler holds all HTTP handlers
type Handler struct {
	db        *db.DB
	cfg       *config.Config
	executor  *fclones.Executor
	scanner   *services.Scanner
	webFS     embed.FS
	funcMap   template.FuncMap
	staticFS  fs.FS
}

// New creates a new Handler
func New(database *db.DB, cfg *config.Config, executor *fclones.Executor, scanner *services.Scanner, webFS embed.FS) (*Handler, error) {
	// Template functions
	funcMap := template.FuncMap{
		"formatBytes":   formatBytes,
		"formatTime":    formatTime,
		"truncateHash":  truncateHash,
		"joinPatterns":  joinPatterns,
		"containsPath":  containsPath,
		"derefInt64":    derefInt64,
	}

	// Get static files
	staticFS, err := fs.Sub(webFS, "web/static")
	if err != nil {
		return nil, err
	}

	return &Handler{
		db:       database,
		cfg:      cfg,
		executor: executor,
		scanner:  scanner,
		webFS:    webFS,
		funcMap:  funcMap,
		staticFS: staticFS,
	}, nil
}

// RegisterRoutes registers all HTTP routes
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(h.staticFS))))

	// Dashboard
	mux.HandleFunc("/", h.Dashboard)

	// Scans
	mux.HandleFunc("/scans", h.Scans)
	mux.HandleFunc("/scans/new", h.ScanForm)
	mux.HandleFunc("/scans/quick", h.QuickScan)
	mux.HandleFunc("/scans/runs/", h.ScanResults)

	// Jobs
	mux.HandleFunc("/jobs", h.Jobs)
	mux.HandleFunc("/jobs/new", h.JobForm)
	mux.HandleFunc("/jobs/", h.JobRoutes)

	// History
	mux.HandleFunc("/history", h.History)

	// Settings
	mux.HandleFunc("/settings", h.Settings)
	mux.HandleFunc("/settings/paths", h.AddPath)
	mux.HandleFunc("/settings/paths/", h.DeletePath)

	// SSE
	mux.HandleFunc("/sse/scan/", h.ScanProgressSSE)
}

// render executes a page template with the base layout
func (h *Handler) render(w http.ResponseWriter, pageName string, data any) {
	// Clone and parse base + specific page template
	tmpl, err := template.New("base.html").Funcs(h.funcMap).ParseFS(h.webFS, "web/templates/base.html", "web/templates/"+pageName)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Template functions

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return formatInt(bytes) + " B"
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return formatFloat(float64(bytes)/float64(div)) + " " + []string{"KB", "MB", "GB", "TB", "PB"}[exp]
}

func formatInt(n int64) string {
	str := ""
	for n > 0 || str == "" {
		if len(str) > 0 && len(str)%4 == 3 {
			str = "," + str
		}
		str = string('0'+byte(n%10)) + str
		n /= 10
	}
	return str
}

func formatFloat(f float64) string {
	if f >= 100 {
		return formatInt(int64(f))
	}
	if f >= 10 {
		return formatInt(int64(f*10)/10) + "." + string('0'+byte(int64(f*10)%10))
	}
	return formatInt(int64(f*100)/100) + "." + string('0'+byte(int64(f*100)/10%10)) + string('0'+byte(int64(f*100)%10))
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

func truncateHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12] + "..."
	}
	return hash
}

func joinPatterns(patterns []string) string {
	return strings.Join(patterns, ", ")
}

func containsPath(paths []int64, id int64) bool {
	for _, p := range paths {
		if p == id {
			return true
		}
	}
	return false
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
