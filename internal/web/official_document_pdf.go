package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/reportpdf"
)

func (s *Server) storeOfficialPDF(r *http.Request, documentType, documentID, documentNumber string, document reportpdf.Document, snapshot any) (*persistence.OfficialDocumentRef, error) {
	user, _ := currentUser(r.Context())
	company, err := s.repository.CompanyInfo(r.Context(), user.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("load company info: %w", err)
	}
	document.CompanyName = company.CompanyName
	document.CompanyAddress = strings.TrimSpace(strings.Join([]string{company.PostalCode, company.Address}, " "))
	document.CompanyPhone = company.Phone
	document.CompanyInvoice = company.InvoiceNumber
	issuedAt := time.Now().UTC()
	if document.IssuedAt == "" {
		document.IssuedAt = issuedAt.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02 15:04:05 MST")
	}
	contents, err := reportpdf.Render(document)
	if err != nil {
		return nil, err
	}
	events, err := s.repository.DocumentEvents(r.Context(), user.OrganizationID, documentType, documentID, 500)
	if err != nil {
		return nil, err
	}
	version := 1
	for _, event := range events {
		if event.OutputFormat == "pdf" && event.ObjectKey != "" {
			version++
		}
	}
	digest := sha256.Sum256(contents)
	sha := hex.EncodeToString(digest[:])
	fileName := fmt.Sprintf("%s-v%03d.pdf", documentNumber, version)
	objectKey := filepath.ToSlash(fmt.Sprintf("official-documents/%s/%s/%s/v%03d-%d-%s.pdf",
		user.OrganizationID, documentType, documentID, version, issuedAt.UnixNano(), sha[:12]))
	size, err := s.objects.Put(r.Context(), objectKey, "application/pdf", bytes.NewReader(contents))
	if err != nil {
		return nil, fmt.Errorf("store official pdf: %w", err)
	}
	snapshotJSON, _ := json.Marshal(snapshot)
	metadata, _ := json.Marshal(map[string]any{
		"official": true, "immutable": true, "version": version, "sha256": sha,
		"sizeBytes": size, "issuedAt": issuedAt, "snapshot": json.RawMessage(snapshotJSON),
	})
	event, err := s.repository.RecordDocumentEvent(r.Context(), user.OrganizationID, user.ID, persistence.DocumentEventInput{
		DocumentType: documentType, DocumentID: documentID, DocumentNumber: documentNumber,
		Action: "download", OutputFormat: "pdf", FileName: fileName,
		StorageDriver: s.objects.Driver(), ObjectKey: objectKey, Metadata: metadata,
	})
	if err != nil {
		_ = s.objects.Delete(r.Context(), objectKey)
		return nil, fmt.Errorf("record official pdf: %w", err)
	}
	return &persistence.OfficialDocumentRef{EventID: event.ID, Version: version, FileName: fileName,
		DownloadURL: "/api/v1/document-events/" + event.ID + "/file", SHA256: sha, SizeBytes: size, IssuedAt: issuedAt}, nil
}

func (s *Server) apiDocumentEventFile(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	event, err := s.repository.DocumentEvent(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil || event.OutputFormat != "pdf" || event.ObjectKey == "" {
		writeAPIError(w, http.StatusNotFound, "document_file_not_found", "保存済みPDFが見つかりません。")
		return
	}
	object, err := s.objects.Get(r.Context(), event.ObjectKey)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "document_file_not_found", "保存済みPDFが見つかりません。")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeDownloadName(event.FileName)+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	defer object.Body.Close()
	_, _ = io.Copy(w, object.Body)
}

func safeDownloadName(value string) string {
	value = strings.TrimSpace(filepath.Base(value))
	if value == "" || !strings.HasSuffix(strings.ToLower(value), ".pdf") {
		return "document.pdf"
	}
	return strings.Map(func(r rune) rune {
		if r < 32 || r == '"' || r == '\\' || r == '/' {
			return '-'
		}
		return r
	}, value)
}

func amount(currency string, value int64) string {
	prefix := "￥"
	if strings.EqualFold(currency, "USD") {
		prefix = "$"
	} else if strings.EqualFold(currency, "EUR") {
		prefix = "€"
	} else if strings.EqualFold(currency, "HKD") {
		prefix = "HK$"
	}
	return prefix + pdfFormatInteger(value)
}

func pdfFormatInteger(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	raw := strconv.FormatInt(value, 10)
	for index := len(raw) - 3; index > 0; index -= 3 {
		raw = raw[:index] + "," + raw[index:]
	}
	if negative {
		return "-" + raw
	}
	return raw
}

