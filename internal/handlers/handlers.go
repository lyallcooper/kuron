package handlers

import (
	"embed"
	"html/template"
	"io/fs"
	"math"
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
		"add":           func(a, b int) int { return a + b },
		"subtract":      func(a, b int) int { return a - b },
		"plural":        func(n int, singular, plural string) string { if n == 1 { return singular }; return plural },
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
	mux.HandleFunc("/scans/", h.ScanConfigRoutes)

	// Jobs
	mux.HandleFunc("/jobs", h.Jobs)
	mux.HandleFunc("/jobs/new", h.JobForm)
	mux.HandleFunc("/jobs/", h.JobRoutes)

	// History
	mux.HandleFunc("/history", h.History)

	// Settings
	mux.HandleFunc("/settings", h.Settings)
	mux.HandleFunc("/settings/paths", h.AddPath)
	mux.HandleFunc("/settings/paths/add-inline", h.AddPathInline)
	mux.HandleFunc("/settings/paths/", h.DeletePath)

	// API
	mux.HandleFunc("/api/paths/suggest", h.SuggestPaths)

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
		return formatInt(int64(math.Round(f)))
	}
	if f >= 10 {
		r := int64(math.Round(f * 10))
		d := r % 10
		if d == 0 {
			return formatInt(r / 10)
		}
		return formatInt(r/10) + "." + string('0'+byte(d))
	}
	r := int64(math.Round(f * 100))
	d1 := r / 10 % 10
	d2 := r % 10
	if d1 == 0 && d2 == 0 {
		return formatInt(r / 100)
	}
	if d2 == 0 {
		return formatInt(r/100) + "." + string('0'+byte(d1))
	}
	return formatInt(r/100) + "." + string('0'+byte(d1)) + string('0'+byte(d2))
}

func formatTime(t any) string {
	switch v := t.(type) {
	case time.Time:
		return v.Format("2006-01-02 15:04")
	case *time.Time:
		if v == nil {
			return "-"
		}
		return v.Format("2006-01-02 15:04")
	case string:
		// Parse string timestamp from SQLite
		parsed, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", v)
		if err != nil {
			parsed, err = time.Parse("2006-01-02 15:04:05-07:00", v)
		}
		if err != nil {
			parsed, err = time.Parse("2006-01-02T15:04:05Z07:00", v)
		}
		if err != nil {
			return v // Return raw string if parsing fails
		}
		return parsed.Format("2006-01-02 15:04")
	default:
		return "-"
	}
}

func truncateHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
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
