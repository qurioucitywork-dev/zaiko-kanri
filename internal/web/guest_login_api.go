package web

import (
	"encoding/json"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"net/http"
)

func (s *Server) apiGuestLoginSave(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireAPIAdmin(w, r, "ゲストログインの登録・編集")
	if !ok {
		return
	}
	var input persistence.GuestLoginInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, 400, "invalid_json", "入力内容を確認してください。")
		return
	}

	record, err := s.repository.SaveGuestLogin(r.Context(), actor.OrganizationID, actor.ID, input)
	if err != nil {
		var validation persistence.GuestLoginValidationError
		if errors.As(err, &validation) || errors.Is(err, database.ErrGuestPasswordInvalid) {
			writeAPIError(w, 400, "guest_validation_failed", err.Error()+" 保存は行われていません。")
			return
		}
		writeAPIError(w, 400, "guest_save_failed", "ゲストID・顧客コードの重複、顧客情報、8文字以上のパスワードを確認してください。保存は行われていません。")
		return
	}
	after, _ := json.Marshal(record) // Never include the input password in audit records.
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: actor.OrganizationID, ActorUserID: actor.ID, TargetType: "user", TargetID: record.ID, Action: "guest.saved", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusOK, record)
}
