package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiPasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r.Context())
	record, previewToken, err := s.repository.QueuePasswordReset(r.Context(), actor.OrganizationID,
		r.PathValue("id"), actor.ID, s.cfg.PublicBaseURL)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, persistence.ErrUserNotFound) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, "password_reset_request_failed", "有効なメールアドレスを持つ利用者を指定してください。")
		return
	}
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: actor.OrganizationID, ActorUserID: actor.ID,
		TargetType: "user", TargetID: r.PathValue("id"), Action: "user.password_reset_requested", Result: "success",
		RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	response := map[string]any{"email": record, "delivery": "queued"}
	if s.cfg.Environment != "production" {
		response["previewToken"] = previewToken
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (s *Server) apiPasswordResetComplete(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "パスワード再設定はPostgreSQLモードで利用してください。")
		return
	}
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	result, err := s.repository.CompletePasswordReset(r.Context(), input.Token, input.Password)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "password_reset_invalid", "再設定URLが無効または期限切れです。")
		return
	}
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: result.OrganizationID,
		TargetType: "user", TargetID: result.UserID, Action: "user.password_reset_completed", Result: "success",
		RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiEmailOutbox(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.EmailOutbox(r.Context(), user.OrganizationID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "email_outbox_unavailable", "メール送信キューを取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}
