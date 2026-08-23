// Package config loads the env-only, PLACARD_-prefixed configuration.
// No config files (construct-server house rule).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// Addr is the HTTP listen address. PLACARD_ADDR, default ":4009".
	Addr string
	// DatabaseURL enables the Postgres-backed parts (URL checks, staged
	// uploads). PLACARD_DATABASE_URL with DATABASE_URL fallback. Empty is a
	// supported mode: marks and the front page serve without a database.
	DatabaseURL string
	// PublicRepo is the owner/name the canonical jsDelivr URLs point at.
	// PLACARD_PUBLIC_REPO, default "Einlanzerous/placard".
	PublicRepo string
	// PublicBaseURL is placard's own public hostname, e.g.
	// "https://placard.zerogravity.industries". When set, every check pass
	// also fetches THROUGH that edge — the front page and one mirror mark —
	// so an Access gate landing on the hostname surfaces as failing checks
	// instead of being invisible to everyone holding a session (PCAD-10:
	// exactly that happened, and only a manual curl noticed). Empty disables
	// the edge checks. PLACARD_PUBLIC_BASE_URL.
	PublicBaseURL string
	// CheckInterval is how often serve re-verifies every canonical URL.
	// PLACARD_CHECK_INTERVAL (Go duration), default 6h; "0" disables the
	// ticker (the `placard check` subcommand still works).
	CheckInterval time.Duration
	// UploadToken gates POST /api/services/{slug}/upload and staged reads.
	// PLACARD_UPLOAD_TOKEN; empty disables uploads entirely — the page and
	// the marks are public internet, and an open write endpoint on a public
	// host is an image-hosting service for whoever finds it first.
	UploadToken string
	// MaxUploadBytes caps one staged upload. PLACARD_MAX_UPLOAD_BYTES,
	// default 5 MiB.
	MaxUploadBytes int64
}

func Load() (Config, error) {
	cfg := Config{
		Addr:           envOr("PLACARD_ADDR", ":4009"),
		DatabaseURL:    os.Getenv("PLACARD_DATABASE_URL"),
		PublicRepo:     envOr("PLACARD_PUBLIC_REPO", "Einlanzerous/placard"),
		PublicBaseURL:  strings.TrimSuffix(os.Getenv("PLACARD_PUBLIC_BASE_URL"), "/"),
		CheckInterval:  6 * time.Hour,
		UploadToken:    os.Getenv("PLACARD_UPLOAD_TOKEN"),
		MaxUploadBytes: 5 << 20,
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	}
	if v := os.Getenv("PLACARD_CHECK_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("PLACARD_CHECK_INTERVAL: %w", err)
		}
		if d < 0 {
			return cfg, fmt.Errorf("PLACARD_CHECK_INTERVAL: negative")
		}
		cfg.CheckInterval = d
	}
	if v := os.Getenv("PLACARD_MAX_UPLOAD_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return cfg, fmt.Errorf("PLACARD_MAX_UPLOAD_BYTES: want a positive integer, got %q", v)
		}
		cfg.MaxUploadBytes = n
	}
	return cfg, nil
}

// CanonicalBase is the URL prefix every canonical asset URL is built on.
func (c Config) CanonicalBase() string {
	return "https://cdn.jsdelivr.net/gh/" + c.PublicRepo + "@main/"
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
