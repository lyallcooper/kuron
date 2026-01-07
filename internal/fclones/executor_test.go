package fclones

import (
	"testing"
)

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int64
	}{
		// Basic cases
		{"empty string", "", 0},
		{"zero", "0", 0},
		{"plain bytes", "1234", 1234},

		// Decimal units (SI - 1000-based)
		{"kilobytes", "1 KB", 1000},
		{"kilobytes no space", "1KB", 1000},
		{"kilobytes decimal", "1.5 KB", 1500},
		{"megabytes", "1 MB", 1000000},
		{"megabytes decimal", "4.5 MB", 4500000},
		{"gigabytes", "1 GB", 1000000000},
		{"gigabytes decimal", "4.5 GB", 4500000000},
		{"terabytes", "1 TB", 1000000000000},
		{"terabytes decimal", "2.5 TB", 2500000000000},

		// Binary units (IEC - 1024-based)
		{"kibibytes", "1 KiB", 1024},
		{"mebibytes", "1 MiB", 1048576},
		{"gibibytes", "1 GiB", 1073741824},
		{"tebibytes", "1 TiB", 1099511627776},

		// Short forms
		{"K short", "5 K", 5000},
		{"M short", "5 M", 5000000},
		{"G short", "5 G", 5000000000},
		{"T short", "5 T", 5000000000000},

		// Case insensitivity
		{"lowercase kb", "1 kb", 1000},
		{"lowercase mb", "1 mb", 1000000},
		{"lowercase gb", "1 gb", 1000000000},
		{"mixed case Gb", "1 Gb", 1000000000},

		// Edge cases
		{"with leading space", " 100 MB", 100000000},
		{"with trailing space", "100 MB ", 100000000},
		{"just B suffix", "500 B", 500},
		{"invalid string", "invalid", 0},
		// Negative numbers return 0 (file sizes can't be negative)
		{"negative number", "-5 GB", 0},

		// Decimal precision
		{"high precision", "1.234 GB", 1234000000},
		{"very small decimal", "0.001 GB", 1000000},

		// Large values
		{"large TB", "999 TB", 999000000000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBytes(tt.input)
			if got != tt.want {
				t.Errorf("parseBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseProgressBar(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantPhase int
		wantTotal int
		wantName  string
		wantPct   float64 // use -1 for indeterminate, -2 to skip check
	}{
		// Standard progress bar formats
		{
			name:      "grouping by contents with bytes",
			input:     "6/6: Grouping by contents [##########] 630.5 MB / 3.6 GB",
			wantPhase: 6,
			wantTotal: 6,
			wantName:  "Grouping by contents",
			wantPct:   -2, // ~17.5%, don't check exact
		},
		{
			name:      "grouping by prefix with counts",
			input:     "4/6: Grouping by prefix [####------] 12027 / 60000",
			wantPhase: 4,
			wantTotal: 6,
			wantName:  "Grouping by prefix",
			wantPct:   -2, // ~20%
		},
		{
			name:      "scanning phase (no total)",
			input:     "1/6: Scanning files [----------] 12345",
			wantPhase: 1,
			wantTotal: 6,
			wantName:  "Scanning files",
			wantPct:   -1, // Indeterminate
		},
		{
			name:      "100% complete",
			input:     "6/6: Grouping by contents [##########] 1 GB / 1 GB",
			wantPhase: 6,
			wantTotal: 6,
			wantName:  "Grouping by contents",
			wantPct:   100,
		},

		// Edge cases
		{
			name:    "empty line",
			input:   "",
			wantNil: true,
		},
		{
			name:    "no progress bar",
			input:   "Some random text without progress",
			wantNil: true,
		},
		{
			name:      "concatenated progress bars - uses last",
			input:     "1/6: Scanning [--] 100 2/6: Grouping [##] 50 / 100",
			wantPhase: 2,
			wantTotal: 6,
			wantPct:   50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProgressBar(tt.input)

			if tt.wantNil {
				if got != nil {
					t.Errorf("parseProgressBar(%q) = %+v, want nil", tt.input, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("parseProgressBar(%q) = nil, want non-nil", tt.input)
			}

			if got.PhaseNum != tt.wantPhase {
				t.Errorf("PhaseNum = %d, want %d", got.PhaseNum, tt.wantPhase)
			}
			if got.PhaseTotal != tt.wantTotal {
				t.Errorf("PhaseTotal = %d, want %d", got.PhaseTotal, tt.wantTotal)
			}
			if tt.wantName != "" && got.PhaseName != tt.wantName {
				t.Errorf("PhaseName = %q, want %q", got.PhaseName, tt.wantName)
			}
			if tt.wantPct == -1 && got.PhasePercent != -1 {
				t.Errorf("PhasePercent = %f, want -1 (indeterminate)", got.PhasePercent)
			}
			if tt.wantPct >= 0 && tt.wantPct != -2 {
				if got.PhasePercent != tt.wantPct {
					t.Errorf("PhasePercent = %f, want %f", got.PhasePercent, tt.wantPct)
				}
			}
		})
	}
}

func TestPhaseNameToPhase(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"scanning", "Scanning files", "scanning"},
		{"scanning uppercase", "SCANNING FILES", "scanning"},
		{"hashing contents", "Grouping by contents", "hashing"},
		{"grouping prefix", "Grouping by prefix", "grouping"},
		{"grouping suffix", "Grouping by suffix", "grouping"},
		{"grouping size", "Grouping by size", "grouping"},
		{"grouping path", "Grouping by path", "grouping"},
		{"initializing", "Initializing", "initializing"},
		{"unknown", "Something else", "processing"},
		{"empty", "", "processing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := phaseNameToPhase(tt.input)
			if got != tt.want {
				t.Errorf("phaseNameToPhase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseBytesRoundTrip verifies that common fclones output values parse correctly
func TestParseBytesRealWorldExamples(t *testing.T) {
	// These are actual values seen in fclones output
	tests := []struct {
		input string
		want  int64
	}{
		{"0 B", 0},
		{"4.0 KB", 4000},
		{"123.4 MB", 123400000},
		{"1.5 GB", 1500000000},
		{"2.34 TB", 2340000000000},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseBytes(tt.input)
			if got != tt.want {
				t.Errorf("parseBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
