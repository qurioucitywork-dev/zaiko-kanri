package persistence

import (
	"context"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestAccessoryRenameRefreshesLinkedProducts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`CREATE TABLE accessories(id TEXT PRIMARY KEY,organization_id TEXT,code TEXT,name TEXT,sort_order INTEGER,is_active BOOLEAN,updated_by TEXT,updated_at DATETIME)`,
		`CREATE TABLE products(id TEXT PRIMARY KEY,organization_id TEXT,accessories TEXT,accessory_supplements TEXT)`,
		`CREATE TABLE product_accessories(product_id TEXT,accessory_id TEXT,quantity INTEGER)`,
		`INSERT INTO accessories VALUES('box','org','0002','箱',2,TRUE,'',NULL),('links','org','0040','コマ（数字）',40,TRUE,'',NULL),('foreign','other','0040','コマ（数字）',40,TRUE,'',NULL)`,
		`INSERT INTO products VALUES('p','org','箱, コマ（数字）','{"0040":"3"}'),('p2','org','もっと古い名前','{}'),('unlinked','org','コマ（数字）','{}'),('foreign-p','other','コマ（数字）','{}')`,
		`INSERT INTO product_accessories VALUES('p','box',1),('p','links',3),('p2','links',1),('foreign-p','foreign',1)`,
	} {
		must(db.Exec(query).Error)
	}
	repo := &Repository{db: db, driver: "sqlite"}
	name := "コマ"
	_, err = repo.UpdateMasterItem(context.Background(), "org", "admin", "accessory", "links", &name, nil, nil, nil, nil, nil, nil)
	must(err)
	get := func(id string) string {
		var text string
		must(db.Table("products").Select("accessories").Where("id=?", id).Scan(&text).Error)
		return text
	}
	if get("p") != "箱, コマ" || get("p2") != "コマ" || get("unlinked") != "コマ（数字）" || get("foreign-p") != "コマ（数字）" {
		t.Fatal("rename did not preserve ID/organization boundaries")
	}
	var supplements string
	must(db.Table("products").Select("accessory_supplements").Where("id='p'").Scan(&supplements).Error)
	if supplements != `{"0040":"3"}` {
		t.Fatal("supplements changed")
	}
	ids, names, err := lookupAccessories(db, "org", []string{"0002", "0040"})
	must(err)
	if len(ids) != 2 || names[1] != "コマ" {
		t.Fatal("save lookup failed after rename")
	}
	// A cache write failure must roll back the master edit too.
	must(db.Exec(`CREATE TRIGGER reject_cache BEFORE UPDATE OF accessories ON products BEGIN SELECT RAISE(ABORT,'blocked'); END`).Error)
	rejected := "別名"
	_, err = repo.UpdateMasterItem(context.Background(), "org", "admin", "accessory", "links", &rejected, nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected failed transaction")
	}
	var saved string
	must(db.Table("accessories").Select("name").Where("id='links'").Scan(&saved).Error)
	if saved != "コマ" || get("p") != "箱, コマ" {
		t.Fatal("partial rename committed")
	}
}
