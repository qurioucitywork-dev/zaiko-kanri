package persistence

import (
	"context"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

// This payload describes a complete replacement of a returned draft. Identity,
// ownership, approval state and inventory transitions remain server-controlled.
type ReturnedDraftInput struct {
	Reason          string                  `json:"reason"`
	OriginalLineIDs []string                `json:"originalLineIds"`
	Purchase        *PurchaseCreateInput    `json:"purchase,omitempty"`
	Sale            *SaleCreateInput        `json:"sale,omitempty"`
	Shipment        *ShipmentCreateInput    `json:"shipment,omitempty"`
	Consignment     *ConsignmentCreateInput `json:"consignment,omitempty"`
	Return          *ReturnCreateInput      `json:"return,omitempty"`
}
type returnedDraftIdentity struct {
	auctionFees   map[string]AuctionFeeSnapshot
	id            string
	lineIDs       []string
	rates         map[string]fxSnapshot
	purchaseRates map[string]struct {
		currency string
		rate     fxSnapshot
	}
}

func returnedDraftNumber(tx *gorm.DB, org, table, prefix, idPrefix string, date, now time.Time, identity *returnedDraftIdentity) (string, string, error) {
	if identity != nil {
		var number string
		result := tx.Table(table).Select("slip_number").Where("organization_id=? AND id=? AND status IN ('draft','cancelled')", org, identity.id).Scan(&number)
		if result.Error != nil {
			return "", "", result.Error
		}
		if number == "" {
			return "", "", ErrReturnedApprovalState
		}
		return identity.id, number, nil
	}
	number, err := nextDailyDocumentNumber(tx, org, prefix, date, now)
	if err != nil {
		return "", "", err
	}
	id, err := database.NewID(idPrefix)
	return id, number, err
}
func returnedDraftLineID(identity *returnedDraftIdentity, index int, prefix string) (string, error) {
	if identity != nil && index < len(identity.lineIDs) && identity.lineIDs[index] != "" {
		return identity.lineIDs[index], nil
	}
	return database.NewID(prefix)
}
func returnedDraftFX(tx *gorm.DB, org, currency string, identity *returnedDraftIdentity) (fxSnapshot, error) {
	if identity != nil {
		if rate, ok := identity.rates[currency]; ok && rate.ID != "" && rate.RateScaled > 0 && rate.Scale > 0 {
			return rate, nil
		}
	}
	return latestFX(tx, org, currency)
}
func returnedPurchaseCostSnapshot(tx *gorm.DB, org string, amount int64, quantity int, currency string, identity *returnedDraftIdentity, index int) (int64, any, any, any, error) {
	if identity != nil && index < len(identity.lineIDs) {
		old, ok := identity.purchaseRates[identity.lineIDs[index]]
		if ok && currency == old.currency && currency != "JPY" && old.rate.ID != "" && old.rate.Scale > 0 {
			converted, err := convertCurrency(amount*int64(quantity), currency, "JPY", old.rate)
			return converted, old.rate.ID, old.rate.RateScaled, old.rate.Scale, err
		}
	}
	return purchaseCostSnapshot(tx, org, amount, quantity, currency)
}

func (r *Repository) rebuildReturnedDraft(ctx context.Context, org, actor string, before ReturnedApprovalDocument, input ReturnedDraftInput) (any, error) {
	if strings.TrimSpace(input.Reason) == "" || len([]rune(input.Reason)) > 1000 {
		return nil, ErrApprovalInvalid
	}
	count := 0
	for _, present := range []bool{input.Purchase != nil, input.Sale != nil, input.Shipment != nil, input.Consignment != nil, input.Return != nil} {
		if present {
			count++
		}
	}
	if count != 1 {
		return nil, ErrApprovalInvalid
	}
	target := before.Approval.TargetID
	identity := &returnedDraftIdentity{auctionFees: map[string]AuctionFeeSnapshot{}, id: target, lineIDs: input.OriginalLineIDs, rates: map[string]fxSnapshot{}, purchaseRates: map[string]struct {
		currency string
		rate     fxSnapshot
	}{}}
	lineTable, fk := "", ""
	switch doc := before.Document.(type) {
	case PurchaseSlipRecord:
		if input.Purchase == nil {
			return nil, ErrApprovalInvalid
		}
		lineTable, fk = "purchase_slip_lines", "purchase_slip_id"
		for _, line := range doc.Lines {
			if line.GeneratedProductCount > 0 || line.GeneratedPartCount > 0 {
				return nil, ErrPurchaseInUse
			}
			identity.auctionFees[line.ID] = line.AuctionFee
			identity.purchaseRates[line.ID] = struct {
				currency string
				rate     fxSnapshot
			}{line.CostCurrency, fxSnapshot{line.FXRateSnapshotID, line.FXRateScaled, line.FXScale}}
		}
		count = len(input.Purchase.Lines)
	case SaleSlipRecord:
		if input.Sale == nil {
			return nil, ErrApprovalInvalid
		}
		lineTable, fk = "sales_lines", "sales_slip_id"
		for _, line := range doc.Lines {
			identity.auctionFees[line.ID] = line.AuctionFee
		}
		count = len(input.Sale.Lines)
		currency := doc.DisplayCurrency
		if currency == "JPY" {
			currency = "USD"
		}
		identity.rates[currency] = fxSnapshot{doc.FXRateSnapshotID, doc.FXRateScaled, doc.FXScale}
		if input.Sale.DisplayCurrency != "JPY" && input.Sale.DisplayCurrency != "USD" && input.Sale.DisplayCurrency != "HKD" {
			return nil, ErrSaleState
		}
		if input.Sale.TaxMode != "taxable" && input.Sale.TaxMode != "out_of_scope" && input.Sale.TaxMode != "exempt" {
			return nil, ErrSaleState
		}
		if input.Sale.TaxRateBasisPoints < 0 || input.Sale.TaxRateBasisPoints > 10000 {
			return nil, ErrSaleState
		}
	case ShipmentSlipRecord:
		if input.Shipment == nil {
			return nil, ErrApprovalInvalid
		}
		lineTable, fk = "shipment_lines", "shipment_slip_id"
		count = len(input.Shipment.ProductCodes)
		if len(normalizeCodes(input.Shipment.ProductCodes)) != count {
			return nil, ErrProductConflict
		}
		identity.rates["USD"] = fxSnapshot{doc.FXRateSnapshotID, doc.FXRateScaled, doc.FXScale}
	case ConsignmentSlipRecord:
		if input.Consignment == nil {
			return nil, ErrApprovalInvalid
		}
		lineTable, fk = "consignment_lines", "consignment_slip_id"
		count = len(input.Consignment.ProductCodes)
		if len(normalizeCodes(input.Consignment.ProductCodes)) != count {
			return nil, ErrProductConflict
		}
		identity.rates["USD"] = fxSnapshot{doc.FXRateSnapshotID, doc.FXRateScaled, doc.FXScale}
	case ReturnSlipRecord:
		if input.Return == nil || input.Return.OperationType != doc.OperationType {
			return nil, ErrApprovalInvalid
		}
		lineTable, fk = "return_lines", "return_slip_id"
		count = len(input.Return.ProductCodes)
		if len(normalizeCodes(input.Return.ProductCodes)) != count {
			return nil, ErrProductConflict
		}
	default:
		return nil, ErrApprovalInvalid
	}
	adjustmentCount := 0
	if input.Return != nil {
		adjustmentCount = len(input.Return.AdjustmentLines)
	}
	if count+adjustmentCount < 1 || count+adjustmentCount > 100 || len(input.OriginalLineIDs) != count {
		return nil, ErrApprovalInvalid
	}
	var oldLines []struct{ ID, ProductID, FromStatus string }
	columns := "id,product_id"
	if lineTable == "purchase_slip_lines" {
		columns = "id"
	}
	if lineTable == "return_lines" {
		columns += ",from_status"
	}
	if err := r.db.Table(lineTable).Select(columns).Where(fk+"=?", target).Find(&oldLines).Error; err != nil {
		return nil, err
	}
	known, seen := map[string]bool{}, map[string]bool{}
	for _, line := range oldLines {
		known[line.ID] = true
	}
	for _, id := range input.OriginalLineIDs {
		if id == "" {
			continue
		}
		if !known[id] || seen[id] {
			return nil, ErrApprovalInvalid
		}
		seen[id] = true
	}
	// Lock inventory in stable order, then detach only this unconfirmed document.
	if lineTable != "purchase_slip_lines" {
		var ids []string
		for _, line := range oldLines {
			ids = append(ids, line.ProductID)
		}
		var locked []string
		if err := r.db.Raw("SELECT id FROM products WHERE organization_id=? AND id IN ? ORDER BY id FOR UPDATE", org, ids).Scan(&locked).Error; err != nil {
			return nil, err
		}
	}
	if err := r.db.Exec("DELETE FROM "+lineTable+" WHERE "+fk+"=?", target).Error; err != nil {
		return nil, err
	}
	for _, line := range oldLines {
		if line.ProductID == "" {
			continue
		}
		var status string
		if err := r.db.Table("products").Select("inventory_status").Where("organization_id=? AND id=?", org, line.ProductID).Scan(&status).Error; err != nil {
			return nil, err
		}
		to := ""
		if doc, ok := before.Document.(ReturnSlipRecord); ok && doc.OperationType == "purchase_return" && status == "return_pending" {
			to = line.FromStatus
		} else if status == "reserved" && (lineTable == "shipment_lines" || lineTable == "sales_lines" || (lineTable == "return_lines" && line.FromStatus == "in_stock")) {
			var holds int64
			if err := r.db.Raw(`SELECT COUNT(*) FROM (
    SELECT 1 FROM shipment_lines l JOIN shipment_slips s ON s.id=l.shipment_slip_id WHERE s.organization_id=? AND l.product_id=? AND s.status='draft'
    UNION ALL SELECT 1 FROM sales_lines l JOIN sales_slips s ON s.id=l.sales_slip_id WHERE s.organization_id=? AND l.product_id=? AND s.status IN ('draft','pending_approval')
    UNION ALL SELECT 1 FROM purchase_requests WHERE organization_id=? AND product_id=? AND status IN ('pending','approved')
    UNION ALL SELECT 1 FROM consignment_lines l JOIN consignment_slips s ON s.id=l.consignment_slip_id WHERE s.organization_id=? AND l.product_id=? AND s.status IN ('draft','confirmed')
    UNION ALL SELECT 1 FROM return_lines l JOIN return_slips s ON s.id=l.return_slip_id WHERE s.organization_id=? AND l.product_id=? AND s.status='draft'
   ) held`, org, line.ProductID, org, line.ProductID, org, line.ProductID, org, line.ProductID, org, line.ProductID).Scan(&holds).Error; err != nil {
				return nil, err
			}
			if holds == 0 {
				to = "in_stock"
			}
		}
		if to != "" && to != status {
			now := time.Now().UTC()
			if err := r.db.Table("products").Where("organization_id=? AND id=?", org, line.ProductID).Updates(map[string]any{"inventory_status": to, "updated_at": now}).Error; err != nil {
				return nil, err
			}
			eventID, err := database.NewID("ive")
			if err != nil {
				return nil, err
			}
			if err := r.db.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at) VALUES(?,?,?,'returned_draft_rebuilt',?,?,?,?,?)`, eventID, org, line.ProductID, status, to, "差戻し修正: "+input.Reason, actor, now).Error; err != nil {
				return nil, err
			}
		}
	}
	// Reuse normal registration validation and calculations, but update the locked
	// draft header in place. No number is allocated and no second slip is created.
	if v := input.Purchase; v != nil {
		v.OrganizationID = org
		v.ActorUserID = actor
		v.rebuild = identity
		return r.CreatePurchase(ctx, *v)
	}
	if v := input.Sale; v != nil {
		v.OrganizationID = org
		v.ActorUserID = actor
		v.rebuild = identity
		return r.CreateSale(ctx, *v)
	}
	if v := input.Shipment; v != nil {
		v.OrganizationID = org
		v.ActorUserID = actor
		v.rebuild = identity
		return r.CreateShipment(ctx, *v)
	}
	if v := input.Consignment; v != nil {
		v.OrganizationID = org
		v.ActorUserID = actor
		v.rebuild = identity
		v.draft = true
		return r.CreateConsignment(ctx, *v)
	}
	v := input.Return
	v.OrganizationID = org
	v.ActorUserID = actor
	v.rebuild = identity
	return r.CreateReturn(ctx, *v)
}
