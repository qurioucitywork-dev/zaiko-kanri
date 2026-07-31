package web

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type dashboardSupplierSlice struct {
	Name    string
	Count   int64
	Percent int
	Dash    string
	Offset  string
	Color   string
	Class   string
}

type dashboardMonthBar struct {
	Label      string
	AmountText string
	X          int
	Y          int
	Height     int
	LabelX     int
}

type dashboardGridline struct {
	Y     int
	Label string
}

type dashboardRequestCard struct {
	Request       database.PurchaseRequest
	Items         []database.PurchaseRequest
	ProductNames  string
	TotalMinor    int64
	Currency      string
	Totals        []dashboardCurrencyTotal
	MixedCurrency bool
}

type dashboardCurrencyTotal struct {
	Currency string
	Amount   int64
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	stats, err := s.store.InventoryStats(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "在庫集計を取得できませんでした", http.StatusInternalServerError)
		return
	}
	products, err := s.store.Products(r.Context(), user.OrganizationID, database.ProductFilter{Page: 1, PageSize: 500})
	if err != nil {
		http.Error(w, "商品を取得できませんでした", http.StatusInternalServerError)
		return
	}
	sales, err := s.store.Sales(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "売上集計を取得できませんでした", http.StatusInternalServerError)
		return
	}
	pendingRequestGroups, err := s.store.PurchaseRequestGroups(r.Context(), user.OrganizationID, "pending")
	if err != nil {
		http.Error(w, "購入依頼を取得できませんでした", http.StatusInternalServerError)
		return
	}
	approvals, err := s.store.Approvals(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "承認依頼を取得できませんでした", http.StatusInternalServerError)
		return
	}

	now := time.Now().In(time.FixedZone("JST", 9*60*60))
	currentMonth := now.Format("2006-01")
	previousMonth := now.AddDate(0, -1, 0).Format("2006-01")

	var purchaseTotal int64
	var purchaseCount int
	for _, product := range products {
		if len(product.PurchaseDate) < 7 || product.PurchaseDate[:7] != currentMonth {
			continue
		}
		purchaseCount++
		if product.CostCurrency == "JPY" {
			purchaseTotal += product.CostAmountMinor
		}
	}

	monthTotals := make(map[string]int64, 6)
	var salesTotal, previousSalesTotal int64
	var salesCount int
	for _, sale := range sales {
		if sale.Status != "confirmed" || len(sale.SalesDate) < 7 {
			continue
		}
		month := sale.SalesDate[:7]
		monthTotals[month] += sale.TotalJPY
		if month == currentMonth {
			salesTotal += sale.TotalJPY
			salesCount++
		}
		if month == previousMonth {
			previousSalesTotal += sale.TotalJPY
		}
	}

	approvalCount := 0
	for _, approval := range approvals {
		if approval.Status == "pending" {
			approvalCount++
		}
	}

	latestProducts := products
	if len(latestProducts) > 5 {
		latestProducts = latestProducts[:5]
	}

	requestCards := buildDashboardRequestCards(pendingRequestGroups)

	trendText := "前月の確定売上なし"
	if previousSalesTotal > 0 {
		change := (salesTotal - previousSalesTotal) * 100 / previousSalesTotal
		trendText = fmt.Sprintf("前月比 %+.0f%%", float64(change))
	} else if salesTotal > 0 {
		trendText = fmt.Sprintf("確定 %d伝票", salesCount)
	}

	s.render(w, "dashboard", http.StatusOK, pageData{
		Title: "ダッシュボード", Active: "dashboard", User: user, CSRF: csrfFromRequest(r),
		Stats: stats, Products: latestProducts, SalesTotalJPY: salesTotal, SalesCount: salesCount,
		RequestCount: len(requestCards), PurchaseTotalJPY: purchaseTotal, PurchaseCount: purchaseCount,
		AlertCount: approvalCount, ApprovalAlertCount: approvalCount, PurchaseAlertCount: len(requestCards),
		NotificationCountsSet: true, SalesTrendText: trendText, DashboardRequests: requestCards,
		DashboardSuppliers: buildSupplierSlices(products, currentMonth),
		DashboardMonths:    buildMonthBars(now, monthTotals), DashboardGridlines: buildGridlines(monthTotals, now),
	})
}

