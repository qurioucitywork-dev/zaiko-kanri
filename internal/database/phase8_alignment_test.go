package database

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPhase8MasterAutoCodeAndCategoryRules(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	brand, err := store.CreateMasterRecord(ctx, SaveMasterInput{
		OrganizationID: "org_preview", Category: "brands", Name: "テストブランド",
		ActorUserID: "usr_admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(brand.Code, "BRD-") {
		t.Fatalf("auto code=%q", brand.Code)
	}
	accessory, err := store.CreateMasterRecord(ctx, SaveMasterInput{
		OrganizationID: "org_preview", Category: "accessories", Name: "warranty card",
		ActorUserID: "usr_admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accessory.Name != "WARRANTY CARD" {
		t.Fatalf("accessory name=%q", accessory.Name)
	}
	supplier, err := store.CreateMasterRecord(ctx, SaveMasterInput{
		OrganizationID: "org_preview", Category: "suppliers", Name: "テスト仕入先",
		Address: "東京都", Contact: "03-0000-0000", InvoiceRegistrationNumber: "T123",
		ActorUserID: "usr_admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if supplier.Address != "東京都" || supplier.Contact != "03-0000-0000" ||
		supplier.InvoiceRegistrationNumber != "T123" {
		t.Fatalf("supplier extra fields=%+v", supplier)
	}
}

func TestPhase8GuestBoxDraftPublishAndVisibility(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(ctx); err != nil {
		t.Fatal(err)
	}

	boxes, err := store.GuestBoxes(ctx, "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	if len(boxes) != 10 || boxes[0].Code != "BOX1" || boxes[9].Code != "BOX10" {
		t.Fatalf("fixed boxes=%+v", boxes)
	}
	companies, err := store.GuestCompanies(ctx, "org_preview")
	if err != nil || len(companies) == 0 {
		t.Fatalf("companies=%+v err=%v", companies, err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM guest_box_products WHERE organization_id=? AND box_id=?`, "org_preview", boxes[9].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveGuestBoxDraft(ctx, "org_preview", companies[0].ID, boxes[9].ID, "usr_admin", true); !errors.Is(err, ErrGuestBoxEmpty) {
		t.Fatalf("empty box draft error=%v", err)
	}

	products, err := store.Products(ctx, "org_preview", ProductFilter{Page: 1, PageSize: 20})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	product := products[0]
	if _, err := store.db.ExecContext(ctx, `
		UPDATE products SET inventory_status='in_stock',publication_status='public'
		WHERE id=? AND organization_id=?`, product.ID, "org_preview"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddGuestBoxProduct(ctx, "org_preview", boxes[9].ID, product.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveGuestBoxDraft(ctx, "org_preview", companies[0].ID, boxes[9].ID, "usr_admin", true); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishGuestBoxSnapshot(ctx, "org_preview", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	visible, err := store.PublicProductsForGuest(ctx, "PREVIEW", companies[0].Code, PublicProductFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range visible {
		found = found || item.ID == product.ID
	}
	if !found {
		t.Fatalf("published product %q is not visible: %+v", product.ID, visible)
	}
}

func TestPhase8ExchangeRateCardsAcceptOnlyPositiveSupportedCurrencies(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	for _, base := range []string{"USD", "EUR", "HKD", "CHF"} {
		if _, err := store.AddExchangeRate(ctx, "org_preview", base, "JPY", 123450000, "manual", "2026-07-30T12:00", "usr_admin"); err != nil {
			t.Fatalf("%s/JPY: %v", base, err)
		}
	}
	if _, err := store.AddExchangeRate(ctx, "org_preview", "USD", "JPY", 0, "manual", "2026-07-30T12:00", "usr_admin"); err == nil {
		t.Fatal("zero rate must be rejected")
	}
	if _, err := store.AddExchangeRate(ctx, "org_preview", "GBP", "JPY", 200000000, "manual", "2026-07-30T12:00", "usr_admin"); err == nil {
		t.Fatal("unsupported currency must be rejected")
	}
}
