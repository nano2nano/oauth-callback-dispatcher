package server

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
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
	Register(state, origin string) bool
	Consume(state string) (store.Entry, bool)
	Cleanup() int
}

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
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.State == "" || req.Origin == "" {
		http.Error(w, "state and origin are required", http.StatusBadRequest)
		return
	}
	if !s.allowlist.Allows(req.Origin) {
		http.Error(w, "origin is not allowed", http.StatusBadRequest)
		return
	}
	setCORS(w, req.Origin)
	if !s.store.Register(req.State, req.Origin) {
		http.Error(w, "state already registered", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRegisterOptions(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" && s.allowlist.Allows(origin) {
		setCORS(w, origin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
	target, err := url.Parse(entry.Origin)
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
