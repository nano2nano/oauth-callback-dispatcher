package server

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/nano2nano/oauth-callback-dispatcher/internal/allowlist"
	"github.com/nano2nano/oauth-callback-dispatcher/internal/store"
)

type Server struct {
	allowlist *allowlist.Allowlist
	store     stateStore
	logger    *slog.Logger
}

type stateStore interface {
	Register(state, origin string) error
	Consume(state string) (store.Entry, bool)
	Cleanup() int
}

const (
	registerHeader      = "X-OAuth-Callback-Dispatcher"
	registerHeaderValue = "register"
	maxRegisterBodySize = 4 << 10
	maxStateLength      = 512
	maxOriginLength     = 2048
)

func New(allow *allowlist.Allowlist, states stateStore, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{allowlist: allow, store: states, logger: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", s.handleRegister)
	mux.HandleFunc("OPTIONS /register", s.handleRegisterOptions)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}

func (s *Server) StartCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				deleted := s.store.Cleanup()
				if deleted > 0 {
					s.logger.Debug("expired state entries cleaned", "count", deleted)
				}
			}
		}
	}()
}

type registerRequest struct {
	State  string `json:"state"`
	Origin string `json:"origin"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(registerHeader) != registerHeaderValue {
		http.Error(w, "registration header is required", http.StatusForbidden)
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRegisterBodySize)
	var req registerRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.State == "" || req.Origin == "" {
		http.Error(w, "state and origin are required", http.StatusBadRequest)
		return
	}
	if len(req.State) > maxStateLength || len(req.Origin) > maxOriginLength {
		http.Error(w, "state or origin is too large", http.StatusBadRequest)
		return
	}
	origin, err := allowlist.NormalizeOrigin(req.Origin)
	if err != nil || !s.allowlist.Allows(origin) {
		http.Error(w, "origin is not allowed", http.StatusBadRequest)
		return
	}
	requestOrigin := r.Header.Get("Origin")
	if requestOrigin != "" {
		normalizedRequestOrigin, err := allowlist.NormalizeOrigin(requestOrigin)
		if err != nil || normalizedRequestOrigin != origin {
			http.Error(w, "request Origin does not match registered origin", http.StatusForbidden)
			return
		}
	}
	setCORS(w, origin)
	if err := s.store.Register(req.State, origin); err != nil {
		if errors.Is(err, store.ErrDuplicateState) {
			http.Error(w, "state already registered", http.StatusConflict)
			return
		}
		if errors.Is(err, store.ErrStoreFull) {
			http.Error(w, "state store is full", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "state registration failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRegisterOptions(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		normalizedOrigin, err := allowlist.NormalizeOrigin(origin)
		if err != nil || !s.allowlist.Allows(normalizedOrigin) {
			w.Header().Set("Vary", "Origin")
			http.Error(w, "origin is not allowed", http.StatusForbidden)
			return
		}
		setCORS(w, normalizedOrigin)
		w.Header().Set("Access-Control-Max-Age", "600")
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+registerHeader)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		writeCallbackError(w, "missing state")
		return
	}
	entry, ok := s.store.Consume(state)
	if !ok {
		writeCallbackError(w, "state is not registered or has expired")
		return
	}
	if !s.allowlist.Allows(entry.Origin) {
		writeCallbackError(w, "registered origin is no longer allowed")
		return
	}
	origin, err := allowlist.NormalizeOrigin(entry.Origin)
	if err != nil {
		writeCallbackError(w, "registered origin is invalid")
		return
	}
	target, err := url.Parse(origin)
	if err != nil || target.Scheme == "" || target.Host == "" {
		writeCallbackError(w, "registered origin is invalid")
		return
	}
	target.Path = joinCallbackPath(target.Path)
	target.RawQuery = r.URL.RawQuery
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func setCORS(w http.ResponseWriter, origin string) {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
}

func joinCallbackPath(base string) string {
	if base == "" || base == "/" {
		return "/auth/callback"
	}
	if base[len(base)-1] == '/' {
		return base + "auth/callback"
	}
	return base + "/auth/callback"
}

func writeCallbackError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_ = callbackErrorTemplate.Execute(w, message)
}

var callbackErrorTemplate = template.Must(template.New("callback-error").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>OAuth callback dispatch failed</title></head>
<body><h1>OAuth callback dispatch failed</h1><p>{{.}}</p></body>
</html>`))
