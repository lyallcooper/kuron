# kuron

A web interface for finding and removing duplicate files using [fclones](https://github.com/pkolaczk/fclones).

## Features

- **Duplicate Detection** - Scan directories to find duplicate files by content hash
- **Deduplication Actions** - Hardlink or reflink duplicates to reclaim disk space
- **Dry Run Preview** - See exactly what operations will be performed before committing
- **Scan Configurations** - Save and reuse scan settings
- **Scheduled Scans** - Run scans automatically on a cron schedule
- **Real-time Progress** - Watch scan progress via server-sent events
- **Dark Mode** - Automatic light/dark theme based on system preference

## Requirements

- [fclones](https://github.com/pkolaczk/fclones) - Must be installed and available in PATH
- Go 1.22+ (for building from source)

## Quick Start

### Docker (Recommended)

```bash
# Clone and start
git clone https://github.com/lyall/kuron.git
cd kuron
docker compose up -d

# Access at http://localhost:8080
```

Mount your directories in `docker-compose.yml`:

```yaml
volumes:
  - /path/to/media:/mnt/media:ro       # Read-only for scanning only
  - /path/to/downloads:/mnt/downloads  # Read-write for deduplication
```

### From Source

```bash
# Install fclones first
# macOS: brew install fclones
# Linux: See https://github.com/pkolaczk/fclones#installation

# Build and run
go build -o kuron ./cmd/server
./kuron
```

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `KURON_PORT` | `8080` | HTTP server port |
| `KURON_DB_PATH` | `./data/kuron.db` | SQLite database path |
| `KURON_SCAN_PATHS` | - | Comma-separated paths (locked in UI) |
| `KURON_RETENTION_DAYS` | `30` | Days to keep scan history |

## Usage

1. **Add Paths** - Go to Settings and add directories to scan
2. **Run Scan** - Use Quick Scan on the dashboard or create a saved configuration
3. **Review Results** - Expand duplicate groups to see all file paths
4. **Take Action** - Select groups and choose Hardlink or Reflink with dry run enabled
5. **Confirm** - Review the preview, then uncheck dry run to execute

### Hardlink vs Reflink

- **Hardlink** - Multiple filenames point to the same data on disk. Editing one file changes all. Works on any filesystem.
- **Reflink** - Copy-on-write clone. Files share data until modified, then diverge. Requires APFS, Btrfs, or XFS.

## Tech Stack

- Go (standard library + SQLite driver)
- SQLite (embedded database)
- HTMX (frontend interactivity)
- fclones (duplicate detection engine)

## License

MIT
