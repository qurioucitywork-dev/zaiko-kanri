package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/web"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if cfg.DatabasePath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o750); err != nil {
			logger.Error("create data directory", "error", err)
			os.Exit(1)
		}
	}
	store, err := database.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		logger.Error("migrate database", "error", err)
		os.Exit(1)
	}
	if cfg.Environment == "development" {
		if err := store.SeedPreview(ctx, cfg.AdminPassword, cfg.WorkerPassword); err != nil {
			logger.Error("seed preview data", "error", err)
			os.Exit(1)
		}
		if err := store.SeedInventoryPreview(ctx); err != nil {
			logger.Error("seed inventory preview data", "error", err)
			os.Exit(1)
		}
		if err := store.SeedMarketPreview(ctx); err != nil {
			logger.Error("seed market preview data", "error", err)
			os.Exit(1)
		}
		if err := store.SeedSalesPreview(ctx); err != nil {
			logger.Error("seed sales preview data", "error", err)
			os.Exit(1)
		}
		if err := store.SeedRequestPreview(ctx); err != nil {
			logger.Error("seed request preview data", "error", err)
			os.Exit(1)
		}
		if err := store.SeedApprovalPreview(ctx); err != nil {
			logger.Error("seed approval preview data", "error", err)
			os.Exit(1)
		}
	}
	app, err := web.New(cfg, store, logger)
	if err != nil {
		logger.Error("create web server", "error", err)
		os.Exit(1)
	}
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logger.Info("server started", "environment", cfg.Environment, "url", "http://"+cfg.Address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve", "error", err)
			os.Exit(1)
		}
	}()
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-stop.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "error", err)
	}
}
