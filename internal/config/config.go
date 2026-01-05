package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration
type Config struct {
	Port         int
	DBPath       string
	ScanPaths    []string // Paths from env vars (locked in UI)
	RetentionDays int
}

// Load reads configuration from environment variables
func Load() *Config {
	cfg := &Config{
		Port:          getEnvInt("KURON_PORT", 8080),
		DBPath:        getEnv("KURON_DB_PATH", "./data/kuron.db"),
		RetentionDays: getEnvInt("KURON_RETENTION_DAYS", 30),
	}

	// Parse comma-separated scan paths
	if paths := getEnv("KURON_SCAN_PATHS", ""); paths != "" {
		for _, p := range strings.Split(paths, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.ScanPaths = append(cfg.ScanPaths, p)
			}
		}
	}

	return cfg
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
	}
	return defaultVal
}
