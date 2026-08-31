package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiApprovals(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	records, err := s.repository.ApprovalRequests(r.Context(), user.OrganizationID, user.ID, user.Role, r.URL.Query().Get("status"))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "approvals_unavailable", "承認申請を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiApprovalCreate(w http.ResponseWriter, r *http.Request) {
	var input persistence.ApprovalCreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	user, _ := currentUser(r.Context())
	record, err := s.repository.CreateApproval(r.Context(), user.OrganizationID, user.ID, input)
	if err != nil {
		writeAPIError(w, http.StatusConflict, "approval_create_failed", "承認対象または伝票状態を確認してください。")
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiApprovalDecision(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Note string `json:"note"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	user, _ := currentUser(r.Context())
	record, err := s.repository.DecideApproval(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, r.PathValue("decision"), input.Note)
	if err != nil {
		status, code, message := http.StatusConflict, "approval_failed", "承認申請を処理できませんでした。"
		if errors.Is(err, persistence.ErrApprovalSelf) {
			status, code, message = http.StatusForbidden, "self_approval_denied", "申請者本人は承認できません。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	writeJSON(w, http.StatusOK, record)
}
