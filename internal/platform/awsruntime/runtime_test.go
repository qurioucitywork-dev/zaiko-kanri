package awsruntime

import (
	"strings"
	"testing"
)

func TestNormalizeConfigDefaultsAWSOriginAndPrefix(t *testing.T) {
	config, err := normalizeConfig(Config{
		PostgresDSN: "postgresql://user:secret@db.example/zaiko?sslmode=verify-full",
		AWSRegion:   "ap-northeast-1",
		S3Bucket:    "private-products",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.S3Endpoint != "https://s3.ap-northeast-1.amazonaws.com" {
		t.Fatalf("endpoint = %q", config.S3Endpoint)
	}
	if config.S3ObjectPrefix != defaultObjectPrefix {
		t.Fatalf("prefix = %q", config.S3ObjectPrefix)
	}
}

func TestNormalizeConfigRejectsUnsafeEndpointWithoutLeakingDSN(t *testing.T) {
	secretDSN := "postgresql://user:very-secret@db.example/zaiko?sslmode=verify-full"
	_, err := normalizeConfig(Config{
		PostgresDSN: secretDSN,
		AWSRegion:   "ap-northeast-1",
		S3Endpoint:  "http://127.0.0.1:9000",
		S3Bucket:    "private-products",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "very-secret") || strings.Contains(err.Error(), secretDSN) {
		t.Fatalf("error leaked DSN: %v", err)
	}
}

func TestRuntimeCloseIsNilSafe(t *testing.T) {
	var runtime *Runtime
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}
