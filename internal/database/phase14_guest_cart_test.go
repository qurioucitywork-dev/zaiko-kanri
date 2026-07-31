package database

import (
	"context"
	"errors"
	"testing"
)

func TestPhase14CreatePurchaseRequestGroupPersistsMultipleDistinctProducts(t *testing.T) {
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
		ProductIDs:       []string{products[0].ID, products[1].ID, products[0].ID},
		GuestName:        "ゲスト会社",
		GuestEmail:       "guest@example.com",
		Message:          "一括購入依頼",
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.ID == "" || len(group.Items) != 2 {
		t.Fatalf("group=%+v", group)
	}
	for _, item := range group.Items {
		if item.RequestGroupID != group.ID {
			t.Fatalf("request group id=%q want=%q", item.RequestGroupID, group.ID)
		}
	}
	groups, err := store.PurchaseRequestGroups(ctx, "org_preview", "pending")
	if err != nil {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
	var persisted *PurchaseRequestGroup
	for index := range groups {
		if groups[index].ID == group.ID {
			persisted = &groups[index]
			break
		}
	}
	if persisted == nil || len(persisted.Items) != 2 {
		t.Fatalf("group %q not persisted as two items: %+v", group.ID, groups)
	}
}

func TestPhase14CreatePurchaseRequestGroupIsAtomicWhenAProductIsUnavailable(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := requestTestProduct(t, store)
	if _, err := store.CreatePurchaseRequestGroup(ctx, PurchaseRequestGroupInput{
		OrganizationCode: "PREVIEW",
		ProductIDs:       []string{product.ID, "not-published"},
		GuestName:        "ゲスト会社",
		GuestEmail:       "guest@example.com",
	}); !errors.Is(err, ErrProductNotAvailable) {
		t.Fatalf("error=%v", err)
	}
	requests, err := store.PurchaseRequests(ctx, "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 0 {
		t.Fatalf("partially persisted requests=%+v", requests)
	}
}
