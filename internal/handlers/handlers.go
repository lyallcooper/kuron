package handlers

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/lyallcooper/kuron/internal/config"
	"github.com/lyallcooper/kuron/internal/db"
	"github.com/lyallcooper/kuron/internal/fclones"
	"github.com/lyallcooper/kuron/internal/services"
)

// Handler holds all HTTP handlers
type Handler struct {
	db          *db.DB
	cfg         *config.Config
	executor    fclones.ExecutorInterface
	scanner     *services.Scanner
	webFS       embed.FS
	funcMap     template.FuncMap
	staticFS    fs.FS
	version     string
	disableCSRF bool
}

// New creates a new Handler
func New(database *db.DB, cfg *config.Config, executor fclones.ExecutorInterface, scanner *services.Scanner, webFS embed.FS, version string, disableCSRF bool) (*Handler, error) {
	// Template functions
	funcMap := template.FuncMap{
		"formatBytes":     formatBytes,
		"formatTime":      formatTime,
		"timeAgo":         timeAgo,
		"truncateHash":    truncateHash,
		"joinPatterns":    joinPatterns,
		"joinLines":       joinLines,
		"derefInt64":      derefInt64,
		"derefInt":        derefInt,
		"add":             func(a, b int) int { return a + b },
		"subtract":        func(a, b int) int { return a - b },
		"formatSizeInput": formatSizeInput,
		"plural": func(n int, singular, plural string) string {
			if n == 1 {
				return singular
			}
			return plural
		},
		"csrfField": func(token string) template.HTML {
			return template.HTML(`<input type="hidden" name="` + csrfFormField + `" value="` + token + `">`)
		},
	}

	// Get static files
	staticFS, err := fs.Sub(webFS, "static")
	if err != nil {
		return nil, err
	}

	return &Handler{
		db:          database,
		cfg:         cfg,
		executor:    executor,
		scanner:     scanner,
		webFS:       webFS,
		funcMap:     funcMap,
		staticFS:    staticFS,
		version:     version,
		disableCSRF: disableCSRF,
	}, nil
}

// redirect performs a redirect appropriate for the context:
// - HTMX requests: HX-Redirect header (client-side navigation)
// - Desktop mode regular forms: JavaScript redirect (Wails webview compatibility)
// - Browser mode regular forms: HTTP 303 redirect
func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, url string) {
	// HTMX requests use HX-Redirect header
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Desktop mode: HTTP redirects don't work reliably through the Wails
	// webview proxy, so use JavaScript navigation instead
	if h.disableCSRF {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		// Using a minimal HTML page with immediate redirect
		fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><script>window.location.replace(%q);</script></head><body></body></html>`, url)
		return
	}

	// Regular browser: standard HTTP redirect
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// RegisterRoutes registers all HTTP routes
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(h.staticFS))))

	// Dashboard
	mux.HandleFunc("/", h.Dashboard)

	// Quick scan and scan results
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

	// API
	mux.HandleFunc("/api/paths/suggest", h.SuggestPaths)

	// SSE
	mux.HandleFunc("/sse/scan/", h.ScanProgressSSE)
}

// render executes a page template with the base layout
func (h *Handler) render(w http.ResponseWriter, pageName string, data any) {
	// Clone and parse base + specific page template
	tmpl, err := template.New("base.html").Funcs(h.funcMap).ParseFS(h.webFS, "templates/base.html", "templates/"+pageName)
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
	// Use decimal (SI) units to match fclones output
	const unit = 1000
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

// formatSizeInput formats bytes for form input fields (e.g., "1 GB", "500 MB")
// Uses decimal units (SI: 1000-based) for display
func formatSizeInput(bytes int64) string {
	if bytes == 0 {
		return ""
	}

	units := []struct {
		suffix string
		size   int64
	}{
		{"PB", 1e15},
		{"TB", 1e12},
		{"GB", 1e9},
		{"MB", 1e6},
		{"KB", 1e3},
	}

	// Find the largest unit where the value is < 1000 (max 3 digits)
	for _, u := range units {
		if bytes >= u.size {
			val := float64(bytes) / float64(u.size)
			if val < 1000 {
				// Format with up to 2 decimal places, trimming trailing zeros
				s := fmt.Sprintf("%.2f", val)
				s = strings.TrimRight(s, "0")
				s = strings.TrimRight(s, ".")
				return s + " " + u.suffix
			}
		}
	}

	// For values < 1000 bytes, just show bytes
	if bytes < 1000 {
		return fmt.Sprintf("%d B", bytes)
	}

	// Fallback (shouldn't normally reach here)
	return fmt.Sprintf("%d B", bytes)
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

// timeAgo returns a human-readable string describing how long ago a time was
func timeAgo(t any) string {
	var target time.Time

	switch v := t.(type) {
	case time.Time:
		target = v
	case *time.Time:
		if v == nil {
			return "-"
		}
		target = *v
	default:
		return "-"
	}

	duration := time.Since(target)

	if duration < time.Minute {
		return "just now"
	}
	if duration < time.Hour {
		mins := int(duration.Minutes())
		return fmt.Sprintf("%d min ago", mins)
	}
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		return fmt.Sprintf("%d hr ago", hours)
	}
	days := int(duration.Hours() / 24)
	if days < 7 {
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
	if days < 30 {
		weeks := days / 7
		if weeks == 1 {
			return "1 wk ago"
		}
		return fmt.Sprintf("%d wk ago", weeks)
	}
	if days < 365 {
		months := days / 30
		if months == 1 {
			return "1 mo ago"
		}
		return fmt.Sprintf("%d mo ago", months)
	}
	years := days / 365
	if years == 1 {
		return "1 yr ago"
	}
	return fmt.Sprintf("%d yr ago", years)
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

func joinLines(items []string) string {
	return strings.Join(items, "\n")
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
