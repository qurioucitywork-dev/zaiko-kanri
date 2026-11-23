package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestPurchaseStaffDetailsPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_RESUBMIT_NOCHANGE_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_resubmit_nochange_test") {
		t.Fatal("non-test database rejected")
	}
	ctx := context.Background()
	r, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(r.Migrate(ctx))
	must(r.SeedPreviewIdentity(ctx, "isolated-test-admin", "isolated-test-worker"))
	var org string
	must(r.db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	name, inv, over, tag, dom := "商品表示", "在庫表示", "Overseas", "T", "国内"
	row, err := r.SavePurchaseStaff(ctx, org, "", PurchaseStaffInput{StaffCode: "9001", DisplayName: &name, InventoryDetail: &inv, OverseasDetail: &over, TagDetail: &tag, DomesticDetail: &dom})
	must(err)
	if row.InventoryDetail != inv || row.OverseasDetail != over || row.TagDetail != tag || row.DomesticDetail != dom {
		t.Fatalf("details lost: %+v", row)
	}
	changed := " 在庫変更 "
	blank := ""
	updated, err := r.SavePurchaseStaff(ctx, org, row.ID, PurchaseStaffInput{InventoryDetail: &changed, TagDetail: &blank})
	must(err)
	if updated.ID != row.ID || updated.StaffCode != row.StaffCode || updated.DisplayName != name || updated.InventoryDetail != "在庫変更" || updated.TagDetail != "" || updated.DomesticDetail != dom {
		t.Fatalf("partial update incorrect: %+v", updated)
	}
	list, err := r.PurchaseStaff(ctx, org)
	must(err)
	found := false
	for _, v := range list {
		if v.ID == row.ID {
			found = true
			if v.InventoryDetail != "在庫変更" || v.OverseasDetail != over {
				t.Fatal("list lost details")
			}
		}
	}
	if !found {
		t.Fatal("missing record")
	}
	if _, err = r.SavePurchaseStaff(ctx, "other-org", row.ID, PurchaseStaffInput{DomesticDetail: &dom}); err == nil {
		t.Fatal("cross-org update allowed")
	}
	long := strings.Repeat("あ", 201)
	if _, err = r.SavePurchaseStaff(ctx, org, row.ID, PurchaseStaffInput{DomesticDetail: &long}); err == nil {
		t.Fatal("oversize detail allowed")
	}
	if _, err = r.SavePurchaseStaff(ctx, org, row.ID, PurchaseStaffInput{StaffCode: "9002", DisplayName: &name}); err == nil {
		t.Fatal("stable code changed")
	}
}
