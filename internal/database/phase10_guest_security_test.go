package database

import (
	"context"
	"testing"
	"time"
)

func TestPhase10GuestSessionAndPublishedAssetsAreCompanyIsolated(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(ctx); err != nil {
		t.Fatal(err)
	}
	companies, err := store.GuestCompanies(ctx, "org_preview")
	if err != nil || len(companies) < 2 {
		t.Fatalf("companies=%d err=%v", len(companies), err)
	}
	boxes, err := store.GuestBoxes(ctx, "org_preview")
	if err != nil || len(boxes) == 0 {
		t.Fatalf("boxes=%d err=%v", len(boxes), err)
	}
	products, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	product := products[0]
	image, err := store.AddProductImage(ctx, ProductImage{
		ProductID: product.ID, StoragePath: "security/image.jpg", OriginalName: "image.jpg",
		ContentType: "image/jpeg", SizeBytes: 4,
	}, "org_preview", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddGuestBoxProduct(ctx, "org_preview", boxes[0].ID, product.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	for index, company := range companies {
		if err := store.SaveGuestBoxDraft(ctx, "org_preview", company.ID, boxes[0].ID, "usr_admin", index == 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.PublishGuestBoxSnapshot(ctx, "org_preview", "usr_admin"); err != nil {
		t.Fatal(err)
	}

	first, err := store.AuthenticateGuest(ctx, "PREVIEW", companies[0].Code, "guest-preview-2026")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AuthenticateGuest(ctx, "PREVIEW", companies[1].Code, "guest-preview-2026")
	if err != nil {
		t.Fatal(err)
	}
	token := first.CompanyID + ".opaque"
	if err := store.CreateGuestSession(ctx, first, token, time.Hour); err != nil {
		t.Fatal(err)
	}
	session, err := store.GuestSession(ctx, "PREVIEW", token)
	if err != nil || session.CompanyID != first.CompanyID {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	if _, err := store.GuestSession(ctx, "PREVIEW", second.CompanyID+".opaque"); err == nil {
		t.Fatal("tampered company prefix accepted")
	}
	if _, err := store.GuestPublishedProductImage(ctx, first.OrganizationID, first.CompanyID, image.ID); err != nil {
		t.Fatalf("published image unavailable: %v", err)
	}
	if _, err := store.GuestPublishedProductImage(ctx, second.OrganizationID, second.CompanyID, image.ID); err == nil {
		t.Fatal("image from another company was exposed")
	}

	baseRequest := PurchaseRequestInput{
		OrganizationCode: "PREVIEW", ProductID: product.ID, GuestName: "guest",
		GuestEmail: "guest@example.com", Message: "request",
	}
	denied := baseRequest
	denied.GuestCompanyID = second.CompanyID
	if _, err := store.CreatePurchaseRequest(ctx, denied); err != ErrProductNotAvailable {
		t.Fatalf("cross-company purchase err=%v", err)
	}
	allowed := baseRequest
	allowed.GuestCompanyID = first.CompanyID
	if _, err := store.CreatePurchaseRequest(ctx, allowed); err != nil {
		t.Fatalf("published-company purchase err=%v", err)
	}
}
