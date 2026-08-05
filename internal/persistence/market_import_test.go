package persistence

import "testing"

func TestNormalizeMarketHeadersSupportsJapaneseLabels(t *testing.T) {
	indexes, err := normalizeMarketHeaders([]string{
		"取込日付", "ブランドコード", "モデル番号", "リファレンス番号", "コンディションコード",
		"仕入価格", "仕入通貨", "相場価格", "相場通貨", "取得元", "備考",
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
