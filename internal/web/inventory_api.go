package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiProduct(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.ProductByID(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "product_not_found", "商品が見つかりません。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiProductCreate(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "商品登録APIはPostgreSQLモードで利用してください。")
		return
	}
	var input struct {
		SupplierCode          string   `json:"supplierCode"`
		StaffCode             string   `json:"staffCode"`
		PurchaseDate          string   `json:"purchaseDate"`
		SKU                   string   `json:"sku"`
		BrandCode             string   `json:"brandCode"`
		ModelNumber           string   `json:"modelNumber"`
		ReferenceNumber       string   `json:"referenceNumber"`
		SerialNumber          string   `json:"serialNumber"`
		ProductType           string   `json:"productType"`
		ShapeCode             string   `json:"shapeCode"`
		MarkingCode           string   `json:"markingCode"`
		MaterialCode          string   `json:"materialCode"`
		MovementCode          string   `json:"movementCode"`
		ConditionCode         string   `json:"conditionCode"`
		AccessoryCodes        []string `json:"accessoryCodes"`
		BeltText              string   `json:"beltText"`
		DialText              string   `json:"dialText"`
		BraceletQuantity      *int     `json:"braceletQuantity"`
		CostAmountMinor       int64    `json:"costAmountMinor"`
		CostCurrency          string   `json:"costCurrency"`
		BaseSalePriceMinor    int64    `json:"baseSalePriceMinor"`
		BaseSaleCurrency      string   `json:"baseSaleCurrency"`
		Notes                 string   `json:"notes"`
		DuplicateSerialReason string   `json:"duplicateSerialReason"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	input.SupplierCode = strings.ToUpper(strings.TrimSpace(input.SupplierCode))
	input.StaffCode = strings.ToUpper(strings.TrimSpace(input.StaffCode))
	input.BrandCode = strings.ToUpper(strings.TrimSpace(input.BrandCode))
	input.CostCurrency = strings.ToUpper(strings.TrimSpace(input.CostCurrency))
	input.BaseSaleCurrency = strings.ToUpper(strings.TrimSpace(input.BaseSaleCurrency))
	if input.SupplierCode == "" || input.BrandCode == "" || input.CostAmountMinor < 0 || input.BaseSalePriceMinor < 0 ||
		!validCurrency(input.CostCurrency) || !validCurrency(input.BaseSaleCurrency) {
		writeAPIError(w, http.StatusBadRequest, "invalid_product", "仕入先・ブランド・金額・通貨を確認してください。")
		return
	}
	if _, err := time.Parse("2006-01-02", input.PurchaseDate); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_purchase_date", "仕入日はYYYY-MM-DDで指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	result, err := s.repository.CreateSingleProduct(r.Context(), persistence.SingleProductInput{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, SupplierCode: input.SupplierCode,
		StaffCode: input.StaffCode, PurchaseDate: input.PurchaseDate, SKU: input.SKU, BrandCode: input.BrandCode,
		ModelNumber: input.ModelNumber, ReferenceNumber: input.ReferenceNumber, SerialNumber: input.SerialNumber,
		ProductType: input.ProductType, ShapeCode: input.ShapeCode, MarkingCode: input.MarkingCode, MaterialCode: input.MaterialCode, MovementCode: input.MovementCode,
		ConditionCode: input.ConditionCode, AccessoryCodes: input.AccessoryCodes, CostAmountMinor: input.CostAmountMinor,
		BeltText: input.BeltText, DialText: input.DialText, BraceletQuantity: input.BraceletQuantity,
		CostCurrency: input.CostCurrency, BaseSalePriceMinor: input.BaseSalePriceMinor,
		BaseSaleCurrency: input.BaseSaleCurrency, Notes: input.Notes, DuplicateSerialReason: input.DuplicateSerialReason,
	})
	if err != nil {
		status, code, message := http.StatusInternalServerError, "product_create_failed", "商品を登録できませんでした。"
		switch {
		case errors.Is(err, persistence.ErrSupplierNotFound):
			status, code, message = http.StatusBadRequest, "supplier_not_found", "仕入先コードが見つかりません。"
		case errors.Is(err, persistence.ErrStaffNotFound):
			status, code, message = http.StatusBadRequest, "staff_not_found", "仕入担当者コードが見つかりません。"
		case errors.Is(err, persistence.ErrMasterCodeNotFound):
			status, code, message = http.StatusBadRequest, "master_not_found", "商品マスタコードが見つかりません。"
		case errors.Is(err, persistence.ErrDuplicateSerialReason):
			status, code, message = http.StatusConflict, "duplicate_serial_reason_required", "同じシリアル番号が存在します。登録理由を入力してください。"
		}
		s.log.Error("create REST product", "error", err, "request_id", requestID(r.Context()))
		writeAPIError(w, status, code, message)
		return
	}
	after, _ := json.Marshal(result)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product", TargetID: result.Product.ID,
		Action: "product.created", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) apiProductUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "商品編集APIはPostgreSQLモードで利用してください。")
		return
	}
	var input persistence.ProductUpdateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	user, _ := currentUser(r.Context())
	before, err := s.repository.ProductByID(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "product_not_found", "商品が見つかりません。")
		return
	}
	record, err := s.repository.UpdateProduct(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, input)
	if err != nil {
		status, code, message := http.StatusBadRequest, "product_update_failed", "商品情報を更新できませんでした。"
		switch {
		case errors.Is(err, persistence.ErrProductUnavailable):
			status, code, message = http.StatusNotFound, "product_not_found", "商品が見つかりません。"
		case errors.Is(err, persistence.ErrProductConflict):
			status, code, message = http.StatusConflict, "product_state_conflict", "出荷・売上・取置などのステータスは各伝票から変更してください。"
		case errors.Is(err, persistence.ErrDuplicateSerialReason):
			status, code, message = http.StatusConflict, "duplicate_serial_reason_required", "同じシリアル番号が存在します。編集理由を入力してください。"
		case errors.Is(err, persistence.ErrPurchaseDateMismatch):
			status, code, message = http.StatusConflict, "purchase_date_mismatch", "仕入伝票から登録された商品の仕入日は、元の仕入伝票と同じ日付で固定されています。"
		case errors.Is(err, persistence.ErrSupplierNotFound), errors.Is(err, persistence.ErrStaffNotFound), errors.Is(err, persistence.ErrMasterCodeNotFound):
			status, code, message = http.StatusBadRequest, "master_not_found", "指定されたマスタコードが見つかりません。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product", TargetID: record.ID,
		Action: "product.updated", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Reason: input.Reason,
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, record)
}

func validCurrency(value string) bool { return value == "JPY" || value == "USD" }
