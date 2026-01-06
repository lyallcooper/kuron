# kuron

A web interface for finding and removing duplicate files using [fclones](https://github.com/pkolaczk/fclones).

## Features

- **Duplicate Detection** - Scan directories to find duplicate files by content hash
- **Deduplication Actions** - Hardlink or reflink duplicates to reclaim disk space
- **Scheduled Scans** - Create jobs to run scans automatically on a cron schedule
- **Scan History** - View past scans, results, and actions taken

## Quick Start

### Docker (Recommended)

`compose.yaml`:
```yaml
services:
  kuron:
    image: ghcr.io/lyallcooper/kuron:latest
    container_name: kuron
    ports:
      - 8080:8080
    user: 1000:1000 # Must have permission for mounted volumes
    volumes:
      - ./kuron-data:/data
      - /path/to/media:/mnt/media:ro       # Read-only for scanning only
      - /path/to/downloads:/mnt/downloads  # Read-write for deduplication
    environment:
      - KURON_ALLOWED_PATHS=/mnt/media,/mnt/downloads
    restart: unless-stopped
```

Access at http://localhost:8080.

### From Source

Requires Go 1.24+ and [fclones](https://github.com/pkolaczk/fclones).

```bash
# Install fclones
# macOS: brew install fclones
# Linux: See https://github.com/pkolaczk/fclones#installation

# Build and run
go build -o kuron ./cmd/server
./kuron
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KURON_PORT` | `8080` | HTTP server port |
| `KURON_DB_PATH` | `./data/kuron.db` | SQLite database path |
| `KURON_RETENTION_DAYS` | `30` | Days to keep scan history (1-9999) |
| `KURON_SCAN_TIMEOUT` | `30m` | Maximum duration for a scan |
| `KURON_ALLOWED_PATHS` | *(unrestricted)* | Comma-separated paths to restrict scanning |

## Usage

1. **Quick Scan** - Run an ad-hoc scan from the dashboard by specifying paths and filters
2. **Create Jobs** - Set up scheduled scans with cron expressions for automated scanning
3. **Review Results** - View duplicate groups, expand to see file paths
4. **Take Action** - Select groups and choose Hardlink or Reflink (use dry run first to preview)
5. **View History** - Track all past scans and actions from the History page

### Hardlink vs Reflink

- **Hardlink** - Multiple filenames point to the same data on disk. Editing one file changes all. Works on any filesystem.
- **Reflink** - Copy-on-write clone. Files share data until modified, then diverge. Requires filesystem support (APFS, Btrfs, XFS, ZFS, etc.).

## License

MIT
