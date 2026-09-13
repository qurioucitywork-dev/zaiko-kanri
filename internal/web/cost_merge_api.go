package web

import (
	"encoding/json"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"net/http"
)

func (s *Server) apiCostMergeStart(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "原価合わせ開始")
	if !ok {
		return
	}
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, 503, "postgres_required", "PostgreSQL接続を確認してください。")
		return
	}
	var input struct {
		ProductIDs []string `json:"productIds"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, 400, "invalid_json", "商品を2点以上選択してください。")
		return
	}
	result, err := s.repository.StartProductCostMerge(r.Context(), user.OrganizationID, user.ID, input.ProductIDs)
	if err != nil {
		writeAPIError(w, 409, "cost_merge_start_failed", "開始できません。在庫中の商品を重複なく2点以上選択してください。原価調整中の商品は使用できません。")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) apiCostMergeConfirm(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "原価合わせ確定")
	if !ok {
		return
	}
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, 503, "postgres_required", "PostgreSQL接続を確認してください。")
		return
	}
	var input struct {
		SKU    string                                `json:"sku"`
		Output persistence.CostAdjustmentOutputDraft `json:"output"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, 400, "invalid_json", "新商品の入力内容を確認してください。")
		return
	}
	result, err := s.repository.ConfirmProductCostMerge(r.Context(), user.OrganizationID, user.ID, r.PathValue("id"), input.SKU, input.Output)
	if err != nil {
		writeAPIError(w, 409, "cost_merge_confirm_failed", "確定できません。商品の状態・マスタ・原価合計を確認してください。確定済みの場合は再度登録できません。")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
