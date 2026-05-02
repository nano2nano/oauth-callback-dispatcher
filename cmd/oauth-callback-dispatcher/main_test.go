package main

import (
	"testing"
)

func TestLoadConfigRequiresAllowedOriginPatternAtValidationTime(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("STATE_TTL_SECONDS", "120")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ALLOWED_ORIGIN_PATTERN", "")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}
	if cfg.allowedOriginPattern != "" {
		t.Fatal("expected empty pattern to remain empty for allowlist validation")
	}
	if cfg.listenHost != "127.0.0.1" {
		t.Fatalf("listenHost = %q", cfg.listenHost)
	}
	if cfg.port != 9999 {
		t.Fatalf("port = %d", cfg.port)
	}
	if cfg.maxStateEntries != 10000 {
		t.Fatalf("maxStateEntries = %d", cfg.maxStateEntries)
	}
}

func TestLoadConfigAcceptsListenHostAndMaxStateEntries(t *testing.T) {
	t.Setenv("LISTEN_HOST", "0.0.0.0")
	t.Setenv("MAX_STATE_ENTRIES", "5")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}
	if cfg.listenHost != "0.0.0.0" {
		t.Fatalf("listenHost = %q", cfg.listenHost)
	}
	if cfg.maxStateEntries != 5 {
		t.Fatalf("maxStateEntries = %d", cfg.maxStateEntries)
	}
}

func TestParseLogLevel(t *testing.T) {
	if _, err := parseLogLevel("bad"); err == nil {
		t.Fatal("expected error")
	}
}
