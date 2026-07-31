package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func phase7Post(t *testing.T, app *Server, session, csrf *http.Cookie, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	values.Set("csrf_token", csrf.Value)
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-Invoice-Preview", "true")
	request.AddCookie(session)
	request.AddCookie(csrf)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestPhase7SalesReturnInvoicePreviewUnlocksCompletion(t *testing.T) {
	app, store := testServer(t)
	for _, seed := range []func() error{
		func() error { return store.SeedInventoryPreview(t.Context()) },
		func() error { return store.SeedMarketPreview(t.Context()) },
		func() error { return store.SeedSalesPreview(t.Context()) },
	} {
		if err := seed(); err != nil {
			t.Fatal(err)
		}
	}
	sales, err := store.Sales(t.Context(), "org_preview")
	if err != nil || len(sales) == 0 {
		t.Fatalf("sales=%v err=%v", sales, err)
	}
	sale, err := store.Sale(t.Context(), "org_preview", sales[0].ID)
	if err != nil || len(sale.Lines) == 0 {
		t.Fatalf("sale=%v err=%v", sale, err)
	}
	if _, err := store.CreateSalesReturn(t.Context(), database.CreateSalesReturnInput{
		OrganizationID: "org_preview",
		SalesSlipID:    sale.ID,
		SalesLineIDs:   []string{sale.Lines[0].ID},
		ReturnDate:     "2026-07-30",
		Reason:         "動作不良",
		Notes:          "請求書テスト",
		ActorUserID:    "usr_admin",
	}); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	before := httptest.NewRequest(http.MethodGet, "/slips/sales-returns/"+sale.ID+"/modal", nil)
	before.AddCookie(session)
	beforeRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(beforeRecorder, before)
	if beforeRecorder.Code != http.StatusOK ||
		!strings.Contains(beforeRecorder.Body.String(), "先に請求書を発行・印刷してプレビューを確認してください") ||
		!strings.Contains(beforeRecorder.Body.String(), `sales-return-complete-button" type="submit" disabled`) {
		t.Fatalf("sales return must be locked before preview: status=%d body=%s", beforeRecorder.Code, beforeRecorder.Body.String())
	}

	invoice := phase7Post(t, app, session, csrf, "/slips/sales-returns/"+sale.ID+"/invoice", url.Values{})
	for _, expected := range []string{
		"売上返品 請求書", "SALES RETURN INVOICE", "INV-", "元売上伝票",
		"売上返品伝票", "ご請求金額（税込）", "商品名 / 詳細", "動作不良",
	} {
		if invoice.Code != http.StatusOK || !strings.Contains(invoice.Body.String(), expected) {
			t.Fatalf("sales return invoice missing %q status=%d body=%s", expected, invoice.Code, invoice.Body.String())
		}
	}

	after := httptest.NewRequest(http.MethodGet, "/slips/sales-returns/"+sale.ID+"/modal", nil)
	after.AddCookie(session)
	afterRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(afterRecorder, after)
	if afterRecorder.Code != http.StatusOK ||
		strings.Contains(afterRecorder.Body.String(), `sales-return-complete-button" type="submit" disabled`) ||
		!strings.Contains(afterRecorder.Body.String(), "請求書はプレビュー済みです") {
		t.Fatalf("sales return must unlock after preview: status=%d body=%s", afterRecorder.Code, afterRecorder.Body.String())
	}
}

func TestPhase7PurchaseReturnSingleBulkAndCSV(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMarketPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedPurchaseReturnPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	returns, err := store.PurchaseReturnSlips(t.Context(), "org_preview")
	if err != nil || len(returns) == 0 {
		t.Fatalf("returns=%v err=%v", returns, err)
	}
	purchaseReturn := returns[0]
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	modal := httptest.NewRequest(http.MethodGet, "/slips/purchase-returns/"+purchaseReturn.ID+"/modal", nil)
	modal.AddCookie(session)
	modalRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(modalRecorder, modal)
	if modalRecorder.Code != http.StatusOK || strings.Contains(modalRecorder.Body.String(), "data-purchase-return-complete disabled") {
		t.Fatalf("purchase return completion must not require invoice: status=%d body=%s", modalRecorder.Code, modalRecorder.Body.String())
	}

	invoice := phase7Post(t, app, session, csrf, "/slips/purchase-returns/"+purchaseReturn.ID+"/invoice", url.Values{})
	for _, expected := range []string{
		"仕入返品 請求書", "PURCHASE RETURN INVOICE", "ご請求金額（税込）",
		"お振込先 / お支払い先", "起票者", "商品名 / 詳細",
	} {
		if invoice.Code != http.StatusOK || !strings.Contains(invoice.Body.String(), expected) {
			t.Fatalf("purchase return invoice missing %q status=%d body=%s", expected, invoice.Code, invoice.Body.String())
		}
	}

	bulk := phase7Post(t, app, session, csrf, "/slips/purchase-returns/invoices/preview", url.Values{"id": {purchaseReturn.ID}})
	for _, expected := range []string{"請求書を発行しますか？", "選択中：1件 / 1仕入先", "CSVダウンロード", "返品伝票番号", "返品理由"} {
		if bulk.Code != http.StatusOK || !strings.Contains(bulk.Body.String(), expected) {
			t.Fatalf("bulk preview missing %q status=%d body=%s", expected, bulk.Code, bulk.Body.String())
		}
	}

	csv := phase7Post(t, app, session, csrf, "/slips/purchase-returns/invoices.csv", url.Values{"id": {purchaseReturn.ID}})
	header := "仕入先,返品伝票番号,返品日,商品コード,ブランド,モデル名,型番,シリアル,仕入金額,返品理由,ステータス"
	if csv.Code != http.StatusOK || !strings.Contains(csv.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(strings.TrimPrefix(csv.Body.String(), "\ufeff"), header) {
		t.Fatalf("purchase return csv mismatch: status=%d content-type=%s body=%s", csv.Code, csv.Header().Get("Content-Type"), csv.Body.String())
	}
}
