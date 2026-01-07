# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
# Build the binary (dev version)
go build -o kuron ./cmd/server

# Build with version info (for releases)
VERSION=$(git describe --tags --always)
COMMIT=$(git rev-parse --short HEAD)
go build -ldflags="-X main.version=${VERSION} -X main.commit=${COMMIT}" -o kuron ./cmd/server

# Run with hot reload (requires air: go install github.com/air-verse/air@latest)
air

# Run directly
go run ./cmd/server

# Docker build with version
docker build --build-arg VERSION=${VERSION} --build-arg COMMIT=${COMMIT} -t kuron .
```

## Architecture Overview

Kuron is a web interface for finding and removing duplicate files using the [fclones](https://github.com/pkolaczk/fclones) CLI tool. It provides scheduled scanning, history tracking, and deduplication actions (hardlink/reflink).

### Key Components

- **cmd/server/main.go**: Application entrypoint. Initializes database, fclones executor, scanner service, scheduler, and HTTP handlers. Embeds web assets via `//go:embed all:web`.

- **internal/fclones/executor.go**: Wrapper around the fclones CLI. Executes `fclones group` for scanning and `fclones link`/`fclones dedupe` for deduplication. Parses JSON output and stderr progress updates.

- **internal/services/scanner.go**: Orchestrates scan operations. Manages active scans with cancellation, broadcasts progress via SSE to subscribers, stores results in database.

- **internal/scheduler/scheduler.go**: Cron-based job scheduler using robfig/cron. Checks enabled jobs every minute, starts scans, and optionally executes post-scan actions (hardlink/reflink).

- **internal/handlers/**: HTTP handlers for the web UI. Uses Go html/template with base layout pattern. Routes registered in `handlers.go`.

- **internal/db/**: SQLite database layer with migrations. Uses WAL mode. Supports both CGO (go-sqlite3) and pure Go (modernc.org/sqlite) drivers via build tags.

### Web Frontend

- Located in `cmd/server/web/` (embedded at build time)
- Uses HTMX for dynamic updates without full page reloads
- SSE for real-time scan progress (`/sse/scan/:id`)
- Templates use `base.html` as layout wrapper

### Database Build Tags

The project supports two SQLite drivers:
- Default (CGO): `github.com/mattn/go-sqlite3`
- Pure Go: Build with `-tags nocgo` to use `modernc.org/sqlite` (used for ARM64 GitHub Actions builds to avoid CGO cross-compilation)

See `internal/db/driver_cgo.go` and `internal/db/driver_nocgo.go` for the build tag configuration.

## Error Handling in Handlers

**Never use `http.Error()` for user-facing validation errors.** This displays a plain text error page which is poor UX.

Instead, re-render the form template with an error message and preserve user input:

```go
// Good: Re-render form with error banner
renderError := func(errMsg string) {
    data := FormData{
        Job:   job,  // Preserve user input
        Error: errMsg,
    }
    h.render(w, "form.html", data)
}

if err != nil {
    renderError(err.Error())
    return
}
```

```go
// Bad: Plain text error page
if err != nil {
    http.Error(w, err.Error(), http.StatusBadRequest)
    return
}
```

Reserve `http.Error()` only for:
- CSRF failures (security, not user error)
- Internal server errors that can't be recovered
- API endpoints returning JSON errors

## Configuration

Environment variables (see README.md for full list):
- `KURON_PORT` (default: 8080)
- `KURON_DB_PATH` (default: ./data/kuron.db)
- `KURON_RETENTION_DAYS` (default: 30)
- `KURON_SCAN_TIMEOUT` (default: 30m)
- `KURON_ALLOWED_PATHS` (comma-separated paths to restrict scanning)
