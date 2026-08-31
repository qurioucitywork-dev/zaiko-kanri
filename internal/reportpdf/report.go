// Package reportpdf creates deterministic A4 accounting documents without
// relying on a browser or printer driver. Japanese text uses the standard
// Adobe-Japan1 CID font mapping supported by PDF readers.
package reportpdf

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

type Line struct {
	Number      int
	Description string
	Amount      string
	Tax         string
	ProductCode string
}

type Document struct {
	Title           string
	Number          string
	TransactionDate string
	IssuedAt        string
	PartnerLabel    string
	PartnerName     string
	CompanyName     string
	CompanyAddress  string
	CompanyPhone    string
	CompanyInvoice  string
	Currency        string
	Subtotal        string
	TaxAmount       string
	Total           string
	TaxLabel        string
	Notes           string
	Lines           []Line
}

type page struct{ commands strings.Builder }

// Render creates an immutable, fixed-layout PDF byte stream.
func Render(document Document) ([]byte, error) {
	if strings.TrimSpace(document.Title) == "" || strings.TrimSpace(document.Number) == "" {
		return nil, fmt.Errorf("document title and number are required")
	}
	pages := buildPages(document)
	return writePDF(pages), nil
}

func buildPages(document Document) []page {
	const rowsPerPage = 13
	linePages := (len(document.Lines) + rowsPerPage - 1) / rowsPerPage
	if linePages < 1 {
		linePages = 1
	}
	pages := make([]page, 0, linePages)
	for pageIndex := 0; pageIndex < linePages; pageIndex++ {
		var current page
		text(&current, 36, 792, 22, document.Title)
		text(&current, 390, 797, 9, "伝票No.")
		text(&current, 455, 797, 10, document.Number)
		text(&current, 390, 778, 9, "発行日時")
		text(&current, 455, 778, 9, document.IssuedAt)
		text(&current, 390, 759, 9, "取引日")
		text(&current, 455, 759, 9, document.TransactionDate)
		line(&current, 36, 735, 559, 735)
		text(&current, 36, 714, 8, document.PartnerLabel)
		text(&current, 36, 692, 15, document.PartnerName+" 御中")
		text(&current, 350, 714, 12, document.CompanyName)
		text(&current, 350, 695, 8, document.CompanyAddress)
		text(&current, 350, 680, 8, "TEL: "+document.CompanyPhone)
		text(&current, 350, 665, 8, "登録番号: "+document.CompanyInvoice)

		text(&current, 36, 635, 8, "表示通貨: "+document.Currency)
		tableY := 610.0
		fillRect(&current, 36, tableY-22, 523, 22, 0.88)
		for _, x := range []float64{36, 70, 360, 445, 559} {
			line(&current, x, tableY, x, tableY-22-float64(rowsPerPage*31))
		}
		line(&current, 36, tableY, 559, tableY)
		text(&current, 44, tableY-15, 8, "No.")
		text(&current, 82, tableY-15, 8, "摘要")
		text(&current, 372, tableY-15, 8, "金額")
		text(&current, 457, tableY-15, 8, "税区分")
		start := pageIndex * rowsPerPage
		end := start + rowsPerPage
		if end > len(document.Lines) {
			end = len(document.Lines)
		}
		for row := 0; row < rowsPerPage; row++ {
			y := tableY - 22 - float64(row*31)
			line(&current, 36, y, 559, y)
			if start+row >= end {
				continue
			}
			item := document.Lines[start+row]
			text(&current, 49, y-20, 8, strconv.Itoa(item.Number))
			text(&current, 78, y-13, 8, truncate(item.Description, 42))
			text(&current, 78, y-24, 7, "管理番号: "+item.ProductCode)
			text(&current, 370, y-19, 8, item.Amount)
			text(&current, 454, y-19, 7, item.Tax)
		}
		bottom := tableY - 22 - float64(rowsPerPage*31)
		line(&current, 36, bottom, 559, bottom)
		if pageIndex == linePages-1 {
			text(&current, 330, bottom-25, 9, "小計")
			text(&current, 440, bottom-25, 10, document.Subtotal)
			text(&current, 330, bottom-45, 9, "税額")
			text(&current, 440, bottom-45, 10, document.TaxAmount)
			line(&current, 330, bottom-54, 559, bottom-54)
			text(&current, 330, bottom-76, 11, "合計")
			text(&current, 440, bottom-76, 12, document.Total)
			text(&current, 36, bottom-102, 8, "税区分: "+document.TaxLabel)
			text(&current, 36, bottom-120, 8, "備考: "+truncate(document.Notes, 70))
		}
		text(&current, 520, 24, 7, fmt.Sprintf("%d / %d", pageIndex+1, linePages))
		pages = append(pages, current)
	}
	return pages
}

func text(p *page, x, y, size float64, value string) {
	if strings.TrimSpace(value) == "" {
		value = "-"
	}
	fmt.Fprintf(&p.commands, "BT /F1 %.1f Tf %.1f %.1f Td <%s> Tj ET\n", size, x, y, utf16Hex(value))
}

func line(p *page, x1, y1, x2, y2 float64) {
	fmt.Fprintf(&p.commands, "0.35 w %.1f %.1f m %.1f %.1f l S\n", x1, y1, x2, y2)
}

func fillRect(p *page, x, y, width, height, gray float64) {
	fmt.Fprintf(&p.commands, "%.2f g %.1f %.1f %.1f %.1f re f 0 g\n", gray, x, y, width, height)
}

func utf16Hex(value string) string {
	units := utf16.Encode([]rune(value))
	var out strings.Builder
	for _, unit := range units {
		fmt.Fprintf(&out, "%04X", unit)
	}
	return out.String()
}

func truncate(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max-1]) + "…"
}

func writePDF(pages []page) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"", // pages tree is populated after page objects are known.
		"<< /Type /Font /Subtype /Type0 /BaseFont /HeiseiKakuGo-W5 /Encoding /UniJIS-UTF16-H /DescendantFonts [4 0 R] >>",
		"<< /Type /Font /Subtype /CIDFontType0 /BaseFont /HeiseiKakuGo-W5 /CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 5 >> >>",
	}
	pageRefs := make([]string, 0, len(pages))
	for _, item := range pages {
		content := item.commands.String()
		contentID := len(objects) + 1
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content))
		pageID := len(objects) + 1
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", contentID))
		pageRefs = append(pageRefs, fmt.Sprintf("%d 0 R", pageID))
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pageRefs, " "), len(pageRefs))

	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index <= len(objects); index++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}
