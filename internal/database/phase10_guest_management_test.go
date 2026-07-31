package database

import (
	"context"
	"testing"
)

func TestPhase10GuestPublicationFreezesFullProductAndBoxSnapshot(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(ctx); err != nil {
		t.Fatal(err)
	}
	boxes, err := store.GuestBoxes(ctx, "org_preview")
	if err != nil || len(boxes) != 10 {
		t.Fatalf("boxes=%d err=%v", len(boxes), err)
	}
	companies, err := store.GuestCompanies(ctx, "org_preview")
	if err != nil || len(companies) != 4 {
		t.Fatalf("companies=%d err=%v", len(companies), err)
	}
	var productID, productCode, brand, model, reference string
	var price int64
	if err := store.db.QueryRowContext(ctx, `
		SELECT id,product_code,brand,product_type,model_number,base_sale_price_minor
		FROM products WHERE organization_id='org_preview' AND deleted_at IS NULL
		ORDER BY product_code LIMIT 1`).
		Scan(&productID, &productCode, &brand, &model, &reference, &price); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE products SET inventory_status='sold',publication_status='private'
		WHERE organization_id='org_preview' AND id=?`, productID); err != nil {
		t.Fatal(err)
	}
	image, err := store.AddProductImage(ctx, ProductImage{
		ProductID: productID, StoragePath: "phase10/original.jpg", OriginalName: "original.jpg",
		ContentType: "image/jpeg", SizeBytes: 1234,
	}, "org_preview", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	box := boxes[9]
	if err := store.RenameGuestBox(ctx, "org_preview", box.ID, "usr_admin", "公開時BOX名"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddGuestBoxProduct(ctx, "org_preview", box.ID, productID, "usr_admin"); err != nil {
		t.Fatalf("private/sold product must remain selectable: %v", err)
	}
	if err := store.SaveGuestBoxDraft(ctx, "org_preview", companies[0].ID, box.ID, "usr_admin", true); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishGuestBoxSnapshot(ctx, "org_preview", "usr_admin"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.ExecContext(ctx, `
		UPDATE products SET product_code='CHANGED',brand='変更後',product_type='変更後モデル',
		  model_number='CHANGED-REF',base_sale_price_minor=1,condition_text='変更後状態'
		WHERE organization_id='org_preview' AND id=?`, productID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE product_images SET storage_path='phase10/changed.jpg' WHERE id=?`, image.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameGuestBox(ctx, "org_preview", box.ID, "usr_admin", "変更後BOX名"); err != nil {
		t.Fatal(err)
	}

	public, err := store.PublicProductsForGuest(ctx, "PREVIEW", companies[0].Code, PublicProductFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var got *PublicProduct
	for index := range public {
		if public[index].ID == productID {
			got = &public[index]
			break
		}
	}
	if got == nil {
		t.Fatalf("snapshot product %s is not visible", productID)
	}
	if got.ProductCode != productCode || got.Brand != brand || got.ProductType != model ||
		got.ModelNumber != reference || got.SalePriceMinor != price {
		t.Fatalf("live product mutation leaked into snapshot: %+v", *got)
	}
	if len(got.Images) != 1 || got.Images[0].StoragePath != "phase10/original.jpg" {
		t.Fatalf("live image mutation leaked into snapshot: %+v", got.Images)
	}
	var snapBoxName string
	if err := store.db.QueryRowContext(ctx, `
		SELECT box_name FROM guest_box_published_products
		WHERE organization_id='org_preview' AND company_id=? AND box_id=? AND product_id=?`,
		companies[0].ID, box.ID, productID).Scan(&snapBoxName); err != nil {
		t.Fatal(err)
	}
	if snapBoxName != "公開時BOX名" {
		t.Fatalf("box snapshot=%q", snapBoxName)
	}
}

func TestPhase10BulkAddMovesProductsToExactlyOneBox(t *testing.T) {
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
	rows, err := store.db.QueryContext(ctx, `
		SELECT id FROM products WHERE organization_id='org_preview' AND deleted_at IS NULL
		ORDER BY product_code LIMIT 2`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 1 {
		second, createErr := store.CreateSingleProduct(ctx, SingleProductInput{
			OrganizationID: "org_preview", SupplierID: "sup_001", PurchaseDate: "2026-07-30",
			SKU: "PHASE10-002", Brand: "テストブランド", ModelNumber: "REF-PHASE10",
			ProductType: "テストモデル", CostAmountMinor: 100000, CostCurrency: "JPY",
			BaseSalePriceMinor: 150000, BaseSaleCurrency: "JPY", Condition: "美品",
			CreatedBy: "usr_admin",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		ids = append(ids, second.ID)
	}
	if len(ids) != 2 {
		t.Fatalf("products=%d", len(ids))
	}
	if err := store.AddGuestBoxProducts(ctx, "org_preview", boxes[0].ID, ids, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddGuestBoxProducts(ctx, "org_preview", boxes[1].ID, ids, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	var oldCount, newCount, totalCount int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM guest_box_products
		WHERE organization_id='org_preview' AND box_id=? AND product_id IN (?,?)`,
		boxes[0].ID, ids[0], ids[1]).Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM guest_box_products
		WHERE organization_id='org_preview' AND box_id=? AND product_id IN (?,?)`,
		boxes[1].ID, ids[0], ids[1]).Scan(&newCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM guest_box_products
		WHERE organization_id='org_preview' AND product_id IN (?,?)`,
		ids[0], ids[1]).Scan(&totalCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 || newCount != 2 || totalCount != 2 {
		t.Fatalf("old=%d new=%d total=%d", oldCount, newCount, totalCount)
	}
}
