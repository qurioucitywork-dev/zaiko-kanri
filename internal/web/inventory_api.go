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
	user, ok := requireAPIAdmin(w, r, "原価・売価を含む商品登録")
	if !ok {
		return
	}
	var input struct {
		ProductCode           string   `json:"productCode"`
		SupplierCode          string   `json:"supplierCode"`
		StaffCode             string   `json:"staffCode"`
		PurchaseDate          string   `json:"purchaseDate"`
		PurchaseTaxMode       string   `json:"purchaseTaxMode"`
		TaxCategory           string   `json:"taxCategory"`
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
		InternalComment       string   `json:"internalComment"`
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
	supplierRequired := persistence.PurchaseSupplierRequired(input.PurchaseTaxMode)
	if (supplierRequired && input.SupplierCode == "") || input.BrandCode == "" || input.CostAmountMinor < 0 || input.BaseSalePriceMinor < 0 ||
		!validCurrency(input.CostCurrency) || !validCurrency(input.BaseSaleCurrency) {
		writeAPIError(w, http.StatusBadRequest, "invalid_product", "仕入区分・仕入先・ブランド・金額・通貨を確認してください。")
		return
	}
	if _, err := time.Parse("2006-01-02", input.PurchaseDate); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_purchase_date", "仕入日はYYYY-MM-DDで指定してください。")
		return
	}
	result, err := s.repository.CreateSingleProduct(r.Context(), persistence.SingleProductInput{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, ProductCode: input.ProductCode, SupplierCode: input.SupplierCode,
		StaffCode: input.StaffCode, PurchaseDate: input.PurchaseDate, PurchaseTaxMode: input.PurchaseTaxMode, TaxCategory: input.TaxCategory,
		SKU: input.SKU, BrandCode: input.BrandCode,
		ModelNumber: input.ModelNumber, ReferenceNumber: input.ReferenceNumber, SerialNumber: input.SerialNumber,
		ProductType: input.ProductType, ShapeCode: input.ShapeCode, MarkingCode: input.MarkingCode, MaterialCode: input.MaterialCode, MovementCode: input.MovementCode,
		ConditionCode: input.ConditionCode, AccessoryCodes: input.AccessoryCodes, CostAmountMinor: input.CostAmountMinor,
		BeltText: input.BeltText, DialText: input.DialText, BraceletQuantity: input.BraceletQuantity,
		CostCurrency: input.CostCurrency, BaseSalePriceMinor: input.BaseSalePriceMinor,
		BaseSaleCurrency: input.BaseSaleCurrency, Notes: input.Notes, InternalComment: input.InternalComment, DuplicateSerialReason: input.DuplicateSerialReason,
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
		case errors.Is(err, persistence.ErrDuplicateProductCode):
			status, code, message = http.StatusConflict, "duplicate_product_code", "この管理番号は既に使用されています。別の管理番号を入力してください。"
		case errors.Is(err, persistence.ErrInvalidProductCode):
			status, code, message = http.StatusBadRequest, "invalid_product_code", "管理番号は日・月・西暦下2桁と4桁連番（例：2908260001）で入力してください。"
		case errors.Is(err, persistence.ErrDailyProductLimit):
			status, code, message = http.StatusConflict, "daily_product_limit", "この仕入日の管理番号が上限（9999件）に達しています。"
		case errors.Is(err, persistence.ErrPurchaseTaxMode):
			status, code, message = http.StatusBadRequest, "invalid_purchase_tax_mode", "仕入区分を確認してください。"
		case errors.Is(err, persistence.ErrPurchaseTaxCategory):
			status, code, message = http.StatusBadRequest, "invalid_purchase_tax_category", "税区分を確認してください。"
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
	user, ok := requireAPIAdmin(w, r, "商品情報・原価・売価の変更")
	if !ok {
		return
	}
	var input persistence.ProductUpdateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
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
		case errors.Is(err, persistence.ErrDuplicateProductCode):
			status, code, message = http.StatusConflict, "duplicate_product_code", "この管理番号は既に使用されています。別の管理番号を入力してください。"
		case errors.Is(err, persistence.ErrInvalidProductCode):
			status, code, message = http.StatusBadRequest, "invalid_product_code", "管理番号は日・月・西暦下2桁と4桁連番（例：2908260001）で入力してください。"
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

func (s *Server) apiProductCostAdjustmentStart(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "原価調整APIはPostgreSQLモードで利用してください。")
		return
	}
	user, ok := requireAPIAdmin(w, r, "原価調整の開始")
	if !ok {
		return
	}
	var input struct {
		Mode    string   `json:"mode"`
		PartIDs []string `json:"partIds"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || (input.Mode != "breakdown" && input.Mode != "combine") {
		writeAPIError(w, http.StatusBadRequest, "invalid_cost_adjustment_mode", "崩しまたは結合モードを選択してください。")
		return
	}
	if input.Mode == "combine" && len(input.PartIDs) == 0 {
		writeAPIError(w, http.StatusBadRequest, "combine_parts_required", "結合するパーツを1点以上読み込んでください。")
		return
	}
	before, err := s.repository.ProductByID(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "product_not_found", "商品が見つかりません。")
		return
	}
	var record persistence.Product
	if input.Mode == "combine" {
		record, err = s.repository.StartCombineCostAdjustment(r.Context(), user.OrganizationID, r.PathValue("id"), input.PartIDs, user.ID)
	} else {
		record, err = s.repository.StartProductCostAdjustment(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	}
	if err != nil {
		if errors.Is(err, persistence.ErrProductUnavailable) {
			writeAPIError(w, http.StatusNotFound, "product_not_found", "商品が見つかりません。")
			return
		}
		if errors.Is(err, persistence.ErrProductConflict) {
			writeAPIError(w, http.StatusConflict, "product_state_conflict", "在庫中の商品だけ原価調整を開始できます。")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "cost_adjustment_start_failed", "原価調整を開始できませんでした。")
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product", TargetID: record.ID,
		Action: "product.cost_adjustment_started", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Reason: map[string]string{"breakdown": "崩し作業を開始", "combine": "結合作業を開始"}[input.Mode],
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiProductCostAdjustmentConfirm(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "原価調整APIはPostgreSQLモードで利用してください。")
		return
	}
	user, ok := requireAPIAdmin(w, r, "原価調整の確定")
	if !ok {
		return
	}
	var input persistence.CostAdjustmentConfirmInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "原価調整の入力内容を確認してください。")
		return
	}
	input.OrganizationID = user.OrganizationID
	input.ActorUserID = user.ID
	input.SourceProductID = r.PathValue("id")
	var result persistence.CostAdjustmentConfirmResult
	var err error
	if input.Mode == "combine" {
		result, err = s.repository.ConfirmCostAdjustmentCombine(r.Context(), input)
	} else {
		result, err = s.repository.ConfirmCostAdjustmentBreakdown(r.Context(), input)
	}
	if err != nil {
		status, code, message := http.StatusInternalServerError, "cost_adjustment_confirm_failed", "原価調整を確定できませんでした。"
		switch {
		case errors.Is(err, persistence.ErrProductUnavailable):
			status, code, message = http.StatusNotFound, "product_not_found", "対象商品が見つかりません。"
		case errors.Is(err, persistence.ErrCostAllocation):
			status, code, message = http.StatusConflict, "cost_allocation_mismatch", "配賦原価の総額が対象商品の原価と一致していません。"
		case errors.Is(err, persistence.ErrCostAdjustmentExists):
			status, code, message = http.StatusConflict, "cost_adjustment_exists", "この商品の原価調整は既に確定されています。"
		case errors.Is(err, persistence.ErrCostAdjustmentState):
			status, code, message = http.StatusConflict, "cost_adjustment_state", "原価調整中の商品と編集済み明細を確認してください。"
		case errors.Is(err, persistence.ErrMasterCodeNotFound), errors.Is(err, persistence.ErrDuplicateSerialReason):
			status, code, message = http.StatusBadRequest, "cost_adjustment_master", "商品・パーツの編集内容またはマスタ選択を確認してください。"
		case errors.Is(err, persistence.ErrDailyProductLimit), errors.Is(err, persistence.ErrDailyPartLimit):
			status, code, message = http.StatusConflict, "daily_code_limit", "本日の管理番号採番上限に達しました。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	afterJSON, _ := json.Marshal(result)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product", TargetID: input.SourceProductID,
		Action: "product.cost_adjustment_confirmed", AfterJSON: string(afterJSON), Reason: map[string]string{"breakdown": "崩し原価調整を確定", "combine": "結合原価調整を確定"}[input.Mode],
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusCreated, result)
}

func validCurrency(value string) bool { return value == "JPY" || value == "USD" || value == "HKD" }
