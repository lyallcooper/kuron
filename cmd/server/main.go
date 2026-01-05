package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lyall/kuron/internal/config"
	"github.com/lyall/kuron/internal/db"
	"github.com/lyall/kuron/internal/fclones"
	"github.com/lyall/kuron/internal/handlers"
	"github.com/lyall/kuron/internal/scheduler"
	"github.com/lyall/kuron/internal/services"
)

//go:embed all:web
var webFS embed.FS

func main() {
	// Load configuration
	cfg := config.Load()

	log.Printf("kuron starting...")
	log.Printf("  Database: %s", cfg.DBPath)
	log.Printf("  Port: %d", cfg.Port)
	log.Printf("  Retention: %d days", cfg.RetentionDays)

	// Initialize database
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Initialize fclones executor
	executor := fclones.NewExecutor()

	// Check fclones is installed
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := executor.CheckInstalled(ctx); err != nil {
		log.Printf("Warning: fclones not found: %v", err)
		log.Printf("  Install fclones to enable scanning: https://github.com/pkolaczk/fclones")
	}
	cancel()

	// Initialize scanner service
	scanner := services.NewScanner(database, executor)

	// Initialize scheduler
	sched := scheduler.New(database, scanner)
	sched.Start()
	defer sched.Stop()

	// Initialize handlers
	h, err := handlers.New(database, cfg, executor, scanner, webFS)
	if err != nil {
		log.Fatalf("Failed to initialize handlers: %v", err)
	}

	// Set up HTTP server
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // No timeout for SSE
		IdleTimeout:  60 * time.Second,
	}

	// Start cleanup goroutine
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			log.Printf("Running cleanup (retention: %d days)", cfg.RetentionDays)
			if err := database.CleanupOldData(cfg.RetentionDays); err != nil {
				log.Printf("Cleanup error: %v", err)
			}
		}
	}()

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	// Start server
	log.Printf("Server listening on http://localhost:%d", cfg.Port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server stopped")
}
