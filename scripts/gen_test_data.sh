#!/bin/bash
# Generate test data for kuron testing
# Creates a directory structure with duplicate files of various sizes

set -e

TEST_DIR="${1:-/tmp/kuron-test}"
echo "Creating test data in: $TEST_DIR"

# Clean up existing test data
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR"/{media,downloads,documents}

# Create unique files
echo "Creating unique files..."

# Small files (1-10 KB)
for i in {1..5}; do
    dd if=/dev/urandom of="$TEST_DIR/documents/unique_small_$i.txt" bs=1K count=$((i * 2)) 2>/dev/null
done

# Medium files (1-10 MB)
for i in {1..3}; do
    dd if=/dev/urandom of="$TEST_DIR/media/unique_medium_$i.bin" bs=1M count=$((i * 3)) 2>/dev/null
done

# Large files (50-100 MB)
dd if=/dev/urandom of="$TEST_DIR/media/unique_large_1.bin" bs=1M count=50 2>/dev/null
dd if=/dev/urandom of="$TEST_DIR/media/unique_large_2.bin" bs=1M count=75 2>/dev/null

echo "Creating duplicate files..."

# Create duplicates - small files
cp "$TEST_DIR/documents/unique_small_1.txt" "$TEST_DIR/downloads/dup_small_1a.txt"
cp "$TEST_DIR/documents/unique_small_1.txt" "$TEST_DIR/downloads/dup_small_1b.txt"
cp "$TEST_DIR/documents/unique_small_2.txt" "$TEST_DIR/media/dup_small_2.txt"

# Create duplicates - medium files
cp "$TEST_DIR/media/unique_medium_1.bin" "$TEST_DIR/downloads/dup_medium_1.bin"
cp "$TEST_DIR/media/unique_medium_1.bin" "$TEST_DIR/documents/dup_medium_1.bin"
cp "$TEST_DIR/media/unique_medium_2.bin" "$TEST_DIR/downloads/dup_medium_2.bin"

# Create duplicates - large files
cp "$TEST_DIR/media/unique_large_1.bin" "$TEST_DIR/downloads/dup_large_1.bin"

# Create nested duplicates
mkdir -p "$TEST_DIR/media/nested/deep"
cp "$TEST_DIR/media/unique_medium_3.bin" "$TEST_DIR/media/nested/dup_nested.bin"
cp "$TEST_DIR/media/unique_medium_3.bin" "$TEST_DIR/media/nested/deep/dup_deep.bin"

echo ""
echo "Test data created:"
echo "=================="
du -sh "$TEST_DIR"/*
echo ""
echo "Total:"
du -sh "$TEST_DIR"
echo ""
echo "Expected duplicate groups:"
echo "  - unique_small_1.txt: 3 copies (1 original + 2 duplicates)"
echo "  - unique_small_2.txt: 2 copies"
echo "  - unique_medium_1.bin: 3 copies"
echo "  - unique_medium_2.bin: 2 copies"
echo "  - unique_medium_3.bin: 3 copies"
echo "  - unique_large_1.bin: 2 copies"
echo ""
echo "Start kuron and scan $TEST_DIR to test"
