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

func (s *Server) apiConsignments(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.ConsignmentSlips(r.Context(), user.OrganizationID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "consignments_unavailable", "委託伝票を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiConsignment(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.ConsignmentSlip(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, persistence.ErrConsignmentNotFound) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, "consignment_not_found", "委託伝票が見つかりません。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiConsignmentCreate(w http.ResponseWriter, r *http.Request) {
	var input persistence.ConsignmentCreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	input.ConsigneeCode = strings.ToUpper(strings.TrimSpace(input.ConsigneeCode))
	if input.ConsigneeCode == "" || len(input.ProductCodes) == 0 || len(input.ProductCodes) > 100 {
		writeAPIError(w, http.StatusBadRequest, "invalid_consignment", "委託先と1～100件の商品を指定してください。")
		return
	}
	if _, err := time.Parse("2006-01-02", input.ConsignmentDate); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_consignment_date", "委託日はYYYY-MM-DDで指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	input.OrganizationID, input.ActorUserID = user.OrganizationID, user.ID
	record, err := s.repository.CreateConsignment(r.Context(), input)
	if err != nil {
		status, code, message := http.StatusConflict, "consignment_failed", "委託伝票を処理できませんでした。"
		switch {
		case errors.Is(err, persistence.ErrBuyerNotFound):
			status, code, message = http.StatusBadRequest, "consignee_not_found", "委託先コードが見つかりません。"
		case errors.Is(err, persistence.ErrProductUnavailable), errors.Is(err, persistence.ErrProductConflict):
			status, code, message = http.StatusConflict, "product_unavailable", "明細の商品は委託できない状態か、別取引で使用中です。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}
