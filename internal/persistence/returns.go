package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var (
	ErrReturnNotFound = errors.New("return slip not found")
	ErrReturnState    = errors.New("return slip cannot be changed in its current state")
)

type ReturnCreateInput struct {
	OrganizationID  string
	ActorUserID     string
	OperationType   string   `json:"operationType"`
	TransactionDate string   `json:"transactionDate"`
	BuyerCode       string   `json:"buyerCode"`
	SupplierCode    string   `json:"supplierCode"`
	PurchaseSlipNo  string   `json:"sourcePurchaseSlipNumber"`
	Carrier         string   `json:"carrier"`
	TrackingNumber  string   `json:"trackingNumber"`
	Reason          string   `json:"reason"`
	Notes           string   `json:"notes"`
	ProductCodes    []string `json:"productCodes"`
}

type ReturnLineRecord struct {
	ID           string `json:"id"`
	LineNumber   int    `json:"lineNumber"`
	ProductID    string `json:"productId"`
	ProductCode  string `json:"productCode"`
	Brand        string `json:"brand"`
	ModelNumber  string `json:"modelNumber"`
	CostAmount   int64  `json:"costAmountMinor"`
	CostCurrency string `json:"costCurrency"`
	FromStatus   string `json:"fromStatus"`
	ToStatus     string `json:"toStatus"`
}

