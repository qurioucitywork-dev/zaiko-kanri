package web

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type performanceMode struct {
	Key         string
	Name        string
	Icon        string
	ChartTitle  string
	RowLabel    string
	AmountLabel string
}

type performanceRow struct {
	Name        string
	Count       int
	AmountJPY   int64
	PercentText string
	Dash        string
	Offset      string
	Color       string
	Class       string
}

type buyerPerformanceSummary struct {
	PurchaseAmountJPY int64
	RevenueJPY        int64
	ExpectedProfitJPY int64
	ActualProfitJPY   int64
	ExpectedMargin    string
	ActualMargin      string
}

type buyerPerformanceRow struct {
	Name               string
	PurchaseCount      int
	PurchaseAmountJPY  int64
	SalesCount         int
	RevenueJPY         int64
	ExpectedProfitJPY  int64
	ActualProfitJPY    int64
	GrossMargin        string
	InventoryAmountJPY int64
	Consumption        int
	RevenueBar         int
	ProfitBar          int
	ColorClass         string
}

type salesDestinationPerformanceSummary struct {
	RevenueJPY        int64
	ExpectedProfitJPY int64
	ActualProfitJPY   int64
	SalesCount        int
	ExpectedMargin    string
	ActualMargin      string
}

type salesDestinationPerformanceRow struct {
	Name               string
	SalesCount         int
	RevenueJPY         int64
	CostJPY            int64
	ExpectedProfitJPY  int64
	ActualProfitJPY    int64
	GrossMargin        string
	Composition        string
	RevenueDash        string
	RevenueOffset      string
	ProfitDash         string
	ProfitOffset       string
	RevenueComposition string
	ProfitComposition  string
	RevenueBar         int
	ProfitBar          int
	Color              string
	ColorClass         string
}

var performanceModes = map[string]performanceMode{
	"suppliers": {
		Key: "suppliers", Name: "仕入先別", Icon: "◧",
		ChartTitle: "仕入先別 構成比（仕入金額）", RowLabel: "仕入先", AmountLabel: "仕入金額",
	},
	"buyers": {
		Key: "buyers", Name: "仕入担当者別", Icon: "♟",
		ChartTitle: "仕入担当者別 構成比（仕入金額）", RowLabel: "仕入担当者", AmountLabel: "仕入金額",
	},
	"sales-destinations": {
		Key: "sales-destinations", Name: "販売先別", Icon: "▦",
		ChartTitle: "販売先別 構成比（売上金額）", RowLabel: "販売先", AmountLabel: "売上金額",
	},
}

func (s *Server) performance(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	mode, dateFrom, dateTo, err := performanceFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	records, err := s.store.Performance(r.Context(), user.OrganizationID, mode.Key, dateFrom, dateTo)
	if err != nil {
		http.Error(w, "実績を集計できませんでした", http.StatusInternalServerError)
		return
	}
	rows, totalAmount, totalCount := buildPerformanceRows(records)
	var buyerRows []buyerPerformanceRow
	var buyerSummary buyerPerformanceSummary
	var destinationRows []salesDestinationPerformanceRow
	var destinationSummary salesDestinationPerformanceSummary
	if mode.Key == "buyers" {
		buyerRecords, buyerErr := s.store.BuyerPerformance(r.Context(), user.OrganizationID, dateFrom, dateTo)
		if buyerErr != nil {
			http.Error(w, "仕入担当者別の実績を集計できませんでした", http.StatusInternalServerError)
			return
		}
		buyerRows, buyerSummary = buildBuyerPerformanceRows(buyerRecords)
	}
	if mode.Key == "sales-destinations" {
		destinationRecords, destinationErr := s.store.SalesDestinationPerformance(r.Context(), user.OrganizationID, dateFrom, dateTo)
		if destinationErr != nil {
			http.Error(w, "販売先別の実績を集計できませんでした", http.StatusInternalServerError)
			return
		}
		destinationRows, destinationSummary = buildSalesDestinationPerformanceRows(destinationRecords)
	}
	alertCount, err := s.pendingAlerts(r, user.OrganizationID)
	if err != nil {
		http.Error(w, "通知件数を取得できませんでした", http.StatusInternalServerError)
		return
	}
	s.render(w, "performance", http.StatusOK, pageData{
		Title: "実績管理", Active: "performance", User: user, CSRF: csrfFromRequest(r),
		PerformanceRows: rows, PerformanceMode: mode, PerformanceFrom: dateFrom, PerformanceTo: dateTo,
		PerformanceTotal: totalAmount, PerformanceCount: totalCount, AlertCount: alertCount,
		BuyerPerformanceRows: buyerRows, BuyerPerformanceSummary: buyerSummary,
		SalesDestinationPerformanceRows: destinationRows, SalesDestinationPerformanceSummary: destinationSummary,
	})
}

