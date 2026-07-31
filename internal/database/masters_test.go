package database

import (
	"context"
	"errors"
	"testing"
)

func TestMasterRecordLifecycleAndSupplierIntegration(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(ctx); err != nil {
		t.Fatal(err)
	}
	brands, err := store.MasterRecords(ctx, "org_preview", "brands")
	if err != nil || len(brands) != 10 {
		t.Fatalf("seed brands=%d err=%v", len(brands), err)
	}
	suppliers, err := store.MasterRecords(ctx, "org_preview", "suppliers")
	if err != nil || len(suppliers) != 5 {
		t.Fatalf("seed suppliers=%d err=%v", len(suppliers), err)
	}

	record, err := store.CreateMasterRecord(ctx, SaveMasterInput{
		OrganizationID: "org_preview", Category: "brands", Code: "brd-011",
		Name: "テストブランド", ActorUserID: "usr_admin",
	})
	if err != nil || record.Code != "BRD-011" {
		t.Fatalf("create record=%+v err=%v", record, err)
	}
	if _, err := store.CreateMasterRecord(ctx, SaveMasterInput{
		OrganizationID: "org_preview", Category: "brands", Code: "BRD-011",
		Name: "重複", ActorUserID: "usr_admin",
	}); !errors.Is(err, ErrMasterCodeExists) {
		t.Fatalf("duplicate error=%v", err)
	}
	if err := store.UpdateMasterRecord(ctx, record.ID, SaveMasterInput{
		OrganizationID: "org_preview", Category: "brands", Code: "BRD-011",
		Name: "更新ブランド", ActorUserID: "usr_admin",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteMasterRecord(ctx, "org_preview", "brands", record.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	brands, err = store.MasterRecords(ctx, "org_preview", "brands")
	if err != nil || len(brands) != 10 {
		t.Fatalf("brands after delete=%d err=%v", len(brands), err)
	}
}