func purchasePDF(record persistence.PurchaseSlipRecord) reportpdf.Document {
	lines := make([]reportpdf.Line, 0, len(record.Lines))
	var subtotal int64
	for _, item := range record.Lines {
		lineAmount := item.ConvertedTotalJPY
		if lineAmount == 0 {
			lineAmount = item.UnitCostMinor * int64(item.Quantity)
		}
		subtotal += lineAmount
		description := strings.TrimSpace(item.BrandName + " / " + item.ModelNumber)
		if len(item.AccessoryCodes) > 0 {
			description += "　付属品: " + strings.Join(item.AccessoryCodes, ", ")
		}
		tax := "対象外"
		if record.PurchaseTaxMode == persistence.PurchaseTaxModeDomestic {
			tax = "消費税(10%)"
		}
		lines = append(lines, reportpdf.Line{Number: item.LineNumber, Description: description,
			Amount: amount("JPY", lineAmount), Tax: tax})
	}
	taxAmount := int64(0)
	taxLabel := "対象外"
	if record.PurchaseTaxMode == persistence.PurchaseTaxModeDomestic {
		taxAmount = subtotal * int64(record.TaxRateBasisPoints) / 10000
		taxLabel = "消費税（10%）"
	}
	return reportpdf.Document{Title: "仕入伝票", Number: record.SlipNumber, TransactionDate: string(record.PurchaseDate),
		PartnerLabel: "仕入先", PartnerName: record.SupplierName, Currency: "JPY", Subtotal: amount("JPY", subtotal),
		TaxAmount: amount("JPY", taxAmount), Total: amount("JPY", subtotal+taxAmount), TaxLabel: taxLabel,
		Notes: record.Notes, Lines: lines}
}

func salePDF(record persistence.SaleSlipRecord) reportpdf.Document {
	lines := make([]reportpdf.Line, 0, len(record.Lines))
	for _, item := range record.Lines {
		description := strings.TrimSpace(item.Brand + " / " + item.ModelNumber)
		if item.Accessories != "" {
			description += "　付属品: " + item.Accessories
		}
		tax := "対象外"
		if record.TaxMode == "taxable" {
			tax = "消費税(10%)"
		}
		lines = append(lines, reportpdf.Line{Number: item.LineNumber, Description: description,
			Amount: amount(record.DisplayCurrency, item.SubtotalMinor), Tax: tax, ProductCode: item.ProductCode})
	}
	return reportpdf.Document{Title: "請求書", Number: record.SlipNumber, TransactionDate: string(record.SaleDate),
		PartnerLabel: "ご請求先", PartnerName: record.BuyerName, Currency: record.DisplayCurrency,
		Subtotal: amount(record.DisplayCurrency, record.SubtotalMinor), TaxAmount: amount(record.DisplayCurrency, record.TaxAmountMinor),
		Total: amount(record.DisplayCurrency, record.TotalMinor), TaxLabel: saleTaxLabel(record.TaxMode), Notes: record.Notes, Lines: lines}
}

func consignmentPDF(record persistence.ConsignmentSlipRecord) reportpdf.Document {
	lines := make([]reportpdf.Line, 0, len(record.Lines))
	var total int64
	for _, item := range record.Lines {
		value := item.ConvertedSalePriceJPY
		// Older records may predate the persisted JPY snapshot. Preserve the
		// registration-time rate from the slip and reconstruct the JPY amount
		// without consulting the current master rate.
		if value <= 0 && item.SalePriceUSDMinor > 0 && record.FXRateScaled > 0 && record.FXScale > 0 {
			converted := item.SalePriceUSDMinor * record.FXRateScaled / record.FXScale
			if item.SalePriceUSDMinor*record.FXRateScaled%record.FXScale != 0 {
				converted++
			}
			value = ((converted + 999) / 1000) * 1000
		}
		total += value
		lines = append(lines, reportpdf.Line{Number: item.LineNumber, Description: strings.TrimSpace(item.Brand + " / " + item.ModelNumber),
			Amount: amount("JPY", value), Tax: "対象外", ProductCode: item.ProductCode})
	}
	return reportpdf.Document{Title: "委託伝票", Number: record.SlipNumber, TransactionDate: string(record.ConsignmentDate),
		PartnerLabel: "委託先", PartnerName: record.ConsigneeName, Currency: "JPY", Subtotal: amount("JPY", total),
		TaxAmount: amount("JPY", 0), Total: amount("JPY", total), TaxLabel: "対象外", Notes: record.Notes, Lines: lines}
}

func saleTaxLabel(mode string) string {
	if mode == "taxable" {
		return "消費税（10%）"
	}
	return "対象外"
}
