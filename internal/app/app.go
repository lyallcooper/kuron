// Package app provides shared application initialization logic used by both
// the server (Docker/CLI) and desktop (Wails) entry points.
package app

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lyallcooper/kuron/internal/config"
	"github.com/lyallcooper/kuron/internal/db"
	"github.com/lyallcooper/kuron/internal/fclones"
	"github.com/lyallcooper/kuron/internal/handlers"
	"github.com/lyallcooper/kuron/internal/scheduler"
	"github.com/lyallcooper/kuron/internal/services"
)

// ServerConfig contains options for creating the application server.
type ServerConfig struct {
	// Port to listen on. If 0, uses config default.
	Port int

	// FclonesBinary path override. If empty, uses system PATH.
	FclonesBinary string

	// Version string for display.
	Version string

	// Commit hash for display.
	Commit string

	// WebFS is the embedded filesystem containing web assets.
	WebFS embed.FS

	// BindAddress is the address to bind to. Defaults to "" (all interfaces).
	// Use "127.0.0.1" for desktop mode to only allow local connections.
	BindAddress string

	// DisableCSRF disables CSRF protection. Use for desktop mode where
	// the server only accepts local connections and CSRF isn't a concern.
	DisableCSRF bool
}

// Server wraps the HTTP server and associated resources.
type Server struct {
	HTTP      *http.Server
	Config    *config.Config
	Database  *db.DB
	Executor  *fclones.Executor
	Scanner   *services.Scanner
	Scheduler *scheduler.Scheduler
}

// CreateServer initializes all application components and returns a Server.
// Call Server.Cleanup() when done to release resources.
func CreateServer(cfg ServerConfig) (*Server, error) {
	// Load configuration from environment
	appCfg := config.Load()

	// Override port if specified
	if cfg.Port > 0 {
		appCfg.Port = cfg.Port
	}

	log.Printf("kuron starting...")
	log.Printf("  Database: %s", appCfg.DBPath)
	log.Printf("  Port: %d", appCfg.Port)

	// Initialize database
	database, err := db.Open(appCfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Load retention from DB if not set via env var
	if !appCfg.RetentionDaysFromEnv {
		if val, err := database.GetSetting("retention_days"); err == nil && val != "" {
			if days, err := strconv.Atoi(val); err == nil && days >= 1 && days <= 365 {
				appCfg.RetentionDays = days
			}
		}
	}
	log.Printf("  Retention: %d days", appCfg.RetentionDays)

	// Initialize fclones executor
	executor := fclones.NewExecutor()
	if cfg.FclonesBinary != "" {
		executor.SetBinaryPath(cfg.FclonesBinary)
	}

	// Check fclones is installed
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := executor.CheckInstalled(ctx); err != nil {
		log.Printf("Warning: fclones not found: %v", err)
		log.Printf("  Install fclones to enable scanning: https://github.com/pkolaczk/fclones")
	}
	cancel()

	// Initialize scanner service
	scanner := services.NewScanner(database, executor, appCfg.ScanTimeout, appCfg.FclonesCacheEnabled, appCfg.FclonesCachePath)

	// Initialize scheduler
	sched := scheduler.New(database, scanner)
	sched.Start()

	// Build version string
	versionStr := buildVersionString(cfg.Version, cfg.Commit)

	// Initialize handlers
	h, err := handlers.New(database, appCfg, executor, scanner, cfg.WebFS, versionStr, cfg.DisableCSRF)
	if err != nil {
		sched.Stop()
		database.Close()
		return nil, fmt.Errorf("failed to initialize handlers: %w", err)
	}

	// Start CSRF token cleanup
	handlers.StartCSRFCleanup()

	// Set up HTTP server
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	bindAddr := cfg.BindAddress
	addr := fmt.Sprintf("%s:%d", bindAddr, appCfg.Port)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // No timeout for SSE
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		HTTP:      server,
		Config:    appCfg,
		Database:  database,
		Executor:  executor,
		Scanner:   scanner,
		Scheduler: sched,
	}, nil
}

// Cleanup releases all resources held by the server.
func (s *Server) Cleanup() {
	if s.Scheduler != nil {
		s.Scheduler.Stop()
	}
	if s.Database != nil {
		s.Database.Close()
	}
}

// StartCleanupLoop starts a background goroutine that periodically cleans up old data.
// Returns a cancel function and a done channel.
func (s *Server) StartCleanupLoop() (cancel func(), done <-chan struct{}) {
	cleanupDone := make(chan struct{})
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())

	go func() {
		defer close(cleanupDone)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
				log.Printf("Running cleanup (retention: %d days)", s.Config.RetentionDays)
				if err := s.Database.CleanupOldData(s.Config.RetentionDays); err != nil {
					log.Printf("Cleanup error: %v", err)
				}
			}
		}
	}()

	return cleanupCancel, cleanupDone
}

func buildVersionString(version, commit string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	shortCommit := commit
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}
	if shortCommit == "" {
		shortCommit = "unknown"
	}
	return version + "-" + shortCommit
}
