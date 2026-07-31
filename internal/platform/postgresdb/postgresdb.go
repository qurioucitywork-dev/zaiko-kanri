// Package postgresdb owns the AWS RDS PostgreSQL database/sql composition
// boundary. It does not run migrations or seed data.
package postgresdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultMaxOpenConnections = 20
	defaultMaxIdleConnections = 5
)

// Config contains process-level pool settings. DSN is expected to come from
// AWS Secrets Manager through an ECS secret environment variable.
type Config struct {
	DSN               string
	ApplicationName   string
	Production        bool
	MaxOpenConns      int
	MaxIdleConns      int
	ConnMaxLifetime   time.Duration
	ConnMaxIdleTime   time.Duration
	ConnectivityProbe time.Duration
}

// Open validates the PostgreSQL URL, creates a pgx-backed database/sql pool,
// and performs a bounded connectivity probe. It never changes schema.
func Open(ctx context.Context, config Config) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rawDSN := strings.TrimSpace(config.DSN)
	if rawDSN == "" {
		return nil, errors.New("postgresdb: DSN is required")
	}
	parsedURL, err := url.Parse(rawDSN)
	if err != nil || (parsedURL.Scheme != "postgres" && parsedURL.Scheme != "postgresql") {
		return nil, errors.New("postgresdb: DSN must be a PostgreSQL URL")
	}
	if parsedURL.Hostname() == "" || strings.TrimPrefix(parsedURL.Path, "/") == "" {
		return nil, errors.New("postgresdb: DSN host and database are required")
	}
	if config.Production && parsedURL.Query().Get("sslmode") != "verify-full" {
		return nil, errors.New("postgresdb: production requires sslmode=verify-full")
	}

	pgxConfig, err := pgx.ParseConfig(rawDSN)
	if err != nil {
		return nil, fmt.Errorf("postgresdb: parse DSN: %w", err)
	}
	if pgxConfig.RuntimeParams == nil {
		pgxConfig.RuntimeParams = make(map[string]string)
	}
	pgxConfig.RuntimeParams["timezone"] = "UTC"
	if applicationName := strings.TrimSpace(config.ApplicationName); applicationName != "" {
		pgxConfig.RuntimeParams["application_name"] = applicationName
	}

	database := stdlib.OpenDB(*pgxConfig)
	maxOpen := config.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = defaultMaxOpenConnections
	}
	maxIdle := config.MaxIdleConns
	if maxIdle < 0 {
		database.Close()
		return nil, errors.New("postgresdb: MaxIdleConns cannot be negative")
	}
	if maxIdle == 0 {
		maxIdle = defaultMaxIdleConnections
	}
	if maxIdle > maxOpen {
		database.Close()
		return nil, errors.New("postgresdb: MaxIdleConns cannot exceed MaxOpenConns")
	}
	database.SetMaxOpenConns(maxOpen)
	database.SetMaxIdleConns(maxIdle)
	if config.ConnMaxLifetime > 0 {
		database.SetConnMaxLifetime(config.ConnMaxLifetime)
	}
	if config.ConnMaxIdleTime > 0 {
		database.SetConnMaxIdleTime(config.ConnMaxIdleTime)
	}

	probeTimeout := config.ConnectivityProbe
	if probeTimeout <= 0 {
		probeTimeout = 5 * time.Second
	}
	probeContext, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if err := database.PingContext(probeContext); err != nil {
		database.Close()
		return nil, fmt.Errorf("postgresdb: connectivity probe failed: %w", err)
	}
	return database, nil
}
