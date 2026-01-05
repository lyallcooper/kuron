package config

import (
	"os"
	"strconv"
)

// Config holds all application configuration
type Config struct {
	Port                 int
	DBPath               string
	RetentionDays        int
	RetentionDaysFromEnv bool // true if set via KURON_RETENTION_DAYS env var
}

// Load reads configuration from environment variables
func Load() *Config {
	retentionFromEnv := os.Getenv("KURON_RETENTION_DAYS") != ""
	return &Config{
		Port:                 getEnvInt("KURON_PORT", 8080),
		DBPath:               getEnv("KURON_DB_PATH", "./data/kuron.db"),
		RetentionDays:        getEnvInt("KURON_RETENTION_DAYS", 30),
		RetentionDaysFromEnv: retentionFromEnv,
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
	}
	return defaultVal
}
