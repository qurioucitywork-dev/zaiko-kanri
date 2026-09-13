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
		"app.html":                    "76bb1f0e4e688a7acd677d58459d5a590954fd7f0fc65c23e70c8a7984f6dbd4",
		"guest.html":                  "258ee73e431791e173c29cb73bde7eed7f9d7a3b8d8832d1a11c7574ef1438c3",
		"css/guest.css":               "cc948cadc00ed1421402448277ca64e98319e750cbe0e0c15b7bee416641ee34",
		"css/market-table.css":        "1fbd958c84da4b6cff7c648fb67166f158c6bcfe0f2f587e84860ba7d72b94d5",
		"css/style.css":               "b139bb087a4de427bdeb96acae6685f3aceea40c02070eaa2a26474257b27882",
		"index.html":                  "fbdd4e26f97c55024b6dd55fe5a4674bb86e297c9353cec197a034a2bfba8112",
		"js/api_bridge.js":            "099faf1095acb8b56458f70961df416c072a634ef1adb751b7b3dcd991bd5287",
		"js/app.js":                   "8c9ed8abba1b57aacc22758893dd047d15a1f01e237f32e9c3f94756a623e722",
		"js/approval.js":              "43de68681b060a67bf00af6eb2a993e01d4acbcff55a1928c795cb4f0f031c1d",
		"js/auth.js":                  "c0c8b6449dd1e94e9d02b63e4878303632d85eb442512ecbc696c0c0980fadad",
		"js/box.js":                   "4ae711817115c0d62cdaa99e11920acc5ec67a0fa4d2e1879488368235840575",
		"js/cost_adjustment_entry.js": "5c6adabbd0666027405363e53ed820279521cf18fc9604137a194824f3bdea2a",
		"js/cost_merge.js":            "4959f15005deb576af3bc237cd8ad1bc9286c3bd528388cae78c577b6aa37329",
		"js/data.js":                  "f52e548cef5aaf6a094ad15dbb47c0e4f278c76ec2108d21400c5270bd8354af",
		"js/guest.js":                 "174dc03d5c978e514e5a674b9967ecf6b2d0caf2d0cc8b8fdbd7305687aa2401",
		"js/guest_shared.js":          "da6589099b995f457d44e2b6eeb05ac07d8ad1c7bef937f4230dd8afd2ca38e4",
		"js/login_info.js":            "3331838630b5a5f539de8a1e842d8bd75a0202692a865e3aa5cdf04bc8b0a77f",
		"js/market_table.js":          "3ac2eac43a73cdaccff3267651dfd426b05c11b13aa6c87e4be9d6fb51f23be3",
		"js/notify.js":                "edae1bc1187f3f29f87086018425aec3f628c5d342a0d8a3d22f854948cb6321",
		"js/purchase_entry.js":        "440de10980b48d6f35df976036a5b6705d2483cf1f6f11e5a8ed143f39e9627c",
		"js/stocktake.js":             "b7592fb37a246ebb424c0b8b81516d928c7c2f2e0c0def3a9228a45750cfb992",
		"js/consignment.js":           "6b8da797cba3fdf6a593cf9e26835576407fc5c8c1b2db553fd721776843bee4",
		"js/qrcode-generator.js":      "18ae399f81182bc9de916e9c77b195df20cc58d6f2d55a62b085a299f1bf1780",
		"js/jsQR.js":                  "bc40c8a15196236b2314db0856f72ca0b49980cd5413b8c852a7349f5fee0859",
		"js/qr_inventory.js":          "e9c87a9ab707bfa793a3c1907e34c2296f60fcf68713f06ce7cdbd5ed867bae0",
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
		"consignment.js", "stocktake.js", "purchase_entry.js", "cost_merge.js", "cost_adjustment_entry.js",
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
	for _, marker := range []string{`class="card-body pe-slip-list-scroll"`, `class="data-table pe-slip-list-table"`, `原価小計 / 原価合計`, `入金日付`} {
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
	if got := strings.Count(html, `class="modal-overlay`); got != 50 {
		t.Errorf("reference admin modal count=%d, want 50", got)
	}

	guestContent, err := reactAssets.ReadFile("react-dist/admin-reference/guest.html")
	if err != nil {
		t.Fatal(err)
	}
	guestHTML := string(guestContent)
	for _, feature := range []string{"guest-product-grid", "guest-box-filter", "guest-cart-overlay", "guest-submit-request"} {
		if !strings.Contains(guestHTML, feature) {
			t.Errorf("reference guest is missing feature %q", feature)
		}
	}
	for _, script := range []string{"data.js", "guest_shared.js", "login_info.js", "guest.js"} {
		if !strings.Contains(guestHTML, `src="js/`+script+`"`) {
			t.Errorf("reference guest is missing script %q", script)
		}
	}
}
