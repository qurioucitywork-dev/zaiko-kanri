package web

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

// These hashes pin the user-supplied Genspark reference snapshot together with
// the deliberate local extensions. A UI change must update the source, built
// assets, browser contract, and this manifest-like test as one change.
func TestReferenceAdminSnapshotIntegrity(t *testing.T) {
	expected := map[string]string{
		"js/return_adjustments.js":    "fe744f74ac21173086b1d01384ba967cfbb88a50d2bcde913ac46060c6636200",
		"js/slip_statement.js":        "bc18eae8871dea76c4ba87fc5f4554f0c3991ef9d7e9486274b61943f24d370b",
		"css/auction_fees.css":        "acc4caa0c585f8a37713ec6c84d633a4c78601684166bfc9f14514a17cf2c8ff",
		"js/auction_fees.js":          "467fa43551cb1594829d0961a96b56ddcc8a3754979c191d49e32cf05c3279de",
		"js/accessory_values.js":      "f7f745314c7aa3b6bba8ad62c6e2273e9578d698bcb9b3e9b69ef8ee40904492",
		"app.html":                    "3a1cece658f7620ba93a1900e85a698f6f93e7cf20691d86c7a6d54c5bb76b90",
		"guest.html":                  "bbaee06ef911462ebb16fb3c5d894fa6da9c4e8d3362991c0949b76520ab8eba",
		"css/guest.css":               "48330bc86e144f845053d78fd4976e551d84571170095e7cc567730e1af506b4",
		"css/market-table.css":        "1fbd958c84da4b6cff7c648fb67166f158c6bcfe0f2f587e84860ba7d72b94d5",
		"css/style.css":               "4cb2df4ded5c89920ecfbd7eee660178aee63ed6eeed5f952410981ac6065e5e",
		"index.html":                  "f69f264c1c4ac57c640c8e78a6e37c7a042f54f2cdb8d1eb555a8ea62cea72b6",
		"js/api_bridge.js":            "feaca0d4ac48f3f60b3ec34626ad7c7f23b72d5c56f8dee28c0ed08d02dcf696",
		"js/app.js":                   "d4ec40b74b23f9774c187d73668b619afe2708a625df7fa54b1f0bd8ac479c0c",
		"js/approval.js":              "7eda29ed85e52157a41c26c91d1657a00ddc88e60fe330e1da5d53a630f42145",
		"js/auth.js":                  "998e576e3b9ff1bdf329063bc837ae8a032e5a2e05ae27396d42caa7b3f8c380",
		"js/box.js":                   "222c54eb3310474002ba4ee0f923acc4b5c9a14d24d0709332637814509be23f",
		"js/cost_adjustment_entry.js": "0f997dbfbb0d9bf9f46f5915d615efcd9fb9b9b613db4ffe91caf195c239063a",
		"js/cost_merge.js":            "ff25a19d544d4d40d0675451915541754fd5c1f0bf73ee7ff792243b392f46fc",
		"js/data.js":                  "b903cffe92793fd6fb7856dbe9b4943ef1c8088a539e28e852624ca0c4ca3221",
		"js/guest.js":                 "6ec8d148dc2fe091fe6b45e5869d0c5f31d8a3e1c90afa65ef1d216d15bee451",
		"js/guest_shared.js":          "d185043a74f87a27aeee005c20940d983d81136c3a645babd6832097fe8a4499",
		"js/login_info.js":            "8c256c208552cbe6cf16a9636ee347addb6bca88eabc1883f230f2827a990b3d",
		"js/market_table.js":          "66768aedf141300b8ff9809831bf6e2a96781d8e90d6bf85f15cc7cb6366394f",
		"js/notify.js":                "1893019ed66da6c3e4c66ac1f0f3594dda3c97325ae66b2a9c27a71405a38ba3",
		"js/purchase_entry.js":        "418a7b56298377ebaabe4a20bb7c239d8e82eb978797f3f60bde42ce2e1bc710",
		"js/stocktake.js":             "f0d7c38f327cabc18874eb331c920b1afa39878175a492c5ca86c25436d092bd",
		"js/consignment.js":           "c9a73eaf1c2d200a8b1c688737e229b44c70b5529110b14a89e4ccce7f73a5fe",
		"js/qrcode-generator.js":      "18ae399f81182bc9de916e9c77b195df20cc58d6f2d55a62b085a299f1bf1780",
		"js/jsQR.js":                  "bc40c8a15196236b2314db0856f72ca0b49980cd5413b8c852a7349f5fee0859",
		"js/qr_inventory.js":          "dcc27f323395b71cef19d46177ddeccb5da0f568818a701d1a3086abb7577fa8",
	}

	for name, want := range expected {
		content, err := reactAssets.ReadFile("react-dist/admin-reference/" + name)
		if err != nil {
			t.Fatalf("read reference asset %s: %v", name, err)
		}
		canonical := strings.ReplaceAll(string(content), "\r\n", "\n")
		canonical = strings.ReplaceAll(canonical, "\r", "\n")
		if got := fmt.Sprintf("%x", sha256.Sum256([]byte(canonical))); got != want {
			t.Errorf("reference asset %s SHA-256=%s, want %s", name, got, want)
		}
	}
}