func (s *Server) performanceCSV(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	mode, dateFrom, dateTo, err := performanceFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	records, err := s.store.Performance(r.Context(), user.OrganizationID, mode.Key, dateFrom, dateTo)
	if err != nil {
		http.Error(w, "CSVを作成できませんでした", http.StatusInternalServerError)
		return
	}
	rows, totalAmount, _ := buildPerformanceRows(records)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "performance",
		TargetID: mode.Key, Action: "performance.csv.export", Result: "success",
		Reason:    fmt.Sprintf("from=%s to=%s rows=%d", dateFrom, dateTo, len(rows)),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	filename := fmt.Sprintf("performance-%s-%s-%s.csv", mode.Key, dateFrom, dateTo)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{mode.Name, "件数", mode.AmountLabel, "構成比"})
	for _, row := range rows {
		_ = writer.Write([]string{
			safeCSVCell(row.Name), strconv.Itoa(row.Count), strconv.FormatInt(row.AmountJPY, 10), row.PercentText,
		})
	}
	_ = writer.Write([]string{"合計", "", strconv.FormatInt(totalAmount, 10), "100.0%"})
	writer.Flush()
}

func performanceFilter(r *http.Request) (performanceMode, string, string, error) {
	modeKey := r.URL.Query().Get("mode")
	if modeKey == "" {
		modeKey = "suppliers"
	}
	mode, ok := performanceModes[modeKey]
	if !ok {
		return performanceMode{}, "", "", database.ErrInvalidPerformanceMode
	}
	now := time.Now().In(time.FixedZone("JST", 9*60*60))
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	if dateFrom == "" {
		dateFrom = fmt.Sprintf("%04d-01-01", now.Year())
	}
	if dateTo == "" {
		dateTo = fmt.Sprintf("%04d-12-31", now.Year())
	}
	if _, err := time.Parse("2006-01-02", dateFrom); err != nil {
		return performanceMode{}, "", "", fmt.Errorf("集計開始日を正しく入力してください")
	}
	if _, err := time.Parse("2006-01-02", dateTo); err != nil || dateFrom > dateTo {
		return performanceMode{}, "", "", fmt.Errorf("集計終了日を正しく入力してください")
	}
	return mode, dateFrom, dateTo, nil
}

func buildPerformanceRows(records []database.PerformanceRecord) ([]performanceRow, int64, int) {
	var totalAmount int64
	var totalCount int
	for _, record := range records {
		totalAmount += record.AmountJPY
		totalCount += record.Count
	}
	colors := []string{"#2f88bd", "#ef821c", "#28ad64", "#8e44ad", "#eb4938", "#607d8b"}
	rows := make([]performanceRow, 0, len(records))
	offset := 0.0
	for index, record := range records {
		basis := float64(0)
		if totalAmount > 0 {
			basis = float64(record.AmountJPY) * 100 / float64(totalAmount)
		} else if totalCount > 0 {
			basis = float64(record.Count) * 100 / float64(totalCount)
		}
		rows = append(rows, performanceRow{
			Name: record.Name, Count: record.Count, AmountJPY: record.AmountJPY,
			PercentText: fmt.Sprintf("%.1f%%", basis), Dash: fmt.Sprintf("%.3f", basis),
			Offset: fmt.Sprintf("%.3f", -offset), Color: colors[index%len(colors)],
			Class: fmt.Sprintf("performance-color-%d", index%len(colors)+1),
		})
		offset += basis
	}
	return rows, totalAmount, totalCount
}

