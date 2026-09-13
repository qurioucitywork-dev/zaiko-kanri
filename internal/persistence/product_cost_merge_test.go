package persistence

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The outer transaction is always rolled back, including sequence reservations.
func TestCostSplitStartedDatePostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_MERGE_TEST_DSN")
	if dsn == "" {
		t.Skip("PostgreSQL test DSN not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	var p Product
	if err := tx.Where(`inventory_status='in_stock' AND deleted_at IS NULL AND purchase_slip_line_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM cost_adjustments c WHERE c.source_product_id=products.id)`).Order("organization_id,id").Take(&p).Error; err != nil {
		t.Fatal(err)
	}
	var actor, brand string
	tx.Raw("SELECT id FROM users WHERE organization_id=? LIMIT 1", p.OrganizationID).Scan(&actor)
	tx.Raw("SELECT code FROM brands WHERE id=?", p.BrandID).Scan(&brand)
	repo := &Repository{db: tx, driver: "postgres"}
	if err := tx.Model(&Product{}).Where("id=?", p.ID).Update("inventory_status", "sold").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.StartProductCostAdjustment(context.Background(), p.OrganizationID, p.ID, actor); err != ErrProductConflict {
		t.Fatalf("sold product start error=%v, want ErrProductConflict", err)
	}
	var soldProduct Product
	if err := tx.Where("id=?", p.ID).Take(&soldProduct).Error; err != nil {
		t.Fatal(err)
	}
	if soldProduct.InventoryStatus != "sold" {
		t.Fatalf("rejected sold product status=%q, want sold", soldProduct.InventoryStatus)
	}
	if err := tx.Model(&Product{}).Where("id=?", p.ID).Update("inventory_status", "in_stock").Error; err != nil {
		t.Fatal(err)
	}
	startedProduct, err := repo.StartProductCostAdjustment(context.Background(), p.OrganizationID, p.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if startedProduct.InventoryStatus != "cost_adjustment" {
		t.Fatalf("split source status after start=%q, want cost_adjustment", startedProduct.InventoryStatus)
	}
	if startedProduct.BaseSalePriceMinor != 0 {
		t.Fatalf("split source planned sale price after start=%d, want 0", startedProduct.BaseSalePriceMinor)
	}
	if p.BaseSalePriceMinor > 0 && !strings.Contains(startedProduct.InternalComment, "もともとの想定売価:") {
		t.Fatalf("split source did not preserve its planned sale price: %q", startedProduct.InternalComment)
	}
	var persistedStartedProduct Product
	if err := tx.Where("id=?", p.ID).Take(&persistedStartedProduct).Error; err != nil {
		t.Fatal(err)
	}
	if persistedStartedProduct.InventoryStatus != "cost_adjustment" {
		t.Fatalf("persisted split source status after start=%q, want cost_adjustment", persistedStartedProduct.InventoryStatus)
	}
	started := time.Date(2026, 9, 10, 16, 30, 0, 0, time.UTC)
	if err := tx.Model(&p).Update("cost_adjustment_started_at", started).Error; err != nil {
		t.Fatal(err)
	}
	var sourceLineBefore struct {
		UnitCostMinor      int64
		ConvertedTotalJPY  int64
		BaseSalePriceMinor int64
	}
	tx.Raw("SELECT unit_cost_minor,converted_total_jpy,base_sale_price_minor FROM purchase_slip_lines WHERE id=?", p.PurchaseSlipLineID).Scan(&sourceLineBefore)
	var splitPurchaseSnapshotBefore string
	tx.Raw(`SELECT COALESCE(jsonb_agg(to_jsonb(line) ORDER BY line.line_number)::text,'[]')
		FROM purchase_slip_lines line WHERE line.purchase_slip_id=(SELECT purchase_slip_id FROM purchase_slip_lines WHERE id=?)`, p.PurchaseSlipLineID).Scan(&splitPurchaseSnapshotBefore)
	total := sourceLineBefore.ConvertedTotalJPY
	result, err := repo.ConfirmCostAdjustmentBreakdown(context.Background(), CostAdjustmentConfirmInput{OrganizationID: p.OrganizationID, ActorUserID: actor, SourceProductID: p.ID, Mode: "breakdown", Outputs: []CostAdjustmentOutputDraft{
		{Position: 1, Kind: "product", AllocatedCostJPYMinor: total, Product: &CostAdjustmentProductDraft{BrandCode: brand, Notes: "保存する備考"}},
		{Position: 2, Kind: "product", Product: &CostAdjustmentProductDraft{BrandCode: brand}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.AdjustmentDate != "2026-09-11" {
		t.Fatal(result.AdjustmentDate)
	}
	for _, out := range result.Outputs {
		if !strings.HasPrefix(out.ManagementCode, "110926") {
			t.Fatal(out.ManagementCode)
		}
	}
	var notes string
	tx.Raw("SELECT notes FROM cost_adjustments WHERE id=?", result.ID).Scan(&notes)
	if notes != "原価分け" {
		t.Fatal(notes)
	}
	var sourceAfter Product
	if err := tx.Where("id=?", p.ID).Take(&sourceAfter).Error; err != nil {
		t.Fatal(err)
	}
	if sourceAfter.CostAmountMinor != p.CostAmountMinor || sourceAfter.CostCurrency != p.CostCurrency {
		t.Fatal("split source historical product cost was changed")
	}
	if !sourceAfter.InventoryValuationExcluded || sourceAfter.InventoryValuationExclusionReason != "cost_split_source" {
		t.Fatal("split source was not excluded from current inventory valuation")
	}
	var sourceLineAfter struct {
		UnitCostMinor      int64
		ConvertedTotalJPY  int64
		BaseSalePriceMinor int64
	}
	tx.Raw("SELECT unit_cost_minor,converted_total_jpy,base_sale_price_minor FROM purchase_slip_lines WHERE id=?", p.PurchaseSlipLineID).Scan(&sourceLineAfter)
	if sourceLineAfter != sourceLineBefore {
		t.Fatalf("split source purchase history changed: before=%+v after=%+v", sourceLineBefore, sourceLineAfter)
	}
	var splitPurchaseSnapshotAfter string
	tx.Raw(`SELECT COALESCE(jsonb_agg(to_jsonb(line) ORDER BY line.line_number)::text,'[]')
		FROM purchase_slip_lines line WHERE line.purchase_slip_id=(SELECT purchase_slip_id FROM purchase_slip_lines WHERE id=?)`, p.PurchaseSlipLineID).Scan(&splitPurchaseSnapshotAfter)
	if splitPurchaseSnapshotAfter != splitPurchaseSnapshotBefore {
		t.Fatal("cost split changed the acquisition-time purchase slip")
	}
	for _, output := range result.Outputs {
		var outputProduct Product
		if err := tx.Where("id=?", output.RecordID).Take(&outputProduct).Error; err != nil {
			t.Fatal(err)
		}
		if outputProduct.PurchaseSlipLineID != "" {
			t.Fatal("cost split output must belong to the adjustment slip, not the purchase slip")
		}
	}
	if err := tx.Exec("SET CONSTRAINTS ALL IMMEDIATE").Error; err != nil {
		t.Fatal(err)
	}
}

func TestProductCostMergePostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_MERGE_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZAIKO_MERGE_TEST_DSN for transactional PostgreSQL verification")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	var inputs []Product
	err = tx.Where(`inventory_status='in_stock' AND deleted_at IS NULL AND purchase_slip_line_id IS NOT NULL
  AND NOT EXISTS(SELECT 1 FROM cost_adjustments c WHERE c.source_product_id=products.id)
  AND NOT EXISTS(SELECT 1 FROM product_cost_merge_inputs c WHERE c.product_id=products.id)`).Order("organization_id,id").Limit(2).Find(&inputs).Error
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || inputs[0].OrganizationID != inputs[1].OrganizationID {
		t.Fatal("requires two available products in one organization")
	}
	org := inputs[0].OrganizationID
	var mergePurchaseSnapshotBefore string
	tx.Raw(`SELECT COALESCE(jsonb_agg(to_jsonb(line) ORDER BY line.line_number)::text,'[]')
		FROM purchase_slip_lines line WHERE line.purchase_slip_id=(SELECT purchase_slip_id FROM purchase_slip_lines WHERE id=?)`, inputs[0].PurchaseSlipLineID).Scan(&mergePurchaseSnapshotBefore)
	var actor string
	if err := tx.Raw("SELECT id FROM users WHERE organization_id=? LIMIT 1", org).Scan(&actor).Error; err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: tx, driver: "postgres"}
	ctx := context.Background()
	if _, err := repo.StartProductCostMerge(ctx, org, actor, []string{inputs[0].ID}); err == nil {
		t.Fatal("accepted one product")
	}
	if err := tx.Model(&Product{}).Where("id=?", inputs[1].ID).Update("inventory_status", "cost_adjustment").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.StartProductCostMerge(ctx, org, actor, []string{inputs[0].ID, inputs[1].ID}); err != ErrCostAdjustmentState {
		t.Fatalf("cost-adjustment source error=%v, want ErrCostAdjustmentState", err)
	}
	var rejected Product
	if err := tx.Where("id=?", inputs[1].ID).Take(&rejected).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.InventoryStatus != "cost_adjustment" {
		t.Fatalf("rejected source status=%q, want unchanged cost_adjustment", rejected.InventoryStatus)
	}
	if err := tx.Model(&Product{}).Where("id=?", inputs[1].ID).Update("inventory_status", "in_stock").Error; err != nil {
		t.Fatal(err)
	}
	session, err := repo.StartProductCostMerge(ctx, org, actor, []string{inputs[0].ID, inputs[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	var startedInputs []Product
	if err := tx.Where("id IN ?", []string{inputs[0].ID, inputs[1].ID}).Order("id").Find(&startedInputs).Error; err != nil {
		t.Fatal(err)
	}
	if len(startedInputs) != 2 {
		t.Fatalf("started merge inputs=%d, want 2", len(startedInputs))
	}
	for _, input := range startedInputs {
		if input.InventoryStatus != "cost_adjustment" {
			t.Fatalf("merge source %s status after start=%q, want cost_adjustment", input.ID, input.InventoryStatus)
		}
		if input.BaseSalePriceMinor != 0 {
			t.Fatalf("merge source %s planned sale price after start=%d, want 0", input.ID, input.BaseSalePriceMinor)
		}
		if !strings.Contains(input.InternalComment, "もともとの想定売価:") {
			t.Fatalf("merge source %s did not preserve its planned sale price: %q", input.ID, input.InternalComment)
		}
	}
	date, err := time.Parse("2006-01-02", string(session.BusinessDate))
	if err != nil {
		t.Fatal(err)
	}
	if !isProductCodeForDate(session.ManagementCode, date) {
		t.Fatal("wrong start-date code")
	}
	var brand string
	if err := tx.Raw("SELECT code FROM brands WHERE id=?", inputs[0].BrandID).Scan(&brand).Error; err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, p := range inputs {
		cost := p.CostAmountMinor
		if p.CostCurrency != "JPY" {
			tx.Raw("SELECT converted_total_jpy FROM purchase_slip_lines WHERE id=?", p.PurchaseSlipLineID).Scan(&cost)
		}
		total += cost
	}
	output := CostAdjustmentOutputDraft{Kind: "product", Position: 1, AllocatedCostJPYMinor: total + 1, Product: &CostAdjustmentProductDraft{BrandCode: brand, ModelNumber: "merge transactional test", Notes: "preserve notes"}}
	if _, err := repo.ConfirmProductCostMerge(ctx, org, actor, session.ID, "MERGE-TEST", output); err != ErrCostAllocation {
		t.Fatalf("mismatch error=%v", err)
	}
	output.AllocatedCostJPYMinor = total
	result, err := repo.ConfirmProductCostMerge(ctx, org, actor, session.ID, "MERGE-TEST", output)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs[0].ManagementCode != session.ManagementCode {
		t.Fatal("reserved code changed")
	}
	if err := tx.Exec("SET CONSTRAINTS ALL IMMEDIATE").Error; err != nil {
		t.Fatal(err)
	}
	var created Product
	if err := tx.Where("id=?", result.Outputs[0].RecordID).Take(&created).Error; err != nil {
		t.Fatal(err)
	}
	if created.CostAmountMinor != total || created.SKU != "MERGE-TEST" || !strings.Contains(created.Notes, result.SlipNumber) {
		t.Fatalf("new product mismatch: %+v", created)
	}
	if created.PurchaseSlipLineID != "" {
		t.Fatal("cost merge output must belong to the adjustment slip, not the purchase slip")
	}
	if created.SupplierID != "" || created.SupplierRoleID != "" {
		t.Fatalf("cost merge output supplier must be blank: supplier_id=%q supplier_role_id=%q", created.SupplierID, created.SupplierRoleID)
	}
	var mergePurchaseSnapshotAfter string
	tx.Raw(`SELECT COALESCE(jsonb_agg(to_jsonb(line) ORDER BY line.line_number)::text,'[]')
		FROM purchase_slip_lines line WHERE line.purchase_slip_id=(SELECT purchase_slip_id FROM purchase_slip_lines WHERE id=?)`, inputs[0].PurchaseSlipLineID).Scan(&mergePurchaseSnapshotAfter)
	if mergePurchaseSnapshotAfter != mergePurchaseSnapshotBefore {
		t.Fatal("cost merge changed the acquisition-time purchase slip")
	}
	for _, p := range inputs {
		var current Product
		tx.Where("id=?", p.ID).Take(&current)
		if current.CostAmountMinor != p.CostAmountMinor || current.CostCurrency != p.CostCurrency {
			t.Fatal("source historical cost was changed")
		}
		if current.InventoryStatus != "cost_adjustment" || !current.InventoryValuationExcluded || current.InventoryValuationExclusionReason != "cost_merge_source" {
			t.Fatal("source was not excluded from current inventory valuation")
		}
	}
	if _, err := repo.ConfirmProductCostMerge(ctx, org, actor, session.ID, "MERGE-TEST", output); err != ErrCostAdjustmentExists {
		t.Fatalf("duplicate error=%v", err)
	}
}

func TestDirectCostAdjustmentIssuesNewSlipWithoutChangingPurchasePostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_MERGE_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZAIKO_MERGE_TEST_DSN for transactional PostgreSQL verification")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	var product Product
	if err := tx.Where(`inventory_status='in_stock' AND deleted_at IS NULL AND purchase_slip_line_id IS NOT NULL
		AND cost_amount_minor > 100 AND NOT EXISTS(SELECT 1 FROM product_cost_merge_inputs c WHERE c.product_id=products.id)`).
		Order("organization_id,id").Take(&product).Error; err != nil {
		t.Fatal(err)
	}
	var actor string
	tx.Raw("SELECT id FROM users WHERE organization_id=? LIMIT 1", product.OrganizationID).Scan(&actor)
	var purchaseSnapshotBefore string
	tx.Raw(`SELECT COALESCE(jsonb_agg(to_jsonb(line) ORDER BY line.line_number)::text,'[]')
		FROM purchase_slip_lines line WHERE line.purchase_slip_id=(SELECT purchase_slip_id FROM purchase_slip_lines WHERE id=?)`, product.PurchaseSlipLineID).Scan(&purchaseSnapshotBefore)
	repo := &Repository{db: tx, driver: "postgres"}
	if _, err := repo.ConfirmDirectCostAdjustment(context.Background(), DirectCostAdjustmentInput{
		OrganizationID: product.OrganizationID, ActorUserID: actor, Direction: "increase",
		Items: []DirectCostAdjustmentItemInput{{ProductID: product.ID, AdjustmentAmountJPY: 100}},
	}); err != ErrCostAdjustmentState {
		t.Fatalf("blank direct-adjustment reason error=%v, want ErrCostAdjustmentState", err)
	}
	if _, err := repo.StartDirectProductCostAdjustment(context.Background(), product.OrganizationID, product.ID, actor); err != nil {
		t.Fatal(err)
	}
	first, err := repo.ConfirmDirectCostAdjustment(context.Background(), DirectCostAdjustmentInput{
		OrganizationID: product.OrganizationID, ActorUserID: actor, Direction: "increase", Notes: "検証",
		Items: []DirectCostAdjustmentItemInput{{ProductID: product.ID, AdjustmentAmountJPY: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.SlipNumber == "" || len(first.Items) != 1 || first.Items[0].AdjustedCostJPY != first.Items[0].PreviousCostJPY+100 {
		t.Fatalf("unexpected first adjustment: %+v", first)
	}
	var firstLineNote string
	if err := tx.Raw("SELECT notes FROM direct_cost_adjustment_lines WHERE cost_adjustment_id=?", first.ID).Scan(&firstLineNote).Error; err != nil {
		t.Fatal(err)
	}
	if firstLineNote != "増額｜理由：検証｜金額：+¥100" {
		t.Fatalf("direct adjustment line note=%q", firstLineNote)
	}
	var purchaseSnapshotAfter string
	tx.Raw(`SELECT COALESCE(jsonb_agg(to_jsonb(line) ORDER BY line.line_number)::text,'[]')
		FROM purchase_slip_lines line WHERE line.purchase_slip_id=(SELECT purchase_slip_id FROM purchase_slip_lines WHERE id=?)`, product.PurchaseSlipLineID).Scan(&purchaseSnapshotAfter)
	if purchaseSnapshotAfter != purchaseSnapshotBefore {
		t.Fatal("direct cost adjustment changed the acquisition-time purchase slip")
	}
	var adjustedProduct Product
	if err := tx.Where("id=?", product.ID).Take(&adjustedProduct).Error; err != nil {
		t.Fatal(err)
	}
	if adjustedProduct.InventoryStatus != "cost_adjustment" {
		t.Fatalf("direct adjustment status=%q, want cost_adjustment", adjustedProduct.InventoryStatus)
	}
	page, err := repo.Products(context.Background(), product.OrganizationID, ProductFilter{Query: product.ProductCode, Page: 1, PageSize: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("load adjusted inventory cost: items=%d error=%v", len(page.Items), err)
	}
	if page.Items[0].InventoryCostJPYMinor != first.Items[0].AdjustedCostJPY {
		t.Fatalf("inventory purchase-rate cost=%d, want adjusted %d", page.Items[0].InventoryCostJPYMinor, first.Items[0].AdjustedCostJPY)
	}
	if _, err := repo.ConfirmDirectCostAdjustment(context.Background(), DirectCostAdjustmentInput{
		OrganizationID: product.OrganizationID, ActorUserID: actor, Direction: "decrease", Notes: "再調整",
		Items: []DirectCostAdjustmentItemInput{{ProductID: product.ID, AdjustmentAmountJPY: 1}},
	}); err != ErrCostAdjustmentState {
		t.Fatalf("cost-adjustment product error=%v, want ErrCostAdjustmentState", err)
	}
	var rejected Product
	if err := tx.Where("id=?", product.ID).Take(&rejected).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.InventoryStatus != "cost_adjustment" || rejected.CostAmountMinor != adjustedProduct.CostAmountMinor {
		t.Fatal("rejected repeated adjustment changed the product")
	}
	if err := tx.Model(&Product{}).Where("id=?", product.ID).Update("inventory_status", "in_stock").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.StartDirectProductCostAdjustment(context.Background(), product.OrganizationID, product.ID, actor); err != nil {
		t.Fatal(err)
	}
	second, err := repo.ConfirmDirectCostAdjustment(context.Background(), DirectCostAdjustmentInput{
		OrganizationID: product.OrganizationID, ActorUserID: actor, Direction: "decrease", Notes: "再調整",
		Items: []DirectCostAdjustmentItemInput{{ProductID: product.ID, AdjustmentAmountJPY: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.SlipNumber == first.SlipNumber {
		t.Fatal("each direct cost adjustment must issue a new slip")
	}
	tx.Raw(`SELECT COALESCE(jsonb_agg(to_jsonb(line) ORDER BY line.line_number)::text,'[]')
		FROM purchase_slip_lines line WHERE line.purchase_slip_id=(SELECT purchase_slip_id FROM purchase_slip_lines WHERE id=?)`, product.PurchaseSlipLineID).Scan(&purchaseSnapshotAfter)
	if purchaseSnapshotAfter != purchaseSnapshotBefore {
		t.Fatal("repeated direct cost adjustment changed the acquisition-time purchase slip")
	}
}
