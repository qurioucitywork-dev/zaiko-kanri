package web

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type productForm struct {
	ProductCode      string
	BuyerID          string
	SupplierID       string
	PurchaseDate     string
	SKU              string
	Brand            string
	ModelNumber      string
	SerialNumber     string
	ProductType      string
	CostAmount       string
	CostCurrency     string
	BaseSalePrice    string
	BaseSaleCurrency string
	Condition        string
	Accessories      string
	Material         string
	Box              string
	Movement         string
	BeltMaterial     string
	Dial             string
	BraceletQty      string
	Features         string
	InternalComment  string
	DuplicateReason  string
}

var braceletQuantityFeaturePattern = regexp.MustCompile(`(?:^|[\s　,、])コマ数[\s　]*[:：][\s　]*[0-9]+`)

func mergeBraceletQuantityFeature(features, quantity string, selected bool) (string, error) {
	cleaned := strings.TrimSpace(braceletQuantityFeaturePattern.ReplaceAllString(features, " "))
	quantity = strings.TrimSpace(quantity)
	if !selected || quantity == "" {
		return cleaned, nil
	}
	count, err := strconv.Atoi(quantity)
	if err != nil || count < 1 || count > 999 {
		return cleaned, errors.New("コマ数は1から999の数字で入力してください")
	}
	if cleaned != "" {
		cleaned += "　"
	}
	return cleaned + "コマ数：" + strconv.Itoa(count), nil
}

func defaultProductForm() productForm {
	return productForm{
		PurchaseDate:     time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02"),
		CostCurrency:     "JPY",
		BaseSaleCurrency: "USD",
	}
}

func (s *Server) products(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	filter := productFilterFromRequest(r, user)
	page, err := s.store.PagedProducts(r.Context(), user.OrganizationID, filter)
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, "在庫を取得できませんでした。条件を変更して再試行してください。")
		return
	}
	suppliers, _ := s.store.Suppliers(r.Context(), user.OrganizationID)
	users, _ := s.store.ProductBuyers(r.Context(), user.OrganizationID)
	// The inventory search uses the same seven catalog brands and order as the mock.
	// Product registration intentionally uses the full ten-item brand master.
	brands := []string{
		"IWC", "オメガ", "カルティエ", "グランドセイコー",
		"パテック・フィリップ", "ブライトリング", "ロレックス",
	}
	currentQuery := r.URL.Query()
	currentQuery.Del("page")
	currentURL := "/products?" + currentQuery.Encode()
	if len(currentQuery) == 0 {
		currentURL = "/products?"
	}
	data := pageData{
		Title: "在庫一覧", Active: "products", User: user, Products: page.Products,
		Query: filter.Query, Brand: filter.Brand, ModelNumber: filter.ModelNumber,
		SerialNumber: filter.SerialNumber, SKU: filter.SKU, SupplierID: filter.SupplierID,
		BuyerID: filter.BuyerID, Box: filter.Box, Accessory: filter.Accessory,
		PurchaseDateFrom: filter.PurchaseDateFrom, PurchaseDateTo: filter.PurchaseDateTo,
		Status: filter.Status, Sort: filter.Sort, IncludeCancelled: filter.IncludeCancelled,
		InventorySearchRequested: inventorySearchRequested(r),
		Suppliers:                suppliers, Users: users, ProductBrands: brands,
		TotalProducts: page.Total, Page: page.Page, TotalPages: page.TotalPages,
		PreviousPage: page.Page - 1, NextPage: page.Page + 1,
		HasPrevious: page.Page > 1, HasNext: page.Page < page.TotalPages,
		CurrentURL: currentURL,
		CSRF:       csrfFromRequest(r),
		Notice:     r.URL.Query().Get("notice"),
	}
	if isHXRequest(r) {
		partial := "product-prompt"
		if data.InventorySearchRequested {
			partial = "product-results"
		}
		s.renderPartial(w, "products", partial, http.StatusOK, data)
		return
	}
	s.render(w, "products", http.StatusOK, data)
}

