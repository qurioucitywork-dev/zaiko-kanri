package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func main() {
	audit := flag.Bool("audit", false, "read-only table count report")
	passwordFile := flag.String("password-file", "", "local protected file containing the generated password")
	username := flag.String("username", "admin", "initial administrator username")
	flag.Parse()
	cfg := config.Load()
	if cfg.DatabaseDriver != "postgres" || cfg.Environment == "development" {
		fail("PostgreSQL and non-development configuration required")
	}
	target, err := url.Parse(cfg.DatabaseURL)
	if err != nil || target.Host == "" {
		fail("invalid database URL")
	}
	fmt.Fprintf(os.Stderr, "Target: %s database=%s organization=%s\n", target.Host, strings.TrimPrefix(target.Path, "/"), cfg.OrganizationCode)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	repo, err := persistence.Open(cfg)
	if err != nil {
		fail("database connection failed; verify the protected local configuration")
	}
	defer repo.Close()
	if *audit {
		rows, err := repo.AuditClean(ctx)
		json.NewEncoder(os.Stdout).Encode(rows)
		if err != nil {
			fail(err.Error())
		}
		return
	}
	if *passwordFile == "" {
		fail("--password-file is required; passwords must not be command-line arguments")
	}
	raw, err := os.ReadFile(*passwordFile)
	if err != nil {
		fail("cannot read protected password file")
	}
	err = repo.BootstrapClean(ctx, cfg.OrganizationCode, *username, strings.TrimSpace(string(raw)))
	if errors.Is(err, persistence.ErrCleanDatabaseNotEmpty) {
		fmt.Fprintln(os.Stderr, "Already initialized or nonempty: refused without changes.")
		os.Exit(3)
	}
	if err != nil {
		fail(err.Error())
	}
	fmt.Println("Clean organization and initial administrator created; no business data seeded.")
}
func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
