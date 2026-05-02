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
	if cfg.port != 9999 {
		t.Fatalf("port = %d", cfg.port)
	}
}

func TestParseLogLevel(t *testing.T) {
	if _, err := parseLogLevel("bad"); err == nil {
		t.Fatal("expected error")
	}
}