func inventorySearchRequested(r *http.Request) bool {
	query := r.URL.Query()
	for _, name := range []string{
		"q", "brand", "model_number", "serial_number", "sku", "supplier_id",
		"buyer_id", "box", "accessory", "status", "purchase_date_from",
		"purchase_date_to", "sort", "page", "include_cancelled",
	} {
		if _, exists := query[name]; exists {
			return true
		}
	}
	return query.Get("show_all") == "1"
}

func productFilterFromRequest(r *http.Request, user database.User) database.ProductFilter {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	accessories := make([]string, 0, len(r.URL.Query()["accessory"]))
	for _, value := range r.URL.Query()["accessory"] {
		for _, accessory := range strings.Split(value, ",") {
			if accessory = strings.ToUpper(strings.TrimSpace(accessory)); accessory != "" {
				accessories = append(accessories, accessory)
			}
		}
	}
	if sort == "" {
		sort = "purchase_desc"
	}
	includeCancelled := user.Role == database.RoleAdmin &&
		(r.URL.Query().Get("include_cancelled") == "1" || status == "cancelled")
	if user.Role != database.RoleAdmin && status == "cancelled" {
		status = ""
	}
	return database.ProductFilter{
		Query:            strings.TrimSpace(r.URL.Query().Get("q")),
		Brand:            strings.TrimSpace(r.URL.Query().Get("brand")),
		ModelNumber:      strings.TrimSpace(r.URL.Query().Get("model_number")),
		SerialNumber:     strings.TrimSpace(r.URL.Query().Get("serial_number")),
		SKU:              strings.TrimSpace(r.URL.Query().Get("sku")),
		SupplierID:       strings.TrimSpace(r.URL.Query().Get("supplier_id")),
		BuyerID:          strings.TrimSpace(r.URL.Query().Get("buyer_id")),
		Box:              strings.TrimSpace(r.URL.Query().Get("box")),
		Accessory:        strings.Join(accessories, ","),
		PurchaseDateFrom: strings.TrimSpace(r.URL.Query().Get("purchase_date_from")),
		PurchaseDateTo:   strings.TrimSpace(r.URL.Query().Get("purchase_date_to")),
		Status:           status,
		Sort:             sort, Page: page, PageSize: 20,
		IncludeCancelled: includeCancelled,
		VisibleInventory: true,
	}
}

func (s *Server) productsCSV(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	filter := productFilterFromRequest(r, user)
	filter.Page = 1
	filter.PageSize = 500
	page, err := s.store.PagedProducts(r.Context(), user.OrganizationID, filter)
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, "CSVを作成できませんでした。")
		return
	}
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "inventory", TargetID: "products", Action: "inventory.cost_csv.exported",
		Reason: fmt.Sprintf("query=%q status=%q rows=%d", filter.Query, filter.Status, len(page.Products)),
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="inventory-products.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"商品コード", "SKU", "ブランド", "型番", "シリアル", "仕入日", "仕入先", "原価", "原価通貨", "基準売価", "売価通貨", "在庫状態"})
	for _, product := range page.Products {
		_ = writer.Write([]string{
			safeCSVCell(product.ProductCode), safeCSVCell(product.SKU), safeCSVCell(product.Brand),
			safeCSVCell(product.ModelNumber), safeCSVCell(product.SerialNumber),
			product.PurchaseDate, safeCSVCell(product.SupplierName), strconv.FormatInt(product.CostAmountMinor, 10),
			product.CostCurrency, strconv.FormatInt(product.BaseSalePriceMinor, 10),
			product.BaseSaleCurrency, product.InventoryStatus,
		})
	}
	writer.Flush()
}

