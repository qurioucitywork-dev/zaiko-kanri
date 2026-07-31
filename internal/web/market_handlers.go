package web

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) marketPrices(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	filter := marketProductFilterFromRequest(r)
	allProducts, err := s.store.ProductMarketPrices(r.Context(), user.OrganizationID, database.ProductMarketPriceFilter{})
	if err != nil {
		http.Error(w, "相場情報を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	brands := make([]string, 0)
	seenBrand := make(map[string]bool)
	for _, product := range allProducts {
		if product.Brand != "" && !seenBrand[product.Brand] {
			seenBrand[product.Brand] = true
			brands = append(brands, product.Brand)
		}
	}
	var records []database.ProductMarketPrice
	requested := marketSearchRequested(r)
	if requested {
		records, err = s.store.ProductMarketPrices(r.Context(), user.OrganizationID, filter)
		if err != nil {
			http.Error(w, "相場情報を検索できませんでした。", http.StatusInternalServerError)
			return
		}
	}
	s.render(w, "market", http.StatusOK, pageData{
		Title: "相場表", Active: "market", User: user, MarketProductPrices: records,
		MarketSearchRequested: requested, ProductBrands: brands,
		Query: filter.Query, Brand: filter.Brand, ModelNumber: filter.ModelNumber,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func marketSearchRequested(r *http.Request) bool {
	query := r.URL.Query()
	return query.Get("show_all") == "1" ||
		query.Has("q") || query.Has("brand") || query.Has("model_number")
}

func marketProductFilterFromRequest(r *http.Request) database.ProductMarketPriceFilter {
	return database.ProductMarketPriceFilter{
		Query:       strings.TrimSpace(r.URL.Query().Get("q")),
		Brand:       strings.TrimSpace(r.URL.Query().Get("brand")),
		ModelNumber: strings.TrimSpace(r.URL.Query().Get("model_number")),
	}
}

func (s *Server) marketPriceModal(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.store.ProductMarketPriceByProductID(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderPartial(w, "market", "market-detail", http.StatusOK, pageData{
		MarketProductPrice: record, CSRF: csrfFromRequest(r),
	})
}

func (s *Server) marketPriceUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	purchasePrice, err := database.ParseMinorAmount(r.FormValue("purchase_market_price"))
	if err != nil {
		http.Error(w, "仕入相場価格を確認してください。", http.StatusUnprocessableEntity)
		return
	}
	salePrice, err := database.ParseMinorAmount(r.FormValue("sale_market_price"))
	if err != nil {
		http.Error(w, "売値相場価格を確認してください。", http.StatusUnprocessableEntity)
		return
	}
	if err := s.store.UpdateProductMarketPrice(
		r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, purchasePrice, salePrice,
	); err != nil {
		http.Error(w, "相場価格を保存できませんでした。", http.StatusUnprocessableEntity)
		return
	}
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "product_market_price", TargetID: r.PathValue("id"),
		Action: "product_market_price.updated",
		Reason: fmt.Sprintf("purchase=%d sale=%d", purchasePrice, salePrice),
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/market-prices?show_all=1&notice="+url.QueryEscape("相場価格を保存しました。"), http.StatusSeeOther)
}

func (s *Server) marketPricesCSV(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	records, err := s.store.ProductMarketPrices(r.Context(), user.OrganizationID, marketProductFilterFromRequest(r))
	if err != nil {
		http.Error(w, "CSVを作成できませんでした。", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="market-prices.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"product_code", "purchase_market_price", "sale_market_price"})
	for _, record := range records {
		_ = writer.Write([]string{
			safeCSVCell(record.ProductCode),
			strconv.FormatInt(record.PurchaseMarketPriceMinor, 10),
			strconv.FormatInt(record.SaleMarketPriceMinor, 10),
		})
	}
	writer.Flush()
}

func (s *Server) marketPricesCSVImport(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	file, header, err := r.FormFile("csv_file")
	if err != nil {
		http.Error(w, "CSVファイルを選択してください。", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > 2<<20 ||
		strings.ToLower(filepath.Ext(header.Filename)) != ".csv" {
		http.Error(w, "2MB以下のCSVファイルを選択してください。", http.StatusUnprocessableEntity)
		return
	}
	count, err := s.store.ImportProductMarketPricesCSV(r.Context(), user.OrganizationID, user.ID, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "product_market_price", TargetID: "csv",
		Action: "product_market_price.csv_imported", Reason: fmt.Sprintf("rows=%d", count),
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/market-prices?show_all=1&notice="+
		url.QueryEscape(fmt.Sprintf("%d件の相場価格を取り込みました。", count)), http.StatusSeeOther)
}

func (s *Server) marketPriceCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	price, err := database.ParseMinorAmount(r.FormValue("price"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	record, err := s.store.AddMarketPrice(r.Context(), database.MarketPriceInput{
		OrganizationID: user.OrganizationID, MarketDate: r.FormValue("market_date"),
		Brand: r.FormValue("brand"), ModelNumber: r.FormValue("model_number"),
		ProductType: r.FormValue("product_type"), PriceMinor: price,
		Currency: r.FormValue("currency"), Source: r.FormValue("source"),
		Notes: r.FormValue("notes"), CreatedBy: user.ID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	after, _ := json.Marshal(record)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "market_price", TargetID: record.ID,
		Action: "market_price.created", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/market-prices?notice="+url.QueryEscape("相場情報を登録しました。"), http.StatusSeeOther)
}

func (s *Server) exchangeRateCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	rateScaled, err := database.ParseRate(r.FormValue("rate"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	rate, err := s.store.AddExchangeRate(r.Context(), user.OrganizationID, "USD", "JPY",
		rateScaled, r.FormValue("provider"), r.FormValue("observed_at"), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	after, _ := json.Marshal(rate)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "exchange_rate", TargetID: rate.ID,
		Action: "exchange_rate.created", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/market-prices?notice="+url.QueryEscape("為替レートをスナップショット保存しました。"), http.StatusSeeOther)
}

func (s *Server) marketImportPage(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	s.render(w, "market-import", http.StatusOK, pageData{
		Title: "相場CSV取込", Active: "market", User: user, CSRF: csrfFromRequest(r),
	})
}

func (s *Server) marketImportPreview(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	file, header, err := r.FormFile("csv_file")
	if err != nil {
		http.Error(w, "CSVファイルを選択してください。", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > 2<<20 {
		http.Error(w, "CSVファイルは2MB以下にしてください。", http.StatusUnprocessableEntity)
		return
	}
	if strings.ToLower(filepath.Ext(header.Filename)) != ".csv" {
		http.Error(w, "拡張子.csvのファイルを選択してください。", http.StatusUnprocessableEntity)
		return
	}
	batch, err := s.store.PreviewMarketCSV(r.Context(), user.OrganizationID, user.ID, filepath.Base(header.Filename), file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	after, _ := json.Marshal(batch)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "market_import_batch", TargetID: batch.ID,
		Action: "market_import.previewed", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/market-prices/import/"+batch.ID, http.StatusSeeOther)
}

func (s *Server) marketImportDetail(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	batch, err := s.store.MarketImportBatch(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "取込バッチが見つかりません。", http.StatusNotFound)
		return
	}
	s.render(w, "market-import-preview", http.StatusOK, pageData{
		Title: "CSV取込プレビュー", Active: "market", User: user, ImportBatch: batch,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) marketImportCommit(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	requireApproval := user.Role != database.RoleAdmin
	batch, err := s.store.CommitMarketImport(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, requireApproval)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	action := "market_import.committed"
	notice := fmt.Sprintf("%d件の相場情報を登録しました。", batch.ValidRows)
	if batch.Status == "pending_approval" {
		action = "market_import.approval_requested"
		notice = "CSV一括更新を承認待ちにしました。管理者の確定後に反映されます。"
	}
	after, _ := json.Marshal(batch)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "market_import_batch", TargetID: batch.ID,
		Action: action, AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	target := "/market-prices"
	if batch.Status == "pending_approval" {
		target = "/market-prices/import/" + batch.ID
	}
	http.Redirect(w, r, target+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}
