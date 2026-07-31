package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestPhase9MarketTableSearchDetailEditAndCSVExport(t *testing.T) {
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

	request := httptest.NewRequest(http.MethodGet, "/market-prices", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	for _, expected := range []string{"相場検索", "CSV取込", "CSV出力", "全件表示"} {
		if recorder.Code != http.StatusOK || !strings.Contains(body, expected) {
			t.Fatalf("initial status=%d missing=%q body=%s", recorder.Code, expected, body)
		}
	}
	for _, removed := range []string{"相場情報を登録", "相場日", "取得元", "備考"} {
		if strings.Contains(body, removed) {
			t.Fatalf("removed field %q remains in page", removed)
		}
	}

	request = httptest.NewRequest(http.MethodGet, "/market-prices?q="+url.QueryEscape(target.ProductCode), nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body = recorder.Body.String()
	for _, expected := range []string{target.ProductCode, "仕入相場価格", "売値相場価格", `data-market-row="` + target.ID + `"`} {
		if recorder.Code != http.StatusOK || !strings.Contains(body, expected) {
			t.Fatalf("search status=%d missing=%q body=%s", recorder.Code, expected, body)
		}
	}

	request = httptest.NewRequest(http.MethodGet, "/market-prices/products/"+target.ID+"/modal", nil)
	request.Header.Set("HX-Request", "true")
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "相場詳細") ||
		!strings.Contains(recorder.Body.String(), "保存") {
		t.Fatalf("modal status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	form := url.Values{
		"csrf_token":            {csrf.Value},
		"purchase_market_price": {"730000"},
		"sale_market_price":     {"980000"},
	}
	request = httptest.NewRequest(http.MethodPost, "/market-prices/products/"+target.ID+"/edit", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(session)
	request.AddCookie(csrf)
	recorder = httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("update status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	updated, err := store.ProductMarketPriceByProductID(t.Context(), "org_preview", target.ID)
	if err != nil || updated.PurchaseMarketPriceMinor != 730_000 || updated.SaleMarketPriceMinor != 980_000 {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}

	request = httptest.NewRequest(http.MethodGet, "/market-prices/export.csv?q="+url.QueryEscape(target.ProductCode), nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	csvBody := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(csvBody, "product_code,purchase_market_price,sale_market_price") ||
		!strings.Contains(csvBody, target.ProductCode+",730000,980000") {
		t.Fatalf("csv status=%d body=%s", recorder.Code, csvBody)
	}
}

func TestPhase9MarketCSVImportFromSamePage(t *testing.T) {
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
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("csrf_token", csrf.Value); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("csv_file", "market-prices.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("product_code,purchase_market_price,sale_market_price\n" +
		target.ProductCode + ",810000,1050000\n"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/market-prices/import.csv", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(session)
	request.AddCookie(csrf)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("import status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	imported, err := store.ProductMarketPriceByProductID(t.Context(), "org_preview", target.ID)
	if err != nil || imported.PurchaseMarketPriceMinor != 810_000 || imported.SaleMarketPriceMinor != 1_050_000 {
		t.Fatalf("imported=%+v err=%v", imported, err)
	}
}
