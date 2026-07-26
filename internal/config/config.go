package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Address          string
	DatabasePath     string
	Environment      string
	SessionTTL       time.Duration
	CookieSecure     bool
	AdminPassword    string
	WorkerPassword   string
	UploadDirectory  string
	OrganizationCode string
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
		DatabasePath:     env("ZAIKO_DATABASE_PATH", ".data/zaiko.db"),
		Environment:      env("ZAIKO_ENV", "development"),
		SessionTTL:       sessionTTL,
		AdminPassword:    env("ZAIKO_PREVIEW_ADMIN_PASSWORD", "preview-admin-2026"),
		WorkerPassword:   env("ZAIKO_PREVIEW_WORKER_PASSWORD", "preview-worker-2026"),
		UploadDirectory:  env("ZAIKO_UPLOAD_DIRECTORY", ".data/uploads"),
		OrganizationCode: env("ZAIKO_ORGANIZATION_CODE", "PREVIEW"),
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
	if strings.TrimSpace(c.UploadDirectory) == "" {
		return errors.New("ZAIKO_UPLOAD_DIRECTORYは必須です")
	}
	if strings.TrimSpace(c.OrganizationCode) == "" {
		return errors.New("ZAIKO_ORGANIZATION_CODEは必須です")
	}
	if c.Environment == "production" {
		if c.DatabasePath == ":memory:" {
			return errors.New("本番環境でインメモリDBは使用できません")
		}
		if !c.CookieSecure {
			return errors.New("本番環境ではZAIKO_COOKIE_SECURE=trueが必須です")
		}
		if !filepath.IsAbs(c.DatabasePath) || !filepath.IsAbs(c.UploadDirectory) {
			return errors.New("本番環境のDBとアップロード先は絶対パスで指定してください")
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
