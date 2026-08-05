package web

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) apiAuditLogs(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.AuditLogs(r.Context(), user.OrganizationID,
		strings.TrimSpace(r.URL.Query().Get("action")), strings.TrimSpace(r.URL.Query().Get("targetType")), limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "audit_unavailable", "監査履歴を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}
