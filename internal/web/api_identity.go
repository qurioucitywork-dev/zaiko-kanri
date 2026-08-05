package web

import (
	"context"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) apiAuthenticate(ctx context.Context, organizationCode, username, password string) (database.User, error) {
	if s.repository.Driver() == "postgres" {
		return s.repository.Authenticate(ctx, organizationCode, username, password)
	}
	return s.store.Authenticate(ctx, organizationCode, username, password)
}

func (s *Server) apiCreateSession(ctx context.Context, user database.User, token, csrf, ip, userAgent string, ttl time.Duration) error {
	if s.repository.Driver() == "postgres" {
		return s.repository.CreateSession(ctx, user, token, csrf, ip, userAgent, ttl)
	}
	return s.store.CreateSession(ctx, user, token, csrf, ip, userAgent, ttl)
}

func (s *Server) apiSession(ctx context.Context, token string) (database.Session, error) {
	if s.repository.Driver() == "postgres" {
		return s.repository.Session(ctx, token)
	}
	return s.store.Session(ctx, token)
}

func (s *Server) apiDeleteSession(ctx context.Context, token string) error {
	if s.repository.Driver() == "postgres" {
		return s.repository.DeleteSession(ctx, token)
	}
	return s.store.DeleteSession(ctx, token)
}

func (s *Server) apiHasPermission(ctx context.Context, user database.User, permission string) bool {
	if s.repository.Driver() == "postgres" {
		return s.repository.HasPermission(ctx, user, permission)
	}
	return s.store.HasPermission(ctx, user, permission)
}

func (s *Server) apiWriteAudit(ctx context.Context, entry database.AuditEntry) error {
	if s.repository.Driver() == "postgres" {
		return s.repository.WriteAudit(ctx, entry)
	}
	return s.store.WriteAudit(ctx, entry)
}
