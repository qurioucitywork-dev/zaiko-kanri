package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestAccessoryDisplayDetailsPostgres(t *testing.T) {
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
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(r.Migrate(ctx))
	must(r.SeedPreviewIdentity(ctx, "test-admin-password", "test-worker-password"))
	var org, actor string
	must(r.db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	must(r.db.Raw("SELECT id FROM users WHERE organization_id=? AND role_key='admin' LIMIT 1", org).Scan(&actor).Error)
	inv, over, tag, dom := "B（仕入返品伝票にも表示）", "（表記無し）", "・", "（表記無し）"
	item, err := r.CreateMasterItem(ctx, org, actor, "accessory", "0001", "RX箱", "", "", 6, "", "", MasterDisplayDetailsInput{&inv, &over, &tag, &dom})
	must(err)
	if item.InventoryDetail != inv || item.OverseasDetail != over || item.TagDetail != tag || item.DomesticDetail != dom {
		t.Fatalf("create details: %+v", item)
	}
	change, blank := "タグ表示", ""
	updated, err := r.UpdateMasterItem(ctx, org, actor, "accessory", item.ID, nil, nil, nil, nil, nil, nil, nil, MasterDisplayDetailsInput{TagDetail: &change, InventoryDetail: &blank})
	must(err)
	if updated.ID != item.ID || updated.Code != "0001" || updated.TagDetail != change || updated.InventoryDetail != "" || updated.OverseasDetail != over || updated.DomesticDetail != dom {
		t.Fatalf("partial update: %+v", updated)
	}
	rows, err := r.MasterItems(ctx, org, "accessory", false)
	must(err)
	if len(rows) != 1 || rows[0].TagDetail != change {
		t.Fatal("details missing in list")
	}
	if _, err = r.UpdateMasterItem(ctx, "other-org", actor, "accessory", item.ID, nil, nil, nil, nil, nil, nil, nil, MasterDisplayDetailsInput{TagDetail: &change}); err == nil {
		t.Fatal("cross-org update allowed")
	}
	long := strings.Repeat("a", 201)
	if _, err = r.UpdateMasterItem(ctx, org, actor, "accessory", item.ID, nil, nil, nil, nil, nil, nil, nil, MasterDisplayDetailsInput{TagDetail: &long}); err == nil {
		t.Fatal("oversize accepted")
	}
	old, err := r.CreateMasterItem(ctx, org, actor, "accessory", "ACC-9003", "Guarantee(NC)", "中期カードギャラ", "", 9003, "NC", "月")
	must(err)
	must(r.db.Table("accessories").Where("id=?", old.ID).Updates(map[string]any{"is_active": false, "is_legacy": true}).Error)
	active, err := r.MasterItems(ctx, org, "accessory", false)
	must(err)
	if len(active) != 1 {
		t.Fatal("retired entry leaked into new catalog")
	}
	ids, names, err := lookupAccessories(r.db, org, []string{"ACC-9003"})
	must(err)
	if len(ids) != 1 || ids[0] != old.ID || names[0] != old.Name {
		t.Fatal("existing product reference lost")
	}
	values, err := normalizeAccessorySupplements(r.db, org, []string{"ACC-9003"}, AccessorySupplementValues{"ACC-9003": "10"})
	must(err)
	if values["ACC-9003"] != "10" {
		t.Fatal("legacy supplement lost")
	}
	if _, _, err = lookupAccessories(r.db, "other-org", []string{"ACC-9003"}); err == nil {
		t.Fatal("cross-org legacy lookup allowed")
	}
}
