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
	items := make([]any, 0, len(records))
	for _, record := range records {
		items = append(items, record)
	}
	if s.repository.Driver() == "postgres" {
		adjustments, adjustmentErr := s.repository.SalesAdjustments(r.Context(), user.OrganizationID, limit)
		if adjustmentErr != nil {
			writeAPIError(w, http.StatusInternalServerError, "sales_unavailable", "売上修正伝票を取得できませんでした。")
			return
		}
		for _, record := range adjustments {
			items = append(items, record)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) apiSale(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.SaleSlip(r.Context(), user.OrganizationID, r.PathValue("id"))
	if errors.Is(err, persistence.ErrSaleNotFound) && s.repository.Driver() == "postgres" {
		adjustment, adjustmentErr := s.repository.SalesAdjustment(r.Context(), user.OrganizationID, r.PathValue("id"))
		if adjustmentErr == nil {
			writeJSON(w, http.StatusOK, adjustment)
			return
		}
	}
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

func (s *Server) apiSalesAdjustmentCreate(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "売上修正伝票の作成にはPostgreSQLが必要です。")
		return
	}
	user, ok := requireAPIAdmin(w, r, "売上修正伝票の作成")
	if !ok {
		return
	}
	var input persistence.SalesAdjustmentInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "売上変更内容を確認してください。")
		return
	}
	input.OrganizationID, input.ActorUserID = user.OrganizationID, user.ID
	record, err := s.repository.CreateSalesAdjustment(r.Context(), r.PathValue("id"), input)
	if err != nil {
		status, code, message := http.StatusInternalServerError, "sales_adjustment_failed", "売上修正伝票を作成できませんでした。"
		if errors.Is(err, persistence.ErrSaleNotFound) {
			status, code, message = http.StatusNotFound, "sale_not_found", "元の売上伝票が見つかりません。"
		}
		if errors.Is(err, persistence.ErrSalePaymentRequired) {
			status, code, message = http.StatusConflict, "sale_payment_required", "売上変更は入金確認後に行えます。"
		}
		if errors.Is(err, persistence.ErrSalesAdjustmentInvalid) {
			status, code, message = http.StatusBadRequest, "invalid_sales_adjustment", "対象商品・増減区分・金額・理由を確認してください。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "sales_adjustment_slip", TargetID: record.ID, Action: "sales_adjustment.created", AfterJSON: string(after),
		Reason: record.Reason, Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiSaleRevision(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "売上伝票の修正にはPostgreSQLが必要です。")
		return
	}
	user, ok := requireAPIAdmin(w, r, "売上伝票の修正")
	if !ok {
		return
	}
	var input persistence.SaleRevisionInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "修正内容を確認してください。")
		return
	}
	input.OrganizationID, input.ActorUserID = user.OrganizationID, user.ID
	before, err := s.repository.SaleSlip(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "sale_not_found", "売上伝票が見つかりません。")
		return
	}
	record, err := s.repository.ReviseSale(r.Context(), r.PathValue("id"), input)
	if err != nil {
		status, code, message := http.StatusInternalServerError, "sale_revision_failed", "売上伝票を保存できませんでした。"
		switch {
		case errors.Is(err, persistence.ErrSaleNotFound):
			status, code, message = http.StatusNotFound, "sale_not_found", "売上伝票が見つかりません。"
		case errors.Is(err, persistence.ErrSlipRevisionUnchanged):
			status, code, message = http.StatusBadRequest, "revision_unchanged", "伝票の内容に変更がありません。保存理由だけでは修正できません。"
		case errors.Is(err, persistence.ErrSaleAlreadyPaid):
			status, code, message = http.StatusConflict, "sale_already_paid", "入金確認済みの売上伝票は修正できません。売上変更をご利用ください。"
		case errors.Is(err, persistence.ErrSaleRevisionInvalid):
			status, code, message = http.StatusBadRequest, "invalid_sale_revision", "保存理由・明細金額・取引通貨を確認してください。"
		case errors.Is(err, persistence.ErrBuyerNotFound):
			status, code, message = http.StatusBadRequest, "buyer_not_found", "販売先を確認してください。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "sales_slip", TargetID: record.ID,
		Action: "sale.revised", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Reason: strings.TrimSpace(input.Reason),
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
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
	user, ok := requireAPIAdmin(w, r, "売上伝票の確定")
	if !ok {
		return
	}
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
	user, ok := requireAPIAdmin(w, r, "売上伝票の入金確認")
	if !ok {
		return
	}
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

	case errors.Is(err, persistence.ErrAuctionFeeChanged):
		status, code, message = http.StatusConflict, "auction_fee_changed", "手数料マスタが変更されています。画面を更新して設定・金額を確認してください。"
	case errors.Is(err, persistence.ErrAuctionFee):
		status, code, message = http.StatusBadRequest, "invalid_auction_fee", "手数料は円取引のみ対応しています。設定内容と商品金額を確認してください（売上は手数料を下回る金額では登録できません）。"
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
