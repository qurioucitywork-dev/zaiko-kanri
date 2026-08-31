package persistence

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestAppliedCostAdjustmentMigrationChecksumsStayStable(t *testing.T) {
	expected := map[string]string{
		"000063_cost_adjustment_breakdown_outputs": "d4fe8a12733fcccc0b4957eb9772034688c19e04b8f2bc06a5bdb1c4c798ec1a",
		"000064_cost_adjustment_combine":           "216f8f9824d310d4c585e3542c97edd26d7628624760899e0aacfb0a9f8fb798",
	}

	for version, expectedChecksum := range expected {
		contents, err := migrationFS.ReadFile("migrations/" + version + ".up.sql")
		if err != nil {
			t.Fatalf("read migration %s: %v", version, err)
		}
		canonical := strings.ReplaceAll(string(contents), "\r\n", "\n")
		canonical = strings.ReplaceAll(canonical, "\r", "\n")
		actualChecksum := fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
		if actualChecksum != expectedChecksum {
			t.Fatalf("migration %s checksum changed: got %s want %s; add a new migration instead of editing an applied migration", version, actualChecksum, expectedChecksum)
		}
	}
}
