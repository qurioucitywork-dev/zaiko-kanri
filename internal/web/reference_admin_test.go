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
		"app.html":               "abb24d44895b9db66b2e57bfc2d0ef0039820c8056f048585d8f8310e47856d5",
		"guest.html":             "258ee73e431791e173c29cb73bde7eed7f9d7a3b8d8832d1a11c7574ef1438c3",
		"css/guest.css":          "cc948cadc00ed1421402448277ca64e98319e750cbe0e0c15b7bee416641ee34",
		"css/market-table.css":   "7fd79f92a5b5a3a42556f862ada62ae8adb79d24e6baa158b1cd668439f3c97e",
		"css/style.css":          "cd38ffd8be492d021b1e65eba05cc9ab2083ccf3598c3d41b13d56d0754237ea",
		"index.html":             "fbdd4e26f97c55024b6dd55fe5a4674bb86e297c9353cec197a034a2bfba8112",
		"js/api_bridge.js":       "d472feb6a96c3dac2b2f24d797c15b5e2f770ae9b5ae98c363b9771c92465059",
		"js/app.js":              "072f21340b9769f2d19a9b3882adb85ba6b65d01432d50cb793f992def75e200",
		"js/approval.js":         "8dbab17a55bdedd9648a49186899b7d46b23df48dc173bc54adc9e2704cc2808",
		"js/auth.js":             "a9f0cc7a928b801342241f4740c055f4f34eb208ac6ed61e25ef82191ad97c50",
		"js/box.js":              "4ae711817115c0d62cdaa99e11920acc5ec67a0fa4d2e1879488368235840575",
		"js/data.js":             "208e865273ec974d02c4e4e77e564489a79265128fd5f4394ed6933784f0dabc",
		"js/guest.js":            "174dc03d5c978e514e5a674b9967ecf6b2d0caf2d0cc8b8fdbd7305687aa2401",
		"js/guest_shared.js":     "da6589099b995f457d44e2b6eeb05ac07d8ad1c7bef937f4230dd8afd2ca38e4",
		"js/login_info.js":       "2fe90eac8cc8f1718828e95d34b2ea4f180cc5eea474348ef0d0c4796d38baad",
		"js/market_table.js":     "e7e899605f42b05c8e06c63e90c83a8aeab6318d8f8a06e88077c219b5dc8374",
		"js/notify.js":           "b18e2604d9c637729a339e7b0d2a9b6bb0fac85f5f106c24d9e772263e16e878",
		"js/purchase_entry.js":   "757226a682a3d0985b06b5c954435d7dcc8116a05bb4e97f11cb8cf6af76b13a",
		"js/stocktake.js":        "b8a460a73319c08717e46f88d7bbefff06cfa62103afa186250773753adc60e3",
		"js/consignment.js":      "c37e2b99b920f08d1b6010bc8e06f1145a8c530366fdda9740cfa9fb3b7c7bb5",
		"js/qrcode-generator.js": "18ae399f81182bc9de916e9c77b195df20cc58d6f2d55a62b085a299f1bf1780",
		"js/jsQR.js":             "bc40c8a15196236b2314db0856f72ca0b49980cd5413b8c852a7349f5fee0859",
		"js/qr_inventory.js":     "fec7d97629f2fbb2f5ea18b5c836eedcb4cd1d357d0b579619800216fe158020",
	}

	for name, want := range expected {
		content, err := reactAssets.ReadFile("react-dist/admin-reference/" + name)
		if err != nil {
			t.Fatalf("read reference asset %s: %v", name, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(content)); got != want {
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
		"sales-list", "shipping", "consignment", "returns", "master", "box", "performance",
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
	if !strings.Contains(html, `> 仕入登録テンプレート`) {
		t.Error("reference admin is missing the purchase-registration template label")
	}
	for _, marker := range []string{`class="card-body pe-slip-list-scroll"`, `class="data-table pe-slip-list-table"`, `仕入小計 / 仕入合計`} {
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
		`'仕入登録テンプレート.csv'`,
		`taxInfo.mode === PE_PURCHASE_TAX_DOMESTIC ? '1' : '2'`,
		`normalized === '1'`,
		`normalized === '2'`,
		`仕入小計（税抜）`,
		`仕入合計（税込）`,
		`仕入合計（対象外）`,
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
	if got := strings.Count(html, `class="modal-overlay`); got != 46 {
		t.Errorf("reference admin modal count=%d, want 46", got)
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
