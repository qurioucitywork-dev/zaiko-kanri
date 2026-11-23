package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiPurchaseReturnRevise(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "仕入返品伝票の修正")
	if !ok {
		return
	}
	var input persistence.PurchaseReturnRevisionInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "修正内容を確認してください。")
		return
	}
	input.OrganizationID, input.ActorUserID = user.OrganizationID, user.ID
	before, err := s.repository.ReturnSlip(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "return_not_found", "仕入返品伝票が見つかりません。")
		return
	}
	record, err := s.repository.RevisePurchaseReturn(r.Context(), r.PathValue("id"), input)
	if err != nil {
		status, code, message := http.StatusInternalServerError, "purchase_return_revision_failed", "仕入返品伝票を保存できませんでした。"
		switch {
		case errors.Is(err, persistence.ErrReturnNotFound):
			status, code, message = http.StatusNotFound, "return_not_found", "仕入返品伝票が見つかりません。"
		case errors.Is(err, persistence.ErrReturnState):
			status, code, message = http.StatusConflict, "return_state", "この伝票は修正できません。"
		case errors.Is(err, persistence.ErrSlipRevisionUnchanged):
			status, code, message = http.StatusBadRequest, "revision_unchanged", "伝票の内容に変更がありません。保存理由だけでは修正できません。"
		case errors.Is(err, persistence.ErrPurchaseReturnRevisionInvalid):
			status, code, message = http.StatusBadRequest, "invalid_purchase_return_revision", "返品日・備考・保存理由を確認してください。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "return_slip", TargetID: record.ID,
		Action: "purchase_return.revised", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Reason: strings.TrimSpace(input.Reason),
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, record)
}
