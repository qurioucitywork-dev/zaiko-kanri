package persistence

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPublicationBoxCleanSave(t *testing.T) {
	dsn := os.Getenv("ZAIKO_BOX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable box test PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/zaiko_clean_test_") {
		t.Fatal("refusing non-test database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	for _, sql := range []string{
		`CREATE TABLE publication_boxes(id TEXT PRIMARY KEY,organization_id TEXT,box_code TEXT,name TEXT,is_active BOOLEAN,created_by TEXT,updated_by TEXT,created_at TIMESTAMPTZ,updated_at TIMESTAMPTZ,UNIQUE(organization_id,box_code))`,
		`CREATE TABLE publication_box_buyers(box_id TEXT REFERENCES publication_boxes(id),buyer_role_id TEXT,created_at TIMESTAMPTZ,PRIMARY KEY(box_id,buyer_role_id))`,
		`CREATE TABLE publication_box_products(box_id TEXT REFERENCES publication_boxes(id),product_id TEXT,created_at TIMESTAMPTZ,PRIMARY KEY(box_id,product_id))`,
		`CREATE TABLE partner_roles(id TEXT PRIMARY KEY,organization_id TEXT,role_type TEXT,role_code TEXT,is_active BOOLEAN)`,
		`CREATE TABLE products(id TEXT PRIMARY KEY,organization_id TEXT,product_code TEXT,inventory_status TEXT DEFAULT 'in_stock',deleted_at TIMESTAMPTZ,publication_status TEXT,updated_at TIMESTAMPTZ)`,
		`INSERT INTO products(id,organization_id,product_code,publication_status) VALUES('p1','org','P1','private'),('p2','org','P2','private'),('p3','other','P3','private')`,
		`INSERT INTO partner_roles VALUES('buyer1','org','buyer','B001',TRUE)`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	migration, err := migrationFS.ReadFile("migrations/000097_box_product_visibility.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(migration)).Error }); err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "postgres"}
	ctx := context.Background()
	boxes, err := repo.PublicationBoxes(ctx, "org")
	if err != nil || len(boxes) != 0 {
		t.Fatal(boxes, err)
	}
	input := PublicationBoxInput{Name: "BOX 1", IsActive: true, BuyerCodes: []string{"B001"}, ProductCodes: []string{"P1", "P2"}}
	saved, err := repo.UpdatePublicationBox(ctx, "org", "BOX-01", "admin", input)
	if err != nil || len(saved.ProductCodes) != 2 || len(saved.BuyerCodes) != 1 {
		t.Fatal(saved, err)
	}
	again, err := repo.UpdatePublicationBox(ctx, "org", "BOX-01", "admin", input)
	if err != nil || again.ID != saved.ID {
		t.Fatal(again, err)
	}
	input.ProductCodes = []string{"P2"}
	if _, err = repo.UpdatePublicationBox(ctx, "org", "BOX-01", "admin", input); err != nil {
		t.Fatal(err)
	}
	boxes, err = repo.PublicationBoxes(ctx, "org")
	if err != nil || len(boxes) != 1 || len(boxes[0].ProductCodes) != 1 || boxes[0].ProductCodes[0] != "P2" {
		t.Fatal(boxes, err)
	}
	input.ProductCodes = []string{"P3"}
	if _, err = repo.UpdatePublicationBox(ctx, "org", "BOX-01", "admin", input); !errors.Is(err, ErrMasterCodeNotFound) {
		t.Fatal("cross-org product accepted", err)
	}
	boxes, _ = repo.PublicationBoxes(ctx, "org")
	if len(boxes[0].ProductCodes) != 1 || boxes[0].ProductCodes[0] != "P2" {
		t.Fatal("failed write did not roll back", boxes)
	}
	if _, err = repo.UpdatePublicationBox(ctx, "org", "BOX-02", "admin", input); !errors.Is(err, ErrMasterCodeNotFound) {
		t.Fatal(err)
	}
	boxes, _ = repo.PublicationBoxes(ctx, "org")
	if len(boxes) != 1 {
		t.Fatal("failed create left empty box", boxes)
	}
	if _, err = repo.UpdatePublicationBox(ctx, "org", "BOX-11", "admin", input); !errors.Is(err, ErrBoxNotFound) {
		t.Fatal(err)
	}
	input.ProductCodes = nil
	if _, err = repo.UpdatePublicationBox(ctx, "org", "BOX-01", "admin", input); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Table("products").Where("organization_id=? AND publication_status='public'", "org").Count(&count)
	if count != 0 {
		t.Fatal("removed products still public")
	}
}
