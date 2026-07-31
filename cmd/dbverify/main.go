package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/migrationexport"
)

func main() {
	artifact := flag.String("artifact", "", "SQLite export artifact directory (required)")
	flag.Parse()
	if *artifact == "" {
		flag.Usage()
		os.Exit(2)
	}
	manifest, err := migrationexport.Verify(*artifact)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf(
		"offline artifact verification complete: schema_version=%d tables=%d\n",
		manifest.SchemaVersion,
		len(manifest.Tables),
	)
}
