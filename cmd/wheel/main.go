// Command wheel serves the wheel-spinning service: a Go HTTP server, Postgres
// for state and server-sent events for live spectating.
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

	"github.com/Alexander-D-Karpov/wheel/internal/config"
	"github.com/Alexander-D-Karpov/wheel/internal/server"
	"github.com/Alexander-D-Karpov/wheel/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL, cfg.DBMaxOpenConns)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	if cfg.AutoMigrate {
		if err := db.Migrate(ctx); err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
	}

	srv, err := server.New(cfg, db, log)
	if err != nil {
		return err
	}
	srv.StartBackground(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
		// WriteTimeout stays unset: event streams live for as long as a
		// spectator keeps the page open. The SSE handler lifts the read
		// deadline for its own connection.
	}
	// Release event-stream listeners on shutdown, otherwise Shutdown waits for
	// connections that would never close on their own.
	httpSrv.RegisterOnShutdown(srv.Hub().Close)

	serveErr := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "base_url", cfg.BaseURL)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
		}
		return nil
	case <-ctx.Done():
	}

	log.Info("shutting down", "timeout", cfg.ShutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

func newLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
