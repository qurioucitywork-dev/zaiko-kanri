package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Address          string
	DatabaseDriver   string
	DatabaseURL      string
	DatabasePath     string
	Environment      string
	SessionTTL       time.Duration
	CookieSecure     bool
	AdminPassword    string
	WorkerPassword   string
	UploadDirectory  string
	StorageDriver    string
	S3Bucket         string
	S3Region         string
	S3Endpoint       string
	OrganizationCode string
	PublicBaseURL    string
}

func Load() Config {
	sessionTTL := 12 * time.Hour
	if raw := strings.TrimSpace(os.Getenv("ZAIKO_SESSION_TTL")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			sessionTTL = parsed
		} else {
			sessionTTL = 0
		}
	}
	cfg := Config{
		Address:          env("ZAIKO_ADDRESS", "127.0.0.1:8080"),
		DatabaseDriver:   env("ZAIKO_DATABASE_DRIVER", "sqlite"),
		DatabaseURL:      strings.TrimSpace(os.Getenv("ZAIKO_DATABASE_URL")),
		DatabasePath:     env("ZAIKO_DATABASE_PATH", ".data/zaiko.db"),
		Environment:      env("ZAIKO_ENV", "development"),
		SessionTTL:       sessionTTL,
		AdminPassword:    env("ZAIKO_PREVIEW_ADMIN_PASSWORD", "preview-admin-2026"),
		WorkerPassword:   env("ZAIKO_PREVIEW_WORKER_PASSWORD", "preview-worker-2026"),
		UploadDirectory:  env("ZAIKO_UPLOAD_DIRECTORY", ".data/uploads"),
		StorageDriver:    env("ZAIKO_STORAGE_DRIVER", "local"),
		S3Bucket:         strings.TrimSpace(os.Getenv("ZAIKO_S3_BUCKET")),
		S3Region:         env("ZAIKO_S3_REGION", "ap-northeast-1"),
		S3Endpoint:       strings.TrimSpace(os.Getenv("ZAIKO_S3_ENDPOINT")),
		OrganizationCode: env("ZAIKO_ORGANIZATION_CODE", "PREVIEW"),
		PublicBaseURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("ZAIKO_PUBLIC_BASE_URL")), "/"),
	}
	if cfg.PublicBaseURL == "" {
		cfg.PublicBaseURL = "http://" + cfg.Address
	}
	cfg.CookieSecure = strings.EqualFold(os.Getenv("ZAIKO_COOKIE_SECURE"), "true")
	return cfg
}

func (c Config) Validate() error {
	switch c.Environment {
	case "development", "test", "production":
	default:
		return fmt.Errorf("ZAIKO_ENVはdevelopment、test、productionのいずれかを指定してください")
	}
	if _, _, err := net.SplitHostPort(c.Address); err != nil {
		return fmt.Errorf("ZAIKO_ADDRESSはhost:port形式で指定してください: %w", err)
	}
	if c.SessionTTL < 5*time.Minute || c.SessionTTL > 7*24*time.Hour {
		return errors.New("ZAIKO_SESSION_TTLは5分以上168時間以下で指定してください")
	}
	if strings.TrimSpace(c.DatabasePath) == "" {
		return errors.New("ZAIKO_DATABASE_PATHは必須です")
	}
	if c.DatabaseDriver != "sqlite" && c.DatabaseDriver != "postgres" {
		return errors.New("ZAIKO_DATABASE_DRIVERはsqliteまたはpostgresを指定してください")
	}
	if c.DatabaseDriver == "postgres" && strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("PostgreSQL利用時はZAIKO_DATABASE_URLが必須です")
	}
	if strings.TrimSpace(c.UploadDirectory) == "" {
		return errors.New("ZAIKO_UPLOAD_DIRECTORYは必須です")
	}
	if c.StorageDriver != "local" && c.StorageDriver != "s3" {
		return errors.New("ZAIKO_STORAGE_DRIVERはlocalまたはs3を指定してください")
	}
	if c.StorageDriver == "s3" && (c.S3Bucket == "" || c.S3Region == "") {
		return errors.New("S3利用時はZAIKO_S3_BUCKETとZAIKO_S3_REGIONが必須です")
	}
	if strings.TrimSpace(c.OrganizationCode) == "" {
		return errors.New("ZAIKO_ORGANIZATION_CODEは必須です")
	}
	if c.PublicBaseURL != "" {
		parsed, err := url.Parse(c.PublicBaseURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("ZAIKO_PUBLIC_BASE_URLはhttp(s) URLで指定してください")
		}
	}
	if c.Environment == "production" {
		if c.DatabasePath == ":memory:" {
			return errors.New("本番環境でインメモリDBは使用できません")
		}
		if !c.CookieSecure {
			return errors.New("本番環境ではZAIKO_COOKIE_SECURE=trueが必須です")
		}
		if c.PublicBaseURL != "" && !strings.HasPrefix(c.PublicBaseURL, "https://") {
			return errors.New("本番環境のZAIKO_PUBLIC_BASE_URLはhttpsを指定してください")
		}
		if c.DatabaseDriver == "sqlite" && !filepath.IsAbs(c.DatabasePath) {
			return errors.New("本番環境でSQLiteを使う場合はDBを絶対パスで指定してください")
		}
		if c.StorageDriver == "local" && !filepath.IsAbs(c.UploadDirectory) {
			return errors.New("本番環境でローカル保存を使う場合はアップロード先を絶対パスで指定してください")
		}
	}
	return nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
