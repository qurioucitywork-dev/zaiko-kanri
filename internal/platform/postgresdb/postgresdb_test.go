package postgresdb

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOpenRejectsInvalidConfigurationBeforeConnecting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "empty", config: Config{}, want: "DSN is required"},
		{name: "wrong scheme", config: Config{DSN: "https://example.test/db"}, want: "PostgreSQL URL"},
		{name: "missing database", config: Config{DSN: "postgres://user:secret@example.test"}, want: "host and database"},
		{
			name:   "production TLS downgrade",
			config: Config{DSN: "postgres://user:secret@example.test/zaiko?sslmode=require", Production: true},
			want:   "sslmode=verify-full",
		},
		{
			name: "negative idle pool",
			config: Config{
				DSN:               "postgres://user:secret@127.0.0.1:1/zaiko?sslmode=disable",
				MaxIdleConns:      -1,
				ConnectivityProbe: time.Millisecond,
			},
			want: "MaxIdleConns cannot be negative",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			database, err := Open(context.Background(), test.config)
			if database != nil {
				database.Close()
				t.Fatal("expected nil database")
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Open() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestOpenHonorsCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	database, err := Open(ctx, Config{DSN: "postgres://user:secret@example.test/zaiko?sslmode=verify-full"})
	if database != nil {
		database.Close()
		t.Fatal("expected nil database")
	}
	if err == nil {
		t.Fatal("expected context error")
	}
}
