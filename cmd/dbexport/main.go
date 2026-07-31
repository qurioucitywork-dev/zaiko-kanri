package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/migrationexport"
)

func main() {
	source := flag.String("db", "", "existing SQLite database path (required; opened read-only)")
	output := flag.String("out", "", "new output directory (required; must not exist)")
	flag.Parse()
	if *source == "" || *output == "" {
		flag.Usage()
		os.Exit(2)
	}
	db, err := migrationexport.OpenReadOnly(*source)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	manifest, err := migrationexport.Export(
		context.Background(), db, *output, time.Now().UTC(),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf(
		"read-only export complete: schema_version=%d tables=%d output=%s\n",
		manifest.SchemaVersion, len(manifest.Tables), *output,
	)
}
