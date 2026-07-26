package config

import (
	"path/filepath"
	"testing"
	"time"
)

func validConfig() Config {
	return Config{
		Address: "127.0.0.1:8080", DatabasePath: ".data/zaiko.db",
		Environment: "development", SessionTTL: 12 * time.Hour,
		UploadDirectory: ".data/uploads", OrganizationCode: "PREVIEW",
	}
}

func TestValidateAcceptsDevelopmentConfiguration(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsUnsafeProductionConfiguration(t *testing.T) {
	cfg := validConfig()
	cfg.Environment = "production"
	cfg.DatabasePath = ":memory:"
	if err := cfg.Validate(); err == nil {
		t.Fatal("production in-memory database must be rejected")
	}
	cfg.DatabasePath = filepath.Join(t.TempDir(), "zaiko.db")
	cfg.UploadDirectory = filepath.Join(t.TempDir(), "uploads")
	if err := cfg.Validate(); err == nil {
		t.Fatal("production without secure cookies must be rejected")
	}
	cfg.CookieSecure = true
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsInvalidAddressAndSessionTTL(t *testing.T) {
	cfg := validConfig()
	cfg.Address = "localhost"
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid address must be rejected")
	}
	cfg = validConfig()
	cfg.SessionTTL = time.Minute
	if err := cfg.Validate(); err == nil {
		t.Fatal("short session TTL must be rejected")
	}
}
