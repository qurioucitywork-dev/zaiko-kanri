package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiDirectCostAdjustmentConfirm(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "原価調整APIはPostgreSQLモードで利用してください。")
		return
	}
	user, ok := requireAPIAdmin(w, r, "原価調整の確定")
	if !ok {
		return
	}
	var input persistence.DirectCostAdjustmentInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "原価調整の入力内容を確認してください。")
		return
	}
	input.OrganizationID, input.ActorUserID = user.OrganizationID, user.ID
	result, err := s.repository.ConfirmDirectCostAdjustment(r.Context(), input)
	if err != nil {
		status, code, message := http.StatusInternalServerError, "direct_cost_adjustment_failed", "原価調整伝票を発行できませんでした。"
		switch {
		case errors.Is(err, persistence.ErrProductUnavailable):
			status, code, message = http.StatusNotFound, "product_not_found", "対象商品が見つかりません。"
		case errors.Is(err, persistence.ErrCostAdjustmentState):
			status, code, message = http.StatusConflict, "cost_adjustment_state", "原価調整を開始済みの商品、理由（必須）、調整額を確認してください。"
		case errors.Is(err, persistence.ErrCostAllocation):
			status, code, message = http.StatusConflict, "cost_adjustment_amount", "減額後の原価が0円未満にならないよう調整額を確認してください。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	afterJSON, _ := json.Marshal(result)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "cost_adjustment", TargetID: result.ID, Action: "cost_adjustment.direct_confirmed", AfterJSON: string(afterJSON),
		Reason: input.Direction, Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusCreated, result)
}