func buildBuyerPerformanceRows(records []database.BuyerPerformanceRecord) ([]buyerPerformanceRow, buyerPerformanceSummary) {
	var summary buyerPerformanceSummary
	var chartMaximum int64
	for _, record := range records {
		summary.PurchaseAmountJPY += record.PurchaseAmountJPY
		summary.RevenueJPY += record.RevenueJPY
		summary.ExpectedProfitJPY += record.ExpectedProfitJPY
		summary.ActualProfitJPY += record.ActualProfitJPY
		if record.RevenueJPY > chartMaximum {
			chartMaximum = record.RevenueJPY
		}
		if record.ActualProfitJPY > chartMaximum {
			chartMaximum = record.ActualProfitJPY
		}
	}
	summary.ExpectedMargin = percentageText(summary.ExpectedProfitJPY, summary.RevenueJPY)
	summary.ActualMargin = percentageText(summary.ActualProfitJPY, summary.RevenueJPY)
	colors := []string{"blue", "orange", "green", "purple", "red", "teal"}
	result := make([]buyerPerformanceRow, 0, len(records))
	for index, record := range records {
		consumption := 0
		if record.PurchaseCount > 0 {
			consumption = record.SalesCount * 100 / record.PurchaseCount
			if consumption > 100 {
				consumption = 100
			}
		}
		result = append(result, buyerPerformanceRow{
			Name: record.Name, PurchaseCount: record.PurchaseCount, PurchaseAmountJPY: record.PurchaseAmountJPY,
			SalesCount: record.SalesCount, RevenueJPY: record.RevenueJPY,
			ExpectedProfitJPY: record.ExpectedProfitJPY, ActualProfitJPY: record.ActualProfitJPY,
			GrossMargin:        percentageText(record.ActualProfitJPY, record.RevenueJPY),
			InventoryAmountJPY: record.InventoryAmountJPY, Consumption: consumption,
			RevenueBar: scaledChartPercent(record.RevenueJPY, chartMaximum),
			ProfitBar:  scaledChartPercent(record.ActualProfitJPY, chartMaximum),
			ColorClass: colors[index%len(colors)],
		})
	}
	return result, summary
}

func buildSalesDestinationPerformanceRows(records []database.SalesDestinationPerformanceRecord) ([]salesDestinationPerformanceRow, salesDestinationPerformanceSummary) {
	var summary salesDestinationPerformanceSummary
	var chartMaximum int64
	var positiveProfitTotal int64
	for _, record := range records {
		summary.RevenueJPY += record.RevenueJPY
		summary.ExpectedProfitJPY += record.ExpectedProfitJPY
		summary.ActualProfitJPY += record.ActualProfitJPY
		summary.SalesCount += record.SalesCount
		if record.RevenueJPY > chartMaximum {
			chartMaximum = record.RevenueJPY
		}
		if record.ActualProfitJPY > chartMaximum {
			chartMaximum = record.ActualProfitJPY
		}
		if record.ActualProfitJPY > 0 {
			positiveProfitTotal += record.ActualProfitJPY
		}
	}
	summary.ExpectedMargin = percentageText(summary.ExpectedProfitJPY, summary.RevenueJPY)
	summary.ActualMargin = percentageText(summary.ActualProfitJPY, summary.RevenueJPY)
	colors := []string{"#2f88bd", "#ef821c", "#28ad64", "#8e44ad", "#eb4938", "#18a087"}
	classes := []string{"blue", "orange", "green", "purple", "red", "teal"}
	result := make([]salesDestinationPerformanceRow, 0, len(records))
	revenueOffset := 0.0
	profitOffset := 0.0
	for index, record := range records {
		revenueBasis := percentageValue(record.RevenueJPY, summary.RevenueJPY)
		profitBasis := percentageValue(record.ActualProfitJPY, positiveProfitTotal)
		row := salesDestinationPerformanceRow{
			Name: record.Name, SalesCount: record.SalesCount, RevenueJPY: record.RevenueJPY,
			CostJPY: record.CostJPY, ExpectedProfitJPY: record.ExpectedProfitJPY,
			ActualProfitJPY: record.ActualProfitJPY,
			GrossMargin:     percentageText(record.ActualProfitJPY, record.RevenueJPY),
			Composition:     fmt.Sprintf("%.1f%%", revenueBasis),
			RevenueDash:     fmt.Sprintf("%.3f", revenueBasis), RevenueOffset: fmt.Sprintf("%.3f", -revenueOffset),
			ProfitDash: fmt.Sprintf("%.3f", profitBasis), ProfitOffset: fmt.Sprintf("%.3f", -profitOffset),
			RevenueComposition: fmt.Sprintf("%.1f%%", revenueBasis), ProfitComposition: fmt.Sprintf("%.1f%%", profitBasis),
			RevenueBar: scaledChartPercent(record.RevenueJPY, chartMaximum),
			ProfitBar:  scaledChartPercent(record.ActualProfitJPY, chartMaximum),
			Color:      colors[index%len(colors)], ColorClass: classes[index%len(classes)],
		}
		result = append(result, row)
		revenueOffset += revenueBasis
		profitOffset += profitBasis
	}
	return result, summary
}

func percentageValue(value, base int64) float64 {
	if value <= 0 || base <= 0 {
		return 0
	}
	return float64(value) * 100 / float64(base)
}

func percentageText(value, base int64) string {
	if base == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", float64(value)*100/float64(base))
}

func scaledChartPercent(value, maximum int64) int {
	if value <= 0 || maximum <= 0 {
		return 0
	}
	percent := int(float64(value) * 92 / float64(maximum))
	if percent < 4 {
		return 4
	}
	return percent
}
