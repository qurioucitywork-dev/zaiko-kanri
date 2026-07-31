package sqliteadapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess/contracttest"
	_ "modernc.org/sqlite"
)

type sqliteHarness struct {
	db      *sql.DB
	adapter *Adapter
}

func newSQLiteHarness(t *testing.T) contracttest.Harness {
	t.Helper()
	path := filepath.Join(t.TempDir(), "contract.sqlite")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(currentReadSchema); err != nil {
		db.Close()
		t.Fatalf("create SQLite contract schema: %v", err)
	}
	return &sqliteHarness{db: db, adapter: New(db)}
}

func (h *sqliteHarness) ProductReader() dataaccess.ProductReader {
	return h.adapter
}

func (h *sqliteHarness) ObjectMetadataReader() dataaccess.ObjectMetadataReader {
	return h.adapter
}

func (h *sqliteHarness) DiagnosticReader() dataaccess.DiagnosticReader {
	return h.adapter
}

func (h *sqliteHarness) ForbiddenDiagnosticFragments() []string {
	return []string{
		"contract-secret-value",
		"private/products/",
		"contract.sqlite",
	}
}

func (h *sqliteHarness) Cleanup() {
	if h.db != nil {
		_ = h.db.Close()
	}
}

func (h *sqliteHarness) Seed(ctx context.Context, fixture contracttest.Fixture) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, product := range fixture.Products {
		userID := "user-" + product.TenantID
		supplierID := product.SupplierID
		if supplierID == "" {
			supplierID = "supplier-default-" + product.TenantID
		}
		slipID := "purchase-" + product.ID
		lineID := "line-" + product.ID
		now := product.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")

		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO organizations(id,code,name,created_at,updated_at)
			VALUES(?,?,?,?,?)`,
			product.TenantID, strings.ToUpper(product.TenantID), product.TenantID, now, now,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO users(
				id,organization_id,username,password_hash,display_name,role_key,created_at,updated_at
			) VALUES(?,?,?,?,?,'worker',?,?)`,
			userID, product.TenantID, userID, "contract-secret-value", userID, now, now,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO suppliers(
				id,organization_id,supplier_code,name,created_at,updated_at
			) VALUES(?,?,?,?,?,?)`,
			supplierID, product.TenantID, supplierID, supplierID, now, now,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_slips(
				id,organization_id,slip_number,supplier_id,purchase_date,status,
				created_by,created_at,updated_at
			) VALUES(?,?,?,?,?,'confirmed',?,?,?)`,
			slipID, product.TenantID, slipID, supplierID, product.PurchaseDate,
			userID, now, now,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_slip_lines(
				id,purchase_slip_id,line_number,quantity,unit_cost_minor,currency,
				brand,model_number,product_type,created_at
			) VALUES(?,?,1,1,?,?,?,?,?,?)`,
			lineID, slipID, product.Cost.AmountMinor, product.Cost.Currency,
			product.Brand, product.ModelNumber, product.ProductType, now,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO products(
				id,organization_id,product_code,sku,brand,model_number,serial_number,
				product_type,purchase_slip_line_id,supplier_id,purchase_date,
				cost_amount_minor,cost_currency,base_sale_price_minor,base_sale_currency,
				inventory_status,publication_status,condition_text,accessories,material_text,
				box_text,movement_text,belt_material_text,dial_text,features_text,
				created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			product.ID, product.TenantID, product.Code, product.SKU, product.Brand,
			product.ModelNumber, product.SerialNumber, product.ProductType, lineID,
			supplierID, product.PurchaseDate, product.Cost.AmountMinor, product.Cost.Currency,
			product.BaseSalePrice.AmountMinor, product.BaseSalePrice.Currency,
			product.InventoryStatus, product.PublicationStatus, product.Condition,
			product.Accessories, product.Material, product.Box, product.Movement,
			product.BeltMaterial, product.Dial, product.Features, now, now,
		); err != nil {
			return err
		}
	}

	for _, object := range fixture.Objects {
		uploaderID := "user-" + object.TenantID
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_images(
				id,organization_id,product_id,storage_path,original_name,content_type,
				size_bytes,sort_order,uploaded_by,created_at
			) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			object.ID, object.TenantID, object.ProductID,
			"private/products/"+object.ProductID+"/"+object.OriginalName,
			object.OriginalName, object.ContentType, object.SizeBytes, object.SortOrder,
			uploaderID, object.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = h.db.ExecContext(ctx, `PRAGMA query_only = ON`)
	return err
}

func TestSQLiteProductReaderContract(t *testing.T) {
	contracttest.RunProductReaderContract(t, newSQLiteHarness)
}

func TestSQLiteObjectMetadataReaderContract(t *testing.T) {
	contracttest.RunObjectMetadataReaderContract(t, newSQLiteHarness)
}

func TestSQLiteDiagnosticReaderContract(t *testing.T) {
	contracttest.RunDiagnosticReaderContract(t, newSQLiteHarness)
}

func TestLegacyProductImageMapsToReadyMetadataWithoutPath(t *testing.T) {
	h := newSQLiteHarness(t).(*sqliteHarness)
	t.Cleanup(h.Cleanup)
	if err := h.Seed(context.Background(), contracttest.StandardFixture()); err != nil {
		t.Fatal(err)
	}
	object, err := h.adapter.GetObjectMetadata(
		context.Background(),
		"contract-tenant-alpha",
		"object-alpha-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if object.Status != dataaccess.ObjectReady || !object.ReadyAt.Equal(object.CreatedAt) {
		t.Fatalf("legacy lifecycle mapping = %#v", object)
	}
	if object.ChecksumSHA256 != "" || !object.DeletedAt.IsZero() {
		t.Fatalf("invented unavailable metadata = %#v", object)
	}
}

func TestDiagnosePreservesQueryOnly(t *testing.T) {
	h := newSQLiteHarness(t).(*sqliteHarness)
	t.Cleanup(h.Cleanup)
	if err := h.Seed(context.Background(), contracttest.StandardFixture()); err != nil {
		t.Fatal(err)
	}
	report, err := h.adapter.Diagnose(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, component := range report.Components {
		if component.Name == "query_only" {
			found = true
			if component.Status != dataaccess.DiagnosticOK {
				t.Fatalf("query_only diagnostic = %#v", component)
			}
		}
	}
	if !found {
		t.Fatal("query_only diagnostic is missing")
	}
	if _, err := h.db.Exec(`INSERT INTO organizations(id,code,name,created_at,updated_at)
		VALUES('write-probe','WRITE','write','now','now')`); err == nil {
		t.Fatal("diagnostic disabled query_only")
	}
}

func TestLiteralLikeWildcardsAreNotExpanded(t *testing.T) {
	h := newSQLiteHarness(t).(*sqliteHarness)
	t.Cleanup(h.Cleanup)
	if err := h.Seed(context.Background(), contracttest.StandardFixture()); err != nil {
		t.Fatal(err)
	}
	page, err := h.adapter.SearchProducts(context.Background(), "contract-tenant-alpha", dataaccess.ProductSearch{
		Query: "%", Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 {
		t.Fatalf("literal wildcard query matched %d rows", page.Total)
	}
}

func TestInvalidReaderArguments(t *testing.T) {
	adapter := New(nil)
	if _, err := adapter.SearchProducts(context.Background(), "tenant", dataaccess.ProductSearch{Page: 1, PageSize: 20}); !errors.Is(err, dataaccess.ErrInvalidArgument) {
		t.Fatalf("nil DB search error = %v", err)
	}
	if _, err := adapter.GetObjectMetadata(context.Background(), "tenant", "object"); !errors.Is(err, dataaccess.ErrInvalidArgument) {
		t.Fatalf("nil DB metadata error = %v", err)
	}
}

const currentReadSchema = `
PRAGMA foreign_keys = ON;

CREATE TABLE organizations (
	id TEXT PRIMARY KEY,
	code TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE users (
	id TEXT PRIMARY KEY,
	organization_id TEXT NOT NULL REFERENCES organizations(id),
	username TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	display_name TEXT NOT NULL,
	role_key TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE (organization_id, username)
);

CREATE TABLE suppliers (
	id TEXT PRIMARY KEY,
	organization_id TEXT NOT NULL REFERENCES organizations(id),
	supplier_code TEXT NOT NULL,
	name TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE (organization_id, supplier_code)
);

CREATE TABLE purchase_slips (
	id TEXT PRIMARY KEY,
	organization_id TEXT NOT NULL REFERENCES organizations(id),
	slip_number TEXT NOT NULL,
	supplier_id TEXT NOT NULL REFERENCES suppliers(id),
	purchase_date TEXT NOT NULL,
	status TEXT NOT NULL,
	created_by TEXT NOT NULL REFERENCES users(id),
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE (organization_id, slip_number)
);

CREATE TABLE purchase_slip_lines (
	id TEXT PRIMARY KEY,
	purchase_slip_id TEXT NOT NULL REFERENCES purchase_slips(id),
	line_number INTEGER NOT NULL,
	quantity INTEGER NOT NULL,
	unit_cost_minor INTEGER NOT NULL,
	currency TEXT NOT NULL,
	brand TEXT NOT NULL,
	model_number TEXT NOT NULL,
	product_type TEXT NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE (purchase_slip_id, line_number)
);

CREATE TABLE products (
	id TEXT PRIMARY KEY,
	organization_id TEXT NOT NULL REFERENCES organizations(id),
	product_code TEXT NOT NULL,
	sku TEXT NOT NULL DEFAULT '',
	brand TEXT NOT NULL,
	model_number TEXT NOT NULL DEFAULT '',
	serial_number TEXT NOT NULL DEFAULT '',
	product_type TEXT NOT NULL,
	purchase_slip_line_id TEXT NOT NULL REFERENCES purchase_slip_lines(id),
	supplier_id TEXT NOT NULL REFERENCES suppliers(id),
	purchase_date TEXT NOT NULL,
	cost_amount_minor INTEGER NOT NULL,
	cost_currency TEXT NOT NULL,
	base_sale_price_minor INTEGER NOT NULL DEFAULT 0,
	base_sale_currency TEXT NOT NULL,
	inventory_status TEXT NOT NULL,
	publication_status TEXT NOT NULL DEFAULT 'private',
	condition_text TEXT NOT NULL DEFAULT '',
	accessories TEXT NOT NULL DEFAULT '',
	material_text TEXT NOT NULL DEFAULT '',
	box_text TEXT NOT NULL DEFAULT '',
	movement_text TEXT NOT NULL DEFAULT '',
	belt_material_text TEXT NOT NULL DEFAULT '',
	dial_text TEXT NOT NULL DEFAULT '',
	features_text TEXT NOT NULL DEFAULT '',
	deleted_at TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE (organization_id, product_code)
);

CREATE TABLE product_images (
	id TEXT PRIMARY KEY,
	organization_id TEXT NOT NULL REFERENCES organizations(id),
	product_id TEXT NOT NULL REFERENCES products(id),
	storage_path TEXT NOT NULL,
	original_name TEXT NOT NULL,
	content_type TEXT NOT NULL,
	size_bytes INTEGER NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	uploaded_by TEXT NOT NULL REFERENCES users(id),
	created_at TEXT NOT NULL
);
`

func ExampleAdapter_Diagnose() {
	fmt.Println("SQLite diagnostics use SELECT and PRAGMA query_only only")
	// Output: SQLite diagnostics use SELECT and PRAGMA query_only only
}
