package web

import (
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestBuildPurchaseRequestGroupViewsCountsGroupsAndEnablesShipmentAfterDecisions(t *testing.T) {
	requestedAt := time.Date(2026, 3, 23, 14, 32, 0, 0, time.Local)
	groups := []database.PurchaseRequestGroup{
		{
			ID: "prg_001",
			Items: []database.PurchaseRequest{
				{
					ID: "req_001", RequestNumber: "RQ-2026-0001", GuestName: "クロノス東京",
					Message: "至急確認お願いします", RequestedAt: requestedAt, Status: "approved",
					SalePriceMinor: 1_180_000, SaleCurrency: "JPY",
				},
				{
					ID: "req_002", RequestNumber: "RQ-2026-0002", GuestName: "クロノス東京",
					Message: "至急確認お願いします", RequestedAt: requestedAt, Status: "pending",
					SalePriceMinor: 780_000, SaleCurrency: "JPY",
				},
			},
		},
	}

	views, pendingGroups := buildPurchaseRequestGroupViews(groups)
	if len(views) != 1 || pendingGroups != 1 {
		t.Fatalf("views=%+v pendingGroups=%d", views, pendingGroups)
	}
	if views[0].DisplayNumber != "PR-001" || views[0].ItemCount != 2 ||
		views[0].ApprovedCount != 1 || views[0].ApprovedTotal != 1_180_000 ||
		views[0].Total != 1_960_000 || views[0].CanCreateShip {
		t.Fatalf("pending group view=%+v", views[0])
	}

	groups[0].Items[1].Status = "rejected"
	views, pendingGroups = buildPurchaseRequestGroupViews(groups)
	if pendingGroups != 0 || !views[0].CanCreateShip || views[0].Status != "approved" {
		t.Fatalf("decided group view=%+v pendingGroups=%d", views[0], pendingGroups)
	}
}

func TestBuildPurchaseRequestGroupViewsUsesProductPricesAndMasterRate(t *testing.T) {
	groups := []database.PurchaseRequestGroup{{
		ID: "prg_price",
		Items: []database.PurchaseRequest{{
			ID: "req_price", RequestNumber: "RQ-2026-0003", GuestName: "クロノス東京",
			Status: "pending", CostAmountMinor: 850_000, CostCurrency: "JPY",
			SalePriceMinor: 10_000, SaleCurrency: "USD",
		}},
	}}
	rate := database.ExchangeRate{RateScaled: 150 * database.RateScale, Scale: database.RateScale}
	views, pending := buildPurchaseRequestGroupViewsWithRate(groups, rate)
	if len(views) != 1 || pending != 1 || len(views[0].Items) != 1 {
		t.Fatalf("unexpected views: %+v pending=%d", views, pending)
	}
	item := views[0].Items[0]
	if !item.PurchaseJPYReady || item.PurchasePriceJPY != 850_000 ||
		!item.SaleUSDReady || item.SalePriceUSD != 10_000 ||
		!item.SaleJPYReady || item.SalePriceJPY != 1_500_000 || views[0].Total != 1_500_000 {
		t.Fatalf("prices were not normalized: item=%+v group=%+v", item, views[0])
	}
}
