package persistence

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestProductBelongsToBuyerThroughConfirmedConsignment(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sales_consignment_link?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE purchase_requests (organization_id TEXT, product_id TEXT, buyer_role_id TEXT, status TEXT)`,
		`CREATE TABLE shipment_slips (id TEXT, organization_id TEXT, buyer_role_id TEXT, status TEXT)`,
		`CREATE TABLE shipment_lines (shipment_slip_id TEXT, product_id TEXT)`,
		`CREATE TABLE consignment_slips (id TEXT, organization_id TEXT, consignee_role_id TEXT, status TEXT)`,
		`CREATE TABLE consignment_lines (consignment_slip_id TEXT, product_id TEXT)`,
		`CREATE TABLE sales_slips (id TEXT, organization_id TEXT, buyer_role_id TEXT, status TEXT)`,
		`CREATE TABLE sales_lines (sales_slip_id TEXT, product_id TEXT)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO consignment_slips(id,organization_id,consignee_role_id,status)
		VALUES ('cns_1','org_1','buyer_1','confirmed')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO consignment_lines(consignment_slip_id,product_id)
		VALUES ('cns_1','product_1')`).Error; err != nil {
		t.Fatal(err)
	}

	linked, err := productBelongsToBuyer(db.WithContext(context.Background()), "org_1", "product_1", "buyer_1")
	if err != nil {
		t.Fatal(err)
	}
	if !linked {
		t.Fatal("a product on a confirmed consignment must belong to that consignee for sales registration")
	}
	otherBuyerLinked, err := productBelongsToBuyer(db.WithContext(context.Background()), "org_1", "product_1", "buyer_2")
	if err != nil {
		t.Fatal(err)
	}
	if otherBuyerLinked {
		t.Fatal("a consigned product must not be available to a different buyer")
	}
}
