package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

type marketPricePayload struct {
	ImportDate         string   `json:"importDate"`
	BrandCode          string   `json:"brandCode"`
	ModelNumber        string   `json:"modelNumber"`
	ReferenceNumber    string   `json:"referenceNumber"`
	SerialNumber       string   `json:"serialNumber"`
	SKU                string   `json:"sku"`
	ConditionCode      string   `json:"conditionCode"`
	PurchasePriceMinor int64    `json:"purchasePriceMinor"`
	PurchaseCurrency   string   `json:"purchaseCurrency"`
	MarketPriceMinor   int64    `json:"marketPriceMinor"`
	MarketCurrency     string   `json:"marketCurrency"`
	SupplierCode       string   `json:"supplierCode"`
	StaffCode          string   `json:"staffCode"`
	MaterialCode       string   `json:"materialCode"`
	MovementCode       string   `json:"movementCode"`
	PurchaseDate       string   `json:"purchaseDate"`
	StatusText         string   `json:"statusText"`
	BoxCode            string   `json:"boxCode"`
	AccessoryCodes     []string `json:"accessoryCodes"`
	AuctionCode        string   `json:"auctionCode"`
	Source             string   `json:"source"`
	Notes              string   `json:"notes"`
}

func normalizeMarketPricePayload(input *marketPricePayload) {
	input.BrandCode = strings.ToUpper(strings.TrimSpace(input.BrandCode))
	input.ConditionCode = strings.ToUpper(strings.TrimSpace(input.ConditionCode))
	input.PurchaseCurrency = strings.ToUpper(strings.TrimSpace(input.PurchaseCurrency))
	input.MarketCurrency = strings.ToUpper(strings.TrimSpace(input.MarketCurrency))
	input.SupplierCode = strings.ToUpper(strings.TrimSpace(input.SupplierCode))
	input.StaffCode = strings.ToUpper(strings.TrimSpace(input.StaffCode))
	input.MaterialCode = strings.ToUpper(strings.TrimSpace(input.MaterialCode))
	input.MovementCode = strings.ToUpper(strings.TrimSpace(input.MovementCode))
	input.BoxCode = strings.ToUpper(strings.TrimSpace(input.BoxCode))
	input.AuctionCode = strings.ToUpper(strings.TrimSpace(input.AuctionCode))
}

func marketPriceInput(user database.User, input marketPricePayload) persistence.MarketPriceInput {
	return persistence.MarketPriceInput{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, ImportDate: input.ImportDate,
		BrandCode: input.BrandCode, ModelNumber: input.ModelNumber, ReferenceNumber: input.ReferenceNumber,
		SerialNumber: input.SerialNumber, SKU: input.SKU, ConditionCode: input.ConditionCode,
		PurchasePriceMinor: input.PurchasePriceMinor, PurchaseCurrency: input.PurchaseCurrency,
		MarketPriceMinor: input.MarketPriceMinor, MarketCurrency: input.MarketCurrency,
		SupplierCode: input.SupplierCode, StaffCode: input.StaffCode, MaterialCode: input.MaterialCode,
		MovementCode: input.MovementCode, PurchaseDate: input.PurchaseDate, StatusText: input.StatusText,
		BoxCode: input.BoxCode, AccessoryCodes: input.AccessoryCodes, AuctionCode: input.AuctionCode,
		Source: input.Source, Notes: input.Notes,
	}
}

func (s *Server) apiMarketPrices(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "相場表APIはPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.MarketPrices(r.Context(), user.OrganizationID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "market_prices_unavailable", "相場表を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiMarketPriceCreate(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "相場表APIはPostgreSQLモードで利用してください。")
		return
	}
	var input marketPricePayload
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	normalizeMarketPricePayload(&input)
	if _, err := time.Parse("2006-01-02", input.ImportDate); err != nil || input.BrandCode == "" ||
		input.PurchasePriceMinor < 0 || input.MarketPriceMinor < 0 || !validCurrency(input.PurchaseCurrency) || !validCurrency(input.MarketCurrency) {
		writeAPIError(w, http.StatusBadRequest, "invalid_market_price", "日付・ブランド・価格・通貨を確認してください。")
		return
	}
	user, _ := currentUser(r.Context())
	record, err := s.repository.CreateMarketPrice(r.Context(), marketPriceInput(user, input))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "market_price_create_failed", "相場データを登録できませんでした。マスタコードを確認してください。")
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "market_price", TargetID: record.ID,
		Action: "market_price.created", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiMarketPriceUpdate(w http.ResponseWriter, r *http.Request) {
	var input marketPricePayload
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	normalizeMarketPricePayload(&input)
	if _, err := time.Parse("2006-01-02", input.ImportDate); err != nil || input.BrandCode == "" ||
		strings.TrimSpace(input.ModelNumber) == "" || input.PurchasePriceMinor < 0 || input.MarketPriceMinor < 0 ||
		!validCurrency(input.PurchaseCurrency) || !validCurrency(input.MarketCurrency) {
		writeAPIError(w, http.StatusBadRequest, "invalid_market_price", "日付・ブランド・モデル・価格・通貨を確認してください。")
		return
	}
	if input.PurchaseDate != "" {
		if _, err := time.Parse("2006-01-02", input.PurchaseDate); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_purchase_date", "仕入日はYYYY-MM-DDで指定してください。")
			return
		}
	}
	user, _ := currentUser(r.Context())
	before, err := s.repository.MarketPrice(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "market_price_not_found", "相場データが見つかりません。")
		return
	}
	record, err := s.repository.UpdateMarketPrice(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, marketPriceInput(user, input))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "market_price_update_failed", "相場データを更新できませんでした。マスタコードを確認してください。")
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "market_price", TargetID: record.ID,
		Action: "market_price.updated", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Result: "success",
		RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, record)
}
