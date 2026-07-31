package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/diagnostics"
	_ "modernc.org/sqlite"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "dbdiag:", err)
		os.Exit(1)
	}
}

func run() error {
	dbPath := flag.String("db", ".data/zaiko.db", "診断するSQLite DBのパス")
	flag.Parse()

	absolute, err := filepath.Abs(*dbPath)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	if info, err := os.Stat(absolute); err != nil {
		return fmt.Errorf("stat database: %w", err)
	} else if info.IsDir() {
		return fmt.Errorf("database path is a directory: %s", absolute)
	}

	dsn := "file:" + filepath.ToSlash(absolute) + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open read-only database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA query_only = ON; PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		return fmt.Errorf("configure read-only connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := diagnostics.CollectSQLiteReport(ctx, db, time.Now())
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

func init() {
	flag.CommandLine.SetOutput(os.Stderr)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-db path]\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
}
