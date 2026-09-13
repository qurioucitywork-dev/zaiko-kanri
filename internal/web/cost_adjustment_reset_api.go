package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiCostAdjustmentStartReset(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "原価管理作業のリセットにはPostgreSQL接続が必要です。")
		return
	}
	user, ok := requireAPIAdmin(w, r, "開始済み原価管理作業のリセット")
	if !ok {
		return
	}
	var input persistence.CostAdjustmentResetInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "リセットする商品を確認してください。")
		return
	}
	input.OrganizationID, input.ActorUserID = user.OrganizationID, user.ID
	result, err := s.repository.ResetCostAdjustmentStart(r.Context(), input)
	if err != nil {
		if errors.Is(err, persistence.ErrCostAdjustmentResetUnavailable) {
			writeAPIError(w, http.StatusConflict, "cost_adjustment_reset_unavailable", "確定前の直近作業だけリセットできます。伝票確定済みの作業は戻せません。")
			return
		}
		s.log.Error("reset cost adjustment start", "error", err, "request_id", requestID(r.Context()))
		writeAPIError(w, http.StatusInternalServerError, "cost_adjustment_reset_failed", "原価管理作業をリセットできませんでした。")
		return
	}
	after, _ := json.Marshal(result)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "cost_adjustment_start", TargetID: result.OperationID,
		Action: "cost_adjustment.start_reset", AfterJSON: string(after), Reason: "入力した管理番号の訂正",
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, result)
}