func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func (s *Server) productNew(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	suppliers, err := s.store.Suppliers(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "仕入先を取得できませんでした", http.StatusInternalServerError)
		return
	}
	users, err := s.store.ProductBuyers(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "仕入担当者を取得できませんでした", http.StatusInternalServerError)
		return
	}
	form := defaultProductForm()
	nextCode, err := s.store.NextProductCode(r.Context(), user.OrganizationID, form.PurchaseDate)
	if err != nil {
		http.Error(w, "商品コードを採番できませんでした", http.StatusInternalServerError)
		return
	}
	brands, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "brands")
	materials, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "materials")
	movements, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "movements")
	conditions, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "conditions")
	accessories, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "accessories")
	usdJPYRate, rateErr := s.store.LatestExchangeRate(r.Context(), user.OrganizationID, "USD", "JPY")
	s.render(w, "product-new", http.StatusOK, pageData{
		Title: "商品登録", Active: "product-new", User: user, Suppliers: suppliers, Users: users,
		ProductForm: form, NextProductCode: nextCode,
		ProductBrandOptions: brands, ProductMaterialOptions: materials,
		ProductMovementOptions: movements, ProductConditionOptions: conditions,
		ProductAccessoryOptions: accessories, USDJPYRate: usdJPYRate,
		USDJPYRateAvailable: rateErr == nil, CSRF: csrfFromRequest(r),
	})
}

func (s *Server) productNextCode(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	purchaseDate := strings.TrimSpace(r.URL.Query().Get("purchase_date"))
	if _, err := time.Parse("2006-01-02", purchaseDate); err != nil {
		writeRequestError(w, r, http.StatusUnprocessableEntity, "仕入日を正しく入力してください。")
		return
	}
	nextCode, err := s.store.NextProductCode(r.Context(), user.OrganizationID, purchaseDate)
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, "商品コードを採番できませんでした。")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{"product_code": nextCode})
}

