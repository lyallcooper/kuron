package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/lyallcooper/kuron/internal/app"
	"github.com/lyallcooper/kuron/internal/webfs"
)

// Version info - injected at build time via ldflags
var (
	version = "dev"
	commit  = "unknown"
)

//go:embed all:build/appicon.png
var iconFS embed.FS

func main() {
	// Set desktop-specific defaults before loading config
	setDesktopDefaults()

	// Find available port for internal server
	port, err := findAvailablePort()
	if err != nil {
		log.Fatalf("Failed to find available port: %v", err)
	}

	// Find bundled fclones binary
	fclonesBinary := findBundledFclones()
	if fclonesBinary != "" {
		log.Printf("Using bundled fclones: %s", fclonesBinary)
	}

	// Create the internal HTTP server
	server, err := app.CreateServer(app.ServerConfig{
		Port:          port,
		FclonesBinary: fclonesBinary,
		Version:       version,
		Commit:        commit,
		WebFS:         webfs.FS,
		BindAddress:   "127.0.0.1", // Only local connections
		DisableCSRF:   true,        // CSRF not needed for desktop app
	})
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Start cleanup loop
	cleanupCancel, cleanupDone := server.StartCleanupLoop()

	// Create reverse proxy to internal server
	targetURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Read app icon
	iconData, err := iconFS.ReadFile("build/appicon.png")
	if err != nil {
		log.Printf("Warning: Could not load app icon: %v", err)
	}

	// Create Wails application
	desktopApp := NewApp()

	err = wails.Run(&options.App{
		Title:     "Kuron",
		Width:     1200,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Handler: proxy,
		},
		OnStartup: func(ctx context.Context) {
			desktopApp.startup(ctx)
			// Start HTTP server in background
			go func() {
				log.Printf("Internal server listening on http://127.0.0.1:%d", port)
				if err := server.HTTP.ListenAndServe(); err != http.ErrServerClosed {
					log.Printf("HTTP server error: %v", err)
				}
			}()
		},
		OnShutdown: func(ctx context.Context) {
			log.Println("Shutting down...")
			server.HTTP.Shutdown(context.Background())
			cleanupCancel()
			<-cleanupDone
			server.Cleanup()
			log.Println("Shutdown complete")
		},
		Bind: []interface{}{
			desktopApp,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: false,
			},
			About: &mac.AboutInfo{
				Title:   "Kuron",
				Message: fmt.Sprintf("Duplicate File Finder\n\nVersion: %s", buildVersionString()),
				Icon:    iconData,
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})

	if err != nil {
		log.Fatalf("Wails error: %v", err)
	}
}

// findAvailablePort finds an available TCP port on localhost.
func findAvailablePort() (int, error) {
	// Try preferred port first
	preferredPort := 18080
	if isPortAvailable(preferredPort) {
		return preferredPort, nil
	}

	// Otherwise find any available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// isPortAvailable checks if a port is available on localhost.
func isPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// setDesktopDefaults sets environment variables for desktop-appropriate defaults
// if they're not already set.
func setDesktopDefaults() {
	// Set default DB path to user's app data directory
	if os.Getenv("KURON_DB_PATH") == "" {
		dataDir := getAppDataDir()
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			log.Printf("Warning: Could not create data directory: %v", err)
		}
		os.Setenv("KURON_DB_PATH", filepath.Join(dataDir, "kuron.db"))
	}
}

// getAppDataDir returns the platform-appropriate application data directory.
func getAppDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Kuron")
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Kuron")
	default: // Linux and others
		if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
			return filepath.Join(xdgData, "kuron")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "kuron")
	}
}

// findBundledFclones looks for a bundled fclones binary.
func findBundledFclones() string {
	// 1. Check environment variable override
	if envPath := os.Getenv("KURON_FCLONES_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	// 2. Look relative to executable (bundled app)
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}
	execDir := filepath.Dir(execPath)

	var candidates []string

	switch runtime.GOOS {
	case "darwin":
		// Inside .app bundle: Kuron.app/Contents/MacOS/Kuron
		// Resources at: Kuron.app/Contents/Resources/
		candidates = []string{
			filepath.Join(execDir, "..", "Resources", "fclones"),
			filepath.Join(execDir, "fclones"),
		}
	case "windows":
		candidates = []string{
			filepath.Join(execDir, "fclones.exe"),
		}
	default: // Linux
		candidates = []string{
			filepath.Join(execDir, "fclones"),
			filepath.Join(execDir, "..", "lib", "kuron", "fclones"),
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// 3. Fall back to system PATH
	if path, err := exec.LookPath("fclones"); err == nil {
		return path
	}

	return "" // Let executor handle the error
}

// buildVersionString creates a display version string.
func buildVersionString() string {
	if version == "dev" {
		return "Development"
	}
	return version
}
