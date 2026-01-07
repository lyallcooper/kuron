# kuron

An easy to use web interface for deduplicating files, powered by [fclones](https://github.com/pkolaczk/fclones).

## Features

- **Duplicate Detection** - Scan directories to find duplicate files by content hash
- **Deduplication** - Hardlink or reflink duplicates to reclaim disk space
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
      - /path/to/media:/mnt/media          # A directory containing files to dedupe
      - /path/to/downloads:/mnt/downloads  # Another directory to dedupe
    environment:
      - KURON_ALLOWED_PATHS=/mnt/media,/mnt/downloads # Optional
    restart: unless-stopped
```

Access at http://localhost:8080.

### Pre-built Binaries

Download the latest binary for your platform from [GitHub Releases](https://github.com/lyallcooper/kuron/releases).

Available platforms:
- Linux (amd64, arm64)
- macOS (amd64, arm64)

Requires [fclones](https://github.com/pkolaczk/fclones) 0.35.0 or newer.

```bash
# Install fclones (>= 0.35.0)
# macOS: brew install fclones
# Linux: See https://github.com/pkolaczk/fclones#installation

# Download and run (example for macOS arm64)
curl -LO https://github.com/lyallcooper/kuron/releases/latest/download/kuron-darwin-arm64
chmod +x kuron-darwin-arm64
./kuron-darwin-arm64
```

### From Source

Requires Go 1.24+ and [fclones](https://github.com/pkolaczk/fclones) 0.35.0 or newer.

```bash
# Install fclones (>= 0.35.0)
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
| `KURON_DB_PATH` | `./data/kuron.db` or `/data/kuron.db` for docker | SQLite database path |
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
