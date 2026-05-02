package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nano2nano/oauth-callback-dispatcher/internal/allowlist"
	"github.com/nano2nano/oauth-callback-dispatcher/internal/store"
)

func newTestServer(t *testing.T, ttl time.Duration, now store.Clock) http.Handler {
	t.Helper()
	a, err := allowlist.New(`^https://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$`, nil)
	if err != nil {
		t.Fatal(err)
	}
	return New(a, store.New(ttl, now), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))).Handler()
}

func TestRegisterCallbackAndReplay(t *testing.T) {
	now := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	handler := newTestServer(t, 10*time.Minute, func() time.Time { return now })

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"state":"s1","origin":"https://feat-a.myapp.localhost"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("register status = %d, body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "https://feat-a.myapp.localhost" {
		t.Fatalf("unexpected CORS origin %q", got)
	}

	cb := httptest.NewRequest(http.MethodGet, "/auth/callback?state=s1&code=c1&scope=email", nil)
	cbRes := httptest.NewRecorder()
	handler.ServeHTTP(cbRes, cb)
	if cbRes.Code != http.StatusFound {
		t.Fatalf("callback status = %d, body=%s", cbRes.Code, cbRes.Body.String())
	}
	want := "https://feat-a.myapp.localhost/auth/callback?state=s1&code=c1&scope=email"
	if got := cbRes.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}

	replayRes := httptest.NewRecorder()
	handler.ServeHTTP(replayRes, cb)
	if replayRes.Code != http.StatusBadRequest {
		t.Fatalf("replay status = %d", replayRes.Code)
	}
}

func TestRegisterRejectsBadRequests(t *testing.T) {
	now := time.Now()
	handler := newTestServer(t, 10*time.Minute, func() time.Time { return now })
	tests := []struct {
		name string
		body string
		want int
	}{
		{"missing state", `{"origin":"https://feat-a.myapp.localhost"}`, http.StatusBadRequest},
		{"missing origin", `{"state":"s1"}`, http.StatusBadRequest},
		{"bad origin", `{"state":"s1","origin":"https://evil.com"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(tt.body))
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != tt.want {
				t.Fatalf("status = %d, want %d", res.Code, tt.want)
			}
		})
	}
}

func TestDuplicateStateReturnsConflict(t *testing.T) {
	now := time.Now()
	handler := newTestServer(t, 10*time.Minute, func() time.Time { return now })
	for i, want := range []int{http.StatusNoContent, http.StatusConflict} {
		req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"state":"s1","origin":"https://feat-a.myapp.localhost"}`))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("request %d status = %d, want %d", i+1, res.Code, want)
		}
	}
}

func TestCallbackUnknownAndExpiredState(t *testing.T) {
	now := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	handler := newTestServer(t, time.Second, func() time.Time { return now })
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/auth/callback?state=missing", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unknown state = %d", res.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"state":"s1","origin":"https://feat-a.myapp.localhost"}`))
	handler.ServeHTTP(httptest.NewRecorder(), req)
	now = now.Add(2 * time.Second)
	expired := httptest.NewRecorder()
	handler.ServeHTTP(expired, httptest.NewRequest(http.MethodGet, "/auth/callback?state=s1", nil))
	if expired.Code != http.StatusBadRequest {
		t.Fatalf("expired state = %d", expired.Code)
	}
}

func TestCallbackRevalidatesStoredOrigin(t *testing.T) {
	a, err := allowlist.New(`^https://[a-z0-9-]+\.myapp\.localhost(:[0-9]+)?$`, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(a, fakeStore{entry: store.Entry{Origin: "https://evil.com", ExpiresAt: time.Now().Add(time.Minute)}}, nil).Handler()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/auth/callback?state=s1&code=c1", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}

type fakeStore struct {
	entry store.Entry
}

func (f fakeStore) Register(state, origin string) bool { return true }
func (f fakeStore) Consume(state string) (store.Entry, bool) {
	return f.entry, true
}
func (f fakeStore) Cleanup() int { return 0 }

func TestHealthzAndPreflight(t *testing.T) {
	handler := newTestServer(t, 10*time.Minute, time.Now)
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK || health.Body.String() != "ok" {
		t.Fatalf("health = %d %q", health.Code, health.Body.String())
	}
	optionsReq := httptest.NewRequest(http.MethodOptions, "/register", nil)
	optionsReq.Header.Set("Origin", "https://feat-a.myapp.localhost")
	options := httptest.NewRecorder()
	handler.ServeHTTP(options, optionsReq)
	if options.Code != http.StatusNoContent {
		t.Fatalf("options = %d", options.Code)
	}
	if got := options.Header().Get("Access-Control-Allow-Origin"); got != "https://feat-a.myapp.localhost" {
		t.Fatalf("preflight origin = %q", got)
	}
}

func TestStartCleanupStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := New(nil, store.New(time.Millisecond, time.Now), nil)
	s.StartCleanup(ctx, time.Millisecond)
	cancel()
}
