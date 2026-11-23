package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiPurchaseReturnWithdraw(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "仕入返品伝票の取り下げ")
	if !ok {
		return
	}
	before, err := s.repository.ReturnSlip(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "return_not_found", "仕入返品伝票が見つかりません。")
		return
	}
	record, err := s.repository.WithdrawPurchaseReturn(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		status, code, message := http.StatusInternalServerError, "return_withdraw_failed", "仕入返品伝票を取り下げできませんでした。"
		switch {
		case errors.Is(err, persistence.ErrReturnNotFound):
			status, code, message = http.StatusNotFound, "return_not_found", "仕入返品伝票が見つかりません。"
		case errors.Is(err, persistence.ErrReturnState), errors.Is(err, persistence.ErrProductUnavailable):
			status, code, message = http.StatusConflict, "return_state", "対象商品の状態が変わっています。画面を再読み込みして確認してください。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	if before.Status != "cancelled" {
		beforeJSON, _ := json.Marshal(before)
		afterJSON, _ := json.Marshal(record)
		_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "return_slip", TargetID: record.ID,
			Action: "purchase_return.withdrawn", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Reason: "仕入返品伝票の取り下げにより対象商品を仕入中へ変更",
			Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent(),
		})
	}
	writeJSON(w, http.StatusOK, record)
}