func buildDashboardRequestCards(groups []database.PurchaseRequestGroup) []dashboardRequestCard {
	cards := make([]dashboardRequestCard, 0, len(groups))
	for _, group := range groups {
		if len(group.Items) == 0 {
			continue
		}
		card := dashboardRequestCard{Request: group.Items[0], Items: group.Items}
		amountByCurrency := make(map[string]int64)
		for _, request := range group.Items {
			currency := strings.ToUpper(strings.TrimSpace(request.SaleCurrency))
			if currency == "" {
				currency = "JPY"
			}
			amountByCurrency[currency] += request.SalePriceMinor
			name := strings.TrimSpace(strings.Join([]string{request.Brand, request.ModelNumber}, " "))
			if name == "" {
				name = request.ProductCode
			}
			if card.ProductNames == "" {
				card.ProductNames = name
			} else {
				card.ProductNames += "、" + name
			}
		}
		currencies := make([]string, 0, len(amountByCurrency))
		for currency := range amountByCurrency {
			currencies = append(currencies, currency)
		}
		sort.Strings(currencies)
		for _, currency := range currencies {
			card.Totals = append(card.Totals, dashboardCurrencyTotal{
				Currency: currency,
				Amount:   amountByCurrency[currency],
			})
		}
		card.MixedCurrency = len(card.Totals) > 1
		if len(card.Totals) == 1 {
			card.Currency = card.Totals[0].Currency
			card.TotalMinor = card.Totals[0].Amount
		}
		cards = append(cards, card)
	}
	return cards
}

func buildSupplierSlices(products []database.Product, currentMonth string) []dashboardSupplierSlice {
	amounts := make(map[string]int64)
	var total int64
	for _, product := range products {
		if len(product.PurchaseDate) < 7 || product.PurchaseDate[:7] != currentMonth || product.CostCurrency != "JPY" {
			continue
		}
		name := product.SupplierName
		if name == "" {
			name = "仕入先未設定"
		}
		amounts[name] += product.CostAmountMinor
		total += product.CostAmountMinor
	}
	if total == 0 {
		return nil
	}
	type pair struct {
		name   string
		amount int64
	}
	pairs := make([]pair, 0, len(amounts))
	for name, amount := range amounts {
		pairs = append(pairs, pair{name: name, amount: amount})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].amount == pairs[j].amount {
			return pairs[i].name < pairs[j].name
		}
		return pairs[i].amount > pairs[j].amount
	})
	if len(pairs) > 5 {
		other := pair{name: "その他"}
		for _, item := range pairs[4:] {
			other.amount += item.amount
		}
		pairs = append(pairs[:4], other)
	}

	colors := []string{"#2f88bd", "#ef821c", "#28ad64", "#8e44ad", "#eb4938"}
	slices := make([]dashboardSupplierSlice, 0, len(pairs))
	offset := 0.0
	percentTotal := 0
	for index, item := range pairs {
		dash := float64(item.amount) * 100 / float64(total)
		percent := int(dash + 0.5)
		if index == len(pairs)-1 {
			percent = 100 - percentTotal
		}
		slices = append(slices, dashboardSupplierSlice{
			Name: item.name, Count: item.amount, Percent: percent,
			Dash: fmt.Sprintf("%.3f", dash), Offset: fmt.Sprintf("%.3f", -offset),
			Color: colors[index], Class: fmt.Sprintf("supplier-color-%d", index+1),
		})
		offset += dash
		percentTotal += percent
	}
	return slices
}

func buildMonthBars(now time.Time, totals map[string]int64) []dashboardMonthBar {
	maximum := dashboardChartMaximum(now, totals)
	bars := make([]dashboardMonthBar, 0, 6)
	for index := 0; index < 6; index++ {
		month := now.AddDate(0, index-5, 0)
		total := totals[month.Format("2006-01")]
		height := 0
		if maximum > 0 && total > 0 {
			height = int(total * 165 / maximum)
			if height < 4 {
				height = 4
			}
		}
		x := 76 + index*94
		bars = append(bars, dashboardMonthBar{
			Label: fmt.Sprintf("%d月", int(month.Month())), AmountText: "¥" + formatInteger(total),
			X: x, Y: 205 - height, Height: height, LabelX: x + 28,
		})
	}
	return bars
}

func buildGridlines(totals map[string]int64, now time.Time) []dashboardGridline {
	maximum := dashboardChartMaximum(now, totals)
	lines := make([]dashboardGridline, 0, 5)
	for index := 0; index < 5; index++ {
		value := maximum * int64(4-index) / 4
		lines = append(lines, dashboardGridline{Y: 40 + index*41, Label: formatManYen(value)})
	}
	return lines
}

func dashboardChartMaximum(now time.Time, totals map[string]int64) int64 {
	var maximum int64
	for index := 0; index < 6; index++ {
		total := totals[now.AddDate(0, index-5, 0).Format("2006-01")]
		if total > maximum {
			maximum = total
		}
	}
	if maximum == 0 {
		return 1_000_000
	}
	const unit int64 = 1_000_000
	return ((maximum + unit - 1) / unit) * unit
}

func formatManYen(value int64) string {
	if value == 0 {
		return "¥0"
	}
	if value%10_000 == 0 {
		return fmt.Sprintf("¥%d万", value/10_000)
	}
	return fmt.Sprintf("¥%.1f万", float64(value)/10_000)
}
