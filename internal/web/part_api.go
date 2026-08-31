package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"gorm.io/gorm"
)

func (s *Server) apiParts(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	items, err := s.repository.Parts(r.Context(), user.OrganizationID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "parts_unavailable", "パーツ一覧を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) apiPartCreate(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "パーツ登録APIはPostgreSQLモードで利用してください。")
		return
	}
	user, ok := requireAPIAdmin(w, r, "原価を含むパーツ登録")
	if !ok {
		return
	}
	var input struct {
		PartCode          string `json:"partCode"`
		PurchaseDate      string `json:"purchaseDate"`
		StaffCode         string `json:"staffCode"`
		SupplierCode      string `json:"supplierCode"`
		PurchaseTaxMode   string `json:"purchaseTaxMode"`
		TaxCategory       string `json:"taxCategory"`
		CostAmountMinor   int64  `json:"costAmountMinor"`
		CostCurrency      string `json:"costCurrency"`
		SKU               string `json:"sku"`
		BrandCode         string `json:"brandCode"`
		ModelName         string `json:"modelName"`
		ReferenceNumber   string `json:"referenceNumber"`
		PartNameCode      string `json:"partNameCode"`
		DetailText        string `json:"detailText"`
		DetailMasterType  string `json:"detailMasterType"`
		DetailMasterCode  string `json:"detailMasterCode"`
		BraceletQuantity  *int   `json:"braceletQuantity"`
		SalePriceUSDMinor int64  `json:"salePriceUsdMinor"`
		Notes             string `json:"notes"`
		InternalComment   string `json:"internalComment"`
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
	input.PartNameCode = strings.ToUpper(strings.TrimSpace(input.PartNameCode))
	input.DetailMasterType = strings.ToLower(strings.TrimSpace(input.DetailMasterType))
	input.DetailMasterCode = strings.ToUpper(strings.TrimSpace(input.DetailMasterCode))
	input.CostCurrency = strings.ToUpper(strings.TrimSpace(input.CostCurrency))
	if _, err := time.Parse("2006-01-02", input.PurchaseDate); err != nil || input.PartNameCode == "" || input.CostAmountMinor <= 0 || input.SalePriceUSDMinor < 0 || !validCurrency(input.CostCurrency) ||
		(persistence.PurchaseSupplierRequired(input.PurchaseTaxMode) && input.SupplierCode == "") {
		writeAPIError(w, http.StatusBadRequest, "invalid_part", "仕入日・仕入区分・仕入先・原価・通貨・パーツ名を確認してください。")
		return
	}
	record, err := s.repository.CreatePart(r.Context(), persistence.PartInput{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, PartCode: input.PartCode, PurchaseDate: input.PurchaseDate,
		StaffCode: input.StaffCode, SupplierCode: input.SupplierCode, PurchaseTaxMode: input.PurchaseTaxMode, TaxCategory: input.TaxCategory,
		CostAmountMinor: input.CostAmountMinor, CostCurrency: input.CostCurrency, SKU: input.SKU, BrandCode: input.BrandCode,
		ModelName: input.ModelName, ReferenceNumber: input.ReferenceNumber, PartNameCode: input.PartNameCode,
		DetailText: input.DetailText, DetailMasterType: input.DetailMasterType, DetailMasterCode: input.DetailMasterCode,
		BraceletQuantity: input.BraceletQuantity, SalePriceUSDMinor: input.SalePriceUSDMinor, Notes: input.Notes, InternalComment: input.InternalComment,
	})
	if err != nil {
		status, code, message := http.StatusInternalServerError, "part_create_failed", "パーツを登録できませんでした。"
		switch {
		case errors.Is(err, persistence.ErrSupplierNotFound):
			status, code, message = http.StatusBadRequest, "supplier_not_found", "仕入先が見つかりません。"
		case errors.Is(err, persistence.ErrStaffNotFound):
			status, code, message = http.StatusBadRequest, "staff_not_found", "バイヤーが見つかりません。"
		case errors.Is(err, persistence.ErrMasterCodeNotFound):
			status, code, message = http.StatusBadRequest, "master_not_found", "ブランド・パーツ名・詳細のマスタが見つかりません。"
		case errors.Is(err, persistence.ErrDuplicatePartCode):
			status, code, message = http.StatusConflict, "duplicate_part_code", "このパーツ管理番号は既に使用されています。"
		case errors.Is(err, persistence.ErrInvalidPartCode):
			status, code, message = http.StatusBadRequest, "invalid_part_code", "パーツ管理番号はP＋日月年下2桁＋4桁連番で入力してください。"
		case errors.Is(err, persistence.ErrDailyPartLimit):
			status, code, message = http.StatusConflict, "daily_part_limit", "この仕入日の採番上限に達しました。"
		case errors.Is(err, persistence.ErrPurchaseTaxMode):
			status, code, message = http.StatusBadRequest, "invalid_purchase_tax_mode", "仕入区分を確認してください。"
		case errors.Is(err, persistence.ErrPurchaseTaxCategory):
			status, code, message = http.StatusBadRequest, "invalid_tax_category", "税区分を確認してください。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "part", TargetID: record.ID, Action: "part.created", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiPartUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "パーツ編集APIはPostgreSQLモードで利用してください。")
		return
	}
	user, ok := requireAPIAdmin(w, r, "原価を含むパーツ編集")
	if !ok {
		return
	}
	partID := strings.TrimSpace(r.PathValue("id"))
	if partID == "" {
		writeAPIError(w, http.StatusBadRequest, "part_required", "編集対象のパーツを指定してください。")
		return
	}
	var input struct {
		StaffCode         string `json:"staffCode"`
		SupplierCode      string `json:"supplierCode"`
		PurchaseTaxMode   string `json:"purchaseTaxMode"`
		TaxCategory       string `json:"taxCategory"`
		CostAmountMinor   int64  `json:"costAmountMinor"`
		CostCurrency      string `json:"costCurrency"`
		SKU               string `json:"sku"`
		BrandCode         string `json:"brandCode"`
		ModelName         string `json:"modelName"`
		ReferenceNumber   string `json:"referenceNumber"`
		PartNameCode      string `json:"partNameCode"`
		DetailText        string `json:"detailText"`
		DetailMasterType  string `json:"detailMasterType"`
		DetailMasterCode  string `json:"detailMasterCode"`
		BraceletQuantity  *int   `json:"braceletQuantity"`
		SalePriceUSDMinor int64  `json:"salePriceUsdMinor"`
		Notes             string `json:"notes"`
		InternalComment   string `json:"internalComment"`
		Status            string `json:"status"`
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
	input.PartNameCode = strings.ToUpper(strings.TrimSpace(input.PartNameCode))
	input.DetailMasterType = strings.ToLower(strings.TrimSpace(input.DetailMasterType))
	input.DetailMasterCode = strings.ToUpper(strings.TrimSpace(input.DetailMasterCode))
	input.CostCurrency = strings.ToUpper(strings.TrimSpace(input.CostCurrency))
	input.Status = strings.TrimSpace(input.Status)
	if input.PartNameCode == "" || input.CostAmountMinor < 0 || input.SalePriceUSDMinor < 0 || !validCurrency(input.CostCurrency) ||
		(persistence.PurchaseSupplierRequired(input.PurchaseTaxMode) && input.SupplierCode == "") {
		writeAPIError(w, http.StatusBadRequest, "invalid_part", "仕入区分・仕入先・原価・通貨・パーツ名を確認してください。")
		return
	}
	var before persistence.Part
	if items, err := s.repository.Parts(r.Context(), user.OrganizationID); err == nil {
		for _, item := range items {
			if item.ID == partID {
				before = item
				break
			}
		}
	}
	record, err := s.repository.UpdatePart(r.Context(), persistence.PartUpdateInput{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, PartID: partID,
		StaffCode: input.StaffCode, SupplierCode: input.SupplierCode, PurchaseTaxMode: input.PurchaseTaxMode,
		TaxCategory: input.TaxCategory, CostAmountMinor: input.CostAmountMinor, CostCurrency: input.CostCurrency,
		SKU: input.SKU, BrandCode: input.BrandCode, ModelName: input.ModelName, ReferenceNumber: input.ReferenceNumber,
		PartNameCode: input.PartNameCode, DetailText: input.DetailText, DetailMasterType: input.DetailMasterType,
		DetailMasterCode: input.DetailMasterCode, BraceletQuantity: input.BraceletQuantity,
		SalePriceUSDMinor: input.SalePriceUSDMinor, Notes: input.Notes, InternalComment: input.InternalComment, Status: input.Status,
	})
	if err != nil {
		status, code, message := http.StatusInternalServerError, "part_update_failed", "パーツを更新できませんでした。"
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			status, code, message = http.StatusNotFound, "part_not_found", "編集対象のパーツが見つかりません。"
		case errors.Is(err, persistence.ErrSupplierNotFound):
			status, code, message = http.StatusBadRequest, "supplier_not_found", "仕入先が見つかりません。"
		case errors.Is(err, persistence.ErrStaffNotFound):
			status, code, message = http.StatusBadRequest, "staff_not_found", "バイヤーが見つかりません。"
		case errors.Is(err, persistence.ErrMasterCodeNotFound):
			status, code, message = http.StatusBadRequest, "master_not_found", "ブランド・パーツ名・詳細のマスタが見つかりません。"
		case errors.Is(err, persistence.ErrPurchaseTaxMode), errors.Is(err, persistence.ErrPurchaseTaxCategory):
			status, code, message = http.StatusBadRequest, "invalid_tax", "仕入区分または税区分を確認してください。"
		case errors.Is(err, persistence.ErrPartStatus):
			status, code, message = http.StatusBadRequest, "invalid_status", "パーツのステータスを確認してください。"
		case errors.Is(err, persistence.ErrPartAdjustmentCostLocked):
			status, code, message = http.StatusConflict, "adjustment_cost_locked", "原価調整から生成されたパーツの原価と通貨は変更できません。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "part", TargetID: record.ID, Action: "part.updated", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON),
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusOK, record)
}
