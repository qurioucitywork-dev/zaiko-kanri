package web

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiReturns(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.ReturnSlips(r.Context(), user.OrganizationID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "returns_unavailable", "返品/持ち帰り伝票を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiReturn(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.ReturnSlip(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, persistence.ErrReturnNotFound) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, "return_not_found", "返品・持ち帰り伝票が見つかりません。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiReturnCreate(w http.ResponseWriter, r *http.Request) {
	var input persistence.ReturnCreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	input.OperationType = strings.ToLower(strings.TrimSpace(input.OperationType))
	input.BuyerCode = strings.ToUpper(strings.TrimSpace(input.BuyerCode))
	input.SupplierCode = strings.ToUpper(strings.TrimSpace(input.SupplierCode))
	input.PurchaseSlipNo = strings.TrimSpace(input.PurchaseSlipNo)
	if (input.OperationType != "return" && input.OperationType != "takeout" && input.OperationType != "purchase_return") ||
		len(input.ProductCodes) == 0 || len(input.ProductCodes) > 100 {
		writeAPIError(w, http.StatusBadRequest, "invalid_return", "処理区分と1～100件の商品を指定してください。")
		return
	}
	if input.OperationType == "purchase_return" && (input.SupplierCode == "" || input.PurchaseSlipNo == "") {
		writeAPIError(w, http.StatusBadRequest, "invalid_purchase_return", "仕入先と返品元の仕入伝票を指定してください。")
		return
	}
	if input.OperationType == "purchase_return" && strings.TrimSpace(input.Notes) == "" {
		writeAPIError(w, http.StatusBadRequest, "purchase_return_notes_required", "備考を入力してください。")
		return
	}
	if len(input.Carrier) > 100 || len(input.TrackingNumber) > 200 {
		writeAPIError(w, http.StatusBadRequest, "invalid_tracking", "配送会社と追跡番号を確認してください。")
		return
	}
	if _, err := time.Parse("2006-01-02", input.TransactionDate); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_return_date", "処理日はYYYY-MM-DDで指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	input.OrganizationID, input.ActorUserID = user.OrganizationID, user.ID
	record, err := s.repository.CreateReturn(r.Context(), input)
	if err != nil {
		log.Printf("return create failed: operation=%s products=%v buyer=%s supplier=%s purchase=%s: %v", input.OperationType, input.ProductCodes, input.BuyerCode, input.SupplierCode, input.PurchaseSlipNo, err)
		writeAPIError(w, http.StatusConflict, "return_failed", "返品/持ち帰り伝票を処理できませんでした。商品状態を確認してください。")
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiReturnConfirm(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "返品伝票の確定")
	if !ok {
		return
	}
	record, err := s.repository.ConfirmReturn(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		writeAPIError(w, http.StatusConflict, "return_confirm_failed", "返品/持ち帰り伝票を確定できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiDocuments(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.Documents(r.Context(), user.OrganizationID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "documents_unavailable", "伝票一覧を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}
