package web

import (
	"encoding/json"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"net/http"
	"time"
)

func (s *Server) apiDocumentChangeRequest(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleWorker {
		writeAPIError(w, 403, "worker_required", "作業者の申請専用です。")
		return
	}
	var input struct {
		persistence.DocumentChangeInput
		ApprovalID        string    `json:"approvalId"`
		ExpectedUpdatedAt time.Time `json:"expectedUpdatedAt"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	d.DisallowUnknownFields()
	if e := d.Decode(&input); e != nil {
		writeAPIError(w, 400, "invalid_json", "申請内容を確認してください。")
		return
	}
	a, e := s.repository.RequestDocumentChange(r.Context(), user.OrganizationID, user.ID, input.DocumentChangeInput, input.ApprovalID, input.ExpectedUpdatedAt)
	if e != nil {
		writeAPIError(w, 409, "change_request_failed", "申請できません。最新の伝票状態・入力内容を確認してください。 "+e.Error())
		return
	}
	writeJSON(w, 202, map[string]any{"approvalRequested": true, "approval": a})
}
