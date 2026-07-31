package database

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestPhase8PublishedLineupRemainsSnapshotUntilBatchPublish(t *testing.T) {
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
	if err != nil || len(companies) == 0 {
		t.Fatalf("companies=%d err=%v", len(companies), err)
	}
	productRows, err := store.db.QueryContext(ctx, `
		SELECT id FROM products WHERE organization_id=? AND deleted_at IS NULL
		ORDER BY product_code LIMIT 1`, "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	var productIDs []string
	for productRows.Next() {
		var id string
		if err := productRows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		productIDs = append(productIDs, id)
	}
	productRows.Close()
	if len(productIDs) < 1 {
		t.Fatalf("products=%d", len(productIDs))
	}
	box := boxes[9]
	if _, err := store.db.ExecContext(ctx, `
		DELETE FROM guest_box_products WHERE organization_id=? AND box_id=?`,
		"org_preview", box.ID); err != nil {
		t.Fatal(err)
	}
	for _, productID := range productIDs {
		if _, err := store.db.ExecContext(ctx, `
			UPDATE products SET inventory_status='in_stock',publication_status='public'
			WHERE organization_id=? AND id=?`, "org_preview", productID); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AddGuestBoxProduct(ctx, "org_preview", box.ID, productIDs[0], "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveGuestBoxDraft(ctx, "org_preview", companies[0].ID, box.ID, "usr_admin", true); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishGuestBoxSnapshot(ctx, "org_preview", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveGuestBoxProduct(ctx, "org_preview", box.ID, productIDs[0]); err != nil {
		t.Fatal(err)
	}
	visible, err := store.PublicProductsForGuest(ctx, "PREVIEW", companies[0].Code, PublicProductFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsPublicProduct(visible, productIDs[0]) {
		t.Fatalf("draft removal changed the published snapshot for %q", productIDs[0])
	}
	if err := store.SaveGuestBoxDraft(ctx, "org_preview", companies[0].ID, box.ID, "usr_admin", false); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishGuestBoxSnapshot(ctx, "org_preview", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	visible, err = store.PublicProductsForGuest(ctx, "PREVIEW", companies[0].Code, PublicProductFilter{})
	if err != nil || containsPublicProduct(visible, productIDs[0]) {
		t.Fatalf("removed product remained after republish: visible=%+v err=%v", visible, err)
	}
	if err := store.AddGuestBoxProduct(ctx, "org_preview", box.ID, productIDs[0], "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveGuestBoxDraft(ctx, "org_preview", companies[0].ID, box.ID, "usr_admin", true); err != nil {
		t.Fatal(err)
	}
	visible, err = store.PublicProductsForGuest(ctx, "PREVIEW", companies[0].Code, PublicProductFilter{})
	if err != nil || containsPublicProduct(visible, productIDs[0]) {
		t.Fatalf("draft addition leaked before republish: visible=%+v err=%v", visible, err)
	}
	if err := store.PublishGuestBoxSnapshot(ctx, "org_preview", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	visible, err = store.PublicProductsForGuest(ctx, "PREVIEW", companies[0].Code, PublicProductFilter{})
	if err != nil || !containsPublicProduct(visible, productIDs[0]) {
		t.Fatalf("republished product missing: visible=%+v err=%v", visible, err)
	}
}

func containsPublicProduct(products []PublicProduct, id string) bool {
	for _, product := range products {
		if product.ID == id {
			return true
		}
	}
	return false
}

func TestPhase8ManagedPasswordOperationsAreTenantScopedAndEffective(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	created, err := store.CreateManagedUser(ctx, ManagedUserInput{
		OrganizationID: "org_preview", Username: "phase8@example.jp",
		DisplayName: "Phase 8", Role: RoleWorker, Password: "initial-phase8-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(ctx, "PREVIEW", created.Username, "initial-phase8-password"); err != nil {
		t.Fatalf("new credential does not authenticate: %v", err)
	}
	if err := store.ChangeManagedUserPassword(ctx, "org_other", created.ID, "changed-phase8-password"); err != ErrManagedUserNotFound {
		t.Fatalf("cross-tenant password change error=%v", err)
	}
	if err := store.ChangeManagedUserPassword(ctx, "org_preview", created.ID, "changed-phase8-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(ctx, "PREVIEW", created.Username, "changed-phase8-password"); err != nil {
		t.Fatalf("changed credential does not authenticate: %v", err)
	}
	if err := store.DeleteManagedUser(ctx, "org_preview", created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(ctx, "PREVIEW", created.Username, "changed-phase8-password"); err == nil {
		t.Fatal("deleted managed user still authenticates")
	}
}

func TestPhase8GuestPasswordRotationChangesCredentialHashes(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(ctx); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.GuestCredentials(ctx, "org_preview")
	if err != nil || len(credentials) != 4 {
		t.Fatalf("credentials=%d err=%v", len(credentials), err)
	}
	count, err := store.RotateGuestPasswords(ctx, "org_preview", "usr_admin", "rotated-phase8-password")
	if err != nil || count != 4 {
		t.Fatalf("rotated=%d err=%v", count, err)
	}
	var hash string
	if err := store.db.QueryRowContext(ctx, `
		SELECT password_hash FROM guest_credentials
		WHERE organization_id=? AND company_id=?`,
		"org_preview", credentials[0].CompanyID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "rotated-phase8-password") {
		t.Fatal("guest password hash was not updated")
	}
}

func TestPhase8SalesDestinationsAndGuestCompaniesShareMockCatalog(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedMasterPreview(ctx); err != nil {
		t.Fatal(err)
	}

	wantCodes := []string{"B001", "B002", "B003", "B004"}
	wantNames := []string{"ウォッチマート", "タイムレス商会", "ラグジュアリーアイランド", "クロノス東京"}
	records, err := store.MasterRecords(ctx, "org_preview", "sales-destinations")
	if err != nil {
		t.Fatal(err)
	}
	var recordCodes, recordNames []string
	for _, record := range records {
		recordCodes = append(recordCodes, record.Code)
		recordNames = append(recordNames, record.Name)
	}
	if !reflect.DeepEqual(recordCodes, wantCodes) || !reflect.DeepEqual(recordNames, wantNames) {
		t.Fatalf("sales destinations codes=%v names=%v", recordCodes, recordNames)
	}

	companies, err := store.GuestCompanies(ctx, "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	var companyCodes, companyNames []string
	for _, company := range companies {
		companyCodes = append(companyCodes, company.Code)
		companyNames = append(companyNames, company.Name)
	}
	if !reflect.DeepEqual(companyCodes, wantCodes) || !reflect.DeepEqual(companyNames, wantNames) {
		t.Fatalf("guest companies codes=%v names=%v", companyCodes, companyNames)
	}
}

func TestPhase8SalesDestinationMigrationUpgradesExistingPreviewRows(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedMasterPreview(ctx); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 4; index++ {
		if _, err := store.db.ExecContext(ctx, `
			UPDATE master_records SET record_code=?,name='旧販売先'
			WHERE organization_id='org_preview' AND category='sales-destinations' AND id=?`,
			fmt.Sprintf("OLD-%d", index), fmt.Sprintf("mst_preview_sales_destinations_%03d", index)); err != nil {
			t.Fatal(err)
		}
	}
	for index := 1; index <= 3; index++ {
		if _, err := store.db.ExecContext(ctx, `
			UPDATE guest_companies SET company_code=?,name='旧ゲスト企業'
			WHERE organization_id='org_preview' AND id=?`,
			fmt.Sprintf("GUEST-%03d", index), fmt.Sprintf("gco_preview_%03d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `
		DELETE FROM schema_migrations WHERE version='000023_phase8_sales_destinations'`); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	records, err := store.MasterRecords(ctx, "org_preview", "sales-destinations")
	if err != nil {
		t.Fatal(err)
	}
	companies, err := store.GuestCompanies(ctx, "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 || len(companies) != 4 ||
		records[0].Code != "B001" || records[3].Code != "B004" ||
		companies[0].Code != "B001" || companies[3].Code != "B004" {
		t.Fatalf("records=%+v companies=%+v", records, companies)
	}
}
