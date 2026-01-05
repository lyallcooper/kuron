#!/bin/bash
# Generate large test data for kuron pagination testing
# Creates 50+ duplicate groups with varying sizes and file counts

set -e

TEST_DIR="${1:-/tmp/kuron-large-test}"
NUM_GROUPS="${2:-60}"

echo "Creating $NUM_GROUPS duplicate groups in: $TEST_DIR"

# Clean up existing test data
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR"/{originals,copies1,copies2,copies3}

echo "Generating duplicate groups..."

for i in $(seq 1 $NUM_GROUPS); do
    # Vary file sizes: small (1-50KB), medium (100-500KB), large (1-5MB)
    case $((i % 3)) in
        0) SIZE=$((1 + (i % 50)))K ;;      # Small: 1-50 KB
        1) SIZE=$((100 + (i * 7 % 400)))K ;; # Medium: 100-500 KB
        2) SIZE=$((1 + (i % 5)))M ;;       # Large: 1-5 MB
    esac

    # Create original file
    dd if=/dev/urandom of="$TEST_DIR/originals/file_$i.bin" bs=$SIZE count=1 2>/dev/null

    # Create 1-3 copies (varying file counts per group)
    COPIES=$((1 + (i % 3)))

    cp "$TEST_DIR/originals/file_$i.bin" "$TEST_DIR/copies1/file_${i}_copy1.bin"

    if [ $COPIES -ge 2 ]; then
        cp "$TEST_DIR/originals/file_$i.bin" "$TEST_DIR/copies2/file_${i}_copy2.bin"
    fi

    if [ $COPIES -ge 3 ]; then
        cp "$TEST_DIR/originals/file_$i.bin" "$TEST_DIR/copies3/file_${i}_copy3.bin"
    fi

    # Progress indicator
    if [ $((i % 10)) -eq 0 ]; then
        echo "  Created $i / $NUM_GROUPS groups..."
    fi
done

echo ""
echo "Test data created:"
echo "=================="
du -sh "$TEST_DIR"/*
echo ""
echo "Total:"
du -sh "$TEST_DIR"
echo ""
echo "Created $NUM_GROUPS duplicate groups with varying sizes and file counts"
echo ""
echo "Run kuron with KURON_SCAN_PATHS=$TEST_DIR to test"
