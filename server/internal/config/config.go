// Package config loads server configuration from environment variables.
//
// We keep configuration to a handful of env vars (12-factor style): it's the
// least surprising way to configure a container, and it needs no config-file
// parsing. See docs/learning/01-getting-started.md for the table of variables.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds everything the server needs to boot.
type Config struct {
	DatabaseURL string // Postgres connection string (required)
	Port        string // HTTP listen port
	LogLevel    string // debug | info | warn | error
}

// Load reads configuration from the environment, applying defaults, and returns
// an error if a required value is missing. Failing fast at boot with a clear
// message is far kinder than a nil-pointer panic on the first request.
func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        getenvDefault("PORT", "8080"),
		LogLevel:    strings.ToLower(getenvDefault("LOG_LEVEL", "info")),
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required (e.g. postgres://user@host:5432/prompthub?sslmode=disable)")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL must be one of debug|info|warn|error, got %q", c.LogLevel)
	}
	return c, nil
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
