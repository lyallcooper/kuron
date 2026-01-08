# kuron

Fast, easy to use file deduplication. Available as desktop or server app. Powered by [fclones](https://github.com/pkolaczk/fclones).

<p align="center">
  <img src="/img/screenshot-1.png" width="400" alt="App dashboard">
  <img src="/img/screenshot-2.png" width="400" alt="Scan results page">
</p>

## Features

- **Detect duplicates**: Scan directories to find duplicate files
- **Dedupe**: Hardlink or reflink duplicates to reclaim disk space
- **Schedule Jobs**: Run scans automatically on a cron schedule
- **View History**: View past scans, results, and actions taken

## Quick Start

### Desktop

Download the latest release for your platform:

| Platform | File |
|----------|------|
| macOS (Apple Silicon) | [`Kuron-macos-arm64.dmg`](https://github.com/lyallcooper/kuron/releases/latest/download/Kuron-macos-arm64.dmg)\* |
| Linux | [`Kuron-linux-amd64.tar.gz`](https://github.com/lyallcooper/kuron/releases/latest/download/Kuron-linux-amd64.tar.gz) |
| Windows | [`Kuron-windows-amd64.zip`](https://github.com/lyallcooper/kuron/releases/latest/download/Kuron-windows-amd64.zip) |

\*Note for macOS: the app is not notarized by Apple, so after attempting to open you need to go into **System Settings** > **Privacy & Security**, then scroll down and click **Open Anyway** to open the app.

### Server

#### Docker (Recommended)

`compose.yaml`:
```yaml
services:
  kuron:
    image: ghcr.io/lyallcooper/kuron:latest
    container_name: kuron
    user: 1000:1000 # Must have permission for mounted volumes
    ports:
      - 8080:8080
    volumes:
      - ./kuron-data:/data
      - /path/to/media:/mnt/media          # A directory containing files to dedupe
      - /path/to/downloads:/mnt/downloads  # Another directory to dedupe
    environment:
      - KURON_ALLOWED_PATHS=/mnt/media,/mnt/downloads # Optional
    restart: unless-stopped
```

Access at http://localhost:8080.

#### Pre-built Binaries

Download the latest binary for your platform from [GitHub Releases](https://github.com/lyallcooper/kuron/releases).

Available platforms:
- Linux (`amd64`, `arm64`)
- macOS (`arm64`)

Requires [fclones](https://github.com/pkolaczk/fclones) `0.35.0` or newer.

#### From Source

Requires Go 1.24+ and [fclones](https://github.com/pkolaczk/fclones) `0.35.0` or newer.

```bash
# Install fclones (>= 0.35.0)
# macOS: brew install fclones
# Linux: See https://github.com/pkolaczk/fclones#installation

# Build and run
go build -o kuron ./cmd/server
./kuron
```

#### Environment Variable Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KURON_PORT` | `8080` | HTTP server port |
| `KURON_DB_PATH` | `./data/kuron.db`, or `/data/kuron.db` on docker | SQLite database path |
| `KURON_RETENTION_DAYS` | `30` | Days to keep scan history (1-9999) |
| `KURON_SCAN_TIMEOUT` | `30m` | Maximum duration for a scan |
| `KURON_ALLOWED_PATHS` | *(unrestricted)* | Comma-separated paths to restrict scanning |

## Usage

1. **Quick Scan**: Run an ad-hoc scan from the dashboard by specifying paths and filters
2. **Create Jobs**: Set up scheduled scans with cron expressions for automated scanning
3. **Review Results**: View duplicate groups, expand to see file paths
4. **Take Action**: Select groups and choose an action (all actions preview first)
5. **View History**: Track all past scans and actions from the History page

### Action Types

- **Hardlink**: Multiple filenames point to the same data on disk. Editing one file changes all. Works on any filesystem.
- **Reflink**: Copy-on-write clone. Files share data until modified, then diverge. Requires filesystem support (APFS, Btrfs, XFS, ZFS, etc.). NB: Files previously deduplicated via reflink will show up again on subsequent scans due to how fclones works.
- **Remove**: Delete duplicate files, keeping one per group based on priority (newest, oldest, most/least nested, etc.).
