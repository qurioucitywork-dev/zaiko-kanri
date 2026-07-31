package web

import (
	"bytes"
	"encoding/base64"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func testServer(t *testing.T) (*Server, *database.Store) {
	t.Helper()
	store, err := database.Open("file:" + t.TempDir() + "/web.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedPreview(t.Context(), "preview-admin-2026", "preview-worker-2026"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Environment: "test", SessionTTL: time.Hour, OrganizationCode: "PREVIEW", UploadDirectory: t.TempDir()}
	app, err := New(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return app, store
}

func loginAs(t *testing.T, app *Server, username, password string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	get := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(getRecorder, get)
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(getRecorder.Body.String())
	if len(csrfMatch) != 2 {
		t.Fatal("login csrf token missing")
	}
	form := url.Values{"username": {username}, "password": {password}, "csrf_token": {csrfMatch[1]}}
	post := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, post)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("login status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var session, csrf *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		switch cookie.Name {
		case sessionCookie:
			session = cookie
		case csrfCookie:
			csrf = cookie
		}
	}
	if session == nil || csrf == nil {
		t.Fatal("session cookies missing")
	}
	return session, csrf
}

func TestProtectedPageRedirectsToLogin(t *testing.T) {
	app, _ := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/login" {
		t.Fatalf("status=%d location=%q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestSalesTakehomeDisablesRequiredProductSearchInput(t *testing.T) {
	app, _ := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("app.js status=%d body=%s", recorder.Code, body)
	}
	if !strings.Contains(body, `row.querySelectorAll("input[name], [data-sales-product-code]")`) ||
		!strings.Contains(body, `input.disabled = !posted`) {
		t.Fatal("takehome toggle must disable and re-enable the required, nameless product-code input")
	}
}

func TestLoginPageHasRoleTabsWithoutExposingPreviewPasswordsOutsideDevelopment(t *testing.T) {
	app, _ := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `data-login-role="admin"`) ||
		!strings.Contains(body, `data-login-role="worker"`) ||
		!strings.Contains(body, `/guest/login`) {
		t.Fatalf("login page status=%d body=%s", recorder.Code, body)
	}
	if strings.Contains(body, "preview-admin-2026") || strings.Contains(body, "preview-worker-2026") {
		t.Fatal("preview credentials leaked outside development")
	}
}

func TestWorkerCannotOpenAdminUsersPage(t *testing.T) {
	app, _ := testServer(t)
	session, _ := loginAs(t, app, "worker", "preview-worker-2026")
	request := httptest.NewRequest(http.MethodGet, "/users", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("worker status=%d, want 403", recorder.Code)
	}
}

func TestUnsafeRequestRequiresCSRF(t *testing.T) {
	app, _ := testServer(t)
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	form := url.Values{"key": {"reservation.duration_hours"}, "value": {"24"}}
	request := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("csrf status=%d, want 403", recorder.Code)
	}
}

func TestPurchaseRegistrationPageAndSalePrice(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	page := httptest.NewRequest(http.MethodGet, "/purchases", nil)
	page.AddCookie(session)
	pageRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(pageRecorder, page)
	body := pageRecorder.Body.String()
	for _, expected := range []string{
		"仕入登録", "仕入伝票番号", "明細追加",
		"No.", "商品コード", "SKU", "仕入金額（JPY）", "売価（USD）",
		"商品登録", "data-purchase-product-dialog", "BOX番号", "特徴・備考",
		`name="purchase_date" value=""`, `name="buyer_id" required`,
		"base_sale_price",
	} {
		if pageRecorder.Code != http.StatusOK || !strings.Contains(body, expected) {
			t.Fatalf("purchase page missing %q status=%d", expected, pageRecorder.Code)
		}
	}
	if strings.Contains(body, "登録済み仕入伝票一覧") {
		t.Fatal("purchase registration page must not include the registered purchase slip list")
	}
	if strings.Contains(body, `value="usr_admin" selected`) {
		t.Fatal("purchase buyer must start with the placeholder selected")
	}

	form := url.Values{
		"csrf_token":      {csrf.Value},
		"supplier_id":     {"sup_001"},
		"buyer_id":        {"usr_admin"},
		"purchase_date":   {"2026-07-27"},
		"product_code":    {"20260727001"},
		"sku":             {"SKU-WEB-001"},
		"brand":           {"ロレックス"},
		"model_number":    {"126610LN"},
		"serial_number":   {"WEB-SERIAL-001"},
		"product_type":    {"サブマリーナ"},
		"quantity":        {"2"},
		"unit_cost":       {"850000"},
		"base_sale_price": {"1180000"},
		"currency":        {"USD"},
		"sale_currency":   {"JPY"},
		"material":        {"ステンレスSS"},
		"movement":        {"自動巻き"},
		"condition":       {"極美品（S）"},
		"belt_material":   {"ステンレス"},
		"dial":            {"ブラック"},
		"box":             {"BOX1"},
		"accessories":     {"BOX, GUARANTEE"},
		"features":        {"WEB登録テスト"},
	}
	create := httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(form.Encode()))
	create.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	create.AddCookie(session)
	create.AddCookie(csrf)
	createRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(createRecorder, create)
	if createRecorder.Code != http.StatusSeeOther || !strings.HasPrefix(createRecorder.Header().Get("Location"), "/purchases?notice=") {
		t.Fatalf("create status=%d location=%q body=%s", createRecorder.Code, createRecorder.Header().Get("Location"), createRecorder.Body.String())
	}
	slips, err := store.PurchaseSlips(t.Context(), "org_preview")
	if err != nil || len(slips) < 2 || slips[0].TotalMinor != 1700000 || slips[0].SaleTotalMinor != 2360000 {
		t.Fatalf("slips=%+v err=%v", slips, err)
	}
	confirmed, err := store.ConfirmPurchase(t.Context(), "org_preview", slips[0].ID, "usr_admin")
	if err != nil || len(confirmed.Products) != 2 {
		t.Fatalf("confirm=%+v err=%v", confirmed, err)
	}
	product, err := store.Product(t.Context(), "org_preview", confirmed.Products[0].ID)
	if err != nil || product.ProductCode != "20260727001" || product.SKU != "SKU-WEB-001" ||
		product.SerialNumber != "WEB-SERIAL-001" || product.Box != "BOX1" ||
		product.Accessories != "BOX, GUARANTEE" || product.Features != "WEB登録テスト" ||
		product.CostCurrency != "JPY" || product.BaseSaleCurrency != "USD" {
		t.Fatalf("confirmed product=%+v err=%v", product, err)
	}

	nextCodeRequest := httptest.NewRequest(http.MethodGet, "/products/next-code?purchase_date=2026-07-27", nil)
	nextCodeRequest.Header.Set("Accept", "application/json")
	nextCodeRequest.AddCookie(session)
	nextCodeRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(nextCodeRecorder, nextCodeRequest)
	if nextCodeRecorder.Code != http.StatusOK || !strings.Contains(nextCodeRecorder.Body.String(), `"product_code":"20260727003"`) {
		t.Fatalf("next product code status=%d body=%s", nextCodeRecorder.Code, nextCodeRecorder.Body.String())
	}

	exportRequest := httptest.NewRequest(http.MethodGet, "/purchases/export.csv", nil)
	exportRequest.AddCookie(session)
	exportRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(exportRecorder, exportRequest)
	exportBody := exportRecorder.Body.String()
	for _, expected := range []string{
		"商品コード,SKU,ブランド名,モデル名,型番,シリアルNo.,数量,仕入金額,仕入通貨,売価,売価通貨,素材,駆動方式,コンディション,ベルト素材,文字盤,BOX番号,付属品,特徴・備考",
		"SKU-WEB-001", "WEB-SERIAL-001", "ステンレスSS", "BOX1",
		`"BOX, GUARANTEE"`, "WEB登録テスト",
	} {
		if exportRecorder.Code != http.StatusOK || !strings.Contains(exportBody, expected) {
			t.Fatalf("purchase CSV missing %q status=%d body=%s", expected, exportRecorder.Code, exportBody)
		}
	}
	if strings.Contains(exportBody, "20260727001,SKU-WEB-001") || !strings.Contains(exportBody, "\n,SKU-WEB-001,") {
		t.Fatalf("purchase CSV must leave product code blank for safe re-import: %s", exportBody)
	}

	roundTripForm := url.Values{
		"csrf_token":      {csrf.Value},
		"supplier_id":     {"sup_001"},
		"buyer_id":        {"usr_admin"},
		"purchase_date":   {"2026-07-27"},
		"product_code":    {""},
		"sku":             {"SKU-WEB-001"},
		"brand":           {"ロレックス"},
		"model_number":    {"126610LN"},
		"serial_number":   {"WEB-SERIAL-001"},
		"product_type":    {"サブマリーナ"},
		"quantity":        {"1"},
		"unit_cost":       {"850000"},
		"base_sale_price": {"1180000"},
		"currency":        {"JPY"},
		"sale_currency":   {"USD"},
		"material":        {"ステンレスSS"},
		"movement":        {"自動巻き"},
		"condition":       {"極美品（S）"},
		"belt_material":   {"ステンレス"},
		"dial":            {"ブラック"},
		"box":             {"BOX1"},
		"accessories":     {"BOX, GUARANTEE"},
		"features":        {"WEB登録テスト"},
	}
	roundTripCreate := httptest.NewRequest(http.MethodPost, "/purchases", strings.NewReader(roundTripForm.Encode()))
	roundTripCreate.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	roundTripCreate.AddCookie(session)
	roundTripCreate.AddCookie(csrf)
	roundTripRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(roundTripRecorder, roundTripCreate)
	if roundTripRecorder.Code != http.StatusSeeOther {
		t.Fatalf("round-trip create status=%d body=%s", roundTripRecorder.Code, roundTripRecorder.Body.String())
	}
	roundTripSlips, err := store.PurchaseSlips(t.Context(), "org_preview")
	if err != nil || len(roundTripSlips) == 0 {
		t.Fatalf("round-trip slips=%+v err=%v", roundTripSlips, err)
	}
	roundTripConfirmed, err := store.ConfirmPurchase(t.Context(), "org_preview", roundTripSlips[0].ID, "usr_admin")
	if err != nil || len(roundTripConfirmed.Products) != 1 {
		t.Fatalf("round-trip confirm=%+v err=%v", roundTripConfirmed, err)
	}
	roundTripProduct := roundTripConfirmed.Products[0]
	if roundTripProduct.ProductCode != "20260727003" || roundTripProduct.ProductCode == product.ProductCode ||
		roundTripProduct.SKU != product.SKU || roundTripProduct.Brand != product.Brand ||
		roundTripProduct.ModelNumber != product.ModelNumber || roundTripProduct.Material != product.Material ||
		roundTripProduct.Box != product.Box || roundTripProduct.Accessories != product.Accessories ||
		roundTripProduct.Features != product.Features || roundTripProduct.CostAmountMinor != product.CostAmountMinor ||
		roundTripProduct.BaseSalePriceMinor != product.BaseSalePriceMinor {
		t.Fatalf("round-trip product did not preserve details or generated a duplicate: original=%+v roundTrip=%+v", product, roundTripProduct)
	}
}

