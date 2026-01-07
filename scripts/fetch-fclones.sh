#!/bin/bash
set -euo pipefail

# Fetch fclones binaries for bundling in desktop app
# Usage: ./scripts/fetch-fclones.sh [version] [output_dir] [platform]
#
# NOTE: macOS binaries are not provided by fclones releases.
# For macOS, use build-fclones-macos.sh to build from source via Cargo.

FCLONES_VERSION="${1:-0.35.0}"
OUTPUT_DIR="${2:-build/fclones}"
PLATFORM="${3:-}"

echo "Fetching fclones v${FCLONES_VERSION}..."

mkdir -p "$OUTPUT_DIR"

GITHUB_BASE="https://github.com/pkolaczk/fclones/releases/download/v${FCLONES_VERSION}"

# Get asset name for platform (based on actual release assets)
# macOS binaries are NOT available - must build from source
get_asset_name() {
    local platform="$1"
    case "$platform" in
        linux-amd64)
            echo "fclones-${FCLONES_VERSION}-linux-glibc-x86_64.tar.gz"
            ;;
        linux-musl-amd64)
            echo "fclones-${FCLONES_VERSION}-linux-musl-x86_64.tar.gz"
            ;;
        windows-amd64)
            echo "fclones-${FCLONES_VERSION}-windows-x86_64.zip"
            ;;
        *)
            echo ""
            ;;
    esac
}

fetch_platform() {
    local platform="$1"
    local asset
    asset=$(get_asset_name "$platform")

    if [ -z "$asset" ]; then
        echo "Error: Unknown or unsupported platform '$platform'"
        echo "Available platforms: linux-amd64 linux-musl-amd64 windows-amd64"
        echo ""
        echo "NOTE: macOS binaries must be built from source using Cargo."
        echo "      See build-fclones-macos.sh or run: cargo install fclones"
        return 1
    fi

    local platform_dir="$OUTPUT_DIR/$platform"
    local binary_name="fclones"
    if [[ "$platform" == windows-* ]]; then
        binary_name="fclones.exe"
    fi

    # Skip if already exists
    if [ -f "$platform_dir/$binary_name" ]; then
        echo "  $platform: Already exists, skipping"
        return 0
    fi

    mkdir -p "$platform_dir"

    local url="${GITHUB_BASE}/${asset}"
    echo "  $platform: Downloading from $url"

    if [[ "$asset" == *.zip ]]; then
        curl -sSL "$url" -o "/tmp/fclones-${platform}.zip"
        unzip -q -o "/tmp/fclones-${platform}.zip" -d "$platform_dir"
        rm "/tmp/fclones-${platform}.zip"
    else
        curl -sSL "$url" | tar -xz -C "$platform_dir"
    fi

    # Handle different archive structures - find the fclones binary
    # Some archives have usr/bin/fclones structure
    if [ -f "$platform_dir/usr/bin/$binary_name" ]; then
        mv "$platform_dir/usr/bin/$binary_name" "$platform_dir/"
        rm -rf "$platform_dir/usr"
    fi

    # Make executable
    if [[ "$platform" != windows-* ]] && [ -f "$platform_dir/fclones" ]; then
        chmod +x "$platform_dir/fclones"
    fi

    echo "    -> $platform_dir/$binary_name"
}

# Determine which platforms to fetch
if [ -n "$PLATFORM" ]; then
    fetch_platform "$PLATFORM"
else
    # Fetch all available pre-built platforms
    for p in linux-amd64 windows-amd64; do
        fetch_platform "$p"
    done
    echo ""
    echo "NOTE: macOS binaries must be built from source."
    echo "      On macOS: brew install fclones"
    echo "      Or: cargo install fclones"
fi

echo "Done! fclones binaries downloaded to $OUTPUT_DIR"
