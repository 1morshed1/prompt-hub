// Command server is the PromptHub API server entrypoint.
//
// Boot sequence: load config → connect to Postgres → apply migrations → start
// the HTTP server (which also serves the static dashboard). Shuts down gracefully
// on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/1morshed1/prompt-hub/server/internal/api"
	"github.com/1morshed1/prompt-hub/server/internal/config"
	"github.com/1morshed1/prompt-hub/server/internal/db"
	"github.com/1morshed1/prompt-hub/server/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogLevel)

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("connected to database")

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	logger.Info("migrations up to date")

	// webDir points at the static dashboard; if it doesn't exist we still run the
	// API (the dashboard is optional).
	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		webDir = "../web"
	}
	if _, statErr := os.Stat(webDir); statErr != nil {
		logger.Warn("dashboard directory not found; serving API only", "web_dir", webDir)
		webDir = ""
	}

	srv := api.New(store.New(pool), logger, webDir)
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run the server in a goroutine so we can wait for a shutdown signal.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", httpServer.Addr, "dashboard", webDir != "")
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
