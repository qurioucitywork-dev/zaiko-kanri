package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiCostAdjustmentSlips(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "原価調整伝票APIはPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.ListCostAdjustmentSlips(r.Context(), user.OrganizationID, limit)
	if err != nil {
		s.log.Error("load cost adjustment slips", "error", err, "request_id", requestID(r.Context()))
		writeAPIError(w, http.StatusInternalServerError, "cost_adjustments_unavailable", "原価調整伝票を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiCostAdjustmentSlipScan(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "原価調整伝票のQR読取はPostgreSQLモードで利用してください。")
		return
	}
	var input struct {
		ProductCode string `json:"productCode"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.ProductCode) == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_product_code", "商品管理番号を指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	result, err := s.repository.ScanCostAdjustmentProduct(r.Context(), user.OrganizationID, r.PathValue("id"), input.ProductCode, user.ID)
	if err != nil {
		status, code, message := http.StatusInternalServerError, "cost_adjustment_scan_failed", "原価調整伝票のQR読取に失敗しました。"
		switch {
		case errors.Is(err, persistence.ErrCostAdjustmentSlipNotFound):
			status, code, message = http.StatusNotFound, "cost_adjustment_slip_not_found", "確定済みの原価調整伝票が見つかりません。"
		case errors.Is(err, persistence.ErrCostAdjustmentSlipProductNotFound):
			status, code, message = http.StatusNotFound, "cost_adjustment_product_not_found", "この原価調整伝票に商品管理番号が見つかりません。"
		case errors.Is(err, persistence.ErrCostAdjustmentSlipScanState):
			status, code, message = http.StatusConflict, "cost_adjustment_scan_state", "この商品は別の原価調整伝票に属しているか、現在の状態では読取できません。"
		default:
			s.log.Error("scan cost adjustment slip", "error", err, "request_id", requestID(r.Context()))
		}
		writeAPIError(w, status, code, message)
		return
	}
	after, _ := json.Marshal(result)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product", TargetID: result.ProductCode,
		Action: "cost_adjustment.slip_scanned", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, result)
}
