package persistence

import (
	"context"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestAccessoryDetailsCreateAndUpdate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	if err = db.Exec("CREATE TABLE accessories (id TEXT PRIMARY KEY,organization_id TEXT,code TEXT,name TEXT,meaning TEXT,symbol TEXT NOT NULL DEFAULT '',supplement TEXT NOT NULL DEFAULT '',is_active BOOLEAN,sort_order INTEGER,created_by TEXT,updated_by TEXT,created_at DATETIME,updated_at DATETIME,UNIQUE(organization_id,code))").Error; err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "sqlite"}
	ctx := context.Background()
	item, err := repo.CreateMasterItem(ctx, "org", "admin", "accessory", "0001", "Guarantee(P)", "紙ギャラ", "", 0, " P ", " 初期補足 ")
	if err != nil || item.Code != "0001" || item.Name != "Guarantee(P)" || item.Meaning != "紙ギャラ" || item.Symbol != "P" || item.Supplement != "初期補足" {
		t.Fatalf("create: %+v %v", item, err)
	}
	detail := "詳細更新"
	symbol := " ○ "
	supplement := " 更新補足 "
	item, err = repo.UpdateMasterItem(ctx, "org", "admin", "accessories", item.ID, nil, &detail, nil, nil, nil, &symbol, &supplement)
	if err != nil || item.Meaning != detail || item.Symbol != "○" || item.Supplement != "更新補足" {
		t.Fatalf("update: %+v %v", item, err)
	}
	items, err := repo.MasterItems(ctx, "org", "accessories", false)
	if err != nil || len(items) != 1 || items[0].Meaning != detail || items[0].Symbol != "○" || items[0].Supplement != "更新補足" {
		t.Fatalf("list: %+v %v", items, err)
	}
	// Unrelated edits preserve the symbol; an explicit empty value clears it.
	item, err = repo.UpdateMasterItem(ctx, "org", "admin", "accessories", item.ID, nil, &detail, nil, nil, nil, nil, nil)
	if err != nil || item.Symbol != "○" || item.Supplement != "更新補足" {
		t.Fatalf("preserve: %+v %v", item, err)
	}
	empty := ""
	item, err = repo.UpdateMasterItem(ctx, "org", "admin", "accessories", item.ID, nil, nil, nil, nil, nil, &empty, &empty)
	if err != nil || item.Symbol != "" || item.Supplement != "" {
		t.Fatalf("clear: %+v %v", item, err)
	}
}
