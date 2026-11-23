package persistence

import (
	"context"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestBeltDetailsCreateAndUpdate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	if err = db.Exec("CREATE TABLE belt_materials (id TEXT PRIMARY KEY,organization_id TEXT,code TEXT,name TEXT,meaning TEXT,is_active BOOLEAN,sort_order INTEGER,created_by TEXT,updated_by TEXT,created_at DATETIME,updated_at DATETIME,UNIQUE(organization_id,code))").Error; err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "sqlite"}
	ctx := context.Background()
	item, err := repo.CreateMasterItem(ctx, "org", "admin", "belt", "0001", "SS", "ステンレススチール", "", 0, "", "")
	if err != nil || item.Code != "0001" || item.Name != "SS" || item.Meaning != "ステンレススチール" {
		t.Fatalf("create: %+v %v", item, err)
	}
	detail := "詳細更新"
	item, err = repo.UpdateMasterItem(ctx, "org", "admin", "belt-materials", item.ID, nil, &detail, nil, nil, nil, nil, nil)
	if err != nil || item.Meaning != detail {
		t.Fatalf("update: %+v %v", item, err)
	}
	items, err := repo.MasterItems(ctx, "org", "belt-materials", false)
	if err != nil || len(items) != 1 || items[0].Meaning != detail {
		t.Fatalf("list: %+v %v", items, err)
	}
}
