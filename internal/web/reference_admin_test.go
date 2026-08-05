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
		"app.html":               "aac705bf08dffc8b3f126f9c5cb99e018ac15c18127da181ec8d6f4cc997fdd4",
		"guest.html":             "258ee73e431791e173c29cb73bde7eed7f9d7a3b8d8832d1a11c7574ef1438c3",
		"css/guest.css":          "cc948cadc00ed1421402448277ca64e98319e750cbe0e0c15b7bee416641ee34",
		"css/market-table.css":   "976ee6e103fdb492cec7ec93f79f52d924f47faa8c1a6464c0e8ff839f8c0b0a",
		"css/style.css":          "ecc4c6d3d3af5cfbcaf4e26c05c1cba0a83e92c9e1bae9d39f34d3315af7d29c",
		"index.html":             "fbdd4e26f97c55024b6dd55fe5a4674bb86e297c9353cec197a034a2bfba8112",
		"js/api_bridge.js":       "fa60cfed51729cc5f3b4b0670bd96e1fba1a9ab115910c784bb016af09957a82",
		"js/app.js":              "91a996c46b3bbfdeda3bf6a6ce8424398f167b790b1d1cefe95bff9f1f969a13",
		"js/approval.js":         "3df4953ce3eb574f6f83719b66be065201abbbe5e52becd918e82774c16b508e",
		"js/auth.js":             "fcda27236b7d98f3823b63effbd773be73fa8cc76c0e5c631f38a4b6e1b5000d",
		"js/box.js":              "4d2e41d7ea036881d4e2566a90e8663c8ccf1b2bb656f4817d12c1d6447140d7",
		"js/data.js":             "59d45a413f5f841665500a405a0a67963421b11c867c0b39a71cc68e7454c5bd",
		"js/guest.js":            "174dc03d5c978e514e5a674b9967ecf6b2d0caf2d0cc8b8fdbd7305687aa2401",
		"js/guest_shared.js":     "da6589099b995f457d44e2b6eeb05ac07d8ad1c7bef937f4230dd8afd2ca38e4",
		"js/login_info.js":       "2fe90eac8cc8f1718828e95d34b2ea4f180cc5eea474348ef0d0c4796d38baad",
		"js/market_table.js":     "47fbff9ee0c8f774b7f50e6be981fe05bcce2965185c69122f96ebad18a41ef3",
		"js/notify.js":           "b18e2604d9c637729a339e7b0d2a9b6bb0fac85f5f106c24d9e772263e16e878",
		"js/purchase_entry.js":   "5ce9eb4fd8abab62124743054c96992c4fb2b645477075610b20705f1e174efd",
		"js/stocktake.js":        "4e4e401fb8bd06a2b169aa425ad4e90138221901fe9e7d892c4bcc2175705e72",
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
		"sales-list", "shipping", "returns", "master", "box", "performance",
		"stocktake", "approval", "purchase-list", "client", "company", "password",
	}
	for _, page := range pages {
		if !strings.Contains(html, `id="page-`+page+`"`) {
			t.Errorf("reference admin is missing page %q", page)
		}
	}
	scripts := []string{
		"qrcode-generator.js", "jsQR.js", "data.js", "guest_shared.js", "login_info.js", "auth.js", "api_bridge.js", "approval.js", "notify.js", "box.js", "market_table.js", "qr_inventory.js", "app.js",
		"stocktake.js", "purchase_entry.js",
	}
	for _, script := range scripts {
		if !strings.Contains(html, `src="js/`+script) {
			t.Errorf("reference admin is missing script %q", script)
		}
	}
	if got := strings.Count(html, `class="modal-overlay`); got != 45 {
		t.Errorf("reference admin modal count=%d, want 45", got)
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
