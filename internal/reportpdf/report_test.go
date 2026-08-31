package reportpdf

import (
	"bytes"
	"testing"
)

func TestRenderCreatesFixedLayoutPDF(t *testing.T) {
	document := Document{
		Title: "仕入伝票", Number: "PI-2026-0001", TransactionDate: "2026-08-16",
		IssuedAt: "2026-08-16 12:34:56 JST", PartnerLabel: "仕入先", PartnerName: "田中商事",
		CompanyName: "株式会社ウォッチプレミアム", Currency: "JPY", Subtotal: "¥100,000",
		TaxAmount: "¥10,000", Total: "¥110,000", TaxLabel: "消費税（10%）",
		Lines: []Line{{Number: 1, Description: "ロレックス / サブマリーナ", Amount: "¥100,000", Tax: "消費税(10%)", ProductCode: "1608260001"}},
	}
	contents, err := Render(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range [][]byte{[]byte("%PDF-1.4"), []byte("/MediaBox [0 0 595 842]"), []byte("/UniJIS-UTF16-H"), []byte("xref"), []byte("%%EOF")} {
		if !bytes.Contains(contents, token) {
			t.Fatalf("generated PDF does not contain %q", token)
		}
	}
	if len(contents) < 1000 {
		t.Fatalf("generated PDF is unexpectedly small: %d bytes", len(contents))
	}
}

func TestRenderPaginatesLongDocuments(t *testing.T) {
	lines := make([]Line, 30)
	for index := range lines {
		lines[index] = Line{Number: index + 1, Description: "商品", Amount: "¥1,000", Tax: "対象外"}
	}
	contents, err := Render(Document{Title: "委託伝票", Number: "CO-2026-0001", Lines: lines})
	if err != nil {
		t.Fatal(err)
	}
	if count := bytes.Count(contents, []byte("/Type /Page ")); count != 3 {
		t.Fatalf("page count = %d, want 3", count)
	}
}