func TestProductRegistrationPageMatchesOperationalForm(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	request := httptest.NewRequest(http.MethodGet, "/products/new", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{
		"商品登録", "基本情報", "商品詳細", "仕入担当者", `name="material"`,
		`name="features"`, `name="internal_comment"`, "商品画像（最大10枚）",
		"手入力またはバーコードスキャン", "S001 — 田中商事",
		"BOX1 — ロレックス特集", "BOX10", `name="product_type" value=""`,
	} {
		if recorder.Code != http.StatusOK || !strings.Contains(body, expected) {
			t.Fatalf("product registration missing %q status=%d", expected, recorder.Code)
		}
	}
	if count := strings.Count(body, "data-product-image-slot"); count != 10 {
		t.Fatalf("image slots=%d want=10", count)
	}
	if strings.Contains(body, "商品単品登録") {
		t.Fatal("old product registration name remains")
	}
	for _, unexpected := range []string{`name="buyer_id" required`, `name="brand" required`, "JPEG・PNG・WebP／1枚8MBまで"} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("product registration contains mock mismatch %q", unexpected)
		}
	}
}

func TestProductCodeNumberingUsesRequestedPurchaseDate(t *testing.T) {
	app, _ := testServer(t)
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	request := httptest.NewRequest(http.MethodGet, "/products/next-code?purchase_date=2026-12-31", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"product_code":"20261231`) {
		t.Fatalf("numbering status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestProductRegistrationStoresDetailsAndImage(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string][]string{
		"csrf_token": {csrf.Value}, "buyer_id": {"usr_admin"}, "supplier_id": {"sup_001"},
		"purchase_date": {"2026-07-27"}, "cost_amount": {"850000"}, "cost_currency": {"JPY"},
		"product_code": {"20260727099"}, "sku": {"FORM-SKU"}, "brand": {"ロレックス"}, "product_type": {"サブマリーナ"},
		"model_number": {"126610LN"}, "serial_number": {"FORM-SERIAL"},
		"material": {"ステンレス"}, "box": {"BOX1"}, "movement": {"自動巻き"},
		"condition": {"極美品"}, "belt_material": {"ステンレス"}, "dial": {"ブラック"},
		"base_sale_price": {"1180000"}, "base_sale_currency": {"JPY"},
		"accessories": {"BOX", "GUARANTEE"}, "features": {"コマ数：8"}, "internal_comment": {"社内確認済み"},
	}
	for name, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(name, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	imagePart, err := writer.CreateFormFile("images", "watch.png")
	if err != nil {
		t.Fatal(err)
	}
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := imagePart.Write(imageBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/products", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(session)
	request.AddCookie(csrf)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{SerialNumber: "FORM-SERIAL"})
	if err != nil || len(products) != 1 {
		t.Fatalf("products=%+v err=%v", products, err)
	}
	product, err := store.Product(t.Context(), "org_preview", products[0].ID)
	if err != nil || product.Material != "ステンレス" || product.Movement != "自動巻き" ||
		product.Features != "コマ数：8" || product.InternalComment != "社内確認済み" ||
		product.ProductCode != "20260727099" || product.SKU != "FORM-SKU" ||
		product.Box != "BOX1" || product.Accessories != "BOX, GUARANTEE" ||
		product.BuyerID != "usr_admin" || product.BeltMaterial != "ステンレス" ||
		product.Dial != "ブラック" || product.BaseSalePriceMinor != 1180000 ||
		product.CostCurrency != "JPY" || product.BaseSaleCurrency != "USD" ||
		product.ImageCount != 1 || len(product.Images) != 1 {
		t.Fatalf("product=%+v images=%+v err=%v", product, product.Images, err)
	}
}

func TestProductRegistrationAllowsOptionalBrandAndModel(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")
	form := url.Values{
		"csrf_token": {csrf.Value}, "supplier_id": {"sup_001"}, "purchase_date": {"2026-07-28"},
		"product_code": {"20260728001"},
		"cost_amount":  {"100000"}, "cost_currency": {"JPY"}, "base_sale_currency": {"JPY"},
	}
	request := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(session)
	request.AddCookie(csrf)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("create optional fields status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{PurchaseDateFrom: "2026-07-28", PurchaseDateTo: "2026-07-28"})
	if err != nil || len(products) != 1 || products[0].Brand != "" || products[0].ProductType != "" ||
		products[0].CostCurrency != "JPY" || products[0].BaseSaleCurrency != "USD" {
		t.Fatalf("products=%+v err=%v", products, err)
	}
	editRequest := httptest.NewRequest(http.MethodGet, "/products/"+products[0].ID+"/edit", nil)
	editRequest.AddCookie(session)
	editRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(editRecorder, editRequest)
	editBody := editRecorder.Body.String()
	if editRecorder.Code != http.StatusOK ||
		!strings.Contains(editBody, `<select name="brand"><option value="" selected>-- 選択 --</option>`) ||
		strings.Contains(editBody, `name="brand" required`) {
		t.Fatalf("optional brand edit status=%d body=%s", editRecorder.Code, editBody)
	}
}

func TestReturnTakehomeListDetailAndCompletion(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedReturnTakehomePreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	list := httptest.NewRequest(http.MethodGet, "/returns?status=pending&q=SL-", nil)
	list.AddCookie(session)
	listRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(listRecorder, list)
	listBody := listRecorder.Body.String()
	if listRecorder.Code != http.StatusOK || !strings.Contains(listBody, "返品/持ち帰り") ||
		!strings.Contains(listBody, "処理待ちあり") || !strings.Contains(listBody, "完了済み") ||
		strings.Contains(listBody, `class="button secondary returns-search"`) ||
		!strings.Contains(listBody, "伝票番号・販売先・ブランド・モデル・商品コード") {
		t.Fatalf("list status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}

	summaries, err := store.ReturnTakehomeSummaries(t.Context(), "org_preview", "pending", "")
	if err != nil || len(summaries) == 0 {
		t.Fatalf("summaries=%+v err=%v", summaries, err)
	}
	modal := httptest.NewRequest(http.MethodGet, "/returns/"+summaries[0].SaleID+"/modal", nil)
	modal.Header.Set("HX-Request", "true")
	modal.AddCookie(session)
	modalRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(modalRecorder, modal)
	modalBody := modalRecorder.Body.String()
	if modalRecorder.Code != http.StatusOK || !strings.Contains(modalBody, "BOX確認") ||
		!strings.Contains(modalBody, "変更なし") || !strings.Contains(modalBody, "備考・管理者コメント（任意）") {
		t.Fatalf("modal status=%d body=%s", modalRecorder.Code, modalBody)
	}
	detail := httptest.NewRequest(http.MethodGet, "/returns/"+summaries[0].SaleID, nil)
	detail.AddCookie(session)
	detailRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(detailRecorder, detail)
	if detailRecorder.Code != http.StatusOK || !strings.Contains(detailRecorder.Body.String(), "処理履歴") {
		t.Fatalf("detail status=%d body=%s", detailRecorder.Code, detailRecorder.Body.String())
	}

	items, err := store.ReturnTakehomeItems(t.Context(), "org_preview", summaries[0].SaleID)
	if err != nil || len(items) == 0 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	form := url.Values{"csrf_token": {csrf.Value}, "notes": {"検品済み"}}
	complete := httptest.NewRequest(http.MethodPost,
		"/returns/"+summaries[0].SaleID+"/items/"+items[0].ID+"/complete",
		strings.NewReader(form.Encode()))
	complete.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	complete.AddCookie(session)
	complete.AddCookie(csrf)
	completeRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(completeRecorder, complete)
	if completeRecorder.Code != http.StatusSeeOther {
		t.Fatalf("complete status=%d body=%s", completeRecorder.Code, completeRecorder.Body.String())
	}
}

func TestPhase7ProductListSupportsFullPageAndPartialRequests(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	full := httptest.NewRequest(http.MethodGet, "/products?q=Rolex&sort=brand_asc", nil)
	full.AddCookie(session)
	fullRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(fullRecorder, full)
	fullBody := fullRecorder.Body.String()
	if fullRecorder.Code != http.StatusOK || !strings.Contains(fullBody, "<!doctype html>") ||
		!strings.Contains(fullBody, `method="get" action="/products"`) {
		t.Fatalf("normal GET must work without JavaScript: status=%d body=%s", fullRecorder.Code, fullBody)
	}

	partial := httptest.NewRequest(http.MethodGet, "/products?q=Rolex&sort=brand_asc", nil)
	partial.Header.Set("HX-Request", "true")
	partial.AddCookie(session)
	partialRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(partialRecorder, partial)
	partialBody := partialRecorder.Body.String()
	if partialRecorder.Code != http.StatusOK || !strings.Contains(partialBody, `id="product-results"`) ||
		strings.Contains(partialBody, "<!doctype html>") {
		t.Fatalf("partial response invalid: status=%d body=%s", partialRecorder.Code, partialBody)
	}
	if partialRecorder.Header().Get("Vary") != "HX-Request" {
		t.Fatalf("partial Vary=%q", partialRecorder.Header().Get("Vary"))
	}
}

func TestProductListStartsWithSearchPromptAndCanShowAll(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	initial := httptest.NewRequest(http.MethodGet, "/products", nil)
	initial.AddCookie(session)
	initialRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(initialRecorder, initial)
	initialBody := initialRecorder.Body.String()
	if initialRecorder.Code != http.StatusOK ||
		!strings.Contains(initialBody, "検索条件を入力して「検索」を押してください") ||
		!strings.Contains(initialBody, `href="/products?show_all=1"`) ||
		strings.Contains(initialBody, `class="inventory-table"`) {
		t.Fatalf("initial inventory state invalid: status=%d body=%s", initialRecorder.Code, initialBody)
	}

	initialPartial := httptest.NewRequest(http.MethodGet, "/products", nil)
	initialPartial.Header.Set("HX-Request", "true")
	initialPartial.AddCookie(session)
	initialPartialRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(initialPartialRecorder, initialPartial)
	initialPartialBody := initialPartialRecorder.Body.String()
	if initialPartialRecorder.Code != http.StatusOK ||
		!strings.Contains(initialPartialBody, "検索条件を入力して「検索」を押してください") ||
		strings.Contains(initialPartialBody, `class="inventory-table"`) ||
		strings.Contains(initialPartialBody, "<!doctype html>") {
		t.Fatalf("initial partial inventory state invalid: status=%d body=%s", initialPartialRecorder.Code, initialPartialBody)
	}

	disabledAll := httptest.NewRequest(http.MethodGet, "/products?show_all=0", nil)
	disabledAll.AddCookie(session)
	disabledAllRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(disabledAllRecorder, disabledAll)
	disabledAllBody := disabledAllRecorder.Body.String()
	if disabledAllRecorder.Code != http.StatusOK ||
		!strings.Contains(disabledAllBody, "検索条件を入力して「検索」を押してください") ||
		strings.Contains(disabledAllBody, `class="inventory-table"`) {
		t.Fatalf("show_all=0 must keep prompt: status=%d body=%s", disabledAllRecorder.Code, disabledAllBody)
	}

	all := httptest.NewRequest(http.MethodGet, "/products?show_all=1", nil)
	all.AddCookie(session)
	allRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(allRecorder, all)
	allBody := allRecorder.Body.String()
	if allRecorder.Code != http.StatusOK || !strings.Contains(allBody, `class="inventory-table"`) {
		t.Fatalf("show all inventory state invalid: status=%d body=%s", allRecorder.Code, allBody)
	}
}

func TestProductListUsesNamedBoxFilter(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	page := httptest.NewRequest(http.MethodGet, "/products", nil)
	page.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, page)
	body := recorder.Body.String()
	for _, expected := range []string{
		`name="box"`, `value="BOX1"`, "BOX1 — ロレックス特集",
		`value="BOX2"`, "BOX2 — 高額品セレクト",
		`value="BOX3"`, "BOX3 — 春の新入荷", `value="BOX10"`,
		`value="purchase_return"`, ">仕入返品</option>",
		`value="hold"`, ">保留</option>",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("named BOX option %q missing: %s", expected, body)
		}
	}
	previous := -1
	for _, brand := range []string{
		`value="IWC"`, `value="オメガ"`, `value="カルティエ"`,
		`value="グランドセイコー"`, `value="パテック・フィリップ"`,
		`value="ブライトリング"`, `value="ロレックス"`,
	} {
		index := strings.Index(body, brand)
		if index <= previous {
			t.Fatalf("inventory brand option %q missing or out of mock order: %s", brand, body)
		}
		previous = index
	}
	if strings.Contains(body, `value="yes"`) || strings.Contains(body, `value="no"`) {
		t.Fatalf("legacy yes/no BOX options must not be rendered: %s", body)
	}
}

func TestPhase7PartialRequestsKeepAuthenticationAndCSRF(t *testing.T) {
	app, _ := testServer(t)
	anonymous := httptest.NewRequest(http.MethodGet, "/products", nil)
	anonymous.Header.Set("HX-Request", "true")
	anonymousRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(anonymousRecorder, anonymous)
	if anonymousRecorder.Code != http.StatusUnauthorized ||
		anonymousRecorder.Header().Get("HX-Redirect") != "/login" {
		t.Fatalf("anonymous partial status=%d redirect=%q", anonymousRecorder.Code, anonymousRecorder.Header().Get("HX-Redirect"))
	}

	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	form := url.Values{"key": {"reservation.duration_hours"}, "value": {"24"}}
	unsafe := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	unsafe.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	unsafe.Header.Set("HX-Request", "true")
	unsafe.AddCookie(session)
	unsafeRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(unsafeRecorder, unsafe)
	if unsafeRecorder.Code != http.StatusForbidden ||
		!strings.Contains(unsafeRecorder.Body.String(), `role="alert"`) {
		t.Fatalf("partial CSRF status=%d body=%s", unsafeRecorder.Code, unsafeRecorder.Body.String())
	}
}

func TestPhase7AdminCanFindCancelledProductsAndCSVExportIsAudited(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	cancelled := products[0]
	if err := store.CancelProduct(t.Context(), "org_preview", cancelled.ID, "usr_admin", "検索テスト"); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	normal := httptest.NewRequest(http.MethodGet, "/products", nil)
	normal.AddCookie(session)
	normalRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(normalRecorder, normal)
	if strings.Contains(normalRecorder.Body.String(), cancelled.ProductCode) {
		t.Fatal("cancelled product appeared in normal inventory list")
	}

	included := httptest.NewRequest(http.MethodGet, "/products?include_cancelled=1&q="+url.QueryEscape(cancelled.ProductCode), nil)
	included.AddCookie(session)
	includedRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(includedRecorder, included)
	if includedRecorder.Code != http.StatusOK || !strings.Contains(includedRecorder.Body.String(), cancelled.ProductCode) {
		t.Fatalf("admin cancelled search status=%d body=%s", includedRecorder.Code, includedRecorder.Body.String())
	}

	export := httptest.NewRequest(http.MethodGet, "/products/export.csv?include_cancelled=1", nil)
	export.AddCookie(session)
	exportRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(exportRecorder, export)
	if exportRecorder.Code != http.StatusOK || !strings.Contains(exportRecorder.Body.String(), "原価") {
		t.Fatalf("csv status=%d body=%s", exportRecorder.Code, exportRecorder.Body.String())
	}
	entries, err := store.AuditLogs(t.Context(), "org_preview", 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.Action == "inventory.cost_csv.exported" && entry.ActorUserID == "usr_admin" {
			found = true
		}
	}
	if !found {
		t.Fatal("cost CSV export audit log missing")
	}
}

func TestWorkerCancellationRoutesEnterApprovalHandler(t *testing.T) {
	app, _ := testServer(t)
	session, csrf := loginAs(t, app, "worker", "preview-worker-2026")
	for _, path := range []string{"/sales/example/cancel", "/shipments/example/cancel"} {
		form := url.Values{"csrf_token": {csrf.Value}, "reason": {"テスト"}}
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.AddCookie(session)
		request.AddCookie(csrf)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code == http.StatusForbidden {
			t.Fatalf("%s worker cancellation should be accepted for approval processing", path)
		}
	}
}

func TestPhase4SalesAndShipmentPagesRender(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMarketPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	for _, path := range []string{"/sales", "/sales/new", "/shipments", "/shipments/new"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(session)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `class="button primary topbar-create"`) {
			t.Fatalf("%s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), `<span class="badge phase">`) || strings.Contains(recorder.Body.String(), "preview-banner") {
			t.Fatalf("%s rendered legacy phase/preview UI: body=%s", path, recorder.Body.String())
		}
	}
}

func TestSlipListTabsFilterAndCSV(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMarketPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedPurchaseReturnPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	for _, path := range []string{"/slips", "/slips?kind=shipments", "/slips?kind=sales", "/slips?kind=sales-returns", "/slips?kind=purchase-returns"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(session)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		body := recorder.Body.String()
		for _, expected := range []string{"伝票一覧", "仕入伝票", "出荷伝票", "売上伝票", "CSV出力"} {
			if recorder.Code != http.StatusOK || !strings.Contains(body, expected) {
				t.Fatalf("%s status=%d expected=%s body=%s", path, recorder.Code, expected, body)
			}
		}
	}
	returnsRequest := httptest.NewRequest(http.MethodGet, "/slips?kind=purchase-returns", nil)
	returnsRequest.AddCookie(session)
	returnsRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(returnsRecorder, returnsRequest)
	if returnsRecorder.Code != http.StatusOK || !strings.Contains(returnsRecorder.Body.String(), "配送番号を入力") ||
		!strings.Contains(returnsRecorder.Body.String(), "PR-RET-") {
		t.Fatalf("purchase returns status=%d body=%s", returnsRecorder.Code, returnsRecorder.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/slips/export.csv?kind=purchases", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(recorder.Body.String(), "伝票番号") {
		t.Fatalf("csv status=%d content-type=%s body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
}

func TestSlipTabCountsAndBadgeFilterOnlyActionRequiredRows(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMarketPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedPurchaseReturnPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	request := httptest.NewRequest(http.MethodGet, "/slips?kind=purchases", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("slips status=%d body=%s", recorder.Code, body)
	}
	for key, label := range map[string]string{
		"purchases":        "仕入伝票",
		"shipments":        "出荷伝票",
		"sales":            "売上伝票",
		"sales-returns":    "売上返品",
		"purchase-returns": "仕入返品",
	} {
		if !strings.Contains(body, `href="/slips?kind=`+key+`&approval=1"`) ||
			!strings.Contains(body, `aria-label="`+label+`の対応が必要な伝票を表示"`) {
			t.Fatalf("%s action-required badge link is missing: body=%s", key, body)
		}
	}
	if count := slipActionRequiredCount([]slipListRow{
		{Status: "pending"}, {Status: "returned"}, {Status: "completed"},
	}); count != 2 {
		t.Fatalf("action-required count=%d want=2", count)
	}

	request = httptest.NewRequest(http.MethodGet, "/slips?kind=purchases&approval=1", nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body = recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "承認タスク表示中") ||
		strings.Contains(body, "検索条件を入力して") {
		t.Fatalf("badge filter did not immediately display action-required rows: status=%d body=%s", recorder.Code, body)
	}
	if strings.Contains(body, `<span class="slips-status completed">`) {
		t.Fatalf("completed rows must not appear in the badge filter: body=%s", body)
	}
}

func TestSalesRegistrationAndSlipListMatchMockWorkflow(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMarketPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	revisedSales, err := store.Sales(t.Context(), "org_preview")
	if err != nil || len(revisedSales) == 0 {
		t.Fatalf("sales=%+v err=%v", revisedSales, err)
	}
	revisedSale, err := store.Sale(t.Context(), "org_preview", revisedSales[0].ID)
	if err != nil || len(revisedSale.Lines) == 0 {
		t.Fatalf("sale=%+v err=%v", revisedSale, err)
	}
	if err := store.UpdateSalesSlip(t.Context(), database.UpdateSalesSlipInput{
		OrganizationID: "org_preview",
		SalesSlipID:    revisedSale.ID,
		SalesDate:      revisedSale.SalesDate,
		Notes:          revisedSale.Notes,
		Memo:           "修正表示の確認",
		ActorUserID:    "usr_admin",
		Lines: []database.SalesEditLine{{
			LineID:         revisedSale.Lines[0].ID,
			SalePriceMinor: revisedSale.Lines[0].UnitPriceMinor,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	registration := httptest.NewRequest(http.MethodGet, "/sales", nil)
	registration.AddCookie(session)
	registrationRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(registrationRecorder, registration)
	registrationBody := registrationRecorder.Body.String()
	for _, expected := range []string{
		"売上伝票", `type="button" data-sales-auto-number>＋ 出荷なし（新規発番）`,
		`name="auto_number" value="0" data-sales-auto-number-state`,
		`name="tax_mode" value="normal"`,
		"data-sales-tax-switch", "通常", "免税を切り替え", "管理番号",
		`list="sales-product-options"`, "明細行を追加", "合計金額（税抜）",
		"消費税（10%）", "税込合計", "売上を確定する",
	} {
		if registrationRecorder.Code != http.StatusOK || !strings.Contains(registrationBody, expected) {
			t.Fatalf("sales registration missing %q status=%d body=%s", expected, registrationRecorder.Code, registrationBody)
		}
	}
	if strings.Contains(registrationBody, "登録済み売上伝票一覧") ||
		strings.Contains(registrationBody, `<select name="product_id"`) {
		t.Fatalf("sales registration still contains the old list or select control: %s", registrationBody)
	}

	initial := httptest.NewRequest(http.MethodGet, "/slips?kind=sales", nil)
	initial.AddCookie(session)
	initialRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(initialRecorder, initial)
	initialBody := initialRecorder.Body.String()
	if initialRecorder.Code != http.StatusOK ||
		!strings.Contains(initialBody, "検索条件を入力して") ||
		!strings.Contains(initialBody, "/slips?kind=sales&show_all=1") ||
		strings.Contains(initialBody, `class="slips-table sales"`) {
		t.Fatalf("initial sales slips must show search prompt: status=%d body=%s", initialRecorder.Code, initialBody)
	}

	all := httptest.NewRequest(http.MethodGet, "/slips?kind=sales&show_all=1", nil)
	all.AddCookie(session)
	allRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(allRecorder, all)
	allBody := allRecorder.Body.String()
	for _, expected := range []string{
		`class="slips-table sales"`, "すべて選択", "伝票番号", "売上日",
		"販売先", "点数", "合計金額", "備考", "ステータス", "修正",
		"請求書発行", "data-sales-slip-open",
	} {
		if allRecorder.Code != http.StatusOK || !strings.Contains(allBody, expected) {
			t.Fatalf("sales slips missing %q status=%d body=%s", expected, allRecorder.Code, allBody)
		}
	}
	if strings.Contains(allBody, `href="/sales/`) || strings.Contains(allBody, ">⌕ 詳細<") {
		t.Fatalf("sales slip rows must open the modal instead of using an action link: %s", allBody)
	}
	if !strings.Contains(allBody, `</strong> <span class="slips-corrected">`) ||
		!strings.Contains(allBody, `class="slips-correction-check" aria-label="修正済">✓</span>`) ||
		strings.Count(allBody, `✎ 修正済</span>`) != 1 {
		t.Fatalf("sales revision must show one number-cell badge and a check icon in the final 修正 column: %s", allBody)
	}

	sales, err := store.Sales(t.Context(), "org_preview")
	if err != nil || len(sales) == 0 {
		t.Fatalf("sales=%+v err=%v", sales, err)
	}
	sale, err := store.Sale(t.Context(), "org_preview", sales[0].ID)
	if err != nil || len(sale.Lines) == 0 {
		t.Fatalf("sale=%+v err=%v", sale, err)
	}
	search := httptest.NewRequest(http.MethodGet,
		"/slips?kind=sales&search=1&q="+url.QueryEscape(sale.Lines[0].ProductCode), nil)
	search.AddCookie(session)
	searchRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(searchRecorder, search)
	if searchRecorder.Code != http.StatusOK || !strings.Contains(searchRecorder.Body.String(), sales[0].SlipNumber) {
		t.Fatalf("sales product-code search status=%d body=%s", searchRecorder.Code, searchRecorder.Body.String())
	}
}

func TestShipmentRegistrationAndSlipListMatchMockWorkflow(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMarketPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedShipmentWorkflowPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	registration := httptest.NewRequest(http.MethodGet, "/shipments", nil)
	registration.AddCookie(session)
	registrationRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(registrationRecorder, registration)
	registrationBody := registrationRecorder.Body.String()
	for _, expected := range []string{
		"出荷伝票", "通関書類", "印刷", "CSV", "伝票番号", "自動",
		"出荷日", "出荷先", "備考", "商品コード", "ブランド", "モデル名",
		"卸値（税抜）", `list="shipment-product-options"`, "明細行を追加",
		"合計卸値（税抜）", "リセット", "出荷を確定",
	} {
		if registrationRecorder.Code != http.StatusOK || !strings.Contains(registrationBody, expected) {
			t.Fatalf("shipment registration missing %q status=%d body=%s", expected, registrationRecorder.Code, registrationBody)
		}
	}
	if strings.Contains(registrationBody, "出荷伝票一覧") ||
		strings.Contains(registrationBody, `<select name="product_id"`) {
		t.Fatalf("shipment registration still contains the old list or product select: %s", registrationBody)
	}

	initial := httptest.NewRequest(http.MethodGet, "/slips?kind=shipments", nil)
	initial.AddCookie(session)
	initialRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(initialRecorder, initial)
	initialBody := initialRecorder.Body.String()
	if initialRecorder.Code != http.StatusOK ||
		!strings.Contains(initialBody, "検索条件を入力して") ||
		!strings.Contains(initialBody, "/slips?kind=shipments&show_all=1") ||
		strings.Contains(initialBody, `class="slips-table shipments"`) {
		t.Fatalf("initial shipment slips must show search prompt: status=%d body=%s", initialRecorder.Code, initialBody)
	}

	all := httptest.NewRequest(http.MethodGet, "/slips?kind=shipments&show_all=1", nil)
	all.AddCookie(session)
	allRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(allRecorder, all)
	allBody := allRecorder.Body.String()
	for _, expected := range []string{
		`class="slips-table shipments"`, "すべて選択", "伝票番号", "出荷日",
		"出荷先", "点数", "合計金額", "備考", "ステータス", "修正",
		"明細書発行", "請求書発行", "data-shipment-slip-open",
	} {
		if allRecorder.Code != http.StatusOK || !strings.Contains(allBody, expected) {
			t.Fatalf("shipment slips missing %q status=%d body=%s", expected, allRecorder.Code, allBody)
		}
	}
	if strings.Contains(allBody, `href="/shipments/`) || strings.Contains(allBody, ">⌕ 詳細<") {
		t.Fatalf("shipment rows must open the modal instead of using an action link: %s", allBody)
	}

	shipments, err := store.Shipments(t.Context(), "org_preview")
	if err != nil || len(shipments) == 0 {
		t.Fatalf("shipments=%+v err=%v", shipments, err)
	}
	modal := httptest.NewRequest(http.MethodGet, "/slips/shipments/"+shipments[0].ID+"/modal", nil)
	modal.SetPathValue("id", shipments[0].ID)
	modal.AddCookie(session)
	modalRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(modalRecorder, modal)
	modalBody := modalRecorder.Body.String()
	for _, expected := range []string{
		"出荷伝票", "出荷日", "出荷先", "住所", "連絡先", "適格番号",
		"合計金額", "商品明細", "商品コード", "ブランド", "モデル", "金額",
		"修正履歴", "閉じる", "伝票修正",
	} {
		if modalRecorder.Code != http.StatusOK || !strings.Contains(modalBody, expected) {
			t.Fatalf("shipment modal missing %q status=%d body=%s", expected, modalRecorder.Code, modalBody)
		}
	}
}

func TestSalesAutoNumberButtonAllocatesUniqueNumberAtSave(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	next := httptest.NewRequest(http.MethodGet, "/sales/next-number?date=2026-07-31", nil)
	next.AddCookie(session)
	nextRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(nextRecorder, next)
	if nextRecorder.Code != http.StatusOK ||
		!strings.Contains(nextRecorder.Body.String(), `"sales_slip_number":"SL-2026-0001"`) {
		t.Fatalf("next number status=%d body=%s", nextRecorder.Code, nextRecorder.Body.String())
	}

	for index := 0; index < 2; index++ {
		form := url.Values{
			"csrf_token":    {csrf.Value},
			"auto_number":   {"1"},
			"slip_number":   {"SL-2026-0001"},
			"sales_date":    {"2026-07-31"},
			"customer_name": {"クロノス東京"},
			"product_id":    {products[0].ID},
			"quantity":      {"1"},
			"unit_price":    {"720000"},
		}
		request := httptest.NewRequest(http.MethodPost, "/sales", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.AddCookie(session)
		request.AddCookie(csrf)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusSeeOther {
			t.Fatalf("sale %d status=%d body=%s", index+1, recorder.Code, recorder.Body.String())
		}
	}

	sales, err := store.Sales(t.Context(), "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	numbers := make(map[string]bool)
	for _, sale := range sales {
		if sale.SalesDate == "2026-07-31" {
			numbers[sale.SlipNumber] = true
		}
	}
	if !numbers["SL-2026-0001"] || !numbers["SL-2026-0002"] || len(numbers) != 2 {
		t.Fatalf("auto-numbered sales=%v, want unique sequential numbers", numbers)
	}
}

func TestShipmentCreateRedirectsToPrefilledMultiLineSale(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	first := products[0]
	second, err := store.CreateSingleProduct(t.Context(), database.SingleProductInput{
		OrganizationID: "org_preview", SupplierID: "sup_001", PurchaseDate: "2026-07-30",
		SKU: "PHASE5-WEB-002", Brand: "オメガ", ModelNumber: "PHASE5-WEB-002",
		SerialNumber: "PHASE5-WEB-SERIAL-002", ProductType: "シーマスター",
		CostAmountMinor: 500000, CostCurrency: "JPY", BaseSalePriceMinor: 720000,
		BaseSaleCurrency: "JPY", Condition: "美品 (A)", CreatedBy: "usr_admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")
	shipmentForm := url.Values{
		"csrf_token": {csrf.Value}, "shipment_date": {"2026-07-30"},
		"recipient_name": {"クロノス東京"}, "notes": {"複数明細"},
		"product_id": {first.ID, second.ID}, "quantity": {"1", "1"},
		"wholesale_price": {"610000", "720000"},
	}
	createShipment := httptest.NewRequest(http.MethodPost, "/shipments", strings.NewReader(shipmentForm.Encode()))
	createShipment.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createShipment.AddCookie(session)
	createShipment.AddCookie(csrf)
	shipmentRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(shipmentRecorder, createShipment)
	if shipmentRecorder.Code != http.StatusSeeOther {
		t.Fatalf("shipment status=%d body=%s", shipmentRecorder.Code, shipmentRecorder.Body.String())
	}
	location := shipmentRecorder.Header().Get("Location")
	locationURL, err := url.Parse(location)
	if err != nil || locationURL.Path != "/sales" || locationURL.Query().Get("shipment_id") == "" {
		t.Fatalf("shipment redirect lost source identifier: %q err=%v", location, err)
	}
	shipmentID := locationURL.Query().Get("shipment_id")
	sourceShipment, err := store.Shipment(t.Context(), "org_preview", shipmentID)
	if err != nil {
		t.Fatal(err)
	}

	lookupRequest := httptest.NewRequest(http.MethodGet,
		"/sales/shipment-prefill?number="+url.QueryEscape(sourceShipment.ShipmentNumber), nil)
	lookupRequest.AddCookie(session)
	lookupRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(lookupRecorder, lookupRequest)
	lookupBody := lookupRecorder.Body.String()
	for _, expected := range []string{
		`"shipment_id":"` + shipmentID + `"`,
		`"shipment_number":"` + sourceShipment.ShipmentNumber + `"`,
		`"sales_slip_number":"SL-2026-`,
		`"recipient_name":"クロノス東京"`,
		`"model":"` + first.ProductType + `"`,
		`"reference":"` + first.ModelNumber + `"`,
		`"serial":"` + first.SerialNumber + `"`,
		`"quantity":1`, `"price":610000`, `"price":720000`,
	} {
		if lookupRecorder.Code != http.StatusOK || !strings.Contains(lookupBody, expected) {
			t.Fatalf("shipment number lookup missing %q status=%d body=%s", expected, lookupRecorder.Code, lookupBody)
		}
	}

	prefillRequest := httptest.NewRequest(http.MethodGet, location, nil)
	prefillRequest.AddCookie(session)
	prefillRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(prefillRecorder, prefillRequest)
	prefillBody := prefillRecorder.Body.String()
	for _, expected := range []string{
		`name="shipment_id" value="` + shipmentID + `"`,
		`data-recipient="クロノス東京"`,
		`<option value="クロノス東京">クロノス東京</option>`,
		`data-product-id="` + first.ID + `"`,
		`data-product-id="` + second.ID + `"`,
		`data-price="610000"`, `data-price="720000"`,
	} {
		if prefillRecorder.Code != http.StatusOK || !strings.Contains(prefillBody, expected) {
			t.Fatalf("sales prefill missing %q status=%d body=%s", expected, prefillRecorder.Code, prefillBody)
		}
	}

	saleForm := url.Values{
		"csrf_token": {csrf.Value}, "slip_number": {sourceShipment.ShipmentNumber},
		"sales_date": {"2026-07-30"}, "customer_name": {"クロノス東京"},
		"product_id": {first.ID, second.ID}, "quantity": {"1", "1"},
		"unit_price": {"1", "1"}, "tax_mode": {"normal"},
	}
	createSale := httptest.NewRequest(http.MethodPost, "/sales", strings.NewReader(saleForm.Encode()))
	createSale.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createSale.AddCookie(session)
	createSale.AddCookie(csrf)
	saleRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(saleRecorder, createSale)
	if saleRecorder.Code != http.StatusSeeOther {
		t.Fatalf("sale status=%d body=%s", saleRecorder.Code, saleRecorder.Body.String())
	}
	saleLocation, err := url.Parse(saleRecorder.Header().Get("Location"))
	if err != nil || !strings.HasPrefix(saleLocation.Path, "/sales/") {
		t.Fatalf("sale redirect=%q err=%v", saleRecorder.Header().Get("Location"), err)
	}
	saleID := strings.TrimPrefix(saleLocation.Path, "/sales/")
	confirmed, err := store.ConfirmSale(t.Context(), "org_preview", saleID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.ShipmentStatus != "complete" || len(confirmed.Lines) != 2 {
		t.Fatalf("confirmed sale did not consume shipment: %+v", confirmed)
	}
	if confirmed.SlipNumber == sourceShipment.ShipmentNumber ||
		confirmed.Lines[0].UnitPriceMinor+confirmed.Lines[1].UnitPriceMinor != 1330000 {
		t.Fatalf("shipment number or tampered prices leaked into sale: %+v", confirmed)
	}
	linked, err := store.Shipment(t.Context(), "org_preview", shipmentID)
	if err != nil || len(linked.Lines) != 2 {
		t.Fatalf("shipment=%+v err=%v", linked, err)
	}
	for _, line := range linked.Lines {
		if line.SalesSlipNumber != confirmed.SlipNumber {
			t.Fatalf("shipment line not linked to sale: %+v", line)
		}
	}
}

func TestPurchaseSlipListSearchPromptShowAllAndProductSearch(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	initial := httptest.NewRequest(http.MethodGet, "/slips?kind=purchases", nil)
	initial.AddCookie(session)
	initialRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(initialRecorder, initial)
	initialBody := initialRecorder.Body.String()
	if initialRecorder.Code != http.StatusOK ||
		!strings.Contains(initialBody, "検索条件を入力して") ||
		!strings.Contains(initialBody, "/slips?kind=purchases&show_all=1") ||
		strings.Contains(initialBody, `class="slips-table purchases"`) {
		t.Fatalf("initial purchase slips must show search prompt: status=%d body=%s", initialRecorder.Code, initialBody)
	}

	all := httptest.NewRequest(http.MethodGet, "/slips?kind=purchases&show_all=1", nil)
	all.AddCookie(session)
	allRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(allRecorder, all)
	allBody := allRecorder.Body.String()
	if allRecorder.Code != http.StatusOK ||
		!strings.Contains(allBody, `class="slips-table purchases"`) ||
		!strings.Contains(allBody, "合計仕入金額") ||
		!strings.Contains(allBody, "ステータス/修正") ||
		!strings.Contains(allBody, "data-purchase-slip-open") ||
		strings.Contains(allBody, `href="/purchases/`) {
		t.Fatalf("show all purchase slips: status=%d body=%s", allRecorder.Code, allBody)
	}

	productSearch := httptest.NewRequest(http.MethodGet, "/slips?kind=purchases&search=1&q=ロレックス", nil)
	productSearch.AddCookie(session)
	productSearchRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(productSearchRecorder, productSearch)
	productSearchBody := productSearchRecorder.Body.String()
	if productSearchRecorder.Code != http.StatusOK ||
		!strings.Contains(productSearchBody, `class="slips-table purchases"`) {
		t.Fatalf("purchase product search: status=%d body=%s", productSearchRecorder.Code, productSearchBody)
	}
}

func TestPurchaseSlipModalMatchesMockDetailFields(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	slips, err := store.PurchaseSlips(t.Context(), "org_preview")
	if err != nil || len(slips) == 0 {
		t.Fatalf("purchase slips=%+v err=%v", slips, err)
	}
	products, err := store.PurchaseProducts(t.Context(), "org_preview", slips[0].ID)
	if err != nil || len(products) == 0 {
		t.Fatalf("purchase products=%+v err=%v", products, err)
	}
	if err := store.UpdatePurchaseSlip(t.Context(), database.UpdatePurchaseSlipInput{
		OrganizationID: "org_preview",
		PurchaseSlipID: slips[0].ID,
		PurchaseDate:   slips[0].PurchaseDate,
		Notes:          slips[0].Notes,
		Memo:           "明細を確認して修正",
		ActorUserID:    slips[0].CreatedByID,
		Products: []database.PurchaseEditProduct{{
			ProductID:          products[0].ProductID,
			SKU:                products[0].SKU,
			CostAmountMinor:    products[0].CostAmountMinor,
			BaseSalePriceMinor: products[0].BaseSalePriceMinor,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/slips/purchases/"+slips[0].ID+"/modal", nil)
	request.Header.Set("HX-Request", "true")
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{
		"仕入伝票", "伝票番号", "仕入日", "仕入先", "仕入担当者",
		"合計仕入金額", "合計売価", "商品コード", "SKU", "ブランド",
		"モデル", "修正日時", "担当バイヤー", "承認管理者", "修正内容",
		"伝票修正", "仕入返品を起票", "閉じる",
	} {
		if recorder.Code != http.StatusOK || !strings.Contains(body, expected) {
			t.Fatalf("purchase modal status=%d expected=%q body=%s", recorder.Code, expected, body)
		}
	}
}

func TestPublicPurchaseRequestFlowRendersWithoutCostData(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedRequestPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	publishProductForGuestCompany(t, store, 0)
	guestSession := loginGuestAs(t, app, "B001")
	get := httptest.NewRequest(http.MethodGet, "/public/products", nil)
	get.AddCookie(guestSession)
	getRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(getRecorder, get)
	body := getRecorder.Body.String()
	if getRecorder.Code != http.StatusOK || !strings.Contains(body, "購入依頼を送る") {
		t.Fatalf("public catalog status=%d body=%s", getRecorder.Code, body)
	}
	if strings.Contains(body, "仕入原価") || strings.Contains(body, "粗利") {
		t.Fatal("public catalog leaked cost or profit data")
	}
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	productMatch := regexp.MustCompile(`data-guest-add-cart data-id="([^"]+)"`).FindStringSubmatch(body)
	if len(csrfMatch) != 2 || len(productMatch) != 2 {
		t.Fatal("public purchase form fields missing")
	}
	form := url.Values{
		"csrf_token": {csrfMatch[1]}, "guest_name": {"公開ゲスト"},
		"guest_email": {"guest@example.com"}, "message": {"購入を希望します"},
		"product_id": {productMatch[1]},
	}
	post := httptest.NewRequest(http.MethodPost, "/public/purchase-requests", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.Header.Set("X-CSRF-Token", csrfMatch[1])
	post.AddCookie(guestSession)
	postRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(postRecorder, post)
	if postRecorder.Code != http.StatusSeeOther {
		t.Fatalf("public request status=%d body=%s", postRecorder.Code, postRecorder.Body.String())
	}

	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	internal := httptest.NewRequest(http.MethodGet, "/purchase-requests", nil)
	internal.AddCookie(session)
	internalRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(internalRecorder, internal)
	if internalRecorder.Code != http.StatusOK || !strings.Contains(internalRecorder.Body.String(), "公開ゲスト") {
		t.Fatalf("internal request list status=%d body=%s", internalRecorder.Code, internalRecorder.Body.String())
	}
}

func TestAdminCanOpenDashboardWithoutCostDataOnPublicPage(t *testing.T) {
	app, _ := testServer(t)
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "最新仕入（直近5件）") ||
		!strings.Contains(body, "月別売上推移") || !strings.Contains(body, "仕入先別 構成比（今月）") {
		t.Fatalf("dashboard status=%d", recorder.Code)
	}

	publicRequest := httptest.NewRequest(http.MethodGet, "/public/products", nil)
	publicRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(publicRecorder, publicRequest)
	body = publicRecorder.Body.String()
	if strings.Contains(body, "仕入原価") || strings.Contains(body, "粗利率") {
		t.Fatal("public page leaked cost fields")
	}
}

func TestPhase8MasterAndGuestManagementPages(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	for _, test := range []struct {
		path     string
		contains []string
		absent   []string
	}{
		{
			path: "/masters?category=currencies",
			contains: []string{"外貨レート", "USD / JPY", "EUR / JPY", "HKD / JPY", "CHF / JPY",
				"プレビュー", "更新日", "更新者"},
		},
		{
			path:     "/masters?category=guests",
			contains: []string{"ゲスト管理へ", "企業ごとの公開設定・商品ラインナップの確認ができます"},
		},
		{
			path: "/guest-management",
			contains: []string{"ゲスト管理", "BOX1", "BOX10", "ゲスト公開情報を一括更新",
				"公開BOX数", "公開商品数", "固定BOX1〜10"},
			absent: []string{"公開設定を下書き保存", "公開先企業数"},
		},
		{
			path:     "/market-prices",
			contains: []string{"相場表"},
			absent:   []string{"取得元", "レート履歴", "プロバイダー"},
		},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.AddCookie(session)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", test.path, recorder.Code, recorder.Body.String())
		}
		body := recorder.Body.String()
		for _, value := range test.contains {
			if !strings.Contains(body, value) {
				t.Errorf("%s missing %q", test.path, value)
			}
		}
		for _, value := range test.absent {
			if strings.Contains(body, value) {
				t.Errorf("%s unexpectedly contains %q", test.path, value)
			}
		}
	}
}

func TestAdminCanManageMasterRecords(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	pageRequest := httptest.NewRequest(http.MethodGet, "/masters?category=brands", nil)
	pageRequest.AddCookie(session)
	pageRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(pageRecorder, pageRequest)
	pageBody := pageRecorder.Body.String()
	if pageRecorder.Code != http.StatusOK || !strings.Contains(pageBody, "ブランド名") ||
		!strings.Contains(pageBody, "BRD-010") || !strings.Contains(pageBody, "新規追加") {
		t.Fatalf("master page status=%d body=%s", pageRecorder.Code, pageBody)
	}

	form := url.Values{
		"csrf_token": {csrf.Value}, "category": {"brands"},
		"code": {"BRD-011"}, "name": {"テストブランド"},
	}
	createRequest := httptest.NewRequest(http.MethodPost, "/masters", strings.NewReader(form.Encode()))
	createRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createRequest.AddCookie(session)
	createRequest.AddCookie(csrf)
	createRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusSeeOther {
		t.Fatalf("master create status=%d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	records, err := store.MasterRecords(t.Context(), "org_preview", "brands")
	if err != nil || len(records) != 11 {
		t.Fatalf("master records=%d err=%v", len(records), err)
	}
}

func TestAdminCanOpenAndCompleteStocktake(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedStocktakePreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	pageRequest := httptest.NewRequest(http.MethodGet, "/stocktakes", nil)
	pageRequest.AddCookie(session)
	pageRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(pageRecorder, pageRequest)
	body := pageRecorder.Body.String()
	if pageRecorder.Code != http.StatusOK || !strings.Contains(body, "棚卸管理") ||
		!strings.Contains(body, "STK-20260727-001") || !strings.Contains(body, "棚卸を確定") {
		t.Fatalf("stocktake page status=%d body=%s", pageRecorder.Code, body)
	}

	list, err := store.Stocktakes(t.Context(), "org_preview")
	if err != nil || len(list) != 1 {
		t.Fatalf("stocktakes=%d err=%v", len(list), err)
	}
	form := url.Values{"csrf_token": {csrf.Value}}
	completeRequest := httptest.NewRequest(
		http.MethodPost, "/stocktakes/"+list[0].ID+"/complete", strings.NewReader(form.Encode()),
	)
	completeRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	completeRequest.AddCookie(session)
	completeRequest.AddCookie(csrf)
	completeRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(completeRecorder, completeRequest)
	if completeRecorder.Code != http.StatusSeeOther {
		t.Fatalf("stocktake complete status=%d body=%s", completeRecorder.Code, completeRecorder.Body.String())
	}
	completed, err := store.Stocktake(t.Context(), "org_preview", list[0].ID)
	if err != nil || completed.Status != "completed" {
		t.Fatalf("completed stocktake=%+v err=%v", completed, err)
	}
}

func TestAdminCanAggregatePerformanceAndExportCSV(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMarketPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	for _, test := range []struct {
		mode     string
		expected string
	}{
		{mode: "suppliers", expected: "仕入先別 構成比"},
		{mode: "buyers", expected: "仕入担当者別 構成比"},
		{mode: "sales-destinations", expected: "販売先別 構成比"},
	} {
		path := "/performance?mode=" + test.mode + "&date_from=2026-01-01&date_to=2026-12-31"
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(session)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.expected) {
			t.Fatalf("mode=%s status=%d body=%s", test.mode, recorder.Code, recorder.Body.String())
		}
	}

	csvRequest := httptest.NewRequest(
		http.MethodGet,
		"/performance/export.csv?mode=suppliers&date_from=2026-01-01&date_to=2026-12-31",
		nil,
	)
	csvRequest.AddCookie(session)
	csvRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(csvRecorder, csvRequest)
	if csvRecorder.Code != http.StatusOK ||
		!strings.Contains(csvRecorder.Header().Get("Content-Disposition"), "performance-suppliers") ||
		!strings.Contains(csvRecorder.Body.String(), "仕入金額") {
		t.Fatalf("performance csv status=%d headers=%v body=%s", csvRecorder.Code, csvRecorder.Header(), csvRecorder.Body.String())
	}
}

func TestFormatIntegerHandlesNegativeAmounts(t *testing.T) {
	if got := formatInteger(-420000); got != "-420,000" {
		t.Fatalf("formatInteger(-420000)=%q", got)
	}
}

func TestProductAdvancedSearchAndModalDetail(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	target := products[0]
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	path := "/products?brand=" + url.QueryEscape(target.Brand) +
		"&model_number=" + url.QueryEscape(target.ModelNumber) +
		"&supplier_id=" + url.QueryEscape(target.SupplierID)
	search := httptest.NewRequest(http.MethodGet, path, nil)
	search.AddCookie(session)
	searchRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(searchRecorder, search)
	body := searchRecorder.Body.String()
	if searchRecorder.Code != http.StatusOK || !strings.Contains(body, "在庫検索") ||
		!strings.Contains(body, `data-product-detail="`+target.ID+`"`) ||
		!strings.Contains(body, `data-product-row="`+target.ID+`"`) ||
		!strings.Contains(body, "<th>ステータス</th><th>BOX</th><th>編集</th>") ||
		!strings.Contains(body, `type="checkbox" name="accessory"`) ||
		!strings.Contains(body, "BOX・GUARANTEE") ||
		strings.Contains(body, `role="button" aria-label="`+target.ProductCode) {
		t.Fatalf("search status=%d body=%s", searchRecorder.Code, body)
	}

	modal := httptest.NewRequest(http.MethodGet, "/products/"+target.ID+"/modal", nil)
	modal.Header.Set("HX-Request", "true")
	modal.AddCookie(session)
	modalRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(modalRecorder, modal)
	modalBody := modalRecorder.Body.String()
	if modalRecorder.Code != http.StatusOK || strings.Contains(modalBody, "<!doctype html>") ||
		!strings.Contains(modalBody, target.ProductCode) || !strings.Contains(modalBody, "仕入担当者") ||
		!strings.Contains(modalBody, "素材（本体）") || !strings.Contains(modalBody, "駆動方式") ||
		!strings.Contains(modalBody, "ベルト素材") || !strings.Contains(modalBody, "文字盤") ||
		!strings.Contains(modalBody, "<dt>BOX</dt>") || !strings.Contains(modalBody, "data-product-edit=") {
		t.Fatalf("modal status=%d body=%s", modalRecorder.Code, modalBody)
	}
}

func TestProductEditModalAndUpdateMatchMockWorkflow(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	target := products[0]
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	editRequest := httptest.NewRequest(http.MethodGet, "/products/"+target.ID+"/edit", nil)
	editRequest.Header.Set("HX-Request", "true")
	editRequest.AddCookie(session)
	editRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(editRecorder, editRequest)
	editBody := editRecorder.Body.String()
	for _, expected := range []string{
		"商品情報を編集", "基本情報", "商品スペック", "価格・仕入情報",
		"仕入金額（税抜・JPY）", "売価（JPY）", "name=\"box\"", "BOX1 — ロレックス特集",
		"name=\"accessories\"", "BRACELET PARTS", "変更メモ（理由・コメント）", "確定する",
	} {
		if !strings.Contains(editBody, expected) {
			t.Fatalf("edit modal missing %q status=%d body=%s", expected, editRecorder.Code, editBody)
		}
	}

	form := url.Values{
		"csrf_token": {csrf.Value}, "buyer_id": {target.BuyerID}, "supplier_id": {target.SupplierID},
		"purchase_date": {target.PurchaseDate}, "brand": {"ロレックス"}, "product_type": {"サブマリーナ 更新"},
		"model_number": {"116610LN-EDIT"}, "serial_number": {target.SerialNumber},
		"inventory_status": {"sold"}, "condition": {"極美品（S）"}, "material": {"ステンレスSS"},
		"movement": {"自動巻き"}, "belt_material": {"ステンレス"}, "dial": {"ブラック"},
		"box": {"BOX1"}, "accessories": {"BOX", "GUARANTEE"}, "features": {"文字盤：黒"},
		"cost_amount": {"851000"}, "base_sale_price": {"1181000"}, "change_memo": {"表示内容を更新"},
	}
	updateRequest := httptest.NewRequest(http.MethodPost, "/products/"+target.ID+"/edit", strings.NewReader(form.Encode()))
	updateRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateRequest.Header.Set("Accept", "application/json")
	updateRequest.AddCookie(session)
	updateRequest.AddCookie(csrf)
	updateRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(updateRecorder, updateRequest)
	if updateRecorder.Code != http.StatusOK || !strings.Contains(updateRecorder.Body.String(), `"ok":true`) {
		t.Fatalf("update status=%d body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}
	updated, err := store.Product(t.Context(), "org_preview", target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ProductType != "サブマリーナ 更新" || updated.ModelNumber != "116610LN-EDIT" ||
		updated.InventoryStatus != "sold" ||
		updated.Box != "BOX1" || updated.Accessories != "BOX, GUARANTEE" ||
		updated.Material != "ステンレスSS" || updated.Movement != "自動巻き" ||
		updated.BeltMaterial != "ステンレス" || updated.Dial != "ブラック" ||
		updated.Features != "文字盤：黒" || updated.CostAmountMinor != 851000 ||
		updated.BaseSalePriceMinor != 1181000 || len(updated.Events) == 0 ||
		updated.Events[0].EventType != "product.edited" || updated.Events[0].Reason != "表示内容を更新" {
		t.Fatalf("updated product=%+v events=%+v", updated, updated.Events)
	}
}

func TestProductEditModalShowsConfirmedPurchaseAsInStock(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	suppliers, err := store.Suppliers(t.Context(), "org_preview")
	if err != nil || len(suppliers) == 0 {
		t.Fatalf("suppliers=%d err=%v", len(suppliers), err)
	}
	slip, err := store.CreatePurchaseDraft(t.Context(), database.CreatePurchaseInput{
		OrganizationID: "org_preview", SupplierID: suppliers[0].ID, PurchaseDate: "2026-08-08",
		CreatedBy: "usr_admin", Lines: []database.PurchaseLineInput{{
			Quantity: 1, UnitCostMinor: 100000, BaseSalePriceMinor: 150000,
			Currency: "JPY", Brand: "ロレックス", ModelNumber: "STATUS-REF", ProductType: "腕時計",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := store.ConfirmPurchase(t.Context(), "org_preview", slip.ID, "usr_admin")
	if err != nil || len(confirmed.Products) != 1 {
		t.Fatalf("confirmed=%+v err=%v", confirmed, err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	request := httptest.NewRequest(http.MethodGet, "/products/"+confirmed.Products[0].ID+"/edit", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK ||
		!strings.Contains(body, `<option value="in_stock" selected>在庫中</option>`) ||
		!strings.Contains(body, `<select name="inventory_status">`) {
		t.Fatalf("in-stock edit status=%d body=%s", recorder.Code, body)
	}
}

func TestProductFilterSupportsMultipleAccessoryCheckboxes(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/products?accessory=CASE&accessory=GUARANTEE", nil)
	filter := productFilterFromRequest(request, database.User{Role: database.RoleAdmin})
	if filter.Accessory != "CASE,GUARANTEE" {
		t.Fatalf("accessory filter=%q", filter.Accessory)
	}
}
