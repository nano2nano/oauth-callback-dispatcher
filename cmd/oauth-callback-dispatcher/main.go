package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nano2nano/oauth-callback-dispatcher/internal/allowlist"
	"github.com/nano2nano/oauth-callback-dispatcher/internal/logging"
	"github.com/nano2nano/oauth-callback-dispatcher/internal/server"
	"github.com/nano2nano/oauth-callback-dispatcher/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "oauth-callback-dispatcher: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	logger := logging.NewLogger(os.Stdout, cfg.logLevel)
	allow, err := allowlist.New(cfg.allowedOriginPattern, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	states := store.New(cfg.stateTTL, time.Now)
	app := server.New(allow, states, logger)
	app.StartCleanup(ctx, time.Minute)

	httpServer := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.port),
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "port", cfg.port)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

type config struct {
	port                 int
	allowedOriginPattern string
	stateTTL             time.Duration
	logLevel             slog.Level
}

func loadConfig() (config, error) {
	port, err := intEnv("PORT", 8888)
	if err != nil {
		return config{}, err
	}
	ttlSeconds, err := intEnv("STATE_TTL_SECONDS", 600)
	if err != nil {
		return config{}, err
	}
	if ttlSeconds <= 0 {
		return config{}, errors.New("STATE_TTL_SECONDS must be positive")
	}
	level, err := parseLogLevel(getenv("LOG_LEVEL", "info"))
	if err != nil {
		return config{}, err
	}
	return config{
		port:                 port,
		allowedOriginPattern: os.Getenv("ALLOWED_ORIGIN_PATTERN"),
		stateTTL:             time.Duration(ttlSeconds) * time.Second,
		logLevel:             level,
	}, nil
}

func intEnv(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch raw {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
}
