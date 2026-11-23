package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCostMergeOptionalAudit(t *testing.T) {
	dsn := os.Getenv("ZAIKO_MERGE_OPTIONAL_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_merge_optional_test") {
		t.Fatal("non-test database rejected")
	}
	ctx := context.Background()
	repo, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repo.Migrate(ctx))
	must(repo.SeedPreviewIdentity(ctx, "cost-test-admin-password", "cost-test-worker-password"))
	must(repo.SeedPreviewMasters(ctx))
	var org, admin, supplier, brand string
	for _, q := range []struct {
		sql   string
		value *string
	}{
		{"SELECT id FROM organizations WHERE code='PREVIEW'", &org},
		{"SELECT id FROM users WHERE username='admin' LIMIT 1", &admin},
		{"SELECT role_code FROM partner_roles WHERE role_type='supplier' AND is_active ORDER BY role_code LIMIT 1", &supplier},
		{"SELECT code FROM brands WHERE is_active ORDER BY code LIMIT 1", &brand},
	} {
		must(repo.db.Raw(q.sql).Scan(q.value).Error)
	}
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('consign-test-rate',?,'USD','JPY',15500,100,'test',?,now(),now())", org, admin).Error)

	for _, table := range []string{"brands", "materials", "movements", "product_conditions", "accessories"} {
		for i := 1; i <= 5; i++ {
			must(seedCatalogItem(repo.db, table, org, admin, catalogSeed{Code: fmt.Sprintf("%04d", i), Name: fmt.Sprintf("audit-%s-%d", table, i)}, i))
		}
	}
	_ = time.Now()
	var ids []string
	for i := 0; i < 2; i++ {
		created, e := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, BrandCode: brand, CostAmountMinor: 150000, CostCurrency: "JPY", BaseSaleCurrency: "USD"})
		must(e)
		must(repo.db.Table("products").Where("id=?", created.Product.ID).Update("inventory_status", "in_stock").Error)
		ids = append(ids, created.Product.ID)
	}
	const raw = `{
  "sku": "OPTIONAL-TEST",
  "output": {
    "position": 1,
    "kind": "product",
    "allocatedCostJpyMinor": 300000,
    "product": {
      "brandCode": "0001",
      "referenceNumber": "REF-123",
      "serialNumber": "SERIAL-123",
      "materialCode": "0001",
      "movementCode": "0001",
      "conditionCode": "0001",
      "salePriceUsdMinor": 12345,
      "notes": "備考テスト",
      "internalComment": "コメントテスト",
      "modelNumber": "任意項目テスト",
      "beltText": "SS",
      "dialText": "グリーン",
      "accessoryCodes": [
        "0001",
        "0002",
        "0003",
        "0004",
        "0005"
      ]
    }
  }
}`
	var all struct {
		SKU    string
		Output CostAdjustmentOutputDraft
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	must(dec.Decode(&all))
	var fields map[string]json.RawMessage
	b, e := json.Marshal(all.Output.Product)
	must(e)
	must(json.Unmarshal(b, &fields))
	names := []string{"required-only", "all", "sku"}
	for key := range fields {
		if key != "brandCode" {
			names = append(names, key)
		}
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tx := repo.db.Begin()
			defer tx.Rollback()
			r := &Repository{db: tx, driver: "postgres"}
			d := &CostAdjustmentProductDraft{BrandCode: all.Output.Product.BrandCode}
			sku := ""
			if name == "all" {
				copy := *all.Output.Product
				d = &copy
				sku = all.SKU
			} else if name == "sku" {
				sku = all.SKU
			} else if name != "required-only" {
				raw, e := json.Marshal(map[string]json.RawMessage{"brandCode": fields["brandCode"], name: fields[name]})
				must(e)
				must(json.Unmarshal(raw, d))
			}
			session, e := r.StartProductCostMerge(ctx, org, admin, ids)
			if e != nil {
				t.Fatal(e)
			}
			out := all.Output
			out.Product = d
			result, e := r.ConfirmProductCostMerge(ctx, org, admin, session.ID, sku, out)
			if e != nil {
				t.Fatal(e)
			}
			saved, e := r.ProductByID(ctx, org, result.Outputs[0].RecordID)
			if e != nil {
				t.Fatal(e)
			}
			if saved.SKU != sku || saved.ModelNumber != d.ModelNumber || saved.ReferenceNumber != d.ReferenceNumber || saved.SerialNumber != d.SerialNumber {
				t.Fatal("text fields not saved")
			}
			fmt.Printf("OPTIONAL %s PASS\n", name)
		})
	}

	t.Run("legacy-accessory-names", func(t *testing.T) {
		tx := repo.db.Begin()
		defer tx.Rollback()
		r := &Repository{db: tx, driver: "postgres"}
		names := []string{"Guarantee(P)", "Guarantee(C)", "Guarantee(NC)", "Guarantee(NNC)", "Guarantee(SVC)"}
		for i, name := range names {
			must(tx.Table("accessories").Where("organization_id=? AND code=?", org, fmt.Sprintf("%04d", i+1)).Update("name", name).Error)
		}
		session, e := r.StartProductCostMerge(ctx, org, admin, ids)
		if e != nil {
			t.Fatal(e)
		}
		output := all.Output
		product := *all.Output.Product
		output.Product = &product
		product.AccessoryCodes = names
		_, e = r.ConfirmProductCostMerge(ctx, org, admin, session.ID, all.SKU, output)
		if e != ErrMasterCodeNotFound {
			t.Fatalf("expected invalid accessory code, got %v", e)
		}
		fmt.Printf("LEGACY_REPRODUCTION: %v\n", e)
		product.AccessoryCodes = []string{"0001", "0002", "0003", "0004", "0005"}
		result, e := r.ConfirmProductCostMerge(ctx, org, admin, session.ID, all.SKU, output)
		if e != nil {
			t.Fatal(e)
		}
		if len(result.Outputs) != 1 {
			t.Fatal("missing output")
		}
		fmt.Println("SAME_SESSION_WITH_CODES: PASS")
	})
	// HTTP uses strict integer decoding for the optional USD price.
	decimal := strings.Replace(raw, "12345", "12345.67", 1)
	dec = json.NewDecoder(strings.NewReader(decimal))
	dec.DisallowUnknownFields()
	err = dec.Decode(&all)
	if err == nil {
		t.Fatal("fraction unexpectedly accepted")
	}
	fmt.Printf("DECIMAL_REJECTION %v\n", err)
}
