package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lyallcooper/kuron/internal/app"
	"github.com/lyallcooper/kuron/internal/webfs"
)

// Version info - injected at build time via ldflags
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	// Create server using shared app package
	server, err := app.CreateServer(app.ServerConfig{
		Version: version,
		Commit:  commit,
		WebFS:   webfs.FS,
	})
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer server.Cleanup()

	// Start cleanup loop
	cleanupCancel, cleanupDone := server.StartCleanupLoop()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Server listening on http://localhost:%d", server.Config.Port)
		if err := server.HTTP.ListenAndServe(); err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for signal or server error
	select {
	case err := <-serverErr:
		log.Fatalf("Server error: %v", err)
	case <-sigChan:
		log.Println("Shutting down...")
	}

	// Cancel cleanup goroutine and wait for it
	cleanupCancel()
	<-cleanupDone

	// Shutdown server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.HTTP.Shutdown(shutdownCtx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
