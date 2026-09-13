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
		"市場調査日", "市場区分", "ブランド", "モデル名", "型番", "コンディション",
		"保証年月", "オークションコード", "取引価格", "取引通貨", "市場調査レート", "SKU", "素材", "駆動方式", "付属品", "コマ数", "備考",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"import_date", "market_category", "brand_text", "market_price", "market_currency", "market_fx_rate", "auction_code", "notes", "sku", "material_text", "movement_text", "accessory_text", "bracelet_quantity", "warranty_year_month"} {
		if _, ok := indexes[key]; !ok {
			t.Fatalf("missing normalized auction header %s", key)
		}
	}
}

func TestNormalizeMarketCategory(t *testing.T) {
	for input, expected := range map[string]string{
		"": "domestic-auction", "国内オークション": "domestic-auction",
		"overseas": "overseas", "国内小売": "domestic-retail",
	} {
		actual, err := normalizeMarketCategory(input)
		if err != nil || actual != expected {
			t.Fatalf("normalize %q: got %q, %v; want %q", input, actual, err, expected)
		}
	}
	if _, err := normalizeMarketCategory("unknown"); err == nil {
		t.Fatal("unknown market category must be rejected")
	}
}

func TestNormalizeMarketWarrantyYearMonth(t *testing.T) {
	for input, expected := range map[string]string{"2026-08": "2026-08", "2026/8": "2026-08", "2026年8月": "2026-08", "": ""} {
		actual, err := normalizeMarketWarrantyYearMonth(input)
		if err != nil || actual != expected {
			t.Fatalf("normalize %q: got %q, %v; want %q", input, actual, err, expected)
		}
	}
	if _, err := normalizeMarketWarrantyYearMonth("2026-13"); err == nil {
		t.Fatal("invalid warranty month must be rejected")
	}
}

func TestMarketDuplicateKeyUsesConditionAccessoriesAndWarranty(t *testing.T) {
	braceletQuantity := 8
	otherBraceletQuantity := 7
	base := marketDuplicateKey("2026-08-29", "Rolex", "Submariner", "126610LN", "CON-001", "BOX・GUARANTEE・BRACELET PARTS", &braceletQuantity, "2026-08", "AUC-01")
	if reordered := marketDuplicateKey("2026-08-29", " rolex ", "Submariner", "126610ln", "con-001", "bracelet parts,guarantee,box", &braceletQuantity, "2026/8", "auc-01"); reordered != base {
		t.Fatal("case, whitespace, accessory order and warranty formatting must not create a distinct duplicate identity")
	}
	cases := map[string]string{
		"condition":        marketDuplicateKey("2026-08-29", "Rolex", "Submariner", "126610LN", "CON-002", "BOX・GUARANTEE・BRACELET PARTS", &braceletQuantity, "2026-08", "AUC-01"),
		"accessory":        marketDuplicateKey("2026-08-29", "Rolex", "Submariner", "126610LN", "CON-001", "BOX", nil, "2026-08", "AUC-01"),
		"braceletQuantity": marketDuplicateKey("2026-08-29", "Rolex", "Submariner", "126610LN", "CON-001", "BOX・GUARANTEE・BRACELET PARTS", &otherBraceletQuantity, "2026-08", "AUC-01"),
		"warranty":         marketDuplicateKey("2026-08-29", "Rolex", "Submariner", "126610LN", "CON-001", "BOX・GUARANTEE・BRACELET PARTS", &braceletQuantity, "2026-09", "AUC-01"),
		"model":            marketDuplicateKey("2026-08-29", "Rolex", "Sea-Dweller", "126610LN", "CON-001", "BOX・GUARANTEE・BRACELET PARTS", &braceletQuantity, "2026-08", "AUC-01"),
		"reference":        marketDuplicateKey("2026-08-29", "Rolex", "Submariner", "126610LV", "CON-001", "BOX・GUARANTEE・BRACELET PARTS", &braceletQuantity, "2026-08", "AUC-01"),
	}
	for dimension, key := range cases {
		if key == base {
			t.Fatalf("different %s must not be treated as a duplicate", dimension)
		}
	}
}

func TestParseMarketBraceletQuantity(t *testing.T) {
	quantity, err := parseMarketBraceletQuantity("8")
	if err != nil || quantity == nil || *quantity != 8 {
		t.Fatalf("parse bracelet quantity: got %#v, %v", quantity, err)
	}
	if quantity, err = parseMarketBraceletQuantity(""); err != nil || quantity != nil {
		t.Fatalf("blank bracelet quantity: got %#v, %v", quantity, err)
	}
	if _, err = parseMarketBraceletQuantity("-1"); err == nil {
		t.Fatal("negative bracelet quantity must be rejected")
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

func TestAuctionHouseIsSupportedByCatalogLookup(t *testing.T) {
	if !isCatalogLookupTable("auction_houses") {
		t.Fatal("auction_houses must be supported by market CSV catalog lookup")
	}
}
