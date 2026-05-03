package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactHandlerDropsSecretKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewRedactHandler(slog.NewJSONHandler(&buf, nil)))
	logger.Info("callback", "code", "secret-code", "state", "secret-state", "access_token", "secret-token", "origin", "https://feat.myapp.localhost")
	out := buf.String()
	for _, secret := range []string{"secret-code", "secret-state", "secret-token", "code", "state", "access_token"} {
		if strings.Contains(out, secret) {
			t.Fatalf("log output leaked %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, "origin") {
		t.Fatalf("expected non-secret key to remain: %s", out)
	}
}

// TestRedactHandlerDoesNotInspectMessageBody documents that message bodies
// pass through verbatim. Callers must never embed OAuth secrets in messages.
func TestRedactHandlerDoesNotInspectMessageBody(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewRedactHandler(slog.NewJSONHandler(&buf, nil)))
	logger.Info("processing code=do-not-do-this")
	if !strings.Contains(buf.String(), "do-not-do-this") {
		t.Fatalf("redaction should not touch message bodies; got %s", buf.String())
	}
}
