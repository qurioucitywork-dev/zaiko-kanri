package web

import (
	"encoding/csv"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type slipListRow struct {
	ID             string
	Number         string
	Date           string
	Partner        string
	Staff          string
	Count          int
	Total          int64
	Notes          string
	Status         string
	StatusText     string
	DetailURL      string
	Corrected      bool
	DeliveryNumber string
	SearchText     string
}

type slipListTab struct {
	Key   string
	Label string
	Icon  string
	Count int
}

type slipListSummary struct {
	Count           int
	Total           int64
	ProductCount    int
	CorrectionCount int
}

var slipKinds = []slipListTab{
	{Key: "purchases", Label: "仕入伝票", Icon: "↪"},
	{Key: "shipments", Label: "出荷伝票", Icon: "▰"},
	{Key: "sales", Label: "売上伝票", Icon: "￥"},
	{Key: "sales-returns", Label: "売上返品", Icon: "↶"},
	{Key: "purchase-returns", Label: "仕入返品", Icon: "▣"},
}

func (s *Server) slips(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	kind := normalizeSlipKind(r.URL.Query().Get("kind"))
	allRows, err := s.loadSlipRows(r, user.OrganizationID)
	if err != nil {
		http.Error(w, "伝票一覧を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	tabs := make([]slipListTab, len(slipKinds))
	copy(tabs, slipKinds)
	for index := range tabs {
		tabs[index].Count = slipActionRequiredCount(allRows[tabs[index].Key])
	}
	rows, partners, summary := filterSlipRows(allRows[kind], r)
	searchPerformed := (kind != "purchases" && kind != "sales" && kind != "shipments") || slipSearchPerformed(r)
	if !searchPerformed {
		rows = nil
		summary = slipListSummary{}
	}
	s.render(w, "slips", http.StatusOK, pageData{
		Title: "伝票一覧", Active: "slips", User: user, CSRF: csrfFromRequest(r),
		SlipRows: rows, SlipTabs: tabs, SlipKind: kind, SlipPartners: partners,
		SlipDateFrom: r.URL.Query().Get("date_from"), SlipDateTo: r.URL.Query().Get("date_to"),
		SlipPartner: r.URL.Query().Get("partner"), Query: r.URL.Query().Get("q"),
		SlipSummary: summary, ApprovalOnly: r.URL.Query().Get("approval") == "1",
		SlipSearchPerformed: searchPerformed,
		Notice:              r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error"),
	})
}

func (s *Server) slipsCSV(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	kind := normalizeSlipKind(r.URL.Query().Get("kind"))
	allRows, err := s.loadSlipRows(r, user.OrganizationID)
	if err != nil {
		http.Error(w, "伝票一覧を出力できませんでした。", http.StatusInternalServerError)
		return
	}
	rows, _, _ := filterSlipRows(allRows[kind], r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="slips.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"伝票番号", "日付", "取引先", "担当者", "点数", "合計金額", "備考", "ステータス"})
	for _, row := range rows {
		_ = writer.Write([]string{
			row.Number, row.Date, row.Partner, row.Staff, strconv.Itoa(row.Count),
			strconv.FormatInt(row.Total, 10), row.Notes, row.StatusText,
		})
	}
	writer.Flush()
}

func (s *Server) loadSlipRows(r *http.Request, organizationID string) (map[string][]slipListRow, error) {
	result := map[string][]slipListRow{
		"purchases": {}, "shipments": {}, "sales": {}, "sales-returns": {}, "purchase-returns": {},
	}
	purchases, err := s.store.PurchaseSlips(r.Context(), organizationID)
	if err != nil {
		return nil, err
	}
	for _, slip := range purchases {
		status, text := slipDisplayStatus(slip.Status)
		searchParts := []string{slip.SlipNumber, slip.SupplierName, slip.CreatedByName, slip.Notes}
		if detail, detailErr := s.store.Purchase(r.Context(), organizationID, slip.ID); detailErr == nil {
			for _, line := range detail.Lines {
				searchParts = append(searchParts, line.ProductCode, line.SKU, line.Brand, line.ModelNumber)
			}
		}
		result["purchases"] = append(result["purchases"], slipListRow{
			ID: slip.ID, Number: slip.SlipNumber, Date: slip.PurchaseDate, Partner: slip.SupplierName,
			Staff: slip.CreatedByName, Count: slip.LineCount, Total: slip.TotalMinor, Notes: slip.Notes,
			Status: status, StatusText: text, DetailURL: "/purchases/" + slip.ID,
			Corrected:  slip.RevisionCount > 0,
			SearchText: strings.Join(searchParts, " "),
		})
	}
	shipments, err := s.store.Shipments(r.Context(), organizationID)
	if err != nil {
		return nil, err
	}
	for _, slip := range shipments {
		detail, detailErr := s.store.Shipment(r.Context(), organizationID, slip.ID)
		if detailErr != nil {
			return nil, detailErr
		}
		status, text := slipDisplayStatus(slip.Status)
		searchParts := []string{slip.ShipmentNumber, slip.RecipientName, slip.Notes}
		for _, line := range detail.Lines {
			searchParts = append(searchParts, line.ProductCode, line.Brand, line.ModelNumber)
		}
		result["shipments"] = append(result["shipments"], slipListRow{
			ID: slip.ID, Number: slip.ShipmentNumber, Date: slip.ShipmentDate, Partner: slip.RecipientName,
			Count: len(detail.Lines), Notes: slip.Notes, Status: status, StatusText: text,
			DetailURL: "/shipments/" + slip.ID, Total: detail.TotalJPY,
			Corrected:  slip.RevisionCount > 0,
			SearchText: strings.Join(searchParts, " "),
		})
	}
	sales, err := s.store.Sales(r.Context(), organizationID)
	if err != nil {
		return nil, err
	}
	for _, slip := range sales {
		detail, detailErr := s.store.Sale(r.Context(), organizationID, slip.ID)
		if detailErr != nil {
			return nil, detailErr
		}
		status, text := slipDisplayStatus(slip.Status)
		searchParts := []string{slip.SlipNumber, slip.CustomerName, slip.Notes}
		for _, line := range detail.Lines {
			searchParts = append(searchParts, line.ProductCode, line.Brand, line.ModelNumber)
		}
		result["sales"] = append(result["sales"], slipListRow{
			ID: slip.ID, Number: slip.SlipNumber, Date: slip.SalesDate, Partner: slip.CustomerName,
			Count: len(detail.Lines), Total: slip.TotalJPY, Notes: slip.Notes, Status: status,
			StatusText: text, DetailURL: "/sales/" + slip.ID, Corrected: slip.RevisionCount > 0,
			SearchText: strings.Join(searchParts, " "),
		})
	}
	returns, err := s.store.SalesReturnSummaries(r.Context(), organizationID)
	if err != nil {
		return nil, err
	}
	for _, item := range returns {
		status, text := "completed", "処理済"
		if item.PendingCount > 0 {
			status, text = "pending", "承認待ち"
		}
		result["sales-returns"] = append(result["sales-returns"], slipListRow{
			ID: item.SaleID, Number: "SR-" + strings.TrimPrefix(item.SlipNumber, "SL-"),
			Date: item.SalesDate, Partner: item.CustomerName, Count: item.ItemCount, Total: item.TotalJPY,
			Status: status, StatusText: text, DetailURL: "/returns/" + item.SaleID,
		})
	}
	purchaseReturns, err := s.store.PurchaseReturnSlips(r.Context(), organizationID)
	if err != nil {
		return nil, err
	}
	for _, item := range purchaseReturns {
		status, text := item.Status, "承認待ち"
		if status == "completed" {
			text = "処理済"
		} else if status == "returned" {
			text = "差戻し"
		}
		detailURL := "/purchases"
		if item.PurchaseSlipID != "" {
			detailURL = "/purchases/" + item.PurchaseSlipID
		}
		result["purchase-returns"] = append(result["purchase-returns"], slipListRow{
			ID: item.ID, Number: item.ReturnNumber, Date: item.ReturnDate, Partner: item.SupplierName,
			Count: item.ItemCount, Total: item.AmountJPY, Notes: item.Reason, Status: status,
			StatusText: text, DetailURL: detailURL, DeliveryNumber: item.DeliveryNumber,
		})
	}
	return result, nil
}

func (s *Server) purchaseReturnDelivery(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := s.store.UpdatePurchaseReturnDelivery(r.Context(), user.OrganizationID, r.PathValue("id"), r.FormValue("delivery_number")); err != nil {
		http.Redirect(w, r, "/slips?kind=purchase-returns&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/slips?kind=purchase-returns&notice="+url.QueryEscape("配送番号を保存しました。"), http.StatusSeeOther)
}

func normalizeSlipKind(kind string) string {
	for _, tab := range slipKinds {
		if tab.Key == kind {
			return kind
		}
	}
	return "purchases"
}

func slipSearchPerformed(r *http.Request) bool {
	query := r.URL.Query()
	return query.Get("show_all") == "1" ||
		query.Get("search") == "1" ||
		query.Get("approval") == "1" ||
		query.Get("date_from") != "" ||
		query.Get("date_to") != "" ||
		query.Get("partner") != "" ||
		strings.TrimSpace(query.Get("q")) != ""
}

func filterSlipRows(rows []slipListRow, r *http.Request) ([]slipListRow, []string, slipListSummary) {
	from, to := r.URL.Query().Get("date_from"), r.URL.Query().Get("date_to")
	partner, query := r.URL.Query().Get("partner"), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	approvalOnly := r.URL.Query().Get("approval") == "1"
	partnerSet := make(map[string]bool)
	filtered := make([]slipListRow, 0, len(rows))
	for _, row := range rows {
		if row.Partner != "" {
			partnerSet[row.Partner] = true
		}
		if from != "" && row.Date < from || to != "" && row.Date > to || partner != "" && row.Partner != partner {
			continue
		}
		haystack := strings.ToLower(row.Number + " " + row.Partner + " " + row.Staff + " " + row.Notes + " " + row.SearchText)
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		if approvalOnly && row.Status != "pending" && row.Status != "returned" {
			continue
		}
		filtered = append(filtered, row)
	}
	partners := make([]string, 0, len(partnerSet))
	for value := range partnerSet {
		partners = append(partners, value)
	}
	sort.Strings(partners)
	summary := slipListSummary{Count: len(filtered)}
	for _, row := range filtered {
		summary.Total += row.Total
		summary.ProductCount += row.Count
		if row.Corrected {
			summary.CorrectionCount++
		}
	}
	return filtered, partners, summary
}

func slipActionRequiredCount(rows []slipListRow) int {
	count := 0
	for _, row := range rows {
		if row.Status == "pending" || row.Status == "returned" {
			count++
		}
	}
	return count
}

func slipDisplayStatus(status string) (string, string) {
	switch status {
	case "confirmed":
		return "completed", "処理済"
	case "cancelled":
		return "returned", "差戻し"
	default:
		return "pending", "承認待ち"
	}
}
