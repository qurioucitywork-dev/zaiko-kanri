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
		"app.html":               "89a8ce96a1b1458c58414b4b0f9beaa3ce99849f73709478cd511f5046a0a1ca",
		"guest.html":             "258ee73e431791e173c29cb73bde7eed7f9d7a3b8d8832d1a11c7574ef1438c3",
		"css/guest.css":          "cc948cadc00ed1421402448277ca64e98319e750cbe0e0c15b7bee416641ee34",
		"css/market-table.css":   "1bf42fc24cd9ac5d84fc2584942d5c8863a727cddf5796f47c7f92f514f377e2",
		"css/style.css":          "c09a28c46ff7771252dc3f14645c520ab4ec236d5e87a8e78eb3987f9ceee129",
		"index.html":             "fbdd4e26f97c55024b6dd55fe5a4674bb86e297c9353cec197a034a2bfba8112",
		"js/api_bridge.js":       "fc02d9dd4c5d56b10488cc5fd36a56d72384995e80018c317cf2005c47c0027d",
		"js/app.js":              "cc3d48b472c924ec1fd06b5633e1182c4ba4d1c59526b58fc9fdb40e15308f6d",
		"js/approval.js":         "43de68681b060a67bf00af6eb2a993e01d4acbcff55a1928c795cb4f0f031c1d",
		"js/auth.js":             "a9f0cc7a928b801342241f4740c055f4f34eb208ac6ed61e25ef82191ad97c50",
		"js/box.js":              "4ae711817115c0d62cdaa99e11920acc5ec67a0fa4d2e1879488368235840575",
		"js/data.js":             "5b161fb936913295f78e031b317ce0b510375eb4215116d449a0ef1982e9a432",
		"js/guest.js":            "174dc03d5c978e514e5a674b9967ecf6b2d0caf2d0cc8b8fdbd7305687aa2401",
		"js/guest_shared.js":     "da6589099b995f457d44e2b6eeb05ac07d8ad1c7bef937f4230dd8afd2ca38e4",
		"js/login_info.js":       "3331838630b5a5f539de8a1e842d8bd75a0202692a865e3aa5cdf04bc8b0a77f",
		"js/market_table.js":     "8b90c10b5a26854ce3cdd1cc82cd8956dbdf847d7b5a11f77cf9891e4c77af51",
		"js/notify.js":           "edae1bc1187f3f29f87086018425aec3f628c5d342a0d8a3d22f854948cb6321",
		"js/purchase_entry.js":   "044141a561c60e3f0a02703c9deebe709008afc05cab9e5b729aa32e7e22f90e",
		"js/stocktake.js":        "a5002932c496174ea0f8f0b509a50d52ab9561242c28c5fcb584990b1d185763",
		"js/consignment.js":      "b5e54c342e0bb61a9c30ee0851b01101714eba9e194f414ed01a67b39f653586",
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
		"dashboard", "market", "inventory", "purchase-entry", "purchase", "sales",
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
	for _, marker := range []string{`class="card-body pe-slip-list-scroll"`, `class="data-table pe-slip-list-table"`, `原価小計 / 原価合計`, `入金日付`} {
		if !strings.Contains(html, marker) {
			t.Errorf("reference admin is missing purchase-slip list layout marker %q", marker)
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
	for _, marker := range []string{
		`['return_pending', '仕入返品', '仕入返品中']`,
		`['cancelled', '取消', '取消済', '取り消し', '仕入返品済']`,
		`normalizeInventoryCollectionStatuses();`,
	} {
		if !strings.Contains(appJS, marker) {
			t.Errorf("inventory status label contract is missing %q", marker)
		}
	}
	if strings.Contains(appJS, `{ key: 'dial'`) || strings.Contains(appJS, `case 'dial':`) {
		t.Error("master management must not expose the dial master")
	}
	if got := strings.Count(html, `class="modal-overlay`); got != 47 {
		t.Errorf("reference admin modal count=%d, want 47", got)
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
