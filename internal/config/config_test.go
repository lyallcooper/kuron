package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Empty path
		{"empty", "", ""},

		// Absolute paths (unchanged except for cleaning)
		{"absolute path", "/usr/local/bin", "/usr/local/bin"},
		{"absolute with trailing slash", "/usr/local/bin/", "/usr/local/bin"},

		// Home expansion
		{"tilde only", "~", home},
		{"tilde with path", "~/documents", filepath.Join(home, "documents")},
		{"tilde nested", "~/a/b/c", filepath.Join(home, "a/b/c")},

		// Relative paths (cleaned but not made absolute)
		{"relative", "foo/bar", "foo/bar"},
		{"relative with dots", "foo/../bar", "bar"},
		{"relative with double dots", "./foo/./bar", "foo/bar"},

		// Path cleaning
		{"redundant slashes", "/usr//local///bin", "/usr/local/bin"},
		{"dot segments", "/usr/./local/../bin", "/usr/bin"},

		// Edge cases
		{"tilde in middle (not expanded)", "/home/~user", "/home/~user"},
		{"tilde not at start (not expanded)", "foo/~/bar", "foo/~/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPath(tt.input)
			if got != tt.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsPathAllowed(t *testing.T) {
	tests := []struct {
		name         string
		allowedPaths []string
		checkPath    string
		want         bool
	}{
		// Empty allowed paths = unrestricted
		{"empty allowed - any path allowed", nil, "/anything/goes", true},
		{"empty slice - any path allowed", []string{}, "/anything/goes", true},

		// Exact matches
		{"exact match", []string{"/home/user"}, "/home/user", true},
		{"exact match root", []string{"/"}, "/", true},

		// Subdirectory matches
		{"subdirectory allowed", []string{"/home/user"}, "/home/user/documents", true},
		{"deep subdirectory", []string{"/home/user"}, "/home/user/a/b/c/d", true},

		// Non-matches
		{"parent not allowed", []string{"/home/user/documents"}, "/home/user", false},
		{"sibling not allowed", []string{"/home/user"}, "/home/other", false},
		{"unrelated path", []string{"/home/user"}, "/etc/passwd", false},

		// Multiple allowed paths
		{"first of multiple", []string{"/home/user", "/tmp"}, "/home/user/file", true},
		{"second of multiple", []string{"/home/user", "/tmp"}, "/tmp/file", true},
		{"none of multiple", []string{"/home/user", "/tmp"}, "/etc/passwd", false},

		// Path traversal attempts - filepath.Clean should handle these
		{"traversal attempt", []string{"/home/user"}, "/home/user/../etc/passwd", false},
		{"traversal normalized", []string{"/home/user"}, "/home/user/./documents/../files", true},

		// Edge cases with trailing slashes
		{"allowed has trailing slash", []string{"/home/user/"}, "/home/user/file", true},
		{"check has trailing slash", []string{"/home/user"}, "/home/user/", true},

		// Prefix attack - /home/user shouldn't allow /home/username
		{"prefix attack prevented", []string{"/home/user"}, "/home/username", false},
		{"prefix attack with file", []string{"/home/user"}, "/home/userfile.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{AllowedPaths: tt.allowedPaths}
			got := cfg.IsPathAllowed(tt.checkPath)
			if got != tt.want {
				t.Errorf("IsPathAllowed(%q) with allowed=%v = %v, want %v",
					tt.checkPath, tt.allowedPaths, got, tt.want)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name       string
		envKey     string
		envValue   string
		defaultVal int
		want       int
	}{
		{"empty env", "TEST_INT_EMPTY", "", 42, 42},
		{"valid int", "TEST_INT_VALID", "123", 42, 123},
		{"invalid int", "TEST_INT_INVALID", "not-a-number", 42, 42},
		{"negative int", "TEST_INT_NEG", "-5", 42, -5},
		{"zero", "TEST_INT_ZERO", "0", 42, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			if tt.envValue != "" {
				os.Setenv(tt.envKey, tt.envValue)
				defer os.Unsetenv(tt.envKey)
			} else {
				os.Unsetenv(tt.envKey)
			}

			got := getEnvInt(tt.envKey, tt.defaultVal)
			if got != tt.want {
				t.Errorf("getEnvInt(%q, %d) = %d, want %d", tt.envKey, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestGetEnvPaths(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name     string
		envKey   string
		envValue string
		want     []string
	}{
		{"empty env", "TEST_PATHS_EMPTY", "", nil},
		{"single path", "TEST_PATHS_SINGLE", "/home/user", []string{"/home/user"}},
		{"multiple paths", "TEST_PATHS_MULTI", "/home/user,/tmp", []string{"/home/user", "/tmp"}},
		{"with spaces", "TEST_PATHS_SPACES", "/home/user, /tmp , /var", []string{"/home/user", "/tmp", "/var"}},
		{"with tilde", "TEST_PATHS_TILDE", "~/documents,/tmp", []string{filepath.Join(home, "documents"), "/tmp"}},
		{"empty segments", "TEST_PATHS_EMPTSEG", "/home/user,,/tmp", []string{"/home/user", "/tmp"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.envKey, tt.envValue)
				defer os.Unsetenv(tt.envKey)
			} else {
				os.Unsetenv(tt.envKey)
			}

			got := getEnvPaths(tt.envKey)

			if tt.want == nil && got != nil {
				t.Errorf("getEnvPaths(%q) = %v, want nil", tt.envKey, got)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("getEnvPaths(%q) = %v (len=%d), want %v (len=%d)",
					tt.envKey, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getEnvPaths(%q)[%d] = %q, want %q", tt.envKey, i, got[i], tt.want[i])
				}
			}
		})
	}
}
