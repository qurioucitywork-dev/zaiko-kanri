// Command pgdiag performs a read-only PostgreSQL connectivity and schema
// diagnostic. It never runs migrations or seed operations.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess/postgresadapter"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/platform/postgresdb"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pgdiag:", err)
		os.Exit(1)
	}
}

func run() error {
	dsn := strings.TrimSpace(os.Getenv("ZAIKO_POSTGRES_DSN"))
	if dsn == "" {
		return errors.New("ZAIKO_POSTGRES_DSN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database, err := postgresdb.Open(ctx, postgresdb.Config{
		DSN:               dsn,
		ApplicationName:   "zaiko-pgdiag",
		Production:        strings.EqualFold(os.Getenv("ZAIKO_ENV"), "production"),
		MaxOpenConns:      1,
		MaxIdleConns:      1,
		ConnectivityProbe: 5 * time.Second,
	})
	if err != nil {
		return err
	}
	defer database.Close()

	adapter, err := postgresadapter.New(database, postgresadapter.Config{
		StorageProvider: "s3",
		StorageBucket:   requiredEnvironment("ZAIKO_S3_BUCKET"),
		ObjectKeyPrefix: requiredEnvironment("ZAIKO_S3_PREFIX"),
	})
	if err != nil {
		return errors.New("invalid PostgreSQL adapter configuration")
	}
	report, err := adapter.Diagnose(ctx)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func requiredEnvironment(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
