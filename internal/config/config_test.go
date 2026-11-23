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
		DatabaseDriver: "sqlite", StorageDriver: "local", S3Region: "ap-northeast-1",
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

func TestMigrationDefaultsPreserveDockerStartup(t *testing.T) {
	t.Setenv("ZAIKO_SKIP_DATABASE_MIGRATIONS", "")
	t.Setenv("ZAIKO_DATABASE_MIGRATION_URL", "")
	cfg := Load()
	if cfg.SkipDatabaseMigrations {
		t.Fatal("startup migrations must remain enabled by default")
	}
	if cfg.DatabaseMigrationURL != "" {
		t.Fatal("migration URL must remain optional")
	}
}

func TestValidateNeonMigrationConnection(t *testing.T) {
	pooled := "postgres://test:example@ep-test-pooler.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"
	direct := "postgres://test:example@ep-test.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"
	cases := []struct {
		name, runtimeURL, migrationURL string
		skip, wantError                bool
	}{
		{name: "pooled startup is rejected", runtimeURL: pooled, wantError: true},
		{name: "pooled runtime with direct migrations", runtimeURL: pooled, migrationURL: direct},
		{name: "pre-migrated pooled runtime", runtimeURL: pooled, skip: true},
		{name: "direct runtime", runtimeURL: direct},
		{name: "Docker postgres unchanged", runtimeURL: "postgres://test:example@db:5432/zaiko?sslmode=disable"},
		{name: "pooled migration URL rejected", runtimeURL: direct, migrationURL: pooled, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.DatabaseDriver = "postgres"
			cfg.DatabaseURL = tc.runtimeURL
			cfg.DatabaseMigrationURL = tc.migrationURL
			cfg.SkipDatabaseMigrations = tc.skip
			if err := cfg.Validate(); (err != nil) != tc.wantError {
				t.Fatalf("Validate() error = %v, wantError %v", err, tc.wantError)
			}
			if cfg.DatabaseURL != tc.runtimeURL {
				t.Fatal("validation must not replace runtime URL")
			}
		})
	}
}

func TestValidateR2GatewayConfiguration(t *testing.T) {
	cases := []struct {
		name, endpoint, token, environment string
		wantError                          bool
	}{
		{name: "production HTTPS", endpoint: "https://inventory.example.workers.dev/__storage", token: "01234567890123456789012345678901", environment: "production"},
		{name: "local test", endpoint: "http://127.0.0.1:1234/__storage", token: "01234567890123456789012345678901", environment: "test"},
		{name: "remote HTTP", endpoint: "http://inventory.example.com/__storage", token: "01234567890123456789012345678901", environment: "test", wantError: true},
		{name: "production HTTP", endpoint: "http://127.0.0.1:1234/__storage", token: "01234567890123456789012345678901", environment: "production", wantError: true},
		{name: "short secret", endpoint: "https://inventory.example.com/__storage", token: "short", wantError: true},
		{name: "URL credentials", endpoint: "https://user:password@inventory.example.com/__storage", token: "01234567890123456789012345678901", wantError: true},
		{name: "URL query", endpoint: "https://inventory.example.com/__storage?token=x", token: "01234567890123456789012345678901", wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{R2GatewayURL: tc.endpoint, R2GatewayToken: tc.token, Environment: tc.environment}
			if err := cfg.ValidateR2Gateway(); (err != nil) != tc.wantError {
				t.Fatalf("error=%v wantError=%v", err, tc.wantError)
			}
		})
	}
}
