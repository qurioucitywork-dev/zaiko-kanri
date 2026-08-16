package web

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type apiUser struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Organization   string `json:"organization"`
	Username       string `json:"username"`
	DisplayName    string `json:"displayName"`
	Role           string `json:"role"`
}

func userPayload(user database.User) apiUser {
	return apiUser{
		ID: user.ID, OrganizationID: user.OrganizationID, Organization: user.Organization,
		Username: user.Username, DisplayName: user.DisplayName, Role: user.Role,
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	var payload apiError
	payload.Error.Code = code
	payload.Error.Message = message
	writeJSON(w, status, payload)
}

func (s *Server) apiHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	httpStatus := http.StatusOK
	if err := s.repository.Ping(r.Context()); err != nil {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}
	writeJSON(w, httpStatus, map[string]any{
		"status":   status,
		"database": map[string]string{"driver": s.repository.Driver(), "orm": "GORM"},
		"storage":  map[string]string{"driver": s.objects.Driver()},
		"api":      "v1",
	})
}

func (s *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeAPIError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "JSON形式で送信してください。")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	user, err := s.apiAuthenticate(r.Context(), s.cfg.OrganizationCode, strings.TrimSpace(input.Username), input.Password)
	if err != nil {
		_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
			TargetType: "session", TargetID: input.Username, Action: "login.failed", Result: "denied",
			Reason: "invalid credentials", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
		})
		writeAPIError(w, http.StatusUnauthorized, "invalid_credentials", "ユーザーIDまたはパスワードが違います。")
		return
	}
	token, tokenErr := database.RandomToken()
	csrf, csrfErr := database.RandomToken()
	if tokenErr != nil || csrfErr != nil || s.apiCreateSession(r.Context(), user, token, csrf, clientIP(r), r.UserAgent(), s.cfg.SessionTTL) != nil {
		writeAPIError(w, http.StatusInternalServerError, "session_failed", "セッションを作成できませんでした。")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.CookieSecure, MaxAge: int(s.cfg.SessionTTL.Seconds())})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: csrf, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.cfg.CookieSecure, MaxAge: int(s.cfg.SessionTTL.Seconds())})
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "session", TargetID: user.ID,
		Action: "login.succeeded", Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"user": userPayload(user), "csrfToken": csrf})
}

func (s *Server) apiLogout(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = s.apiDeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteStrictMode})
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "session", TargetID: user.ID,
		Action: "logout", Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiMe(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"user": userPayload(user), "csrfToken": csrfFromRequest(r)})
}

func (s *Server) apiDashboard(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	months, _ := strconv.Atoi(r.URL.Query().Get("months"))
	dashboard, err := s.repository.Dashboard(r.Context(), user.OrganizationID, months)
	if err != nil {
		s.log.Error("load REST dashboard", "error", err, "request_id", requestID(r.Context()))
		writeAPIError(w, http.StatusInternalServerError, "dashboard_unavailable", "ダッシュボードを取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dashboard": dashboard})
}

func (s *Server) apiProducts(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	result, err := s.repository.Products(r.Context(), user.OrganizationID, persistence.ProductFilter{
		Query: strings.TrimSpace(r.URL.Query().Get("q")), Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Sort: strings.TrimSpace(r.URL.Query().Get("sort")), Page: page, PageSize: pageSize,
		IncludeCancelled: canViewCancelledInventory(user.Role, r.URL.Query().Get("includeCancelled")),
	})
	if err != nil {
		s.log.Error("load REST products", "error", err, "request_id", requestID(r.Context()))
		writeAPIError(w, http.StatusInternalServerError, "products_unavailable", "在庫一覧を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func canViewCancelledInventory(role, requested string) bool {
	return requested == "true" && (role == database.RoleAdmin || role == database.RoleWorker)
}

func (s *Server) apiAuthenticated(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			writeAPIError(w, http.StatusUnauthorized, "authentication_required", "ログインが必要です。")
			return
		}
		session, err := s.apiSession(r.Context(), cookie.Value)
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
			writeAPIError(w, http.StatusUnauthorized, "session_expired", "セッションの有効期限が切れました。")
			return
		}
		if isUnsafe(r.Method) {
			token := r.Header.Get("X-CSRF-Token")
			if subtle.ConstantTimeCompare([]byte(database.TokenHash(token)), []byte(session.CSRFTokenHash)) != 1 {
				writeAPIError(w, http.StatusForbidden, "csrf_rejected", "画面を再読み込みしてから操作してください。")
				return
			}
		}
		if permission != "" && !s.apiHasPermission(r.Context(), session.User, permission) {
			writeAPIError(w, http.StatusForbidden, "permission_denied", "この操作を行う権限がありません。")
			return
		}
		ctx := withSession(withUser(r.Context(), session.User), session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
