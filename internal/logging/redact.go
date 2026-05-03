// Package logging wraps slog.Handler with key-based redaction.
//
// Redaction is intentionally scoped to slog.Attr keys only. The log
// message body (the format string passed to logger.Info etc.) and any
// source location are NOT inspected. Callers must therefore avoid
// embedding OAuth secrets (state, code, tokens) into message text or
// non-secret-looking attribute keys.
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

type RedactHandler struct {
	next slog.Handler
}

func NewRedactHandler(next slog.Handler) *RedactHandler {
	return &RedactHandler{next: next}
}

func NewLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	return slog.New(NewRedactHandler(slog.NewJSONHandler(w, opts)))
}

func (h *RedactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *RedactHandler) Handle(ctx context.Context, record slog.Record) error {
	filtered := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		if !isSecretKey(attr.Key) {
			filtered.AddAttrs(attr)
		}
		return true
	})
	return h.next.Handle(ctx, filtered)
}

func (h *RedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	filtered := attrs[:0]
	for _, attr := range attrs {
		if !isSecretKey(attr.Key) {
			filtered = append(filtered, attr)
		}
	}
	return &RedactHandler{next: h.next.WithAttrs(filtered)}
}

func (h *RedactHandler) WithGroup(name string) slog.Handler {
	return &RedactHandler{next: h.next.WithGroup(name)}
}

func isSecretKey(key string) bool {
	switch strings.ToLower(key) {
	case "code", "state", "access_token", "refresh_token", "client_secret":
		return true
	default:
		return false
	}
}
