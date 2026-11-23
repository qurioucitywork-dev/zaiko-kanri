package web

import (
	"encoding/json"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"net/http"
)

func (s *Server) apiPurchaseStaffSave(w http.ResponseWriter, r *http.Request) {
	var input persistence.PurchaseStaffInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, 400, "invalid_json", "入力内容を確認してください。")
		return
	}
	actor, _ := currentUser(r.Context())
	record, err := s.repository.SavePurchaseStaff(r.Context(), actor.OrganizationID, r.PathValue("id"), input)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, persistence.ErrUserInvalid) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, persistence.ErrUserNotFound) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, "purchase_staff_save_failed", "バイヤーコードの重複・氏名・登録内容を確認してください。")
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: actor.OrganizationID, ActorUserID: actor.ID, TargetType: "purchase_staff", TargetID: record.ID, Action: "purchase_staff.saved", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}
	writeJSON(w, status, record)
}
