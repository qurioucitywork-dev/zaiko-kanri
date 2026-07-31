package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestPhase13DashboardMatchesMockContract(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedApprovalPreview(t.Context()); err != nil {
		t.Fatal(err)
	}

	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{Page: 1, PageSize: 500})
	if err != nil {
		t.Fatal(err)
	}
	var available []database.Product
	for _, product := range products {
		if product.InventoryStatus == "in_stock" {
			available = append(available, product)
		}
	}
	if len(available) < 2 {
		t.Fatalf("need two in-stock products, got %d", len(available))
	}
	for _, product := range available[:2] {
		if err := store.SetProductPublication(t.Context(), "org_preview", product.ID, "usr_admin", "public"); err != nil {
			t.Fatal(err)
		}
	}
	for index, input := range []struct {
		product database.Product
		message string
		groupID string
	}{
		{available[0], "至急確認お願いします", "dashboard-group-urgent"},
		{available[1], "至急確認お願いします", "dashboard-group-urgent"},
		{available[0], "急ぎではありません", "dashboard-group-normal"},
	} {
		_, err := store.CreatePurchaseRequest(t.Context(), database.PurchaseRequestInput{
			OrganizationCode: "PREVIEW", ProductID: input.product.ID,
			RequestGroupID: input.groupID,
			GuestName:      "クロノス東京", GuestEmail: "buyer@example.com",
			GuestPhone: "03-9999-0000", Message: input.message,
		})
		if err != nil {
			t.Fatalf("request %d: %v", index, err)
		}
	}

	stats, err := store.InventoryStats(t.Context(), "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()

	kpis := []string{"在庫点数", "今月売上", "今月仕入金額", "未対応購入リクエスト"}
	last := -1
	for _, label := range kpis {
		position := strings.Index(body, label)
		if position <= last {
			t.Fatalf("KPI order mismatch at %q", label)
		}
		last = position
	}
	if !strings.Contains(body, fmt.Sprintf("<small>在庫点数</small><strong>%d", stats.InStock)) {
		t.Fatalf("inventory KPI must use only in_stock count=%d: %s", stats.InStock, body)
	}
	for _, expected := range []string{
		`aria-label="未対応承認 1件"`,
		`class="nav-alert-count">2</b>`,
		"最新仕入（直近5件）", "商品コード", "ブランド / モデル", "仕入金額", "ステータス",
		`class="dashboard-product-row" data-product-row="`, `tabindex="0" role="button"`, `data-product-modal`, "一覧を見る →",
		"購入リクエスト（未対応）", "すべて見る →", "至急確認お願いします", "急ぎではありません",
		"2点", `href="/purchase-requests"`, `data-purchase-request-open="dashboard-request-`,
		`data-purchase-request-dialog`, "購入依頼商品",
		"仕入先別 構成比（今月）", `class="donut-slice"`,
		"月別売上推移", `class="sales-bar"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("dashboard missing %q: %s", expected, body)
		}
	}
	for _, removed := range []string{"dashboard-settings-summary", "今月売上目標", "今月仕入予算", "商品を登録する"} {
		if strings.Contains(body, removed) {
			t.Fatalf("dashboard still contains mock-external UI %q: %s", removed, body)
		}
	}
	wantLatest := len(products)
	if wantLatest > 5 {
		wantLatest = 5
	}
	if count := strings.Count(body, `data-product-row="`); count != wantLatest {
		t.Fatalf("latest purchases count=%d want=%d", count, wantLatest)
	}
	if count := strings.Count(body, `class="dashboard-request-card"`); count != 2 {
		t.Fatalf("pending request card count=%d want=2", count)
	}

	modalRequest := httptest.NewRequest(http.MethodGet, "/products/"+products[0].ID+"/modal", nil)
	modalRequest.AddCookie(session)
	modal := httptest.NewRecorder()
	app.Handler().ServeHTTP(modal, modalRequest)
	if modal.Code != http.StatusOK || !strings.Contains(modal.Body.String(), products[0].ProductCode) ||
		!strings.Contains(modal.Body.String(), "編集する") {
		t.Fatalf("dashboard product modal endpoint status=%d body=%s", modal.Code, modal.Body.String())
	}
}

func TestPhase13DashboardGroupingAndSupplierAmountChart(t *testing.T) {
	now := time.Date(2026, 7, 30, 10, 15, 0, 0, time.FixedZone("JST", 9*60*60))
	requests := []database.PurchaseRequest{
		{ID: "one", RequestGroupID: "group-a", GuestName: "ゲスト", GuestEmail: "guest@example.com", Message: "同じ依頼", RequestedAt: now, Brand: "A", ModelNumber: "One", SalePriceMinor: 100, SaleCurrency: "JPY"},
		{ID: "two", RequestGroupID: "group-a", GuestName: "ゲスト", GuestEmail: "guest@example.com", Message: "同じ依頼", RequestedAt: now.Add(20 * time.Second), Brand: "B", ModelNumber: "Two", SalePriceMinor: 200, SaleCurrency: "USD"},
		{ID: "three", RequestGroupID: "group-b", GuestName: "ゲスト", GuestEmail: "guest@example.com", Message: "同じ依頼", RequestedAt: now.Add(20 * time.Second), Brand: "C", ModelNumber: "Three", SalePriceMinor: 300, SaleCurrency: "JPY"},
	}
	groups := []database.PurchaseRequestGroup{
		{ID: "group-a", Items: requests[:2]},
		{ID: "group-b", Items: requests[2:]},
	}
	cards := buildDashboardRequestCards(groups)
	if len(cards) != 2 || len(cards[0].Items) != 2 || !cards[0].MixedCurrency ||
		len(cards[0].Totals) != 2 || cards[0].Totals[0].Currency != "JPY" ||
		cards[0].Totals[0].Amount != 100 || cards[0].Totals[1].Currency != "USD" ||
		cards[0].Totals[1].Amount != 200 || cards[0].ProductNames != "A One、B Two" {
		t.Fatalf("cards=%+v", cards)
	}

	slices := buildSupplierSlices([]database.Product{
		{SupplierName: "大口仕入先", PurchaseDate: "2026-07-01", CostCurrency: "JPY", CostAmountMinor: 900},
		{SupplierName: "小口仕入先", PurchaseDate: "2026-07-02", CostCurrency: "JPY", CostAmountMinor: 100},
		{SupplierName: "対象外", PurchaseDate: "2026-06-30", CostCurrency: "JPY", CostAmountMinor: 10000},
	}, "2026-07")
	if len(slices) != 2 || slices[0].Name != "大口仕入先" || slices[0].Percent != 90 ||
		slices[1].Percent != 10 {
		t.Fatalf("supplier slices must use purchase amounts: %+v", slices)
	}
}
