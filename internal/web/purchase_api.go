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

func (s *Server) apiPurchases(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "仕入APIはPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.PurchaseSlips(r.Context(), user.OrganizationID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "purchases_unavailable", "仕入伝票を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiPurchase(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.PurchaseSlip(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, persistence.ErrPurchaseNotFound) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, "purchase_not_found", "仕入伝票が見つかりません。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiPurchaseCreate(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "仕入APIはPostgreSQLモードで利用してください。")
		return
	}
	var input persistence.PurchaseCreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	input.SupplierCode = strings.ToUpper(strings.TrimSpace(input.SupplierCode))
	input.StaffCode = strings.ToUpper(strings.TrimSpace(input.StaffCode))
	if input.SupplierCode == "" || len(input.Lines) == 0 || len(input.Lines) > 100 {
		writeAPIError(w, http.StatusBadRequest, "invalid_purchase", "仕入先と1～100件の明細を指定してください。")
		return
	}
	if _, err := time.Parse("2006-01-02", input.PurchaseDate); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_purchase_date", "仕入日はYYYY-MM-DDで指定してください。")
		return
	}
	for index := range input.Lines {
		line := &input.Lines[index]
		if line.Quantity != 1 {
			writeAPIError(w, http.StatusBadRequest, "invalid_purchase_quantity", "商品は1点ごとに明細を分けて登録してください。")
			return
		}
		line.BrandCode = strings.ToUpper(strings.TrimSpace(line.BrandCode))
		line.MaterialCode = strings.ToUpper(strings.TrimSpace(line.MaterialCode))
		line.MovementCode = strings.ToUpper(strings.TrimSpace(line.MovementCode))
		line.ConditionCode = strings.ToUpper(strings.TrimSpace(line.ConditionCode))
		line.CostCurrency = strings.ToUpper(strings.TrimSpace(line.CostCurrency))
		line.BaseSaleCurrency = strings.ToUpper(strings.TrimSpace(line.BaseSaleCurrency))
		if line.Quantity < 1 || line.Quantity > 100 || line.UnitCostMinor < 0 ||
			line.BaseSalePriceMinor < 0 || !validCurrency(line.CostCurrency) || !validCurrency(line.BaseSaleCurrency) {
			writeAPIError(w, http.StatusBadRequest, "invalid_purchase_line", "明細の数量・金額・通貨を確認してください。入力済みのマスタコードは有効な値を指定してください。")
			return
		}
	}
	user, _ := currentUser(r.Context())
	input.OrganizationID = user.OrganizationID
	input.ActorUserID = user.ID
	record, err := s.repository.CreatePurchase(r.Context(), input)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "purchase_slip", TargetID: record.ID,
		Action: "purchase.created", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiPurchaseConfirm(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "仕入APIはPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	record, err := s.repository.ConfirmPurchase(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "purchase_slip", TargetID: record.ID,
		Action: "purchase.confirmed", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiPurchaseIssue(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleAdmin {
		writeAPIError(w, http.StatusForbidden, "admin_required", "仕入伝票を発行できるのは管理者のみです。")
		return
	}
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "仕入APIはPostgreSQLモードで利用してください。")
		return
	}
	record, err := s.repository.IssuePurchase(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "purchase_slip", TargetID: record.ID,
		Action: "purchase.issued", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, record)
}

func writePurchaseError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusInternalServerError, "purchase_failed", "仕入伝票を処理できませんでした。"
	switch {
	case errors.Is(err, persistence.ErrSupplierNotFound):
		status, code, message = http.StatusBadRequest, "supplier_not_found", "仕入先コードが見つかりません。"
	case errors.Is(err, persistence.ErrStaffNotFound):
		status, code, message = http.StatusBadRequest, "staff_not_found", "仕入担当者コードが見つかりません。"
	case errors.Is(err, persistence.ErrMasterCodeNotFound):
		status, code, message = http.StatusBadRequest, "master_not_found", "明細のマスタコードが見つかりません。"
	case errors.Is(err, persistence.ErrDuplicateSerial), errors.Is(err, persistence.ErrDuplicateSerialReason):
		status, code, message = http.StatusConflict, "duplicate_serial", "同じシリアル番号が既に存在します。登録理由を指定してください。"
	case errors.Is(err, persistence.ErrQuantitySerialConflict):
		status, code, message = http.StatusBadRequest, "quantity_serial_conflict", "数量が2以上の明細にはシリアル番号を指定できません。"
	case errors.Is(err, persistence.ErrPurchaseQuantity):
		status, code, message = http.StatusBadRequest, "invalid_purchase_quantity", "商品は1点ごとに明細を分けて登録してください。"
	case errors.Is(err, persistence.ErrPurchaseTaxMode):
		status, code, message = http.StatusBadRequest, "invalid_purchase_tax_mode", "仕入区分は国内仕入または海外仕入を指定してください。"
	case errors.Is(err, persistence.ErrPurchaseNotFound):
		status, code, message = http.StatusNotFound, "purchase_not_found", "仕入伝票が見つかりません。"
	case errors.Is(err, persistence.ErrPurchaseState):
		status, code, message = http.StatusConflict, "invalid_purchase_state", "現在の状態では仕入伝票を確定できません。"
	}
	writeAPIError(w, status, code, message)
}
