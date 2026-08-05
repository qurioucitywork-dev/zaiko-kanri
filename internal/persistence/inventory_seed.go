package persistence

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) SeedPreviewInventory(ctx context.Context) error {
	if r.driver != "postgres" {
		return nil
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var organizationID, adminID, staffID, supplierRoleID, brandID, conditionID string
		lookups := []struct {
			query string
			args  []any
			dest  *string
		}{
			{`SELECT id FROM organizations WHERE code='PREVIEW'`, nil, &organizationID},
			{`SELECT id FROM users WHERE organization_id=(SELECT id FROM organizations WHERE code='PREVIEW') AND username='admin'`, nil, &adminID},
			{`SELECT id FROM staff_profiles WHERE user_id=(SELECT id FROM users WHERE username='admin' AND organization_id=(SELECT id FROM organizations WHERE code='PREVIEW'))`, nil, &staffID},
			{`SELECT id FROM partner_roles WHERE organization_id=(SELECT id FROM organizations WHERE code='PREVIEW') AND role_type='supplier' AND role_code='S001'`, nil, &supplierRoleID},
			{`SELECT id FROM brands WHERE organization_id=(SELECT id FROM organizations WHERE code='PREVIEW') AND code='BRD-001'`, nil, &brandID},
			{`SELECT id FROM product_conditions WHERE organization_id=(SELECT id FROM organizations WHERE code='PREVIEW') AND code='C03'`, nil, &conditionID},
		}
		for _, lookup := range lookups {
			if err := tx.Raw(lookup.query, lookup.args...).Scan(lookup.dest).Error; err != nil {
				return err
			}
			if *lookup.dest == "" {
				return fmt.Errorf("preview inventory dependency is missing")
			}
		}

		now := time.Now().UTC()
		if err := tx.Exec(`
			INSERT INTO purchase_slips(
				id,organization_id,slip_number,supplier_role_id,purchase_staff_profile_id,purchase_date,status,
				is_simple,notes,confirmed_at,confirmed_by,created_by,created_at,updated_at
			) VALUES('purchase_preview_001',?,?,?,?,?,'confirmed',TRUE,'',?,?,?, ?,?)
			ON CONFLICT (organization_id,slip_number) DO NOTHING`,
			organizationID, "PI-2026-0001", supplierRoleID, staffID, "2026-07-26", now, adminID, adminID, now, now).Error; err != nil {
			return err
		}
		var purchaseID string
		if err := tx.Raw(`SELECT id FROM purchase_slips WHERE organization_id=? AND slip_number='PI-2026-0001'`, organizationID).
			Scan(&purchaseID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO purchase_slip_lines(
				id,purchase_slip_id,line_number,quantity,unit_cost_minor,cost_currency,base_sale_price_minor,
				base_sale_currency,brand_id,condition_id,brand_text,model_number,reference_number,serial_number,
				product_type,sku,generated_product_count,created_at
			) VALUES('purchase_line_preview_001',?,1,1,850000,'JPY',7613,'USD',?,?,'ロレックス','サブマリーナ','116610LN','ZX123456','腕時計','ROLEX-SUB-001',1,?)
			ON CONFLICT (purchase_slip_id,line_number) DO NOTHING`, purchaseID, brandID, conditionID, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO products(
				id,organization_id,product_code,sku,brand,brand_id,model_number,reference_number,serial_number,product_type,
				condition_id,supplier_id,supplier_role_id,purchase_staff_profile_id,purchase_slip_line_id,purchase_date,
				cost_amount_minor,cost_currency,base_sale_price_minor,base_sale_currency,inventory_status,publication_status,
				condition_text,accessories,notes,created_at,updated_at
			) VALUES('product_preview_001',?,'20260726001','ROLEX-SUB-001','ロレックス',?,'サブマリーナ','116610LN','ZX123456','腕時計',?, ?,?,?,
				'purchase_line_preview_001','2026-07-26',850000,'JPY',7613,'USD','in_stock','private','極美品 (S)','BOX, GUARANTEE','プレビュー用在庫',?,?)
			ON CONFLICT (organization_id,product_code) DO NOTHING`,
			organizationID, brandID, conditionID, supplierRoleID, supplierRoleID, staffID, now, now).Error; err != nil {
			return err
		}
		for _, accessoryCode := range []string{"ACC-001", "ACC-003"} {
			if err := tx.Exec(`
				INSERT INTO product_accessories(product_id,accessory_id,quantity)
				SELECT 'product_preview_001',id,1 FROM accessories WHERE organization_id=? AND code=?
				ON CONFLICT DO NOTHING`, organizationID, accessoryCode).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`
			INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
			VALUES('inventory_event_preview_001',?,'product_preview_001','purchase_confirmed','','in_stock','プレビュー初期登録',?,?)
			ON CONFLICT (id) DO NOTHING`, organizationID, adminID, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,observed_at,created_by,created_at)
			VALUES('fx_preview_001',?,'USD','JPY',15425000000,100000000,'手動登録','2026-07-26T10:00:00+09:00',?,?)
			ON CONFLICT (id) DO NOTHING`, organizationID, adminID, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO market_price_records(
				id,organization_id,import_date,brand_id,brand_text,model_number,reference_number,condition_id,
				purchase_price_minor,purchase_currency,market_price_minor,market_currency,source,notes,is_active,
				created_by,updated_by,created_at,updated_at
			) VALUES('market_preview_001',?,'2026-07-26',?,'ロレックス','サブマリーナ','116610LN',?,850000,'JPY',7742,'USD','手動登録','プレビュー用相場',TRUE,?,?,?,?)
			ON CONFLICT (id) DO NOTHING`, organizationID, brandID, conditionID, adminID, adminID, now, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO publication_box_products(box_id,product_id,created_at)
			VALUES('publication_box_BOX-01','product_preview_001',?) ON CONFLICT DO NOTHING`, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE products SET publication_status='public',updated_at=?
			WHERE organization_id=? AND id='product_preview_001'`, now, organizationID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return r.seedPreviewInventoryByMonth(ctx)
}

// seedPreviewInventoryByMonth keeps the PREVIEW environment useful for
// end-to-end checks. It fills each month from April through August 2026 to 20
// products by using the same CreatePurchase/ConfirmPurchase path as the UI.
// Because products are always created from the confirmed slip, the slip date,
// product purchase date, and date prefix in the product code stay identical.
func (r *Repository) seedPreviewInventoryByMonth(ctx context.Context) error {
	var identity struct {
		OrganizationID string
		AdminID        string
	}
	result := r.db.WithContext(ctx).Raw(`
		SELECT o.id AS organization_id,u.id AS admin_id
		FROM organizations o
		JOIN users u ON u.organization_id=o.id AND u.username='admin'
		WHERE o.code='PREVIEW'`).Scan(&identity)
	if result.Error != nil {
		return result.Error
	}
	if identity.OrganizationID == "" || identity.AdminID == "" {
		return fmt.Errorf("preview inventory identity is missing")
	}

	type monthPlan struct {
		start string
		end   string
		dates []string
	}
	plans := []monthPlan{
		{start: "2026-04-01", end: "2026-05-01", dates: []string{"2026-04-03", "2026-04-10", "2026-04-17", "2026-04-24"}},
		{start: "2026-05-01", end: "2026-06-01", dates: []string{"2026-05-01", "2026-05-08", "2026-05-15", "2026-05-22"}},
		{start: "2026-06-01", end: "2026-07-01", dates: []string{"2026-06-02", "2026-06-09", "2026-06-16", "2026-06-23"}},
		{start: "2026-07-01", end: "2026-08-01", dates: []string{"2026-07-01", "2026-07-08", "2026-07-15", "2026-07-22"}},
		{start: "2026-08-01", end: "2026-09-01", dates: []string{"2026-08-01", "2026-08-04", "2026-08-02", "2026-08-03"}},
	}
	brands := []struct {
		code  string
		model string
		ref   string
	}{
		{code: "BRD-001", model: "Datejust", ref: "126234"},
		{code: "BRD-002", model: "Speedmaster", ref: "310.30.42.50.01.001"},
		{code: "BRD-003", model: "Aquanaut", ref: "5167A-001"},
		{code: "BRD-004", model: "Santos", ref: "WSSA0018"},
		{code: "BRD-005", model: "Portugieser", ref: "IW371604"},
		{code: "BRD-006", model: "Navitimer", ref: "AB0138211B1P1"},
		{code: "BRD-007", model: "Carrera", ref: "CBN2A1A.BA0643"},
		{code: "BRD-008", model: "Prospex", ref: "SBDC101"},
		{code: "BRD-009", model: "Heritage Collection", ref: "SBGA211"},
		{code: "BRD-010", model: "Classic Watch", ref: "OTHER-001"},
	}
	suppliers := []string{"S001", "S002", "S003", "S004", "S005"}
	staff := []string{"STF-000", "STF-001", "STF-002", "STF-003", "STF-004", "STF-005"}
	conditions := []string{"C01", "C02", "C03", "C04", "C05", "C06", "C07"}

	const targetPerMonth = 20
	for monthIndex, plan := range plans {
		var current int64
		if err := r.db.WithContext(ctx).Table("products").Where(
			"organization_id=? AND deleted_at IS NULL AND purchase_date>=? AND purchase_date<?",
			identity.OrganizationID, plan.start, plan.end).Count(&current).Error; err != nil {
			return err
		}
		remaining := targetPerMonth - int(current)
		for batchIndex := 0; remaining > 0; batchIndex++ {
			batchSize := 5
			if remaining < batchSize {
				batchSize = remaining
			}
			lines := make([]PurchaseLineInput, 0, batchSize)
			for lineIndex := 0; lineIndex < batchSize; lineIndex++ {
				sequence := int(current) + batchIndex*5 + lineIndex + 1
				sampleIndex := (monthIndex*targetPerMonth + sequence - 1) % len(brands)
				sample := brands[sampleIndex]
				cost := int64(180000 + (sampleIndex * 65000) + (sequence%5)*22000)
				lines = append(lines, PurchaseLineInput{
					Quantity:           1,
					SKU:                fmt.Sprintf("PREVIEW-%s-%03d", plan.start[0:7], sequence),
					BrandCode:          sample.code,
					ModelNumber:        sample.model,
					ReferenceNumber:    sample.ref,
					SerialNumber:       fmt.Sprintf("PV26%02d%03d", monthIndex+4, sequence),
					ProductType:        "watch",
					ConditionCode:      conditions[(sequence+monthIndex)%len(conditions)],
					UnitCostMinor:      cost,
					CostCurrency:       "JPY",
					BaseSalePriceMinor: 1800 + int64(sampleIndex*475+(sequence%4)*125),
					BaseSaleCurrency:   "USD",
					Notes:              "",
				})
			}
			purchase, err := r.CreatePurchase(ctx, PurchaseCreateInput{
				OrganizationID: identity.OrganizationID,
				ActorUserID:    identity.AdminID,
				SupplierCode:   suppliers[(monthIndex+batchIndex)%len(suppliers)],
				StaffCode:      staff[(monthIndex+batchIndex)%len(staff)],
				PurchaseDate:   plan.dates[batchIndex%len(plan.dates)],
				Notes:          "",
				Lines:          lines,
			})
			if err != nil {
				return fmt.Errorf("create preview purchase for %s: %w", plan.start, err)
			}
			if _, err := r.ConfirmPurchase(ctx, identity.OrganizationID, purchase.ID, identity.AdminID); err != nil {
				return fmt.Errorf("confirm preview purchase for %s: %w", plan.start, err)
			}
			remaining -= batchSize
		}
	}
	return nil
}
