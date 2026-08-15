package persistence

import "testing"

func TestNormalizeMarketHeadersSupportsJapaneseLabels(t *testing.T) {
	indexes, err := normalizeMarketHeaders([]string{
		"取込日付", "ブランド", "モデル番号", "リファレンス番号", "コンディション",
		"仕入価格", "仕入通貨", "相場価格", "相場通貨", "オークションコード", "備考",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range requiredMarketHeaders {
		if _, ok := indexes[required]; !ok {
			t.Fatalf("missing normalized header %s", required)
		}
	}
}

func TestNormalizeMarketHeadersSupportsAuctionTemplate(t *testing.T) {
	indexes, err := normalizeMarketHeaders([]string{
		"オークション開催日", "ブランド", "モデル名", "型番", "コンディション",
		"オークションコード", "落札価格（JPY）", "SKU", "素材", "駆動方式", "付属品", "備考",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"import_date", "brand_text", "market_price", "auction_code", "notes", "sku", "material_text", "movement_text", "accessory_text"} {
		if _, ok := indexes[key]; !ok {
			t.Fatalf("missing normalized auction header %s", key)
		}
	}
}

func TestSplitMarketCodes(t *testing.T) {
	actual := splitMarketCodes("BOX・guarantee, BOX")
	if len(actual) != 2 || actual[0] != "BOX" || actual[1] != "GUARANTEE" {
		t.Fatalf("unexpected codes: %#v", actual)
	}
}

func TestParseImportAmount(t *testing.T) {
	for input, expected := range map[string]int64{"￥1,250,000": 1250000, "$7,613": 7613, "": 0} {
		actual, err := parseImportAmount(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("parse %q: got %d, want %d", input, actual, expected)
		}
	}
}
