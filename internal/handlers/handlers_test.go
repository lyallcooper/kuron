package handlers

import (
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		// Basic cases
		{"zero", 0, "0 B"},
		{"small bytes", 500, "500 B"},
		{"max bytes before KB", 999, "999 B"},

		// Kilobytes (SI: 1000-based)
		{"1 KB", 1000, "1 KB"},
		{"1.5 KB", 1500, "1.5 KB"},
		{"999 KB", 999000, "999 KB"},

		// Megabytes
		{"1 MB", 1000000, "1 MB"},
		{"1.23 MB", 1230000, "1.23 MB"},
		{"999 MB", 999000000, "999 MB"},

		// Gigabytes
		{"1 GB", 1000000000, "1 GB"},
		{"4.5 GB", 4500000000, "4.5 GB"},

		// Terabytes
		{"1 TB", 1000000000000, "1 TB"},
		{"2.5 TB", 2500000000000, "2.5 TB"},

		// Petabytes
		{"1 PB", 1000000000000000, "1 PB"},

		// Edge cases around unit boundaries
		{"just under 1KB", 999, "999 B"},
		{"exactly 1KB", 1000, "1 KB"},
		{"just over 1KB", 1001, "1 KB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytes(tt.input)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{"zero", 0, "0"},
		{"single digit", 5, "5"},
		{"two digits", 42, "42"},
		{"three digits", 999, "999"},
		{"four digits with comma", 1000, "1,000"},
		{"five digits", 12345, "12,345"},
		{"six digits", 123456, "123,456"},
		{"seven digits", 1234567, "1,234,567"},
		{"million", 1000000, "1,000,000"},
		{"billion", 1000000000, "1,000,000,000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatInt(tt.input)
			if got != tt.want {
				t.Errorf("formatInt(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatSizeInput(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{"zero", 0, ""},
		{"exact KB", 1000, "1 KB"},
		{"exact MB", 1000000, "1 MB"},
		{"exact GB", 1000000000, "1 GB"},
		{"exact TB", 1000000000000, "1 TB"},
		{"non-round KB", 1500, "1.5 KB"},
		// Prefers larger units - uses max 3 digits before decimal
		{"1500000 bytes", 1500000, "1.5 MB"},
		{"small bytes", 500, "500 B"},

		// Uses largest unit where value < 1000
		{"2500000000 bytes", 2500000000, "2.5 GB"},
		{"100 MB", 100000000, "100 MB"},

		// Values not evenly divisible use decimals with largest suitable unit
		{"non-round value", 1234567, "1.23 MB"},

		// Edge cases around unit boundaries
		{"999 KB", 999000, "999 KB"},
		{"1000 KB becomes 1 MB", 1000000, "1 MB"},
		{"999.9 KB", 999900, "999.9 KB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSizeInput(tt.input)
			if got != tt.want {
				t.Errorf("formatSizeInput(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	// Fixed reference time
	refTime := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name  string
		input any
		want  string
	}{
		// time.Time
		{"time.Time", refTime, "2024-06-15 14:30"},

		// *time.Time
		{"*time.Time", &refTime, "2024-06-15 14:30"},
		{"nil *time.Time", (*time.Time)(nil), "-"},

		// String formats (SQLite)
		{"string with nanoseconds", "2024-06-15 14:30:00.123456789-07:00", "2024-06-15 14:30"},
		{"string without nanoseconds", "2024-06-15 14:30:00-07:00", "2024-06-15 14:30"},

		// Invalid/other types
		{"int", 12345, "-"},
		{"nil", nil, "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTime(tt.input)
			if got != tt.want {
				t.Errorf("formatTime(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name  string
		input any
		want  string
	}{
		// Recent times
		{"30 seconds ago", now.Add(-30 * time.Second), "just now"},
		{"1 minute ago", now.Add(-1 * time.Minute), "1 min ago"},
		{"5 minutes ago", now.Add(-5 * time.Minute), "5 min ago"},
		{"59 minutes ago", now.Add(-59 * time.Minute), "59 min ago"},

		// Hours
		{"1 hour ago", now.Add(-1 * time.Hour), "1 hr ago"},
		{"5 hours ago", now.Add(-5 * time.Hour), "5 hr ago"},
		{"23 hours ago", now.Add(-23 * time.Hour), "23 hr ago"},

		// Days
		{"1 day ago", now.Add(-24 * time.Hour), "1 day ago"},
		{"3 days ago", now.Add(-72 * time.Hour), "3 days ago"},
		{"6 days ago", now.Add(-144 * time.Hour), "6 days ago"},

		// Weeks
		{"1 week ago", now.Add(-7 * 24 * time.Hour), "1 wk ago"},
		{"2 weeks ago", now.Add(-14 * 24 * time.Hour), "2 wk ago"},
		{"4 weeks ago", now.Add(-28 * 24 * time.Hour), "4 wk ago"},

		// Months
		{"1 month ago", now.Add(-32 * 24 * time.Hour), "1 mo ago"},
		{"6 months ago", now.Add(-180 * 24 * time.Hour), "6 mo ago"},

		// Years
		{"1 year ago", now.Add(-366 * 24 * time.Hour), "1 yr ago"},
		{"2 years ago", now.Add(-730 * 24 * time.Hour), "2 yr ago"},

		// Pointer type
		{"pointer to time", &now, "just now"},

		// Nil pointer
		{"nil pointer", (*time.Time)(nil), "-"},

		// Invalid type
		{"invalid type", "not a time", "-"},

		// Future time - BUG: returns "just now" for future times
		// This is because time.Since returns negative duration for future times,
		// and negative duration < time.Minute is true
		{"future time (bug)", now.Add(1 * time.Hour), "just now"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeAgo(tt.input)
			if got != tt.want {
				t.Errorf("timeAgo(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateHash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"short hash", "abc", "abc"},
		{"exactly 7", "abcdefg", "abcdefg"},
		{"8 chars", "abcdefgh", "abcdefg"},
		{"long hash", "abc123def456789", "abc123d"},
		{"SHA256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "e3b0c44"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateHash(tt.input)
			if got != tt.want {
				t.Errorf("truncateHash(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestJoinPatterns(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{"empty", nil, ""},
		{"single", []string{"*.txt"}, "*.txt"},
		{"multiple", []string{"*.txt", "*.doc", "*.pdf"}, "*.txt, *.doc, *.pdf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinPatterns(tt.input)
			if got != tt.want {
				t.Errorf("joinPatterns(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDerefInt64(t *testing.T) {
	val := int64(42)
	tests := []struct {
		name  string
		input *int64
		want  int64
	}{
		{"nil", nil, 0},
		{"non-nil", &val, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := derefInt64(tt.input)
			if got != tt.want {
				t.Errorf("derefInt64(%v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestDerefInt(t *testing.T) {
	val := 42
	tests := []struct {
		name  string
		input *int
		want  int
	}{
		{"nil", nil, 0},
		{"non-nil", &val, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := derefInt(tt.input)
			if got != tt.want {
				t.Errorf("derefInt(%v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestFormatSizeInputRoundTrip tests that formatSizeInput produces output
// that can be parsed back to the original value
func TestFormatSizeInputParseSizeRoundTrip(t *testing.T) {
	// These are common values that should round-trip perfectly
	values := []int64{
		0,
		1000,          // 1 KB
		1000000,       // 1 MB
		1000000000,    // 1 GB
		1000000000000, // 1 TB
		2500000000,    // 2.5 GB
	}

	for _, v := range values {
		formatted := formatSizeInput(v)
		if formatted == "" {
			continue // 0 returns empty string
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
