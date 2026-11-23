package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"os"
	"strings"
	"testing"
)

// Requires an explicitly named disposable database, never the app database.
func TestLinkageRoundTripPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_LINKAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_linkage_test") {
		t.Fatal("refusing non-test database")
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
	must(repo.SeedPreviewIdentity(ctx, "test-only-password-admin", "test-only-password-worker"))
	must(repo.SeedPreviewMasters(ctx))
	var org, admin, worker, supplier, buyer, brand string
	for _, q := range []struct {
		SQL string
		To  *string
	}{
		{"SELECT id FROM organizations WHERE code='PREVIEW'", &org},
		{"SELECT id FROM users WHERE username='admin' LIMIT 1", &admin},
		{"SELECT id FROM users WHERE role_key='worker' LIMIT 1", &worker},
		{"SELECT role_code FROM partner_roles WHERE role_type='supplier' AND is_active ORDER BY role_code LIMIT 1", &supplier},
		{"SELECT role_code FROM partner_roles WHERE role_type='buyer' AND is_active ORDER BY role_code LIMIT 1", &buyer},
		{"SELECT code FROM brands WHERE is_active ORDER BY code LIMIT 1", &brand},
	} {
		must(repo.db.Raw(q.SQL).Scan(q.To).Error)
		if *q.To == "" {
			t.Fatalf("seed lookup: %s", q.SQL)
		}
	}
	must(repo.db.Exec(`INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('linkage-fx',?,'USD','JPY',15500,100,'test',?,now(),now())`, org, admin).Error)
	product := func(box string) SingleProductResult {
		t.Helper()
		p, e := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 100, BaseSaleCurrency: "USD", BoxCode: box})
		must(e)
		return p
	}
	p := product("BOX-01")
	boxes, e := repo.PublicationBoxes(ctx, org)
	must(e)
	if len(boxes) != 10 || len(boxes[0].ProductCodes) != 1 || boxes[0].ProductCodes[0] != p.Product.ProductCode {
		t.Fatalf("product BOX lost: %+v", boxes)
	}
	var countBefore, countAfter int64
	must(repo.db.Table("products").Count(&countBefore).Error)
	_, e = repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 100, BaseSaleCurrency: "USD", BoxCode: "BOX-99"})
	if !errors.Is(e, ErrBoxNotFound) {
		t.Fatalf("invalid box: %v", e)
	}
	must(repo.db.Table("products").Count(&countAfter).Error)
	if countAfter != countBefore {
		t.Fatal("failed BOX left product behind")
	}
	purchase, e := repo.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, SupplierCode: supplier, PurchaseDate: "2026-09-17", Lines: []PurchaseLineInput{{Quantity: 1, BrandCode: brand, CostCurrency: "JPY", UnitCostMinor: 1234, BaseSaleCurrency: "USD", BaseSalePriceMinor: 100, BoxCode: "BOX-02"}}})
	must(e)
	purchase, e = repo.ConfirmPurchase(ctx, org, purchase.ID, admin)
	must(e)
	boxes, e = repo.PublicationBoxes(ctx, org)
	must(e)
	found := false
	for _, b := range boxes {
		if b.BoxCode == "BOX-02" && len(b.ProductCodes) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("purchase BOX lost")
	}
	// Header corrections must propagate to the original product records.
	var roleID, otherSupplier string
	must(repo.db.Table("partner_roles").Select("id").Where("organization_id=? AND role_type='supplier' AND role_code<>? AND is_active", org, supplier).Order("role_code").Limit(1).Scan(&roleID).Error)
	if roleID == "" {
		t.Fatal("second supplier missing")
	}
	must(repo.db.Table("partner_roles").Select("role_code").Where("id=?", roleID).Scan(&otherSupplier).Error)
	var staff struct{ ID, StaffCode string }
	must(repo.db.Table("staff_profiles").Where("organization_id=?", org).Order("staff_code").First(&staff).Error)
	changes := PurchaseRevisionInput{PurchaseDate: "2026-09-17", OrganizationID: org, ActorUserID: admin, SupplierCode: otherSupplier, StaffCode: staff.StaffCode, Reason: "audit correction", Lines: []PurchaseRevisionLine{{ID: purchase.Lines[0].ID, SKU: purchase.Lines[0].SKU, UnitCostMinor: 1234, BaseSalePriceMinor: 100}}}
	_, e = repo.RevisePurchase(ctx, purchase.ID, changes)
	must(e)
	var corrected struct{ SupplierRoleID, PurchaseStaffProfileID string }
	must(repo.db.Table("products").Where("purchase_slip_line_id=?", purchase.Lines[0].ID).First(&corrected).Error)
	if corrected.SupplierRoleID != roleID || corrected.PurchaseStaffProfileID != staff.ID {
		t.Fatalf("purchase correction not linked: %+v", corrected)
	}
	// Exact manually entered yen survives create, confirm, revision and reload.
	yen := int64(123456)
	ship, e := repo.CreateShipment(ctx, ShipmentCreateInput{OrganizationID: org, ActorUserID: admin, BuyerCode: buyer, ShipmentDate: "2026-09-17", DisplayCurrency: "JPY", ProductCodes: []string{p.Product.ProductCode}, Items: []ShipmentPriceInput{{ProductCode: p.Product.ProductCode, SalePriceJPYMinor: &yen}}})
	must(e)
	ship, e = repo.ConfirmShipment(ctx, org, ship.ID, admin)
	must(e)
	if ship.Lines[0].ConvertedSalePriceJPY != yen {
		t.Fatal("entered yen changed")
	}
	revise := ShipmentRevisionInput{OrganizationID: org, ActorUserID: admin, ShipmentDate: "2026-09-18", BuyerCode: buyer, Reason: "entered correction", Notes: "saved note", ExpectedUpdatedAt: ship.UpdatedAt, Lines: []ShipmentRevisionLine{{ID: ship.Lines[0].ID, UnitPriceMinor: 123457}}}
	ship, e = repo.ReviseShipment(ctx, ship.ID, revise)
	must(e)
	loaded, e := repo.ShipmentSlip(ctx, org, ship.ID)
	must(e)
	if loaded.Lines[0].ConvertedSalePriceJPY != 123457 || !strings.Contains(loaded.Notes, "entered correction") {
		t.Fatal("shipment revision lost")
	}
	revise.ExpectedUpdatedAt = loaded.UpdatedAt
	revise.Notes = loaded.Notes
	_, e = repo.ReviseShipment(ctx, ship.ID, revise)
	if !errors.Is(e, ErrSlipRevisionUnchanged) {
		t.Fatalf("reason-only revision: %v", e)
	}
	var destination struct{ Code, Name, Region string }
	must(repo.db.Table("partner_roles AS pr").Select("pr.role_code AS code,bp.legal_name AS name,bp.region_type AS region").Joins("JOIN business_partners bp ON bp.id=pr.partner_id").Where("pr.organization_id=? AND pr.role_type='buyer' AND pr.role_code<>? AND pr.is_active", org, buyer).Order("pr.role_code").Limit(1).Scan(&destination).Error)
	if destination.Code == "" {
		t.Fatal("second destination missing")
	}
	revise.BuyerCode = destination.Code
	ship, e = repo.ReviseShipment(ctx, ship.ID, revise)
	must(e)
	wantTax := 0
	if destination.Region == "domestic" {
		wantTax = 1000
	}
	if ship.RecipientName != destination.Name || ship.BuyerCode != destination.Code || ship.TaxRateBasisPoints != wantTax || ship.Lines[0].ConvertedSalePriceJPY != 123457 {
		t.Fatal("destination revision not linked")
	}
	_, e = repo.ReviseShipment(ctx, ship.ID, revise)
	if !errors.Is(e, ErrShipmentState) {
		t.Fatalf("stale editor accepted: %v", e)
	}
	_, e = repo.ReturnShipmentProduct(ctx, org, ship.ID, p.Product.ProductCode, admin)
	must(e)
	// Historical consignment status and QR must remain scoped to their own cycle.
	cons, e := repo.CreateConsignment(ctx, ConsignmentCreateInput{OrganizationID: org, ActorUserID: admin, ConsigneeCode: buyer, ConsignmentDate: "2026-09-17", ProductCodes: []string{p.Product.ProductCode}})
	must(e)
	_, e = repo.ReturnConsignmentProduct(ctx, org, cons.ID, p.Product.ProductCode, admin)
	must(e)
	newer, e := repo.CreateConsignment(ctx, ConsignmentCreateInput{OrganizationID: org, ActorUserID: admin, ConsigneeCode: buyer, ConsignmentDate: "2026-09-18", ProductCodes: []string{p.Product.ProductCode}})
	must(e)
	old, e := repo.ConsignmentSlip(ctx, org, cons.ID)
	must(e)
	if old.ProcessingStatus != "completed" || newer.ProcessingStatus != "processing" {
		t.Fatal("consignment history reopened")
	}
	_, e = repo.ReturnConsignmentProduct(ctx, org, cons.ID, p.Product.ProductCode, admin)
	if !errors.Is(e, ErrShipmentLineCompleted) {
		t.Fatalf("old QR accepted: %v", e)
	}
	live, e := repo.ProductByID(ctx, org, p.Product.ID)
	must(e)
	if live.InventoryStatus != "consigned" {
		t.Fatal("old QR changed new consignment")
	}
	_, e = repo.ReturnConsignmentProduct(ctx, org, newer.ID, p.Product.ProductCode, admin)
	must(e)
	// Persisted sales taxes: two fractional lines must round per line.
	p2 := product("")
	p3 := product("")
	sale, e := repo.CreateSale(ctx, SaleCreateInput{OrganizationID: org, ActorUserID: admin, BuyerCode: buyer, SaleDate: "2026-09-17", DisplayCurrency: "JPY", TaxMode: "taxable", TaxRateBasisPoints: 1000, Lines: []SaleLineInput{{ProductCode: p2.Product.ProductCode, UnitPriceMinor: 105}, {ProductCode: p3.Product.ProductCode, UnitPriceMinor: 105}}})
	must(e)
	if sale.TaxAmountMinor != 22 || sale.TotalMinor != 232 {
		t.Fatalf("sale tax %+v", sale)
	}
	// Worker reason, approval request, cross-user read and resolution persist together.
	session, e := repo.StartStocktake(ctx, org, admin)
	must(e)
	if len(session.Lines) == 0 {
		t.Fatal("stocktake empty")
	}
	line := session.Lines[0]
	otherUpdates := map[string]struct{ Reason, Note string }{}
	for _, other := range session.Lines[1:] {
		otherUpdates[other.ID] = struct{ Reason, Note string }{Reason: "その他", Note: "admin checked"}
	}
	session, e = repo.SaveStocktakeWithActor(ctx, org, session.ID, admin, true, otherUpdates, nil)
	must(e)
	update := map[string]struct{ Reason, Note string }{line.ID: {Reason: "その他", Note: "hand entered note"}}
	session, e = repo.SaveStocktakeWithActor(ctx, org, session.ID, worker, false, update, nil)
	must(e)
	approved, e := repo.ApprovalRequests(ctx, org, admin, database.RoleAdmin, "pending")
	must(e)
	var req ApprovalRequestRecord
	for _, a := range approved {
		if a.TargetID == line.ID {
			req = a
		}
	}
	if req.ID == "" {
		t.Fatal("admin cannot see worker request")
	}
	_, e = repo.SaveStocktakeWithActor(ctx, org, session.ID, worker, false, update, nil)
	must(e)
	var requestCount int64
	must(repo.db.Table("approval_requests").Where("target_type='stocktake_line' AND target_id=?", line.ID).Count(&requestCount).Error)
	if requestCount != 1 {
		t.Fatal("duplicate approval")
	}
	session, e = repo.CurrentStocktake(ctx, org)
	must(e)
	state := ""
	for _, l := range session.Lines {
		if l.ID == line.ID {
			state = l.ApprovalStatus
		}
	}
	if state != "pending" {
		t.Fatal("pending state lost")
	}
	_, e = repo.CompleteStocktake(ctx, org, session.ID, admin)
	if e == nil {
		t.Fatal("completed before approval")
	}
	_, e = repo.SaveStocktakeWithActor(ctx, org, session.ID, worker, false, nil, []string{line.ID})
	if e == nil {
		t.Fatal("worker bypassed approval")
	}
	_, e = repo.DecideApproval(ctx, org, req.ID, worker, "approved", "")
	if !errors.Is(e, ErrApprovalSelf) {
		t.Fatalf("self approval: %v", e)
	}
	_, e = repo.DecideApproval(ctx, org, req.ID, admin, "approved", "checked")
	must(e)
	session, e = repo.CurrentStocktake(ctx, org)
	must(e)
	for _, l := range session.Lines {
		if l.ID == line.ID && (l.ApprovalStatus != "approved" || l.ResolvedAt == nil || l.Note != "hand entered note") {
			t.Fatalf("approval not linked: %+v", l)
		}
	}
	_, e = repo.CompleteStocktake(ctx, org, session.ID, admin)
	must(e)
}
