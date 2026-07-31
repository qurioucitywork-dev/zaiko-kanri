package database

import (
	"context"
	"testing"
)

func TestPurchaseRequestGroupOnlyCreatesShipmentAfterEveryItemIsDecided(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedGuestCatalogPreview(ctx); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil || len(products) < 2 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	for _, product := range products[:2] {
		if err := store.SetProductPublication(ctx, "org_preview", product.ID, "usr_admin", "public"); err != nil {
			t.Fatal(err)
		}
	}
	group, err := store.CreatePurchaseRequestGroup(ctx, PurchaseRequestGroupInput{
		OrganizationCode: "PREVIEW",
		ProductIDs:       []string{products[0].ID, products[1].ID},
		GuestName:        "クロノス東京",
		GuestEmail:       "guest@example.com",
		Message:          "至急確認お願いします",
	})
	if err != nil {
		t.Fatal(err)
	}

	input := CreateShipmentInput{
		OrganizationID:         "org_preview",
		PurchaseRequestGroupID: group.ID,
		ShipmentDate:           "2026-07-31",
		RecipientName:          "クロノス東京",
		CreatedBy:              "usr_admin",
		Lines: []ShipmentLineInput{
			{ProductID: products[0].ID, Quantity: 1},
			{ProductID: products[1].ID, Quantity: 1},
		},
	}
	if _, err := store.CreateShipmentDraft(ctx, input); err == nil {
		t.Fatal("shipment must not be created while request items remain pending")
	}
	for _, item := range group.Items {
		if _, err := store.ApprovePurchaseRequest(ctx, "org_preview", item.ID, "usr_admin"); err != nil {
			t.Fatal(err)
		}
	}
	shipment, err := store.CreateShipmentDraft(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var linkedGroupID string
	if err := store.db.QueryRowContext(ctx,
		`SELECT purchase_request_group_id FROM shipment_slips WHERE id=?`,
		shipment.ID,
	).Scan(&linkedGroupID); err != nil {
		t.Fatal(err)
	}
	if linkedGroupID != group.ID {
		t.Fatalf("purchase_request_group_id=%q want=%q", linkedGroupID, group.ID)
	}
}

func TestPurchaseRequestGroupShipmentRejectsUnapprovedProducts(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedGuestCatalogPreview(ctx); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil || len(products) < 2 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	for _, product := range products[:2] {
		if err := store.SetProductPublication(ctx, "org_preview", product.ID, "usr_admin", "public"); err != nil {
			t.Fatal(err)
		}
	}
	group, err := store.CreatePurchaseRequestGroup(ctx, PurchaseRequestGroupInput{
		OrganizationCode: "PREVIEW",
		ProductIDs:       []string{products[0].ID},
		GuestName:        "クロノス東京",
		GuestEmail:       "guest@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApprovePurchaseRequest(ctx, "org_preview", group.Items[0].ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateShipmentDraft(ctx, CreateShipmentInput{
		OrganizationID:         "org_preview",
		PurchaseRequestGroupID: group.ID,
		ShipmentDate:           "2026-07-31",
		RecipientName:          "クロノス東京",
		CreatedBy:              "usr_admin",
		Lines: []ShipmentLineInput{
			{ProductID: products[1].ID, Quantity: 1},
		},
	}); err == nil {
		t.Fatal("shipment must reject a product that was not approved in the request group")
	}
}