func (s *Server) productCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	accessoryValues := r.Form["accessories"]
	braceletSelected := false
	for _, accessory := range accessoryValues {
		if strings.EqualFold(strings.TrimSpace(accessory), "BRACELET PARTS") {
			braceletSelected = true
			break
		}
	}
	form := productForm{
		ProductCode: r.FormValue("product_code"), BuyerID: r.FormValue("buyer_id"),
		SupplierID: r.FormValue("supplier_id"), PurchaseDate: r.FormValue("purchase_date"),
		SKU: r.FormValue("sku"), Brand: r.FormValue("brand"), ModelNumber: r.FormValue("model_number"),
		SerialNumber: r.FormValue("serial_number"), ProductType: r.FormValue("product_type"),
		CostAmount: r.FormValue("cost_amount"), CostCurrency: r.FormValue("cost_currency"),
		BaseSalePrice: r.FormValue("base_sale_price"), BaseSaleCurrency: r.FormValue("base_sale_currency"),
		Condition: r.FormValue("condition"), Accessories: strings.Join(accessoryValues, ", "),
		Material: r.FormValue("material"), Box: r.FormValue("box"), Movement: r.FormValue("movement"),
		BeltMaterial: r.FormValue("belt_material"), Dial: r.FormValue("dial"),
		BraceletQty: r.FormValue("bracelet_qty"), Features: r.FormValue("features"), InternalComment: r.FormValue("internal_comment"),
		DuplicateReason: r.FormValue("duplicate_reason"),
	}
	// Enforce the currency policy on the server. Existing products retain
	// their persisted currencies when edited.
	form.CostCurrency = "JPY"
	form.BaseSaleCurrency = "USD"
	var braceletErr error
	form.Features, braceletErr = mergeBraceletQuantityFeature(form.Features, form.BraceletQty, braceletSelected)
	imageHeaders := []*multipart.FileHeader{}
	if r.MultipartForm != nil {
		imageHeaders = r.MultipartForm.File["images"]
	}
	if len(imageHeaders) > 10 {
		s.renderProductRegistrationError(w, r, user, form, nil, errors.New("商品画像は10枚までです"))
		return
	}
	for _, header := range imageHeaders {
		if err := validateProductImageHeader(header); err != nil {
			s.renderProductRegistrationError(w, r, user, form, nil, err)
			return
		}
	}
	cost, costErr := database.ParseMinorAmount(form.CostAmount)
	salePrice, priceErr := database.ParseMinorAmount(form.BaseSalePrice)
	var createErr error
	if braceletErr != nil {
		createErr = braceletErr
	} else if strings.TrimSpace(form.ProductCode) == "" {
		createErr = errors.New("商品コードを入力するか「採番」を押してください")
	} else if costErr != nil {
		createErr = costErr
	} else if priceErr != nil {
		createErr = priceErr
	}
	var product database.Product
	if createErr == nil {
		createdBy := user.ID
		if form.BuyerID != "" {
			users, usersErr := s.store.Users(r.Context(), user.OrganizationID)
			if usersErr != nil {
				createErr = usersErr
			} else {
				found := false
				for _, candidate := range users {
					if candidate.ID == form.BuyerID && candidate.Active {
						createdBy, found = candidate.ID, true
						break
					}
				}
				if !found {
					createErr = errors.New("仕入担当者を選択してください")
				}
			}
		}
		if createErr == nil {
			product, createErr = s.store.CreateSingleProduct(r.Context(), database.SingleProductInput{
				OrganizationID: user.OrganizationID, SupplierID: form.SupplierID, PurchaseDate: form.PurchaseDate,
				RequestedProductCode: form.ProductCode,
				SKU:                  form.SKU, Brand: form.Brand, ModelNumber: form.ModelNumber, SerialNumber: form.SerialNumber,
				ProductType: form.ProductType, CostAmountMinor: cost, CostCurrency: form.CostCurrency,
				BaseSalePriceMinor: salePrice, BaseSaleCurrency: form.BaseSaleCurrency, Condition: form.Condition,
				Accessories: form.Accessories, Material: form.Material, Box: form.Box, Movement: form.Movement,
				BeltMaterial: form.BeltMaterial, Dial: form.Dial, Features: form.Features,
				InternalComment: form.InternalComment, DuplicateReason: form.DuplicateReason, CreatedBy: createdBy,
			})
		}
	}
	if createErr != nil {
		var duplicateErr *database.SerialDuplicateError
		var duplicates []database.Product
		if errors.As(createErr, &duplicateErr) {
			duplicates = duplicateErr.Candidates
		}
		s.renderProductRegistrationError(w, r, user, form, duplicates, createErr)
		return
	}
	for _, header := range imageHeaders {
		if _, err := s.saveProductImage(r.Context(), user, product.ID, header); err != nil {
			http.Redirect(w, r, "/products/"+product.ID+"?notice="+url.QueryEscape("商品を登録しましたが、一部の画像を保存できませんでした。詳細画面から再登録してください。"), http.StatusSeeOther)
			return
		}
	}
	after, _ := json.Marshal(product)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product", TargetID: product.ID,
		Action: "product.single_created", AfterJSON: string(after), Reason: form.DuplicateReason, Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/products/"+product.ID+"?notice="+url.QueryEscape("商品を登録しました。簡易仕入伝票も自動生成されています。"), http.StatusSeeOther)
}

func (s *Server) renderProductRegistrationError(w http.ResponseWriter, r *http.Request, user database.User, form productForm, duplicates []database.Product, renderErr error) {
	suppliers, _ := s.store.Suppliers(r.Context(), user.OrganizationID)
	users, _ := s.store.ProductBuyers(r.Context(), user.OrganizationID)
	brands, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "brands")
	materials, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "materials")
	movements, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "movements")
	conditions, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "conditions")
	accessories, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "accessories")
	usdJPYRate, rateErr := s.store.LatestExchangeRate(r.Context(), user.OrganizationID, "USD", "JPY")
	nextCode, _ := s.store.NextProductCode(r.Context(), user.OrganizationID, form.PurchaseDate)
	s.render(w, "product-new", http.StatusUnprocessableEntity, pageData{
		Title: "商品登録", Active: "product-new", User: user, Suppliers: suppliers, Users: users,
		ProductForm: form, Duplicates: duplicates, NextProductCode: nextCode,
		ProductBrandOptions: brands, ProductMaterialOptions: materials,
		ProductMovementOptions: movements, ProductConditionOptions: conditions,
		ProductAccessoryOptions: accessories, USDJPYRate: usdJPYRate,
		USDJPYRateAvailable: rateErr == nil, CSRF: csrfFromRequest(r), Error: renderErr.Error(),
	})
}

