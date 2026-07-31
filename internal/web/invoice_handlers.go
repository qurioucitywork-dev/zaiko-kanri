package web

import (
	"encoding/csv"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type invoiceCompany struct {
	Name                   string
	PostalCode             string
	Address                string
	Phone                  string
	Fax                    string
	Email                  string
	QualifiedInvoiceNumber string
	BankName               string
	BankBranch             string
	BankAccountType        string
	BankAccountNumber      string
	BankAccountHolder      string
}

type salesInvoiceLine struct {
	SlipNumber   string
	SalesDate    string
	ProductCode  string
	ProductName  string
	ModelNumber  string
	SerialNumber string
	Quantity     int
	UnitPrice    int64
	Amount       int64
	Notes        string
}

type salesInvoiceGroup struct {
	CustomerName    string
	CustomerAddress string
	CustomerPhone   string
	InvoiceNumber   string
	IssueDate       string
	Lines           []salesInvoiceLine
	Total           int64
}

type purchaseReturnInvoiceLine struct {
	ReturnNumber string
	ReturnDate   string
	ProductCode  string
	Brand        string
	ModelNumber  string
	TypeNumber   string
	SerialNumber string
	Quantity     int
	UnitPrice    int64
	Amount       int64
	Reason       string
	Status       string
}

type purchaseReturnInvoiceGroup struct {
	SupplierName  string
	InvoiceNumber string
	IssueDate     string
	ReturnIDs     []string
	Rows          []purchaseReturnInvoiceLine
	ReturnCount   int
	ItemCount     int
	Total         int64
}

func defaultInvoiceCompany() invoiceCompany {
	return invoiceCompany{
		Name:                   "株式会社ウォッチプレミアム",
		PostalCode:             "〒160-0022",
		Address:                "東京都新宿区新宿1-1-1",
		Phone:                  "03-1234-5678",
		Fax:                    "03-1234-5679",
		Email:                  "info@watch-premium.example.jp",
		QualifiedInvoiceNumber: "T1234567890123",
		BankName:               "ウォッチ銀行",
		BankBranch:             "新宿支店",
		BankAccountType:        "普通",
		BankAccountNumber:      "1234567",
		BankAccountHolder:      "カ）ウォッチプレミアム",
	}
}

func selectedInvoiceIDs(r *http.Request) []string {
	_ = r.ParseForm()
	values := append([]string{}, r.Form["id"]...)
	values = append(values, r.Form["slip_id"]...)
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func (s *Server) salesInvoicesPreview(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	groups, err := s.salesInvoiceGroups(r, user.OrganizationID)
	if err != nil {
		writeRequestError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.renderPartial(w, "invoice-previews", "sales-bulk-preview", http.StatusOK, pageData{
		Title: "売上請求書プレビュー", Active: "slips", User: user, CSRF: csrfFromRequest(r),
		InvoiceCompany: defaultInvoiceCompany(), SalesInvoiceGroups: groups,
		InvoiceSelectedCount: len(selectedInvoiceIDs(r)), InvoicePartnerCount: len(groups),
	})
}

func (s *Server) salesInvoiceGroups(r *http.Request, organizationID string) ([]salesInvoiceGroup, error) {
	ids := selectedInvoiceIDs(r)
	if len(ids) == 0 {
		return nil, errors.New("請求書を発行する売上伝票を選択してください。")
	}
	groupIndex := make(map[string]int)
	groups := make([]salesInvoiceGroup, 0)
	for _, id := range ids {
		sale, err := s.store.Sale(r.Context(), organizationID, id)
		if err != nil {
			return nil, errors.New("選択した売上伝票を確認できませんでした。")
		}
		index, ok := groupIndex[sale.CustomerName]
		if !ok {
			index = len(groups)
			groupIndex[sale.CustomerName] = index
			groups = append(groups, salesInvoiceGroup{
				CustomerName: sale.CustomerName, CustomerAddress: sale.CustomerAddress,
				CustomerPhone: sale.CustomerPhone, InvoiceNumber: "INV-" + sale.SlipNumber,
				IssueDate: time.Now().Format("2006-01-02"),
			})
		}
		for _, line := range sale.Lines {
			unit := line.ConvertedUnitPriceJPY
			if unit == 0 && line.SaleCurrency == "JPY" {
				unit = line.UnitPriceMinor
			}
			serial := "—"
			if product, productErr := s.store.Product(r.Context(), organizationID, line.ProductID); productErr == nil && product.SerialNumber != "" {
				serial = product.SerialNumber
			}
			amount := unit * int64(line.Quantity)
			groups[index].Lines = append(groups[index].Lines, salesInvoiceLine{
				SlipNumber: sale.SlipNumber, SalesDate: sale.SalesDate, ProductCode: line.ProductCode,
				ProductName: strings.TrimSpace(line.Brand + " " + line.ModelNumber),
				ModelNumber: line.ModelNumber, SerialNumber: serial, Quantity: line.Quantity,
				UnitPrice: unit, Amount: amount, Notes: sale.Notes,
			})
			groups[index].Total += amount
		}
	}
	return groups, nil
}

func (s *Server) purchaseReturnInvoicesPreview(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	groups, count, err := s.purchaseReturnInvoiceGroups(r, user.OrganizationID)
	if err != nil {
		writeRequestError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.renderPartial(w, "invoice-previews", "purchase-return-bulk-preview", http.StatusOK, pageData{
		Title: "仕入返品請求書プレビュー", Active: "slips", User: user, CSRF: csrfFromRequest(r),
		InvoiceCompany: defaultInvoiceCompany(), PurchaseInvoiceGroups: groups,
		InvoiceSelectedCount: count, InvoicePartnerCount: len(groups),
	})
}

func (s *Server) purchaseReturnInvoiceGroups(r *http.Request, organizationID string) ([]purchaseReturnInvoiceGroup, int, error) {
	ids := selectedInvoiceIDs(r)
	if len(ids) == 0 {
		return nil, 0, errors.New("請求書を発行する仕入返品伝票を選択してください。")
	}
	groupIndex := make(map[string]int)
	groups := make([]purchaseReturnInvoiceGroup, 0)
	for _, id := range ids {
		item, err := s.store.PurchaseReturn(r.Context(), organizationID, id)
		if err != nil {
			return nil, 0, errors.New("選択した仕入返品伝票を確認できませんでした。")
		}
		lines, err := s.store.PurchaseReturnLines(r.Context(), organizationID, item)
		if err != nil {
			return nil, 0, errors.New("仕入返品明細を確認できませんでした。")
		}
		index, ok := groupIndex[item.SupplierName]
		if !ok {
			index = len(groups)
			groupIndex[item.SupplierName] = index
			groups = append(groups, purchaseReturnInvoiceGroup{
				SupplierName: item.SupplierName, InvoiceNumber: "INV-" + item.ReturnNumber,
				IssueDate: time.Now().Format("2006-01-02"),
			})
		}
		groups[index].ReturnCount++
		groups[index].ReturnIDs = append(groups[index].ReturnIDs, item.ID)
		for _, line := range lines {
			typeNumber, serial := "—", "—"
			if product, productErr := s.store.Product(r.Context(), organizationID, line.ProductID); productErr == nil {
				if product.ModelNumber != "" {
					typeNumber = product.ModelNumber
				}
				if product.SerialNumber != "" {
					serial = product.SerialNumber
				}
			}
			groups[index].Rows = append(groups[index].Rows, purchaseReturnInvoiceLine{
				ReturnNumber: item.ReturnNumber, ReturnDate: item.ReturnDate,
				ProductCode: line.ProductCode, Brand: line.Brand, ModelNumber: line.ModelNumber,
				TypeNumber: typeNumber, SerialNumber: serial, Quantity: 1,
				UnitPrice: line.AmountJPY, Amount: line.AmountJPY, Reason: item.Reason,
				Status: purchaseReturnStatusText(item.Status),
			})
			groups[index].ItemCount++
			groups[index].Total += line.AmountJPY
		}
		if len(lines) == 0 {
			groups[index].Total += item.AmountJPY
		}
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].SupplierName < groups[j].SupplierName })
	return groups, len(ids), nil
}

func purchaseReturnStatusText(status string) string {
	switch status {
	case "completed":
		return "処理済"
	case "returned":
		return "差戻し"
	default:
		return "承認待ち"
	}
}

func (s *Server) purchaseReturnInvoicesCSV(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	groups, _, err := s.purchaseReturnInvoiceGroups(r, user.OrganizationID)
	if err != nil {
		writeRequestError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="purchase-return-invoices.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"仕入先", "返品伝票番号", "返品日", "商品コード", "ブランド", "モデル名", "型番", "シリアル", "仕入金額", "返品理由", "ステータス"})
	for _, group := range groups {
		for _, line := range group.Rows {
			_ = writer.Write([]string{
				group.SupplierName, line.ReturnNumber, line.ReturnDate, line.ProductCode,
				line.Brand, line.ModelNumber, line.TypeNumber, line.SerialNumber,
				strconv.FormatInt(line.Amount, 10), line.Reason, line.Status,
			})
		}
	}
	writer.Flush()
}