func TestReferenceAdminContainsEveryRequiredScreenAndScript(t *testing.T) {
	content, err := reactAssets.ReadFile("react-dist/admin-reference/app.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(content)
	pages := []string{
		"dashboard", "market", "market-entry", "inventory", "purchase-entry", "purchase", "cost-adjustment", "cost-adjustment-slips", "sales",
		"sales-list", "deleted-slips", "shipping", "consignment", "sales-adjustment-entry", "purchase-adjustment-entry", "master", "box", "performance",
		"stocktake", "approval", "purchase-list", "client", "company", "password",
	}
	for _, page := range pages {
		if !strings.Contains(html, `id="page-`+page+`"`) {
			t.Errorf("reference admin is missing page %q", page)
		}
	}
	scripts := []string{
		"qrcode-generator.js", "jsQR.js", "data.js", "guest_shared.js", "login_info.js", "auth.js", "api_bridge.js", "approval.js", "notify.js", "box.js", "market_table.js", "qr_inventory.js", "app.js",
		"return_adjustments.js", "return_adjustments.js", "consignment.js", "stocktake.js", "purchase_entry.js", "cost_merge.js", "cost_adjustment_entry.js",
	}
	for _, script := range scripts {
		if !strings.Contains(html, `src="js/`+script) {
			t.Errorf("reference admin is missing script %q", script)
		}
	}
	for _, marker := range []string{`id="sl-currency-hkd"`, `switchSalesEntryCurrency('HKD')`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing HKD sales control %q", marker)
		}
	}
	for _, marker := range []string{`<select class="form-control" id="pu-belt"`, `<select class="form-control" id="ie-belt"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing belt-material master select %q", marker)
		}
	}
	if !strings.Contains(html, `> 相場表テンプレート`) {
		t.Error("reference admin is missing the market-table template label")
	}
	for _, marker := range []string{`id="marketNavGroup"`, `id="marketNavToggle"`, `id="marketNavSubmenu"`, `>相場関連<`, `data-page="market-entry"`, `id="page-market-entry"`, `id="market-csv-import-button"`, `id="market-basic-category"`, `id="market-basic-auction"`, `id="market-basic-research-date"`, `id="marketDraftAddCount"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing market registration marker %q", marker)
		}
	}
	for _, marker := range []string{`class="card-body pe-slip-list-scroll"`, `class="data-table pe-slip-list-table"`, `class="pe-purchase-total-heading">原価<span`, `入金日付`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing purchase-slip list layout marker %q", marker)
		}
	}
	for _, marker := range []string{`id="pe-payment-cash"`, `id="pe-payment-bank-transfer"`, `id="pe-payment-card"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing purchase payment-method control %q", marker)
		}
	}
	for _, marker := range []string{`id="pu-currency-jpy"`, `id="pu-currency-usd"`, `id="pu-currency-hkd"`, `id="pu-purchase-type-personal"`, `id="pu-tax-category"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing product-registration procurement control %q", marker)
		}
	}
	for _, marker := range []string{`data-page="inventory"`, `> 在庫管理`, `data-inv-col="purchaseRate"`, `data-inv-col="purchasePriceAtPurchaseRate"`, `原価（現在レート）`, `data-inv-col="grossMargin"`, `粗利率`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing inventory rate-comparison marker %q", marker)
		}
	}
	for _, retiredMarker := range []string{`id="inventoryNavGroup"`, `id="inventoryNavSubmenu"`, `onclick="navigateTo('parts-management')"`, `id="registration-mode-part"`} {
		if strings.Contains(html, retiredMarker) {
			t.Errorf("reference admin still exposes retired part-management feature %q", retiredMarker)
		}
	}
	if !strings.Contains(html, `data-page="consignment"`) || !strings.Contains(html, `</span> 委託登録`) {
		t.Error("reference admin is missing the shortened consignment registration label")
	}
	if strings.Contains(html, "委託伝票登録") {
		t.Error("reference admin still contains the former consignment registration label")
	}
	for _, marker := range []string{
		`id="page-sales-adjustment-entry"`, `id="page-purchase-adjustment-entry"`,
		`id="sae-part-code"`, `id="sae-product-code"`, `openBarcodeScanner('sales-adjustment')`,
		`id="sae-load-button"`, `id="sae-target-result"`, `id="sae-linked-slip"`,
		`id="sae-reason"`, `name="sae-adjustment-type" value="discount"`, `name="sae-adjustment-type" value="markup"`,
		`id="sae-amount"`, `id="sae-register-button"`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing adjustment-registration marker %q", marker)
		}
	}
	for _, retiredMarker := range []string{`id="adjustmentEntryNavGroup"`, `id="adjustmentEntryNavToggle"`, `id="adjustmentEntryNavSubmenu"`, `data-page="sales-adjustment-entry"`, `data-page="purchase-adjustment-entry"`} {
		if strings.Contains(html, retiredMarker) {
			t.Errorf("reference admin still exposes retired adjustment registration navigation %q", retiredMarker)
		}
	}
	if strings.Contains(html, `data-ca-mode="swap"`) || strings.Contains(html, `> 入替`) {
		t.Error("reference admin still contains the removed cost-adjustment swap mode")
	}
	if strings.Contains(html, `id="slipBulkControls"`) {
		t.Error("reference admin still contains the undefined slip bulk download and print controls")
	}
	for _, marker := range []string{`id="costManagementNavGroup"`, `id="costManagementNavSubmenu"`, `data-page="cost-adjustment-slips"`, `id="page-cost-adjustment-slips"`, `id="sharedSlipListCard"`, `> 調整伝票`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing dedicated adjustment-slip page marker %q", marker)
		}
	}
	for _, removedMarker := range []string{`id="sltab-purchaseadjustment"`, `id="sltab-salesadjustment"`, `id="sltab-adjustment"`} {
		if strings.Contains(html, removedMarker) {
			t.Errorf("reference admin still contains removed adjustment-slip tab %q", removedMarker)
		}
	}

	purchaseContent, err := reactAssets.ReadFile("react-dist/admin-reference/js/purchase_entry.js")
	if err != nil {
		t.Fatal(err)
	}
	purchaseJS := string(purchaseContent)
	if strings.Contains(purchaseJS, `'文字盤コード'`) {
		t.Error("purchase CSV must not contain the dial-code column")
	}
	for _, marker := range []string{
		`'仕入伝票CSVテンプレート.csv'`,
		`'マーキングコード', '売価', '形状コード', 'SKU'`,
		`CSV取込エラー`,
		`_peCSVCellRef`,
		`getPurchaseListAmounts(slip)`,
		`formatPurchaseRateCell(slip)`,
	} {
		if !strings.Contains(purchaseJS, marker) {
			t.Errorf("purchase CSV contract is missing %q", marker)
		}
	}
	appContent, err := reactAssets.ReadFile("react-dist/admin-reference/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	appJS := string(appContent)
	if strings.Contains(appJS, `_prInjectInvoiceBtn`) || strings.Contains(appJS, `slip-generic-select`) {
		t.Error("reference admin still injects removed slip bulk-selection controls")
	}
	for _, marker := range []string{
		`['return_pending', '仕入返品', '仕入返品中', '仕入返品処理中']`,
		`['cancelled', '取消', '取消済', '取り消し', '仕入返品済', '仕入返品処理済']`,
		`normalizeInventoryCollectionStatuses();`,
		`getInventoryRegisteredPurchaseRate(item)`,
		`getInventoryCurrentPurchaseRate(getInventoryPurchaseCurrency(item))`,
		`getInventoryGrossMarginPercent(item)`,
		`switchItemDetailPriceCurrency(priceType, currency)`,
		`原価（現在レート）`,
		`isPendingPurchaseArrivalStatus(value)`,
		`getPurchaseArrivalStatus(record)`,
		`openPurchaseArrivalScanner(purchaseID)`,
		`_handlePurchaseArrivalScan(code, elements)`,
		`入荷スキャン`,
		`openPurchaseSlipLineDetail(code)`,
		`purchase-slip-product-row`,
		`getPurchaseSlipStatusKeys(record)`,
		`renderPurchaseSlipStatusBadges(record`,
		`支払確認`,
		`支払日付`,
		`costAdjustmentDropProduct(event)`,
		`costAdjustmentConfirmBreakdown()`,
		`costAdjustmentOpenItemEditor(index)`,
		`costAdjustmentCompleteItemEdit()`,
		`costAdjustmentFinalize()`,
		`原価が完全一致しました`,
		`原価調整済`,
		`原価調整中`,
		`['combined', '結合済み']`,
		`'consignment': '委託登録'`,
		`'purchaseadjustment'`,
		`'salesadjustment'`,
		`APP_DATA.purchaseAdjustments`,
		`APP_DATA.salesAdjustments`,
		`APP_DATA.costAdjustments`,
		`function openCostAdjustmentSlipDetail(id)`,
		`function buildCostAdjustmentRecordTemplateHTML(slip)`,
		`伝票をプレビューで見る`,
		`原価調整（${getCostAdjustmentPatternLabel(record.adjustmentType)}）`,
		`'sales-adjustment-entry': '売上調整登録'`,
		`'purchase-adjustment-entry': '仕入調整登録'`,
		`function loadSalesAdjustmentTarget()`,
		`function applySalesAdjustmentScannedCode(rawCode)`,
		`function registerSalesAdjustment()`,
		`'salesAdjustments'`,
		`sourceSlipId: sourceRecord.id`,
		`value="${_escHtml(trackingNumber)}"`,
	} {
		if !strings.Contains(appJS, marker) {
			t.Errorf("inventory status label contract is missing %q", marker)
		}
	}
	if strings.Contains(appJS, `{ key: 'dial'`) || strings.Contains(appJS, `case 'dial':`) {
		t.Error("master management must not expose the dial master")
	}
	authContent, err := reactAssets.ReadFile("react-dist/admin-reference/js/auth.js")
	if err != nil {
		t.Fatal(err)
	}
	authJS := string(authContent)
	for _, marker := range []string{`'sales-adjustment-entry'`, `'purchase-adjustment-entry'`} {
		if !strings.Contains(authJS, marker) {
			t.Errorf("worker navigation permissions are missing adjustment-registration page %q", marker)
		}
	}
	if got := strings.Count(html, `class="modal-overlay`); got != 51 {
		t.Errorf("reference admin modal count=%d, want 51", got)
	}

	guestContent, err := reactAssets.ReadFile("react-dist/admin-reference/guest.html")
	if err != nil {
		t.Fatal(err)
	}
	guestHTML := string(guestContent)
	if strings.Contains(guestHTML, "guest-box-filter") {
		t.Error("guest must not show BOX selection")
	}
	for _, feature := range []string{"guest-product-grid", "guest-brand-filter", "guest-cart-overlay", "guest-submit-request"} {
		if !strings.Contains(guestHTML, feature) {
			t.Errorf("reference guest is missing feature %q", feature)
		}
	}
	for _, script := range []string{"data.js", "guest_shared.js", "login_info.js", "guest.js"} {
		if !strings.Contains(guestHTML, `src="js/`+script+`"`) && !strings.Contains(guestHTML, `src="js/`+script+`?`) {
			t.Errorf("reference guest is missing script %q", script)
		}
	}
}
