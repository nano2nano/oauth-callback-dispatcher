package allowlist

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewAllowsAnchoredEscapedPattern(t *testing.T) {
	a, err := New(`^https://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$`, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if !a.Allows("https://feat-a.myapp.localhost:3000") {
		t.Fatal("expected origin to be allowed")
	}
	if a.Allows("https://feat-a.myapp.localhost.evil.com") {
		t.Fatal("expected bypass origin to be rejected")
	}
}

func TestNewRejectsInvalidPatterns(t *testing.T) {
	tests := []string{
		"",
		`https://[a-z0-9-]+\.myapp\.localhost`,
		`^https://[a-z0-9-]+.myapp.localhost$`,
		`^https://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$[`,
	}
	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			if _, err := New(tt, nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestHTTPPatternWarnsButSucceeds(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	_, err := New(`^https?://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$`, logger)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "plain HTTP") {
		t.Fatalf("expected HTTP warning, got %q", buf.String())
	}
}
