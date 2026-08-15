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
		"app.html":               "8df299505f1878bdfe8a90495be678ad2f59aaf26a106e66a521461ccf391007",
		"guest.html":             "258ee73e431791e173c29cb73bde7eed7f9d7a3b8d8832d1a11c7574ef1438c3",
		"css/guest.css":          "cc948cadc00ed1421402448277ca64e98319e750cbe0e0c15b7bee416641ee34",
		"css/market-table.css":   "7fd79f92a5b5a3a42556f862ada62ae8adb79d24e6baa158b1cd668439f3c97e",
		"css/style.css":          "540bb23ccd3410ef13a9295df7bcd1c11e61ed31bff43c223455578a5603fc4a",
		"index.html":             "fbdd4e26f97c55024b6dd55fe5a4674bb86e297c9353cec197a034a2bfba8112",
		"js/api_bridge.js":       "3f5e22f8f859bc1d12a424c951d87565214e098f5ccaffca9ff332bb33b46e36",
		"js/app.js":              "de985f6effc5753fcd1d8396e4a2401df71bebaf87ca31d3bb482b9cae076a28",
		"js/approval.js":         "23b78eb379f3b380ed9c9d2a4b563a3e0d0abbf199bd31941d7dac539bd5b0a0",
		"js/auth.js":             "a9f0cc7a928b801342241f4740c055f4f34eb208ac6ed61e25ef82191ad97c50",
		"js/box.js":              "4d2e41d7ea036881d4e2566a90e8663c8ccf1b2bb656f4817d12c1d6447140d7",
		"js/data.js":             "4fb3bf6c708542c1fab86d1be871247a09551ab0f0f9ad0c2d2a4757cc873a31",
		"js/guest.js":            "174dc03d5c978e514e5a674b9967ecf6b2d0caf2d0cc8b8fdbd7305687aa2401",
		"js/guest_shared.js":     "da6589099b995f457d44e2b6eeb05ac07d8ad1c7bef937f4230dd8afd2ca38e4",
		"js/login_info.js":       "2fe90eac8cc8f1718828e95d34b2ea4f180cc5eea474348ef0d0c4796d38baad",
		"js/market_table.js":     "e7e899605f42b05c8e06c63e90c83a8aeab6318d8f8a06e88077c219b5dc8374",
		"js/notify.js":           "b18e2604d9c637729a339e7b0d2a9b6bb0fac85f5f106c24d9e772263e16e878",
		"js/purchase_entry.js":   "4d44be2041849dfaf96c3875da1edb72fe34d728954f94eaf5e1033f4c075ff0",
		"js/stocktake.js":        "b07ad147b3ab5c37422f897431059b6543859b61bbc4337305f5a4c9277fa98b",
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