func (s *Server) productDetail(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	product, err := s.store.Product(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "商品が見つかりません。", http.StatusNotFound)
		return
	}
	s.render(w, "product-detail", http.StatusOK, pageData{
		Title: "商品詳細", Active: "products", User: user, Product: product,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) productModal(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	product, err := s.store.Product(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "商品が見つかりません。")
		return
	}
	s.renderPartial(w, "product-modal", "content", http.StatusOK, pageData{
		Title: "商品詳細", Active: "products", User: user, Product: product, CSRF: csrfFromRequest(r),
	})
}

func (s *Server) productEditModal(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	product, err := s.store.Product(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "商品が見つかりません。")
		return
	}
	suppliers, _ := s.store.Suppliers(r.Context(), user.OrganizationID)
	users, _ := s.store.ProductBuyers(r.Context(), user.OrganizationID)
	brands, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "brands")
	materials, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "materials")
	movements, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "movements")
	conditions, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "conditions")
	s.renderPartial(w, "product-edit-modal", "content", http.StatusOK, pageData{
		Title: "商品情報を編集", Active: "products", User: user, Product: product,
		Suppliers: suppliers, Users: users, ProductBrandOptions: brands,
		ProductMaterialOptions: materials, ProductMovementOptions: movements,
		ProductConditionOptions: conditions, CSRF: csrfFromRequest(r),
	})
}

func (s *Server) productUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	productID := r.PathValue("id")
	before, err := s.store.Product(r.Context(), user.OrganizationID, productID)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "商品が見つかりません。")
		return
	}
	cost, costErr := database.ParseMinorAmount(r.FormValue("cost_amount"))
	salePrice, saleErr := database.ParseMinorAmount(r.FormValue("base_sale_price"))
	if costErr != nil || saleErr != nil {
		writeRequestError(w, r, http.StatusUnprocessableEntity, "金額を0以上の整数で入力してください。")
		return
	}
	err = s.store.UpdateProduct(r.Context(), database.UpdateProductInput{
		OrganizationID: user.OrganizationID, ProductID: productID, ActorID: user.ID,
		BuyerID: r.FormValue("buyer_id"), SupplierID: r.FormValue("supplier_id"),
		PurchaseDate: r.FormValue("purchase_date"), Brand: r.FormValue("brand"),
		ProductType: r.FormValue("product_type"), ModelNumber: r.FormValue("model_number"),
		SerialNumber: r.FormValue("serial_number"), InventoryStatus: r.FormValue("inventory_status"),
		Condition: r.FormValue("condition"), Material: r.FormValue("material"),
		Movement: r.FormValue("movement"), BeltMaterial: r.FormValue("belt_material"),
		Dial: r.FormValue("dial"), Box: r.FormValue("box"),
		Accessories: strings.Join(r.Form["accessories"], ", "), Features: r.FormValue("features"),
		CostAmountMinor: cost, BaseSalePriceMinor: salePrice, ChangeMemo: r.FormValue("change_memo"),
	})
	if err != nil {
		writeRequestError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}
	after, _ := s.store.Product(r.Context(), user.OrganizationID, productID)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product", TargetID: productID,
		Action: "inventory.product_updated", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON),
		Reason: strings.TrimSpace(r.FormValue("change_memo")), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "product_id": productID})
		return
	}
	http.Redirect(w, r, "/products?show_all=1&notice="+url.QueryEscape("商品情報を更新しました。"), http.StatusSeeOther)
}

