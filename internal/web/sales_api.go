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

func (s *Server) apiSales(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.SaleSlips(r.Context(), user.OrganizationID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "sales_unavailable", "売上伝票を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiSale(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.SaleSlip(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, persistence.ErrSaleNotFound) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, "sale_not_found", "売上伝票が見つかりません。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiSaleCreate(w http.ResponseWriter, r *http.Request) {
	var input persistence.SaleCreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	input.BuyerCode = strings.ToUpper(strings.TrimSpace(input.BuyerCode))
	input.DisplayCurrency = strings.ToUpper(strings.TrimSpace(input.DisplayCurrency))
	if input.TaxMode == "" {
		input.TaxMode = "taxable"
	}
	if input.TaxRateBasisPoints == 0 && input.TaxMode == "taxable" {
		input.TaxRateBasisPoints = 1000
	}
	if input.BuyerCode == "" || (input.DisplayCurrency != "JPY" && input.DisplayCurrency != "USD" && input.DisplayCurrency != "EUR" && input.DisplayCurrency != "HKD") ||
		(input.TaxMode != "taxable" && input.TaxMode != "tax_exempt" && input.TaxMode != "out_of_scope") || len(input.Lines) == 0 || len(input.Lines) > 100 {
		writeAPIError(w, http.StatusBadRequest, "invalid_sale", "販売先・通貨・税区分・明細を確認してください。")
		return
	}
	if _, err := time.Parse("2006-01-02", input.SaleDate); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_sale_date", "売上日はYYYY-MM-DDで指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	input.OrganizationID, input.ActorUserID = user.OrganizationID, user.ID
	record, err := s.repository.CreateSale(r.Context(), input)
	if err != nil {
		writeSaleError(w, err)
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "sales_slip", TargetID: record.ID,
		Action: "sale.created", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiSaleConfirm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.ConfirmSale(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		writeSaleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiSaleIssue(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleAdmin {
		writeAPIError(w, http.StatusForbidden, "admin_required", "売上伝票を発行できるのは管理者のみです。")
		return
	}
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "売上APIはPostgreSQLモードで利用してください。")
		return
	}
	record, err := s.repository.IssueSale(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		writeSaleError(w, err)
		return
	}
	pdfRef, err := s.storeOfficialPDF(r, "sale", record.ID, record.SlipNumber, salePDF(record), record)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "sale_pdf_failed", "売上伝票は発行されましたが、正式PDFを保存できませんでした。再発行してください。")
		return
	}
	record.OfficialPDF = pdfRef
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "sales_slip", TargetID: record.ID,
		Action: "sale.issued", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiSalePaid(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.MarkSalePaid(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if errors.Is(err, persistence.ErrSaleNotFound) {
		writeAPIError(w, http.StatusNotFound, "sale_not_found", "売上伝票が見つかりません。")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "sale_payment_failed", "入金確認を保存できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func writeSaleError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusConflict, "sale_failed", "売上伝票を処理できませんでした。"
	switch {
	case errors.Is(err, persistence.ErrBuyerNotFound):
		status, code, message = http.StatusBadRequest, "buyer_not_found", "販売先コードが見つかりません。"
	case errors.Is(err, persistence.ErrExchangeRate):
		status, code, message = http.StatusBadRequest, "exchange_rate_required", "USD/JPY為替レートを登録してください。"
	case errors.Is(err, persistence.ErrSaleNotFound):
		status, code, message = http.StatusNotFound, "sale_not_found", "売上伝票が見つかりません。"
	case errors.Is(err, persistence.ErrProductUnavailable), errors.Is(err, persistence.ErrProductConflict):
		status, code, message = http.StatusConflict, "product_unavailable", "明細の商品は販売できない状態か、別取引で使用中です。"
	}
	writeAPIError(w, status, code, message)
}
