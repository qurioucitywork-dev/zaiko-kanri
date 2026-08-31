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
		"app.html":               "2b60e841c16b00162639042658300c67caffae7fe35d70bb1ce0f53454c12205",
		"guest.html":             "258ee73e431791e173c29cb73bde7eed7f9d7a3b8d8832d1a11c7574ef1438c3",
		"css/guest.css":          "cc948cadc00ed1421402448277ca64e98319e750cbe0e0c15b7bee416641ee34",
		"css/market-table.css":   "1fbd958c84da4b6cff7c648fb67166f158c6bcfe0f2f587e84860ba7d72b94d5",
		"css/style.css":          "4b3d382d9893ba60cf59f944f9882dbfb682a91ddca3d8578d1ff1609ff7fc9c",
		"index.html":             "fbdd4e26f97c55024b6dd55fe5a4674bb86e297c9353cec197a034a2bfba8112",
		"js/api_bridge.js":       "82f698a9f47853f77746484f5c5795c6012e4f471c6c8e3cd8cfd99b3034c98a",
		"js/app.js":              "6f4c897242f632cc63673dad7dd59ef09294e40f9adf43f265393023198cbf40",
		"js/approval.js":         "43de68681b060a67bf00af6eb2a993e01d4acbcff55a1928c795cb4f0f031c1d",
		"js/auth.js":             "8a37d5385ade35fba91ccd0fb2fa9acc45b5828b1b3af4c86f9d0406758ee694",
		"js/box.js":              "4ae711817115c0d62cdaa99e11920acc5ec67a0fa4d2e1879488368235840575",
		"js/data.js":             "48546c83100a61f98635e99c1f3fbabdf67ac1c3e2a5166074666a89399b2953",
		"js/guest.js":            "174dc03d5c978e514e5a674b9967ecf6b2d0caf2d0cc8b8fdbd7305687aa2401",
		"js/guest_shared.js":     "da6589099b995f457d44e2b6eeb05ac07d8ad1c7bef937f4230dd8afd2ca38e4",
		"js/login_info.js":       "3331838630b5a5f539de8a1e842d8bd75a0202692a865e3aa5cdf04bc8b0a77f",
		"js/market_table.js":     "3ac2eac43a73cdaccff3267651dfd426b05c11b13aa6c87e4be9d6fb51f23be3",
		"js/notify.js":           "edae1bc1187f3f29f87086018425aec3f628c5d342a0d8a3d22f854948cb6321",
		"js/purchase_entry.js":   "440de10980b48d6f35df976036a5b6705d2483cf1f6f11e5a8ed143f39e9627c",
		"js/stocktake.js":        "fd7b6cf4ae52ba64ed5c08839098551e191737a93a1c7be1681fc366142ee8d1",
		"js/consignment.js":      "6b8da797cba3fdf6a593cf9e26835576407fc5c8c1b2db553fd721776843bee4",
		"js/qrcode-generator.js": "18ae399f81182bc9de916e9c77b195df20cc58d6f2d55a62b085a299f1bf1780",
		"js/jsQR.js":             "bc40c8a15196236b2314db0856f72ca0b49980cd5413b8c852a7349f5fee0859",
		"js/qr_inventory.js":     "e9c87a9ab707bfa793a3c1907e34c2296f60fcf68713f06ce7cdbd5ed867bae0",
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
		"dashboard", "market", "market-entry", "inventory", "parts-management", "purchase-entry", "purchase", "cost-adjustment", "sales",
		"sales-list", "deleted-slips", "shipping", "consignment", "master", "box", "performance",
		"stocktake", "approval", "purchase-list", "client", "company", "password",
	}
	for _, page := range pages {
		if !strings.Contains(html, `id="page-`+page+`"`) {
			t.Errorf("reference admin is missing page %q", page)
		}
	}
	scripts := []string{
		"qrcode-generator.js", "jsQR.js", "data.js", "guest_shared.js", "login_info.js", "auth.js", "api_bridge.js", "approval.js", "notify.js", "box.js", "market_table.js", "qr_inventory.js", "app.js",
		"consignment.js", "stocktake.js", "purchase_entry.js",
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
	for _, marker := range []string{`id="inventoryNavGroup"`, `>在庫管理<`, `data-page="inventory"`, `data-page="parts-management"`, `id="page-parts-management"`, `> 商品管理`, `> パーツ管理`, `data-inv-col="purchaseRate"`, `data-inv-col="purchasePriceAtPurchaseRate"`, `原価（現在レート）`, `data-inv-col="grossMargin"`, `粗利率`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing inventory rate-comparison marker %q", marker)
		}
	}
	if !strings.Contains(html, `data-page="consignment"`) || !strings.Contains(html, `</span> 委託登録`) {
		t.Error("reference admin is missing the shortened consignment registration label")
	}
	if strings.Contains(html, "委託伝票登録") {
		t.Error("reference admin still contains the former consignment registration label")
	}
	if strings.Contains(html, `data-ca-mode="swap"`) || strings.Contains(html, `> 入替`) {
		t.Error("reference admin still contains the removed cost-adjustment swap mode")
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
		`崩し済み`,
		`原価調整中`,
		`['combined', '結合済み']`,
		`'consignment': '委託登録'`,
	} {
		if !strings.Contains(appJS, marker) {
			t.Errorf("inventory status label contract is missing %q", marker)
		}
	}
	if strings.Contains(appJS, `{ key: 'dial'`) || strings.Contains(appJS, `case 'dial':`) {
		t.Error("master management must not expose the dial master")
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
