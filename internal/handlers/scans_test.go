package handlers

import (
	"testing"
)

func TestParseSizeWithError(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		// Basic units
		{"empty", "", 0, false},
		{"bytes", "500 B", 500, false},
		{"kilobytes", "1 KB", 1000, false},
		{"megabytes", "1 MB", 1000000, false},
		{"gigabytes", "1 GB", 1000000000, false},
		{"terabytes", "1 TB", 1000000000000, false},

		// Decimal values
		{"decimal KB", "1.5 KB", 1500, false},
		{"decimal MB", "4.5 MB", 4500000, false},
		{"decimal GB", "2.5 GB", 2500000000, false},

		// Binary units (IEC)
		{"kibibytes", "1 KiB", 1024, false},
		{"mebibytes", "1 MiB", 1048576, false},
		{"gibibytes", "1 GiB", 1073741824, false},

		// Case insensitivity
		{"lowercase kb", "1 kb", 1000, false},
		{"lowercase mb", "1 mb", 1000000, false},

		// Without space
		{"no space", "100MB", 100000000, false},

		// Plain numbers
		{"plain number", "12345", 12345, false},

		// Invalid
		{"invalid", "not a size", 0, true},
		{"negative", "-5 GB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSizeWithError(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSizeWithError(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseSizeWithError(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSizeRoundTrip(t *testing.T) {
	// Values that should round-trip through formatSizeInput -> parseSizeWithError
	values := []int64{
		1000,          // 1 KB
		1000000,       // 1 MB
		1000000000,    // 1 GB
		1000000000000, // 1 TB
		2500000000,    // 2.5 GB (may not round-trip due to "2500 MB" preference)
	}

	for _, v := range values {
		formatted := formatSizeInput(v)
		if formatted == "" {
			continue
		}

		parsed, err := parseSizeWithError(formatted)
		if err != nil {
			t.Errorf("parseSizeWithError(%q) error: %v", formatted, err)
			continue
		}

		if parsed != v {
			t.Errorf("Round-trip failed: %d -> %q -> %d", v, formatted, parsed)
		}
	}
}
