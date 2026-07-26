package web

import (
	"context"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type contextKey string

const (
	userKey      contextKey = "user"
	sessionKey   contextKey = "session"
	requestIDKey contextKey = "request-id"
)

func withUser(ctx context.Context, user database.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func currentUser(ctx context.Context) (database.User, bool) {
	user, ok := ctx.Value(userKey).(database.User)
	return user, ok
}

func withSession(ctx context.Context, session database.Session) context.Context {
	return context.WithValue(ctx, sessionKey, session)
}

func currentSession(ctx context.Context) (database.Session, bool) {
	session, ok := ctx.Value(sessionKey).(database.Session)
	return session, ok
}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}
