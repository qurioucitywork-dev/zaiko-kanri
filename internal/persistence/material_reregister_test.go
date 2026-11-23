package persistence

import (
	"context"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestMaterialDeleteAndReregister(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	err = db.Exec("CREATE TABLE materials (id TEXT PRIMARY KEY, organization_id TEXT, code TEXT, name TEXT, meaning TEXT, is_active BOOLEAN, sort_order INTEGER, created_by TEXT, updated_by TEXT, created_at DATETIME, updated_at DATETIME, UNIQUE(organization_id,code))").Error
	if err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "sqlite"}
	ctx := context.Background()
	original, err := repo.CreateMasterItem(ctx, "org", "admin", "material", "2", "old", "old detail", "", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CreateMasterItem(ctx, "org", "admin", "material", "2", "must not overwrite", "bad", "", 0, "", ""); err == nil {
		t.Fatal("active duplicate was accepted")
	}
	unchanged, err := repo.MasterItem(ctx, "org", "material", original.ID)
	if err != nil || unchanged.Name != "old" {
		t.Fatalf("active row modified: %+v, %v", unchanged, err)
	}
	inactive := false
	if _, err = repo.UpdateMasterItem(ctx, "org", "admin", "material", original.ID, nil, nil, nil, &inactive, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	items, err := repo.MasterItems(ctx, "org", "material", false)
	if err != nil || len(items) != 0 {
		t.Fatalf("deleted material still visible: %+v %v", items, err)
	}
	saved, err := repo.CreateMasterItem(ctx, "org", "admin", "material", "2", "K18YG", "金（ホワイトゴールド）　含有率75％", "", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != original.ID || !saved.IsActive || saved.Name != "K18YG" || saved.Meaning != "金（ホワイトゴールド）　含有率75％" {
		t.Fatalf("incorrect re-registration: %+v", saved)
	}
	if _, err = repo.CreateMasterItem(ctx, "org2", "admin", "materials", "2", "other org", "", "", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	items, err = repo.MasterItems(ctx, "org", "material", false)
	if err != nil || len(items) != 1 || items[0].Name != "K18YG" {
		t.Fatalf("incorrect active list: %+v %v", items, err)
	}
}
