// Command migrate applies PostgreSQL migrations without starting HTTP services.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func main() {
	cfg := config.Load()
	cfg.SkipDatabaseMigrations = false
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid migration configuration", "error", err)
		os.Exit(1)
	}
	if cfg.DatabaseDriver != "postgres" {
		slog.Error("standalone migrations require ZAIKO_DATABASE_DRIVER=postgres")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := persistence.MigrateConfigured(ctx, cfg); err != nil {
		slog.Error("migrate PostgreSQL schema", "error", err)
		os.Exit(1)
	}
	slog.Info("PostgreSQL migrations completed")
}
