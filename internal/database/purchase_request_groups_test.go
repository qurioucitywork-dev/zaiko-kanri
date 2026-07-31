package database

import (
	"sync"
	"testing"
	"time"
)

func TestPurchaseRequestGroupsUsePersistentIDUnderConcurrentCreation(t *testing.T) {
	store := testStore(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(t.Context(), "org_preview", ProductFilter{Status: "in_stock", Page: 1, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(products) < 1 {
		t.Fatal("need at least one in-stock product")
	}
	if err := store.SetProductPublication(
		t.Context(), "org_preview", products[0].ID, "usr_admin", "public",
	); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 7, 30, 12, 34, 20, 0, time.UTC)
	store.now = func() time.Time { return fixed }

	start := make(chan struct{})
	results := make(chan PurchaseRequest, 2)
	errs := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			request, createErr := store.CreatePurchaseRequest(t.Context(), PurchaseRequestInput{
				OrganizationCode: "PREVIEW",
				RequestGroupID:   "persistent-group-a",
				ProductID:        products[0].ID,
				GuestName:        "同一購入者",
				GuestEmail:       "same-minute@example.com",
				Message:          "同一分でもIDだけで結合",
			})
			results <- request
			errs <- createErr
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errs)
	for createErr := range errs {
		if createErr != nil {
			t.Fatalf("concurrent purchase request: %v", createErr)
		}
	}
	for request := range results {
		if request.RequestGroupID != "persistent-group-a" {
			t.Fatalf("request group=%q", request.RequestGroupID)
		}
	}

	separate, err := store.CreatePurchaseRequest(t.Context(), PurchaseRequestInput{
		OrganizationCode: "PREVIEW",
		RequestGroupID:   "persistent-group-b",
		ProductID:        products[0].ID,
		GuestName:        "同一購入者",
		GuestEmail:       "same-minute@example.com",
		Message:          "同一分でもIDだけで結合",
	})
	if err != nil {
		t.Fatal(err)
	}
	if separate.RequestedAt != fixed {
		t.Fatalf("requested at=%v, want %v", separate.RequestedAt, fixed)
	}

	groups, err := store.PurchaseRequestGroups(t.Context(), "org_preview", "pending")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups=%+v", groups)
	}
	sizes := map[string]int{}
	for _, group := range groups {
		sizes[group.ID] = len(group.Items)
	}
	if sizes["persistent-group-a"] != 2 || sizes["persistent-group-b"] != 1 {
		t.Fatalf("group sizes=%v", sizes)
	}
}