type ReturnSlipRecord struct {
	ID              string             `json:"id"`
	SlipNumber      string             `json:"slipNumber"`
	OperationType   string             `json:"operationType"`
	TransactionDate DateString         `json:"transactionDate"`
	BuyerCode       string             `json:"buyerCode,omitempty"`
	BuyerName       string             `json:"buyerName,omitempty"`
	SupplierCode    string             `json:"supplierCode,omitempty"`
	SupplierName    string             `json:"supplierName,omitempty"`
	PurchaseSlipID  string             `json:"sourcePurchaseSlipId,omitempty"`
	PurchaseSlipNo  string             `json:"sourcePurchaseSlipNumber,omitempty"`
	Carrier         string             `json:"carrier"`
	TrackingNumber  string             `json:"trackingNumber"`
	Status          string             `json:"status"`
	Reason          string             `json:"reason"`
	Notes           string             `json:"notes"`
	ConfirmedAt     *time.Time         `json:"confirmedAt,omitempty"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
	Lines           []ReturnLineRecord `gorm:"-" json:"lines,omitempty"`
}

func (r *Repository) CreateReturn(ctx context.Context, input ReturnCreateInput) (ReturnSlipRecord, error) {
	date, err := time.Parse("2006-01-02", input.TransactionDate)
	if err != nil {
		return ReturnSlipRecord{}, err
	}
	productCodes := normalizeCodes(input.ProductCodes)
	if len(productCodes) == 0 || len(productCodes) > 100 ||
		(input.OperationType != "return" && input.OperationType != "takeout" && input.OperationType != "purchase_return") {
		return ReturnSlipRecord{}, ErrReturnState
	}
	var returnID string
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var buyerRoleID string
		var supplierRoleID string
		var purchaseSlipID string
		if input.OperationType == "purchase_return" {
			if strings.TrimSpace(input.SupplierCode) == "" || strings.TrimSpace(input.PurchaseSlipNo) == "" {
				return ErrReturnState
			}
			supplierRoleID, err = lookupSupplierRole(tx, input.OrganizationID, input.SupplierCode)
			if err != nil {
				return err
			}
			result := tx.Table("purchase_slips").Select("id").Where(
				"organization_id=? AND slip_number=?", input.OrganizationID, strings.TrimSpace(input.PurchaseSlipNo)).Scan(&purchaseSlipID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrPurchaseNotFound
			}
		} else if strings.TrimSpace(input.BuyerCode) != "" {
			buyerRoleID, err = lookupBuyerRole(tx, input.OrganizationID, input.BuyerCode)
			if err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		sequence, err := nextDocumentSequence(tx, input.OrganizationID, "return", date.Year(), now)
		if err != nil {
			return err
		}
		returnID, err = database.NewID("rtn")
		if err != nil {
			return err
		}
		slipNumber := fmt.Sprintf("RT-%04d-%04d", date.Year(), sequence)
		if err := tx.Exec(`INSERT INTO return_slips(
			id,organization_id,slip_number,operation_type,transaction_date,buyer_role_id,supplier_role_id,source_purchase_slip_id,status,reason,notes,
			carrier,tracking_number,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,'draft',?,?,?,?,?,?,?)`, returnID, input.OrganizationID, slipNumber, input.OperationType,
			date, nullIfEmpty(buyerRoleID), nullIfEmpty(supplierRoleID), nullIfEmpty(purchaseSlipID), strings.TrimSpace(input.Reason), strings.TrimSpace(input.Notes),
			strings.TrimSpace(input.Carrier), strings.TrimSpace(input.TrackingNumber), input.ActorUserID, now, now).Error; err != nil {
			return err
		}
		for index, productCode := range productCodes {
			var product struct{ ID, InventoryStatus, PurchaseSlipID string }
			result := tx.Raw(`SELECT p.id,p.inventory_status,COALESCE(pl.purchase_slip_id,'') AS purchase_slip_id
				FROM products p
				LEFT JOIN purchase_slip_lines pl ON pl.id=p.purchase_slip_line_id
				WHERE p.organization_id=? AND p.product_code=? AND p.deleted_at IS NULL FOR UPDATE OF p`,
				input.OrganizationID, productCode).Scan(&product)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrProductUnavailable
			}
			toStatus := "in_stock"
			if input.OperationType == "return" {
				if !map[string]bool{"sold": true, "shipped": true, "reserved": true}[product.InventoryStatus] {
					return ErrReturnState
				}
			} else if input.OperationType == "takeout" {
				toStatus = "shipped"
				if !map[string]bool{"in_stock": true, "reserved": true}[product.InventoryStatus] {
					return ErrReturnState
				}
				if product.InventoryStatus == "reserved" && buyerRoleID != "" {
					linked, err := productBelongsToBuyer(tx, input.OrganizationID, product.ID, buyerRoleID)
					if err != nil || !linked {
						return ErrProductConflict
					}
				}
				if product.InventoryStatus == "in_stock" {
					if err := tx.Exec(`UPDATE products SET inventory_status='reserved',updated_at=? WHERE id=?`, now, product.ID).Error; err != nil {
						return err
					}
				}
			} else {
				if product.PurchaseSlipID != purchaseSlipID || !map[string]bool{"in_stock": true, "purchasing": true}[product.InventoryStatus] {
					return ErrReturnState
				}
				toStatus = "cancelled"
				if err := tx.Exec(`UPDATE products SET inventory_status='return_pending',updated_at=? WHERE id=?`, now, product.ID).Error; err != nil {
					return err
				}
			}
			lineID, _ := database.NewID("rtl")
			if err := tx.Exec(`INSERT INTO return_lines(
				id,return_slip_id,line_number,product_id,from_status,to_status,created_at
			) VALUES(?,?,?,?,?,?,?)`, lineID, returnID, index+1, product.ID, product.InventoryStatus, toStatus, now).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ReturnSlipRecord{}, err
	}
	return r.ReturnSlip(ctx, input.OrganizationID, returnID)
}

func (r *Repository) ReturnSlips(ctx context.Context, organizationID string, limit int) ([]ReturnSlipRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var records []ReturnSlipRecord
	err := r.db.WithContext(ctx).Table("return_slips AS r").
		Select(`r.id,r.slip_number,r.operation_type,r.transaction_date,COALESCE(br.role_code,'') AS buyer_code,
			COALESCE(bp.legal_name,'') AS buyer_name,COALESCE(sr.role_code,'') AS supplier_code,
			COALESCE(sp.legal_name,'') AS supplier_name,COALESCE(r.source_purchase_slip_id,'') AS purchase_slip_id,
			COALESCE(ps.slip_number,'') AS purchase_slip_no,r.carrier,r.tracking_number,r.status,r.reason,r.notes,r.confirmed_at,r.created_at,r.updated_at`).
		Joins("LEFT JOIN partner_roles br ON br.id=r.buyer_role_id").Joins("LEFT JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("LEFT JOIN partner_roles sr ON sr.id=r.supplier_role_id").Joins("LEFT JOIN business_partners sp ON sp.id=sr.partner_id").
		Joins("LEFT JOIN purchase_slips ps ON ps.id=r.source_purchase_slip_id").
		Where("r.organization_id=?", organizationID).Order("r.transaction_date DESC,r.slip_number DESC").
		Limit(limit).Scan(&records).Error
	return records, err
}

func (r *Repository) ReturnSlip(ctx context.Context, organizationID, returnID string) (ReturnSlipRecord, error) {
	var record ReturnSlipRecord
	result := r.db.WithContext(ctx).Table("return_slips AS r").
		Select(`r.id,r.slip_number,r.operation_type,r.transaction_date,COALESCE(br.role_code,'') AS buyer_code,
			COALESCE(bp.legal_name,'') AS buyer_name,COALESCE(sr.role_code,'') AS supplier_code,
			COALESCE(sp.legal_name,'') AS supplier_name,COALESCE(r.source_purchase_slip_id,'') AS purchase_slip_id,
			COALESCE(ps.slip_number,'') AS purchase_slip_no,r.carrier,r.tracking_number,r.status,r.reason,r.notes,r.confirmed_at,r.created_at,r.updated_at`).
		Joins("LEFT JOIN partner_roles br ON br.id=r.buyer_role_id").Joins("LEFT JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("LEFT JOIN partner_roles sr ON sr.id=r.supplier_role_id").Joins("LEFT JOIN business_partners sp ON sp.id=sr.partner_id").
		Joins("LEFT JOIN purchase_slips ps ON ps.id=r.source_purchase_slip_id").
		Where("r.organization_id=? AND r.id=?", organizationID, returnID).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return ReturnSlipRecord{}, ErrReturnNotFound
	}
	if result.Error != nil {
		return ReturnSlipRecord{}, result.Error
	}
	if err := r.db.WithContext(ctx).Table("return_lines AS l").
		Select("l.id,l.line_number,l.product_id,p.product_code,p.brand,p.model_number,p.cost_amount_minor AS cost_amount,p.cost_currency,l.from_status,l.to_status").
		Joins("JOIN products p ON p.id=l.product_id").Where("l.return_slip_id=?", returnID).
		Order("l.line_number").Scan(&record.Lines).Error; err != nil {
		return ReturnSlipRecord{}, err
	}
	return record, nil
}

func (r *Repository) UpdateReturnTracking(ctx context.Context, organizationID, returnID, carrier, trackingNumber string) (ReturnSlipRecord, error) {
	result := r.db.WithContext(ctx).Table("return_slips").Where("organization_id=? AND id=? AND status<>'cancelled'", organizationID, returnID).
		Updates(map[string]any{"carrier": strings.TrimSpace(carrier), "tracking_number": strings.TrimSpace(trackingNumber), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return ReturnSlipRecord{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ReturnSlipRecord{}, ErrReturnNotFound
	}
	return r.ReturnSlip(ctx, organizationID, returnID)
}

func (r *Repository) ConfirmReturn(ctx context.Context, organizationID, returnID, actorUserID string) (ReturnSlipRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct{ Status, OperationType string }
		result := tx.Raw(`SELECT status,operation_type FROM return_slips WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, returnID).Scan(&slip)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrReturnNotFound
		}
		if slip.Status == "confirmed" {
			return nil
		}
		if slip.Status != "draft" {
			return ErrReturnState
		}
		var lines []struct{ ProductID, FromStatus, ToStatus string }
		if err := tx.Table("return_lines").Select("product_id,from_status,to_status").Where("return_slip_id=?", returnID).Order("line_number").Scan(&lines).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, line := range lines {
			var current string
			if err := tx.Raw(`SELECT inventory_status FROM products WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, line.ProductID).Scan(&current).Error; err != nil {
				return err
			}
			if slip.OperationType == "return" && !map[string]bool{"sold": true, "shipped": true, "reserved": true}[current] {
				return ErrReturnState
			}
			if slip.OperationType == "takeout" && current != "reserved" && current != "in_stock" {
				return ErrReturnState
			}
			if slip.OperationType == "purchase_return" && current != "return_pending" {
				return ErrReturnState
			}
			if err := tx.Exec(`UPDATE products SET inventory_status=?,updated_at=? WHERE id=?`, line.ToStatus, now, line.ProductID).Error; err != nil {
				return err
			}
			eventID, _ := database.NewID("ive")
			if err := tx.Exec(`INSERT INTO inventory_events(
				id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
			) VALUES(?,?,?,?,?,?,?, ?,?)`, eventID, organizationID, line.ProductID,
				"return_"+slip.OperationType+"_confirmed", current, line.ToStatus, "返品/持ち帰り伝票確定", actorUserID, now).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`UPDATE return_slips SET status='confirmed',confirmed_at=?,confirmed_by=?,updated_at=?
			WHERE organization_id=? AND id=?`, now, actorUserID, now, organizationID, returnID).Error
	})
	if err != nil {
		return ReturnSlipRecord{}, err
	}
	return r.ReturnSlip(ctx, organizationID, returnID)
}