func (s *Server) productStatus(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	productID := r.PathValue("id")
	before, err := s.store.Product(r.Context(), user.OrganizationID, productID)
	if err != nil {
		http.Error(w, "商品が見つかりません。", http.StatusNotFound)
		return
	}
	toStatus := r.FormValue("status")
	reason := r.FormValue("reason")
	if err := s.store.UpdateProductStatus(r.Context(), user.OrganizationID, productID, user.ID, toStatus, reason); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	after, _ := s.store.Product(r.Context(), user.OrganizationID, productID)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product", TargetID: productID,
		Action: "inventory.status_changed", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON),
		Reason: reason, Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/products/"+productID+"?notice="+url.QueryEscape("在庫状態を更新しました。"), http.StatusSeeOther)
}

func (s *Server) purchases(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	slips, err := s.store.PurchaseSlips(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "仕入伝票を取得できませんでした", http.StatusInternalServerError)
		return
	}
	suppliers, err := s.store.Suppliers(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "仕入先を取得できませんでした", http.StatusInternalServerError)
		return
	}
	users, err := s.store.Users(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "仕入担当者を取得できませんでした", http.StatusInternalServerError)
		return
	}
	brands, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "brands")
	materials, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "materials")
	movements, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "movements")
	conditions, _ := s.store.MasterRecords(r.Context(), user.OrganizationID, "conditions")
	usdJPYRate, rateErr := s.store.LatestExchangeRate(r.Context(), user.OrganizationID, "USD", "JPY")
	todayISO := time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
	nextNumber, err := s.store.NextPurchaseSlipNumber(r.Context(), user.OrganizationID, todayISO)
	if err != nil {
		http.Error(w, "仕入伝票番号を採番できませんでした", http.StatusInternalServerError)
		return
	}
	s.render(w, "purchases", http.StatusOK, pageData{
		Title: "仕入登録", Active: "purchases", User: user, Purchases: slips,
		Suppliers: suppliers, Users: users, TodayISO: todayISO, NextPurchaseNumber: nextNumber,
		ProductBrandOptions: brands, ProductMaterialOptions: materials,
		ProductMovementOptions: movements, ProductConditionOptions: conditions,
		USDJPYRate: usdJPYRate, USDJPYRateAvailable: rateErr == nil,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) purchaseNew(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/purchases", http.StatusSeeOther)
}

func (s *Server) purchaseCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	productCodes := r.Form["product_code"]
	skus := r.Form["sku"]
	brands := r.Form["brand"]
	models := r.Form["model_number"]
	serials := r.Form["serial_number"]
	types := r.Form["product_type"]
	quantities := r.Form["quantity"]
	costs := r.Form["unit_cost"]
	salePrices := r.Form["base_sale_price"]
	materials := r.Form["material"]
	movements := r.Form["movement"]
	conditions := r.Form["condition"]
	beltMaterials := r.Form["belt_material"]
	dials := r.Form["dial"]
	boxes := r.Form["box"]
	accessories := r.Form["accessories"]
	features := r.Form["features"]
	formValueAt := func(values []string, index int) string {
		if index < len(values) {
			return values[index]
		}
		return ""
	}
	var lines []database.PurchaseLineInput
	for index, brand := range brands {
		if index >= len(models) || index >= len(types) || index >= len(quantities) || index >= len(costs) || index >= len(salePrices) {
			http.Error(w, "仕入明細の形式が正しくありません。", http.StatusBadRequest)
			return
		}
		quantity, quantityErr := strconv.Atoi(quantities[index])
		cost, costErr := database.ParseMinorAmount(costs[index])
		salePrice, salePriceErr := database.ParseMinorAmount(salePrices[index])
		if quantityErr != nil || costErr != nil || salePriceErr != nil {
			http.Error(w, "数量、仕入金額、売価を確認してください。", http.StatusUnprocessableEntity)
			return
		}
		lines = append(lines, database.PurchaseLineInput{
			Quantity: quantity, UnitCostMinor: cost, BaseSalePriceMinor: salePrice,
			Currency: "JPY", SaleCurrency: "USD",
			ProductCode: formValueAt(productCodes, index), SKU: formValueAt(skus, index),
			Brand: brand, ModelNumber: models[index], SerialNumber: formValueAt(serials, index), ProductType: types[index],
			Material: formValueAt(materials, index), Movement: formValueAt(movements, index),
			Condition: formValueAt(conditions, index), BeltMaterial: formValueAt(beltMaterials, index),
			Dial: formValueAt(dials, index), Box: formValueAt(boxes, index),
			Accessories: formValueAt(accessories, index), Features: formValueAt(features, index),
		})
	}
	createdBy := user.ID
	if requestedBuyer := strings.TrimSpace(r.FormValue("buyer_id")); requestedBuyer != "" {
		users, usersErr := s.store.Users(r.Context(), user.OrganizationID)
		if usersErr != nil {
			http.Error(w, "仕入担当者を確認できませんでした。", http.StatusInternalServerError)
			return
		}
		found := false
		for _, candidate := range users {
			if candidate.ID == requestedBuyer && candidate.Active {
				createdBy, found = candidate.ID, true
				break
			}
		}
		if !found {
			http.Error(w, "仕入担当者を選択してください。", http.StatusUnprocessableEntity)
			return
		}
	}
	slip, confirmed, err := s.store.CreateConfirmedPurchase(r.Context(), database.CreatePurchaseInput{
		OrganizationID: user.OrganizationID, SupplierID: r.FormValue("supplier_id"),
		PurchaseDate: r.FormValue("purchase_date"), Notes: r.FormValue("notes"),
		CreatedBy: createdBy, Lines: lines,
	}, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	after, _ := json.Marshal(struct {
		Slip      database.PurchaseSlip
		Confirmed database.ConfirmResult
	}{Slip: slip, Confirmed: confirmed})
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "purchase_slip", TargetID: slip.ID,
		Action: "purchase.created_and_confirmed", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	notice := fmt.Sprintf("仕入伝票 %s を登録し、在庫に%d件反映しました。", slip.SlipNumber, len(confirmed.Products))
	http.Redirect(w, r, "/purchases?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

func (s *Server) purchasesCSV(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	slips, err := s.store.PurchaseSlips(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "仕入伝票を出力できませんでした", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="purchases.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{
		"商品コード", "SKU", "ブランド名", "モデル名", "型番", "シリアルNo.", "数量",
		"仕入金額", "仕入通貨", "売価", "売価通貨", "素材", "駆動方式", "コンディション",
		"ベルト素材", "文字盤", "BOX番号", "付属品", "特徴・備考",
	})
	for _, slip := range slips {
		detail, detailErr := s.store.Purchase(r.Context(), user.OrganizationID, slip.ID)
		if detailErr != nil {
			http.Error(w, "仕入明細を出力できませんでした", http.StatusInternalServerError)
			return
		}
		for _, line := range detail.Lines {
			_ = writer.Write([]string{
				"", safeCSVCell(line.SKU), safeCSVCell(line.Brand),
				safeCSVCell(line.ProductType), safeCSVCell(line.ModelNumber), safeCSVCell(line.SerialNumber),
				strconv.Itoa(line.Quantity), strconv.FormatInt(line.UnitCostMinor, 10),
				line.Currency, strconv.FormatInt(line.BaseSalePriceMinor, 10), line.SaleCurrency,
				safeCSVCell(line.Material), safeCSVCell(line.Movement), safeCSVCell(line.Condition),
				safeCSVCell(line.BeltMaterial), safeCSVCell(line.Dial), safeCSVCell(line.Box),
				safeCSVCell(line.Accessories), safeCSVCell(line.Features),
			})
		}
	}
	writer.Flush()
}

func (s *Server) purchaseDetail(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	slip, err := s.store.Purchase(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "仕入伝票が見つかりません。", http.StatusNotFound)
		return
	}
	s.render(w, "purchase-detail", http.StatusOK, pageData{
		Title: "仕入伝票詳細", Active: "purchases", User: user, Purchase: slip,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) purchaseConfirm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	slipID := r.PathValue("id")
	result, err := s.store.ConfirmPurchase(r.Context(), user.OrganizationID, slipID, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	after, _ := json.Marshal(result)
	action := "purchase.confirmed"
	if result.AlreadyConfirmed {
		action = "purchase.confirm_replayed"
	}
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "purchase_slip", TargetID: slipID,
		Action: action, AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	notice := fmt.Sprintf("仕入を確定し、商品を%d件生成しました。", len(result.Products))
	if result.AlreadyConfirmed {
		notice = "この伝票は確定済みです。商品は重複生成されていません。"
	}
	http.Redirect(w, r, "/purchases/"+slipID+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

func (s *Server) productImageUpload(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	productID := r.PathValue("id")
	if _, err := s.store.Product(r.Context(), user.OrganizationID, productID); err != nil {
		http.Error(w, "商品が見つかりません。", http.StatusNotFound)
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "画像を選択してください。", http.StatusBadRequest)
		return
	}
	_ = file.Close()
	if _, err := s.saveProductImage(r.Context(), user, productID, header); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/products/"+productID+"?notice="+url.QueryEscape("商品画像を登録しました。"), http.StatusSeeOther)
}

func validateProductImageHeader(header *multipart.FileHeader) error {
	if header == nil || header.Size <= 0 || header.Size > 8<<20 {
		return errors.New("画像は1枚8MB以下にしてください")
	}
	file, err := header.Open()
	if err != nil {
		return errors.New("画像を読み取れませんでした")
	}
	defer file.Close()
	buffer := make([]byte, 512)
	read, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return errors.New("画像を読み取れませんでした")
	}
	contentType := http.DetectContentType(buffer[:read])
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		return errors.New("JPEG、PNG、WebP画像だけ登録できます")
	}
	return nil
}

func (s *Server) saveProductImage(ctx context.Context, user database.User, productID string, header *multipart.FileHeader) (database.ProductImage, error) {
	if err := validateProductImageHeader(header); err != nil {
		return database.ProductImage{}, err
	}
	file, err := header.Open()
	if err != nil {
		return database.ProductImage{}, errors.New("画像を読み取れませんでした")
	}
	defer file.Close()
	buffer := make([]byte, 512)
	read, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return database.ProductImage{}, errors.New("画像を読み取れませんでした")
	}
	contentType := http.DetectContentType(buffer[:read])
	extensions := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}
	extension := extensions[contentType]
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return database.ProductImage{}, errors.New("画像を読み取れませんでした")
	}
	imageID, _ := database.NewID("img")
	relativePath := filepath.Join(user.OrganizationID, productID, imageID+extension)
	targetPath := filepath.Join(s.cfg.UploadDirectory, relativePath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return database.ProductImage{}, errors.New("画像保存先を準備できませんでした")
	}
	destination, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return database.ProductImage{}, errors.New("画像を保存できませんでした")
	}
	written, copyErr := io.Copy(destination, io.LimitReader(file, 8<<20))
	closeErr := destination.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(targetPath)
		return database.ProductImage{}, errors.New("画像を保存できませんでした")
	}
	image, err := s.store.AddProductImage(ctx, database.ProductImage{
		ID: imageID, ProductID: productID, StoragePath: relativePath, OriginalName: filepath.Base(header.Filename),
		ContentType: contentType, SizeBytes: written,
	}, user.OrganizationID, user.ID)
	if err != nil {
		_ = os.Remove(targetPath)
		return database.ProductImage{}, err
	}
	after, _ := json.Marshal(image)
	_ = s.store.WriteAudit(ctx, database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product_image", TargetID: image.ID,
		Action: "product_image.uploaded", AfterJSON: string(after), Result: "success",
	})
	return image, nil
}

func (s *Server) productImage(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	image, err := s.store.ProductImage(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	base, err := filepath.Abs(s.cfg.UploadDirectory)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	target, err := filepath.Abs(filepath.Join(base, image.StoragePath))
	if err != nil || (!strings.HasPrefix(target, base+string(os.PathSeparator)) && target != base) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", image.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="`+url.PathEscape(image.OriginalName)+`"`)
	http.ServeFile(w, r, target)
}
