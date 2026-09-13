package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiShipments(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.ShipmentSlips(r.Context(), user.OrganizationID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "shipments_unavailable", "出荷伝票を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiShipment(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.ShipmentSlip(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, persistence.ErrShipmentNotFound) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, "shipment_not_found", "出荷伝票が見つかりません。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiShipmentCreate(w http.ResponseWriter, r *http.Request) {
	var input persistence.ShipmentCreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	input.BuyerCode = strings.ToUpper(strings.TrimSpace(input.BuyerCode))
	if input.BuyerCode == "" || len(input.ProductCodes) == 0 || len(input.ProductCodes) > 100 {
		writeAPIError(w, http.StatusBadRequest, "invalid_shipment", "販売先と1～100件の商品を指定してください。")
		return
	}
	if _, err := time.Parse("2006-01-02", input.ShipmentDate); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_shipment_date", "出荷日はYYYY-MM-DDで指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	input.OrganizationID, input.ActorUserID = user.OrganizationID, user.ID
	record, err := s.repository.CreateShipment(r.Context(), input)
	if err != nil {
		writeShipmentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiShipmentConfirm(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "出荷伝票の確定")
	if !ok {
		return
	}
	record, err := s.repository.ConfirmShipment(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		writeShipmentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiShipmentReturnScan(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "出荷伝票の商品返却")
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Code) == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_product_code", "商品管理番号を読み取ってください。")
		return
	}
	result, err := s.repository.ReturnShipmentProduct(r.Context(), user.OrganizationID, r.PathValue("id"), input.Code, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, persistence.ErrShipmentNotFound):
			writeAPIError(w, http.StatusNotFound, "shipment_not_found", "出荷伝票が見つかりません。")
		case errors.Is(err, persistence.ErrShipmentProductNotFound):
			writeAPIError(w, http.StatusNotFound, "shipment_product_not_found", "この出荷伝票に含まれない商品です。")
		case errors.Is(err, persistence.ErrShipmentState):
			writeAPIError(w, http.StatusConflict, "shipment_not_confirmed", "確定済みの出荷伝票だけ返却処理できます。")
		case errors.Is(err, persistence.ErrShipmentReturnState):
			writeAPIError(w, http.StatusConflict, "shipment_return_state", "この商品は出荷済ではないため在庫中へ変更できません。")
		default:
			writeAPIError(w, http.StatusInternalServerError, "shipment_return_failed", "返却処理を完了できませんでした。")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeShipmentError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusConflict, "shipment_failed", "出荷伝票を処理できませんでした。"
	switch {
	case errors.Is(err, persistence.ErrBuyerNotFound):
		status, code, message = http.StatusBadRequest, "buyer_not_found", "販売先コードが見つかりません。"
	case errors.Is(err, persistence.ErrShipmentNotFound):
		status, code, message = http.StatusNotFound, "shipment_not_found", "出荷伝票が見つかりません。"
	case errors.Is(err, persistence.ErrProductUnavailable), errors.Is(err, persistence.ErrProductConflict):
		status, code, message = http.StatusConflict, "product_unavailable", "明細の商品は出荷できない状態か、別取引で使用中です。"
	}
	writeAPIError(w, status, code, message)
}
