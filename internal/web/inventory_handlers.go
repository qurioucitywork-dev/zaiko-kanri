package web

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type productForm struct {
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
	DuplicateReason  string
}

func defaultProductForm() productForm {
	return productForm{
		PurchaseDate:     time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02"),
		ProductType:      "腕時計",
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
	data := pageData{
		Title: "在庫一覧", Active: "products", User: user, Products: page.Products,
		Query: filter.Query, Status: filter.Status, Sort: filter.Sort, IncludeCancelled: filter.IncludeCancelled,
		TotalProducts: page.Total, Page: page.Page, TotalPages: page.TotalPages,
		PreviousPage: page.Page - 1, NextPage: page.Page + 1,
		HasPrevious: page.Page > 1, HasNext: page.Page < page.TotalPages,
		CSRF:   csrfFromRequest(r),
		Notice: r.URL.Query().Get("notice"),
	}
	if isHXRequest(r) {
		s.renderPartial(w, "products", "product-results", http.StatusOK, data)
		return
	}
	s.render(w, "products", http.StatusOK, data)
}

func productFilterFromRequest(r *http.Request, user database.User) database.ProductFilter {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort == "" {
		sort = "purchase_desc"
	}
	includeCancelled := (user.Role == database.RoleAdmin || user.Role == database.RoleWorker) &&
		(r.URL.Query().Get("include_cancelled") == "1" || status == "cancelled")
	if user.Role != database.RoleAdmin && user.Role != database.RoleWorker && status == "cancelled" {
		status = ""
	}
	return database.ProductFilter{
		Query: strings.TrimSpace(r.URL.Query().Get("q")), Status: status,
		Sort: sort, Page: page, PageSize: 20,
		IncludeCancelled: includeCancelled,
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
	_ = writer.Write([]string{"管理番号", "SKU", "ブランド", "型番", "シリアル", "仕入日", "仕入先", "原価", "原価通貨", "売価", "売価通貨", "ステータス"})
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
	s.render(w, "product-new", http.StatusOK, pageData{
		Title: "商品単品登録", Active: "product-new", User: user, Suppliers: suppliers,
		ProductForm: defaultProductForm(), CSRF: csrfFromRequest(r),
	})
}

func (s *Server) productCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	form := productForm{
		SupplierID: r.FormValue("supplier_id"), PurchaseDate: r.FormValue("purchase_date"),
		SKU: r.FormValue("sku"), Brand: r.FormValue("brand"), ModelNumber: r.FormValue("model_number"),
		SerialNumber: r.FormValue("serial_number"), ProductType: r.FormValue("product_type"),
		CostAmount: r.FormValue("cost_amount"), CostCurrency: r.FormValue("cost_currency"),
		BaseSalePrice: r.FormValue("base_sale_price"), BaseSaleCurrency: "USD",
		Condition: r.FormValue("condition"), Accessories: r.FormValue("accessories"),
		DuplicateReason: r.FormValue("duplicate_reason"),
	}
	cost, costErr := database.ParseMinorAmount(form.CostAmount)
	salePrice, priceErr := database.ParseMinorAmount(form.BaseSalePrice)
	var createErr error
	if costErr != nil {
		createErr = costErr
	} else if priceErr != nil {
		createErr = priceErr
	}
	var product database.Product
	if createErr == nil {
		product, createErr = s.store.CreateSingleProduct(r.Context(), database.SingleProductInput{
			OrganizationID: user.OrganizationID, SupplierID: form.SupplierID, PurchaseDate: form.PurchaseDate,
			SKU: form.SKU, Brand: form.Brand, ModelNumber: form.ModelNumber, SerialNumber: form.SerialNumber,
			ProductType: form.ProductType, CostAmountMinor: cost, CostCurrency: form.CostCurrency,
			BaseSalePriceMinor: salePrice, BaseSaleCurrency: form.BaseSaleCurrency, Condition: form.Condition,
			Accessories: form.Accessories, DuplicateReason: form.DuplicateReason, CreatedBy: user.ID,
		})
	}
	if createErr != nil {
		suppliers, _ := s.store.Suppliers(r.Context(), user.OrganizationID)
		var duplicateErr *database.SerialDuplicateError
		var duplicates []database.Product
		if errors.As(createErr, &duplicateErr) {
			duplicates = duplicateErr.Candidates
		}
		s.render(w, "product-new", http.StatusUnprocessableEntity, pageData{
			Title: "商品単品登録", Active: "product-new", User: user, Suppliers: suppliers,
			ProductForm: form, Duplicates: duplicates, CSRF: csrfFromRequest(r), Error: createErr.Error(),
		})
		return
	}
	after, _ := json.Marshal(product)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product", TargetID: product.ID,
		Action: "product.single_created", AfterJSON: string(after), Reason: form.DuplicateReason, Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/products/"+product.ID+"?notice="+url.QueryEscape("商品を登録しました。簡易仕入伝票も自動生成されています。"), http.StatusSeeOther)
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
	s.render(w, "purchases", http.StatusOK, pageData{
		Title: "仕入管理", Active: "purchases", User: user, Purchases: slips,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) purchaseNew(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	suppliers, err := s.store.Suppliers(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "仕入先を取得できませんでした", http.StatusInternalServerError)
		return
	}
	s.render(w, "purchase-new", http.StatusOK, pageData{
		Title: "仕入伝票登録", Active: "purchases", User: user, Suppliers: suppliers,
		CSRF: csrfFromRequest(r),
	})
}

func (s *Server) purchaseCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	brands := r.Form["brand"]
	models := r.Form["model_number"]
	types := r.Form["product_type"]
	quantities := r.Form["quantity"]
	costs := r.Form["unit_cost"]
	currencies := r.Form["currency"]
	var lines []database.PurchaseLineInput
	for index, brand := range brands {
		if index >= len(models) || index >= len(types) || index >= len(quantities) || index >= len(costs) || index >= len(currencies) {
			http.Error(w, "仕入明細の形式が正しくありません。", http.StatusBadRequest)
			return
		}
		quantity, quantityErr := strconv.Atoi(quantities[index])
		cost, costErr := database.ParseMinorAmount(costs[index])
		if quantityErr != nil || costErr != nil {
			http.Error(w, "数量と原価を確認してください。", http.StatusUnprocessableEntity)
			return
		}
		lines = append(lines, database.PurchaseLineInput{
			Quantity: quantity, UnitCostMinor: cost, Currency: currencies[index],
			Brand: brand, ModelNumber: models[index], ProductType: types[index],
		})
	}
	slip, err := s.store.CreatePurchaseDraft(r.Context(), database.CreatePurchaseInput{
		OrganizationID: user.OrganizationID, SupplierID: r.FormValue("supplier_id"),
		PurchaseDate: r.FormValue("purchase_date"), Notes: r.FormValue("notes"),
		CreatedBy: user.ID, Lines: lines,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	after, _ := json.Marshal(slip)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "purchase_slip", TargetID: slip.ID,
		Action: "purchase.created", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/purchases/"+slip.ID+"?notice="+url.QueryEscape("仕入伝票を下書き保存しました。"), http.StatusSeeOther)
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
	defer file.Close()
	if header.Size <= 0 || header.Size > 8<<20 {
		http.Error(w, "画像は8MB以下にしてください。", http.StatusUnprocessableEntity)
		return
	}
	buffer := make([]byte, 512)
	read, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "画像を読み取れませんでした。", http.StatusBadRequest)
		return
	}
	contentType := http.DetectContentType(buffer[:read])
	extensions := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}
	extension, allowed := extensions[contentType]
	if !allowed {
		http.Error(w, "JPEG、PNG、WebP画像だけ登録できます。", http.StatusUnprocessableEntity)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "画像を読み取れませんでした。", http.StatusBadRequest)
		return
	}
	imageID, _ := database.NewID("img")
	relativePath := path.Join(user.OrganizationID, productID, imageID+extension)
	written, err := s.objects.Put(r.Context(), relativePath, contentType, io.LimitReader(file, 8<<20))
	if err != nil {
		http.Error(w, "画像を保存できませんでした。", http.StatusInternalServerError)
		return
	}
	image, err := s.store.AddProductImage(r.Context(), database.ProductImage{
		ID: imageID, ProductID: productID, StoragePath: relativePath, OriginalName: path.Base(strings.ReplaceAll(header.Filename, "\\", "/")),
		ContentType: contentType, SizeBytes: written,
	}, user.OrganizationID, user.ID)
	if err != nil {
		_ = s.objects.Delete(r.Context(), relativePath)
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	after, _ := json.Marshal(image)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "product_image", TargetID: image.ID,
		Action: "product_image.uploaded", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/products/"+productID+"?notice="+url.QueryEscape("商品画像を登録しました。"), http.StatusSeeOther)
}

func (s *Server) productImage(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	image, err := s.store.ProductImage(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	object, err := s.objects.Get(r.Context(), image.StoragePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer object.Body.Close()
	w.Header().Set("Content-Type", image.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="`+url.PathEscape(image.OriginalName)+`"`)
	_, _ = io.Copy(w, object.Body)
}
