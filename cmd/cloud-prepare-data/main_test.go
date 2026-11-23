package main

import (
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestValidateTargetRefusesRemoteOrBusinessDatabase(t *testing.T) {
	cases := []struct {
		name, url, expected string
		reject              bool
	}{
		{"staging socket", "postgres://zaiko_recovery@/zaiko_recovery?host=/var/run/postgresql&sslmode=disable", "zaiko_recovery", false},
		{"staging loopback", "postgres://test@127.0.0.1/zaiko_recovery", "zaiko_recovery", false},
		{"Neon forbidden", "postgres://test@ep-example.aws.neon.tech/zaiko_recovery", "zaiko_recovery", true},
		{"original DB forbidden", "postgres://test@127.0.0.1/zaiko_clean", "zaiko_clean", true},
		{"mismatched staging", "postgres://test@127.0.0.1/zaiko_recovery_other", "zaiko_recovery", true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			cfg, err := pgx.ParseConfig(item.url)
			if err != nil {
				t.Fatal(err)
			}
			if err = validateTarget(cfg, item.expected); (err != nil) != item.reject {
				t.Fatalf("error=%v reject=%v", err, item.reject)
			}
		})
	}
}
