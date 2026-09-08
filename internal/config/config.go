// Package config loads server configuration exclusively from the environment,
// keeping the container image free of config-file parsing dependencies.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all runtime settings for the server.
type Config struct {
	Addr              string
	ContentDir        string
	TLSCertFile       string
	TLSKeyFile        string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
}

// Load builds a Config from environment variables, applying safe defaults
// suitable for a hardened, non-root, read-only-root-filesystem container.
func Load() (Config, error) {
	cfg := Config{
		Addr:           getEnv("WWWEE_ADDR", ":8080"),
		ContentDir:     getEnv("WWWEE_CONTENT_DIR", "/srv/content"),
		TLSCertFile:    getEnv("WWWEE_TLS_CERT_FILE", ""),
		TLSKeyFile:     getEnv("WWWEE_TLS_KEY_FILE", ""),
		MaxHeaderBytes: 1 << 20, // 1 MiB
	}

	var err error
	if cfg.ReadHeaderTimeout, err = getDuration("WWWEE_READ_HEADER_TIMEOUT", 5*time.Second); err != nil {
		return cfg, err
	}
	if cfg.ReadTimeout, err = getDuration("WWWEE_READ_TIMEOUT", 10*time.Second); err != nil {
		return cfg, err
	}
	if cfg.WriteTimeout, err = getDuration("WWWEE_WRITE_TIMEOUT", 10*time.Second); err != nil {
		return cfg, err
	}
	if cfg.IdleTimeout, err = getDuration("WWWEE_IDLE_TIMEOUT", 120*time.Second); err != nil {
		return cfg, err
	}
	if cfg.ShutdownTimeout, err = getDuration("WWWEE_SHUTDOWN_TIMEOUT", 15*time.Second); err != nil {
		return cfg, err
	}

	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return cfg, fmt.Errorf("both WWWEE_TLS_CERT_FILE and WWWEE_TLS_KEY_FILE must be set to enable TLS")
	}

	return cfg, nil
}

// TLSEnabled reports whether both TLS certificate and key were configured.
func (c Config) TLSEnabled() bool {
	return c.TLSCertFile != "" && c.TLSKeyFile != ""
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", key, err)
	}
	return d, nil
}
