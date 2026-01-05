package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration
type Config struct {
	Port                 int
	DBPath               string
	RetentionDays        int
	RetentionDaysFromEnv bool // true if set via KURON_RETENTION_DAYS env var
	ScanTimeout          time.Duration
	AllowedPaths         []string // Restrict scanning/autocomplete to these paths (empty = unrestricted)
}

// Load reads configuration from environment variables
func Load() *Config {
	retentionFromEnv := os.Getenv("KURON_RETENTION_DAYS") != ""
	return &Config{
		Port:                 getEnvInt("KURON_PORT", 8080),
		DBPath:               getEnv("KURON_DB_PATH", "./data/kuron.db"),
		RetentionDays:        getEnvInt("KURON_RETENTION_DAYS", 30),
		RetentionDaysFromEnv: retentionFromEnv,
		ScanTimeout:          getEnvDuration("KURON_SCAN_TIMEOUT", 30*time.Minute),
		AllowedPaths:         getEnvPaths("KURON_ALLOWED_PATHS"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
		log.Printf("config: invalid value for %s=%q, using default %d", key, val, defaultVal)
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
		log.Printf("config: invalid value for %s=%q, using default %v", key, val, defaultVal)
	}
	return defaultVal
}

func getEnvPaths(key string) []string {
	val := os.Getenv(key)
	if val == "" {
		return nil
	}

	var paths []string
	for _, p := range strings.Split(val, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// IsPathAllowed checks if a path is within the allowed paths.
// Returns true if no allowed paths are configured (unrestricted) or if the path is a subpath of an allowed path.
func (c *Config) IsPathAllowed(path string) bool {
	if len(c.AllowedPaths) == 0 {
		return true
	}

	for _, allowed := range c.AllowedPaths {
		if path == allowed || strings.HasPrefix(path, allowed+"/") {
			return true
		}
	}
	return false
}
