package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
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

func (s *Server) apiConsignmentIssue(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleAdmin {
		writeAPIError(w, http.StatusForbidden, "admin_required", "委託伝票を発行できるのは管理者のみです。")
		return
	}
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "委託APIはPostgreSQLモードで利用してください。")
		return
	}
	record, err := s.repository.IssueConsignment(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		if errors.Is(err, persistence.ErrConsignmentNotFound) {
			writeAPIError(w, http.StatusNotFound, "consignment_not_found", "委託伝票が見つかりません。")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "consignment_issue_failed", "委託伝票を発行できませんでした。")
		return
	}
	pdfRef, err := s.storeOfficialPDF(r, "consignment", record.ID, record.SlipNumber, consignmentPDF(record), record)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "consignment_pdf_failed", "委託伝票は発行されましたが、正式PDFを保存できませんでした。再発行してください。")
		return
	}
	record.OfficialPDF = pdfRef
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "consignment_slip", TargetID: record.ID, Action: "consignment.issued", AfterJSON: string(after),
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusOK, record)
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
	user, ok := requireAPIAdmin(w, r, "委託伝票の登録")
	if !ok {
		return
	}
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

func (s *Server) apiConsignmentReturnScan(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "委託伝票の商品返却")
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
	result, err := s.repository.ReturnConsignmentProduct(r.Context(), user.OrganizationID, r.PathValue("id"), input.Code, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, persistence.ErrConsignmentNotFound):
			writeAPIError(w, http.StatusNotFound, "consignment_not_found", "委託伝票が見つかりません。")
		case errors.Is(err, persistence.ErrConsignmentProductNotFound):
			writeAPIError(w, http.StatusNotFound, "consignment_product_not_found", "この委託伝票に含まれない商品です。")
		case errors.Is(err, persistence.ErrConsignmentState):
			writeAPIError(w, http.StatusConflict, "consignment_not_confirmed", "確定済みの委託伝票だけ返却処理できます。")
		case errors.Is(err, persistence.ErrConsignmentReturnState):
			writeAPIError(w, http.StatusConflict, "consignment_return_state", "この商品は委託中ではないため在庫中へ変更できません。")
		default:
			writeAPIError(w, http.StatusInternalServerError, "consignment_return_failed", "返却処理を完了できませんでした。")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}
