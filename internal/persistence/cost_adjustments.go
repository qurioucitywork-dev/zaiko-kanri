package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCostAdjustmentState  = errors.New("invalid cost adjustment state")
	ErrCostAllocation       = errors.New("cost allocation does not match source cost")
	ErrCostAdjustmentExists = errors.New("cost adjustment already confirmed")
)

type CostAdjustmentProductDraft struct {
	BrandCode         string   `json:"brandCode"`
	ModelNumber       string   `json:"modelNumber"`
	ReferenceNumber   string   `json:"referenceNumber"`
	SerialNumber      string   `json:"serialNumber"`
	MaterialCode      string   `json:"materialCode"`
	MovementCode      string   `json:"movementCode"`
	ConditionCode     string   `json:"conditionCode"`
	AccessoryCodes    []string `json:"accessoryCodes"`
	BeltText          string   `json:"beltText"`
	DialText          string   `json:"dialText"`
	Notes             string   `json:"notes"`
	InternalComment   string   `json:"internalComment"`
	SalePriceUSDMinor int64    `json:"salePriceUsdMinor"`
}

type CostAdjustmentPartDraft struct {
	BrandCode         string `json:"brandCode"`
	ModelName         string `json:"modelName"`
	ReferenceNumber   string `json:"referenceNumber"`
	PartNameCode      string `json:"partNameCode"`
	DetailText        string `json:"detailText"`
	DetailMasterType  string `json:"detailMasterType"`
	DetailMasterCode  string `json:"detailMasterCode"`
	BraceletQuantity  *int   `json:"braceletQuantity"`
	Notes             string `json:"notes"`
	InternalComment   string `json:"internalComment"`
	SalePriceUSDMinor int64  `json:"salePriceUsdMinor"`
}

type CostAdjustmentOutputDraft struct {
	Position              int                         `json:"position"`
	Kind                  string                      `json:"kind"`
	AllocatedCostJPYMinor int64                       `json:"allocatedCostJpyMinor"`
	Product               *CostAdjustmentProductDraft `json:"product,omitempty"`
	Part                  *CostAdjustmentPartDraft    `json:"part,omitempty"`
}

type CostAdjustmentConfirmInput struct {
	OrganizationID  string
	ActorUserID     string
	SourceProductID string                      `json:"-"`
	Mode            string                      `json:"mode"`
	PartIDs         []string                    `json:"partIds,omitempty"`
	Outputs         []CostAdjustmentOutputDraft `json:"outputs"`
}

type CostAdjustmentOutputResult struct {
	Position       int    `json:"position"`
	Kind           string `json:"kind"`
	ManagementCode string `json:"managementCode"`
	RecordID       string `json:"recordId"`
	CostJPYMinor   int64  `json:"costJpyMinor"`
}

type CostAdjustmentConfirmResult struct {
	ID                string                       `json:"id"`
	AdjustmentDate    string                       `json:"adjustmentDate"`
	SourceProductCode string                       `json:"sourceProductCode"`
	SourceStatus      string                       `json:"sourceStatus"`
	Outputs           []CostAdjustmentOutputResult `json:"outputs"`
}

type costAdjustmentSourceLine struct {
	PurchaseSlipID    string
	PurchaseDate      time.Time
	SlipStatus        string
	LineNumber        int
	ConvertedTotalJPY int64
	PurchaseTaxMode   string
	TaxCategory       string
}

type costAdjustmentCombinePart struct {
	ID                 string
	PartCode           string
	FixedCostJPYMinor  int64
	PartName           string
	DetailText         string
	DetailMasterType   string
	DetailMasterCode   string
	BraceletQuantity   *int
	Notes              string
	Status             string
	PurchaseSlipLineID string
}

func costAdjustmentInternalComment(sourceCode, value string) string {
	prefix := "対象商品管理番号: " + strings.TrimSpace(sourceCode)
	extra := strings.TrimSpace(value)
	if extra == "" || strings.Contains(extra, prefix) {
		return prefix
	}
	return prefix + "\n" + extra
}

// ConfirmCostAdjustmentBreakdown atomically redistributes the original JPY
// acquisition value to newly numbered products and parts. The source remains
// on its original purchase slip as a zero-cost audit row.
func (r *Repository) ConfirmCostAdjustmentBreakdown(ctx context.Context, input CostAdjustmentConfirmInput) (CostAdjustmentConfirmResult, error) {
	if strings.TrimSpace(input.Mode) != "breakdown" || len(input.Outputs) < 2 || len(input.Outputs) > 20 {
		return CostAdjustmentConfirmResult{}, ErrCostAdjustmentState
	}
	positions := map[int]bool{}
	var allocatedTotal int64
	for _, output := range input.Outputs {
		if output.Position < 1 || output.Position > 20 || positions[output.Position] || output.AllocatedCostJPYMinor < 0 {
			return CostAdjustmentConfirmResult{}, ErrCostAdjustmentState
		}
		positions[output.Position] = true
		allocatedTotal += output.AllocatedCostJPYMinor
		if (output.Kind == "product" && output.Product == nil) || (output.Kind == "part" && output.Part == nil) || (output.Kind != "product" && output.Kind != "part") {
			return CostAdjustmentConfirmResult{}, ErrCostAdjustmentState
		}
	}

	result := CostAdjustmentConfirmResult{}
	jst := time.FixedZone("JST", 9*60*60)
	adjustmentDate := time.Now().In(jst)
	dateOnly, _ := time.Parse("2006-01-02", adjustmentDate.Format("2006-01-02"))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var source Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"organization_id=? AND id=? AND deleted_at IS NULL", input.OrganizationID, input.SourceProductID).Take(&source).Error; err != nil {
			return ErrProductUnavailable
		}
		if source.InventoryStatus != "cost_adjustment" || source.PurchaseSlipLineID == "" {
			return ErrCostAdjustmentState
		}
		var existing int64
		if err := tx.Table("cost_adjustments").Where("organization_id=? AND source_product_id=?", input.OrganizationID, source.ID).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return ErrCostAdjustmentExists
		}

		var sourceLine costAdjustmentSourceLine
		lineQuery := tx.Raw(`SELECT l.purchase_slip_id,s.purchase_date,s.status AS slip_status,l.line_number,
			COALESCE(l.converted_total_jpy,CASE WHEN l.cost_currency='JPY' THEN l.unit_cost_minor ELSE 0 END) AS converted_total_jpy,
			s.purchase_tax_mode,s.tax_category
			FROM purchase_slip_lines l JOIN purchase_slips s ON s.id=l.purchase_slip_id
			WHERE l.id=? AND s.organization_id=? FOR UPDATE`, source.PurchaseSlipLineID, input.OrganizationID).Scan(&sourceLine)
		if lineQuery.Error != nil || lineQuery.RowsAffected == 0 || sourceLine.SlipStatus != "confirmed" {
			return ErrCostAdjustmentState
		}
		if sourceLine.ConvertedTotalJPY < 0 || allocatedTotal != sourceLine.ConvertedTotalJPY {
			return ErrCostAllocation
		}

		adjustmentID, err := database.NewID("cad")
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO cost_adjustments(
			id,organization_id,source_product_id,source_purchase_slip_id,source_purchase_slip_line_id,
			adjustment_type,adjustment_date,source_product_code,source_cost_jpy_minor,allocated_cost_jpy_minor,
			status,confirmed_at,confirmed_by,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,'breakdown',?,?,?,?, 'confirmed',?,?,?,?,?)`,
			adjustmentID, input.OrganizationID, source.ID, sourceLine.PurchaseSlipID, source.PurchaseSlipLineID,
			dateOnly, source.ProductCode, sourceLine.ConvertedTotalJPY, allocatedTotal,
			now, input.ActorUserID, input.ActorUserID, now, now).Error; err != nil {
			return err
		}

		var nextLineNumber int
		if err := tx.Raw("SELECT COALESCE(MAX(line_number),0) FROM purchase_slip_lines WHERE purchase_slip_id=?", sourceLine.PurchaseSlipID).Scan(&nextLineNumber).Error; err != nil {
			return err
		}
		result = CostAdjustmentConfirmResult{
			ID: adjustmentID, AdjustmentDate: dateOnly.Format("2006-01-02"),
			SourceProductCode: source.ProductCode, SourceStatus: "broken_down",
			Outputs: make([]CostAdjustmentOutputResult, 0, len(input.Outputs)),
		}

		for _, output := range input.Outputs {
			nextLineNumber++
			lineID, err := database.NewID("pul")
			if err != nil {
				return err
			}
			itemID, err := database.NewID("cai")
			if err != nil {
				return err
			}
			if output.Kind == "product" {
				created, err := r.createCostAdjustmentProduct(tx, source, sourceLine.PurchaseSlipID, lineID, adjustmentID, itemID,
					nextLineNumber, output, dateOnly, now, input)
				if err != nil {
					return err
				}
				result.Outputs = append(result.Outputs, created)
			} else {
				created, err := r.createCostAdjustmentPart(tx, source, sourceLine, lineID, adjustmentID, itemID,
					nextLineNumber, output, dateOnly, now, input)
				if err != nil {
					return err
				}
				result.Outputs = append(result.Outputs, created)
			}
		}

		if err := tx.Exec(`UPDATE purchase_slip_lines SET unit_cost_minor=0,converted_total_jpy=0,
			base_sale_price_minor=0,cost_adjustment_id=? WHERE id=?`, adjustmentID, source.PurchaseSlipLineID).Error; err != nil {
			return err
		}
		if err := tx.Model(&Product{}).Where("organization_id=? AND id=?", input.OrganizationID, source.ID).Updates(map[string]any{
			"cost_amount_minor": 0, "base_sale_price_minor": 0, "inventory_status": "broken_down",
			"cost_adjustment_id": adjustmentID, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		eventID, _ := database.NewID("ive")
		return tx.Exec(`INSERT INTO inventory_events(
			id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
		) VALUES(?,?,?,'cost_adjustment_breakdown_confirmed','cost_adjustment','broken_down',?,?,?)`,
			eventID, input.OrganizationID, source.ID, fmt.Sprintf("原価調整 %s を確定", adjustmentID), input.ActorUserID, now).Error
	})
	if err != nil {
		return CostAdjustmentConfirmResult{}, err
	}
	return result, nil
}

// ConfirmCostAdjustmentCombine consumes the selected parts into the existing
// product. The purchase-slip price columns remain untouched as immutable
// purchase snapshots; only the inventory product receives the combined JPY
// cost and a management number based on the confirmation date.
func (r *Repository) ConfirmCostAdjustmentCombine(ctx context.Context, input CostAdjustmentConfirmInput) (CostAdjustmentConfirmResult, error) {
	if strings.TrimSpace(input.Mode) != "combine" || len(input.PartIDs) == 0 || len(input.PartIDs) > 20 {
		return CostAdjustmentConfirmResult{}, ErrCostAdjustmentState
	}
	partIDs := make([]string, 0, len(input.PartIDs))
	seen := map[string]bool{}
	for _, raw := range input.PartIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			return CostAdjustmentConfirmResult{}, ErrCostAdjustmentState
		}
		seen[id] = true
		partIDs = append(partIDs, id)
	}

	result := CostAdjustmentConfirmResult{}
	jst := time.FixedZone("JST", 9*60*60)
	adjustmentDate := time.Now().In(jst)
	dateOnly, _ := time.Parse("2006-01-02", adjustmentDate.Format("2006-01-02"))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var source Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"organization_id=? AND id=? AND deleted_at IS NULL", input.OrganizationID, input.SourceProductID).Take(&source).Error; err != nil {
			return ErrProductUnavailable
		}
		if source.InventoryStatus != "cost_adjustment" || source.PurchaseSlipLineID == "" {
			return ErrCostAdjustmentState
		}
		var existing int64
		if err := tx.Table("cost_adjustments").Where("organization_id=? AND source_product_id=?", input.OrganizationID, source.ID).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return ErrCostAdjustmentExists
		}

		var sourceLine costAdjustmentSourceLine
		lineQuery := tx.Raw(`SELECT l.purchase_slip_id,s.purchase_date,s.status AS slip_status,l.line_number,
			COALESCE(l.converted_total_jpy,CASE WHEN l.cost_currency='JPY' THEN l.unit_cost_minor ELSE 0 END) AS converted_total_jpy,
			s.purchase_tax_mode,s.tax_category
			FROM purchase_slip_lines l JOIN purchase_slips s ON s.id=l.purchase_slip_id
			WHERE l.id=? AND s.organization_id=? FOR UPDATE`, source.PurchaseSlipLineID, input.OrganizationID).Scan(&sourceLine)
		if lineQuery.Error != nil || lineQuery.RowsAffected == 0 || sourceLine.SlipStatus != "confirmed" {
			return ErrCostAdjustmentState
		}

		var parts []costAdjustmentCombinePart
		partsQuery := tx.Raw(`SELECT p.id,p.part_code,p.fixed_cost_jpy_minor,p.part_name_text AS part_name,
			p.detail_text,p.detail_master_type,p.detail_master_code,p.bracelet_quantity,p.notes,p.status,
			COALESCE(p.purchase_slip_line_id,'') AS purchase_slip_line_id
			FROM parts p WHERE p.organization_id=? AND p.id IN ? FOR UPDATE`, input.OrganizationID, partIDs).Scan(&parts)
		if partsQuery.Error != nil {
			return partsQuery.Error
		}
		if len(parts) != len(partIDs) {
			return ErrCostAdjustmentState
		}

		combinedCost := sourceLine.ConvertedTotalJPY
		accessoryNames := make([]string, 0)
		accessorySet := map[string]bool{}
		for _, value := range strings.Split(source.Accessories, ",") {
			name := strings.TrimSpace(value)
			if name != "" && !accessorySet[strings.ToUpper(name)] {
				accessorySet[strings.ToUpper(name)] = true
				accessoryNames = append(accessoryNames, name)
			}
		}
		materialID := source.MaterialID
		beltText := source.BeltText
		dialText := source.DialText
		braceletQuantity := 0
		braceletSelected := source.BraceletQuantity != nil
		if source.BraceletQuantity != nil {
			braceletQuantity = *source.BraceletQuantity
		}
		noteLines := make([]string, 0)
		if strings.TrimSpace(source.Notes) != "" {
			noteLines = append(noteLines, strings.TrimSpace(source.Notes))
		}
		commentLines := make([]string, 0)
		if strings.TrimSpace(source.InternalComment) != "" {
			commentLines = append(commentLines, strings.TrimSpace(source.InternalComment))
		}
		partCodes := make([]string, 0, len(parts))
		accessoryIDs := make([]string, 0)
		for _, part := range parts {
			if part.Status != "cost_adjustment" || part.FixedCostJPYMinor < 0 {
				return ErrCostAdjustmentState
			}
			combinedCost += part.FixedCostJPYMinor
			partCodes = append(partCodes, part.PartCode)
			commentLines = append(commentLines, "結合パーツ管理番号: "+part.PartCode)
			partName := strings.TrimSpace(part.PartName)
			detail := strings.TrimSpace(part.DetailText)
			switch partName {
			case "素材":
				if part.DetailMasterType == "material" && strings.TrimSpace(part.DetailMasterCode) != "" {
					id, _, err := lookupCatalog(tx, "materials", input.OrganizationID, part.DetailMasterCode, true)
					if err != nil {
						return err
					}
					materialID = id
				}
			case "ベルト素材":
				if part.DetailMasterType == "belt" && strings.TrimSpace(part.DetailMasterCode) != "" {
					_, name, err := lookupCatalog(tx, "belt_materials", input.OrganizationID, part.DetailMasterCode, true)
					if err != nil {
						return err
					}
					beltText = name
				} else if detail != "" {
					beltText = detail
				}
			case "文字盤":
				if part.DetailMasterType == "dial" && strings.TrimSpace(part.DetailMasterCode) != "" {
					_, name, err := lookupCatalog(tx, "dials", input.OrganizationID, part.DetailMasterCode, true)
					if err != nil {
						return err
					}
					dialText = name
				} else if detail != "" {
					dialText = detail
				}
			case "BRACELET PARTS":
				braceletSelected = true
				if part.BraceletQuantity != nil {
					braceletQuantity += *part.BraceletQuantity
				}
			default:
				var accessory struct{ ID, Name string }
				lookup := tx.Raw(`SELECT id,name FROM accessories WHERE organization_id=? AND UPPER(BTRIM(name))=UPPER(BTRIM(?)) AND is_active=TRUE LIMIT 1`,
					input.OrganizationID, partName).Scan(&accessory)
				if lookup.Error != nil {
					return lookup.Error
				}
				if accessory.ID != "" {
					accessoryIDs = append(accessoryIDs, accessory.ID)
					if !accessorySet[strings.ToUpper(accessory.Name)] {
						accessorySet[strings.ToUpper(accessory.Name)] = true
						accessoryNames = append(accessoryNames, accessory.Name)
					}
				} else {
					line := partName
					if detail != "" {
						line += ": " + detail
					}
					if strings.TrimSpace(part.Notes) != "" {
						line += "（" + strings.TrimSpace(part.Notes) + "）"
					}
					noteLines = append(noteLines, line)
				}
			}
		}

		if braceletSelected {
			var braceletAccessory struct{ ID, Name string }
			if err := tx.Raw(`SELECT id,name FROM accessories WHERE organization_id=? AND UPPER(BTRIM(name))='BRACELET PARTS' AND is_active=TRUE LIMIT 1`,
				input.OrganizationID).Scan(&braceletAccessory).Error; err != nil {
				return err
			}
			if braceletAccessory.ID != "" {
				accessoryIDs = append(accessoryIDs, braceletAccessory.ID)
				if !accessorySet["BRACELET PARTS"] {
					accessorySet["BRACELET PARTS"] = true
					accessoryNames = append(accessoryNames, braceletAccessory.Name)
				}
			}
		}

		sequence, err := nextProductSequence(tx, input.OrganizationID, dateOnly, now)
		if err != nil {
			return err
		}
		newCode := formatProductCode(dateOnly, sequence)
		adjustmentID, err := database.NewID("cad")
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO cost_adjustments(
			id,organization_id,source_product_id,source_purchase_slip_id,source_purchase_slip_line_id,
			adjustment_type,adjustment_date,source_product_code,source_cost_jpy_minor,allocated_cost_jpy_minor,
			status,confirmed_at,confirmed_by,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,'combine',?,?,?,?, 'confirmed',?,?,?,?,?)`,
			adjustmentID, input.OrganizationID, source.ID, sourceLine.PurchaseSlipID, source.PurchaseSlipLineID,
			dateOnly, source.ProductCode, sourceLine.ConvertedTotalJPY, combinedCost,
			now, input.ActorUserID, input.ActorUserID, now, now).Error; err != nil {
			return err
		}
		for _, part := range parts {
			if err := tx.Exec(`INSERT INTO cost_adjustment_input_parts(cost_adjustment_id,part_id,source_part_code,source_cost_jpy_minor,created_at)
				VALUES(?,?,?,?,?)`, adjustmentID, part.ID, part.PartCode, part.FixedCostJPYMinor, now).Error; err != nil {
				return err
			}
		}
		quantityValue := any(nil)
		if braceletSelected {
			quantityValue = braceletQuantity
		}
		if err := tx.Model(&Product{}).Where("organization_id=? AND id=?", input.OrganizationID, source.ID).Updates(map[string]any{
			"product_code": newCode, "cost_amount_minor": combinedCost, "cost_currency": "JPY",
			"material_id": nullIfEmpty(materialID), "belt_text": beltText, "dial_text": dialText,
			"accessories": strings.Join(accessoryNames, ", "), "bracelet_quantity": quantityValue,
			"notes": strings.Join(noteLines, "\n"), "internal_comment": strings.Join(commentLines, "\n"),
			"inventory_status": "cost_adjustment", "cost_adjustment_id": adjustmentID, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		for _, accessoryID := range accessoryIDs {
			if err := tx.Exec(`INSERT INTO product_accessories(product_id,accessory_id,quantity) VALUES(?,?,1)
				ON CONFLICT (product_id,accessory_id) DO NOTHING`, source.ID, accessoryID).Error; err != nil {
				return err
			}
		}
		if err := tx.Table("parts").Where("organization_id=? AND id IN ?", input.OrganizationID, partIDs).Updates(map[string]any{
			"status": "invalid", "cost_amount_minor": 0, "fixed_cost_jpy_minor": 0,
			"cost_adjustment_id": adjustmentID, "updated_by": input.ActorUserID, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		itemID, _ := database.NewID("cai")
		if err := tx.Exec(`INSERT INTO cost_adjustment_items(
			id,cost_adjustment_id,position,item_kind,output_product_id,management_code,allocated_cost_jpy_minor,status,created_at
		) VALUES(?,?,1,'product',?,?,?,'cost_adjustment',?)`, itemID, adjustmentID, source.ID, newCode, combinedCost, now).Error; err != nil {
			return err
		}
		eventID, _ := database.NewID("ive")
		if err := tx.Exec(`INSERT INTO inventory_events(
			id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
		) VALUES(?,?,?,'cost_adjustment_combine_confirmed','cost_adjustment','cost_adjustment',?,?,?)`,
			eventID, input.OrganizationID, source.ID, fmt.Sprintf("パーツ %s を結合し管理番号を %s に変更", strings.Join(partCodes, ", "), newCode), input.ActorUserID, now).Error; err != nil {
			return err
		}
		result = CostAdjustmentConfirmResult{
			ID: adjustmentID, AdjustmentDate: dateOnly.Format("2006-01-02"), SourceProductCode: source.ProductCode,
			SourceStatus: "cost_adjustment", Outputs: []CostAdjustmentOutputResult{{
				Position: 1, Kind: "product", ManagementCode: newCode, RecordID: source.ID, CostJPYMinor: combinedCost,
			}},
		}
		return nil
	})
	if err != nil {
		return CostAdjustmentConfirmResult{}, err
	}
	return result, nil
}

func (r *Repository) createCostAdjustmentProduct(tx *gorm.DB, source Product, purchaseSlipID, lineID, adjustmentID, itemID string,
	lineNumber int, output CostAdjustmentOutputDraft, date, now time.Time, input CostAdjustmentConfirmInput) (CostAdjustmentOutputResult, error) {
	draft := output.Product
	brandID, brandName, err := lookupCatalog(tx, "brands", input.OrganizationID, draft.BrandCode, true)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	materialID, _, err := lookupCatalog(tx, "materials", input.OrganizationID, draft.MaterialCode, false)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	movementID, _, err := lookupCatalog(tx, "movements", input.OrganizationID, draft.MovementCode, false)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	conditionID, conditionName, err := lookupCatalog(tx, "product_conditions", input.OrganizationID, draft.ConditionCode, false)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	accessoryIDs, accessoryNames, err := lookupAccessories(tx, input.OrganizationID, draft.AccessoryCodes)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	if serial := strings.TrimSpace(draft.SerialNumber); serial != "" {
		var duplicates int64
		if err := tx.Table("products").Where("organization_id=? AND UPPER(serial_number)=? AND deleted_at IS NULL AND inventory_status<>'cancelled'",
			input.OrganizationID, strings.ToUpper(serial)).Count(&duplicates).Error; err != nil {
			return CostAdjustmentOutputResult{}, err
		}
		if duplicates > 0 {
			return CostAdjustmentOutputResult{}, ErrDuplicateSerialReason
		}
	}
	sequence, err := nextProductSequence(tx, input.OrganizationID, date, now)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	code := formatProductCode(date, sequence)
	productID, err := database.NewID("prd")
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	accessoryJSON, _ := json.Marshal(draft.AccessoryCodes)
	comment := costAdjustmentInternalComment(source.ProductCode, draft.InternalComment)
	if err := tx.Exec(`INSERT INTO purchase_slip_lines(
		id,purchase_slip_id,line_number,quantity,unit_cost_minor,cost_currency,base_sale_price_minor,base_sale_currency,
		brand_id,material_id,movement_id,condition_id,shape_id,marking_id,brand_text,model_number,reference_number,serial_number,
		product_type,sku,generated_product_count,generated_part_count,converted_total_jpy,accessory_codes,belt_text,dial_text,
		notes,line_item_kind,cost_adjustment_id,created_at
	) VALUES(?,?,?,1,?,'JPY',?,'USD',?,?,?,?,?,?,?,?,?,?,?,?,1,0,?,?,?,?,?,'product',?,?)`,
		lineID, purchaseSlipID, lineNumber, output.AllocatedCostJPYMinor, draft.SalePriceUSDMinor,
		brandID, nullIfEmpty(materialID), nullIfEmpty(movementID), nullIfEmpty(conditionID),
		nullIfEmpty(source.ShapeID), nullIfEmpty(source.MarkingID), brandName, strings.TrimSpace(draft.ModelNumber),
		strings.TrimSpace(draft.ReferenceNumber), strings.TrimSpace(draft.SerialNumber), source.ProductType, source.SKU,
		output.AllocatedCostJPYMinor, string(accessoryJSON), strings.TrimSpace(draft.BeltText), strings.TrimSpace(draft.DialText),
		strings.TrimSpace(draft.Notes), adjustmentID, now).Error; err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	if err := tx.Exec(`INSERT INTO products(
		id,organization_id,product_code,sku,brand,brand_id,model_number,reference_number,serial_number,product_type,
		material_id,movement_id,condition_id,shape_id,marking_id,supplier_id,supplier_role_id,purchase_staff_profile_id,purchase_slip_line_id,
		purchase_date,cost_amount_minor,cost_currency,base_sale_price_minor,base_sale_currency,inventory_status,publication_status,
		condition_text,accessories,belt_text,dial_text,notes,internal_comment,cost_adjustment_id,created_at,updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'JPY',?,'USD','cost_adjustment','private',?,?,?,?,?,?,?,?,?)`,
		productID, input.OrganizationID, code, source.SKU, brandName, brandID, strings.TrimSpace(draft.ModelNumber),
		strings.TrimSpace(draft.ReferenceNumber), strings.TrimSpace(draft.SerialNumber), source.ProductType,
		nullIfEmpty(materialID), nullIfEmpty(movementID), nullIfEmpty(conditionID), nullIfEmpty(source.ShapeID), nullIfEmpty(source.MarkingID),
		source.SupplierID, nullIfEmpty(source.SupplierRoleID), nullIfEmpty(source.PurchaseStaffID), lineID, date,
		output.AllocatedCostJPYMinor, draft.SalePriceUSDMinor, conditionName, strings.Join(accessoryNames, ", "),
		strings.TrimSpace(draft.BeltText), strings.TrimSpace(draft.DialText), strings.TrimSpace(draft.Notes), comment, adjustmentID, now, now).Error; err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	for _, accessoryID := range accessoryIDs {
		if err := tx.Exec("INSERT INTO product_accessories(product_id,accessory_id,quantity) VALUES(?,?,1)", productID, accessoryID).Error; err != nil {
			return CostAdjustmentOutputResult{}, err
		}
	}
	if err := tx.Exec(`INSERT INTO cost_adjustment_items(
		id,cost_adjustment_id,position,item_kind,output_product_id,management_code,allocated_cost_jpy_minor,status,created_at
	) VALUES(?,?,?,'product',?,?,?,'cost_adjustment',?)`, itemID, adjustmentID, output.Position, productID, code, output.AllocatedCostJPYMinor, now).Error; err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	eventID, _ := database.NewID("ive")
	if err := tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
		VALUES(?,?,?,'cost_adjustment_output_created','','cost_adjustment',?,?,?)`, eventID, input.OrganizationID, productID,
		fmt.Sprintf("対象商品 %s から生成", source.ProductCode), input.ActorUserID, now).Error; err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	return CostAdjustmentOutputResult{Position: output.Position, Kind: "product", ManagementCode: code, RecordID: productID, CostJPYMinor: output.AllocatedCostJPYMinor}, nil
}

func (r *Repository) createCostAdjustmentPart(tx *gorm.DB, source Product, sourceLine costAdjustmentSourceLine, lineID, adjustmentID, itemID string,
	lineNumber int, output CostAdjustmentOutputDraft, date, now time.Time, input CostAdjustmentConfirmInput) (CostAdjustmentOutputResult, error) {
	draft := output.Part
	brandID, brandName, err := lookupCatalog(tx, "brands", input.OrganizationID, draft.BrandCode, false)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	partNameID, partName, err := lookupCatalog(tx, "part_names", input.OrganizationID, draft.PartNameCode, true)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	detailType, detailCode, detailText, err := resolvePartDetailMaster(tx, input.OrganizationID, partName,
		draft.DetailMasterType, draft.DetailMasterCode, draft.DetailText)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	quantity := draft.BraceletQuantity
	if !strings.EqualFold(strings.TrimSpace(partName), "BRACELET PARTS") {
		quantity = nil
	}
	sequence, err := nextPartSequence(tx, input.OrganizationID, date, now)
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	code := formatPartCode(date, sequence)
	partID, err := database.NewID("prt")
	if err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	comment := costAdjustmentInternalComment(source.ProductCode, draft.InternalComment)
	if err := tx.Exec(`INSERT INTO purchase_slip_lines(
		id,purchase_slip_id,line_number,quantity,unit_cost_minor,cost_currency,base_sale_price_minor,base_sale_currency,
		brand_id,brand_text,model_number,reference_number,product_type,sku,generated_product_count,generated_part_count,
		converted_total_jpy,notes,line_item_kind,cost_adjustment_id,created_at
	) VALUES(?,?,?,1,?,'JPY',?,'USD',?,?,?,?,?,?,0,1,?,?,'part',?,?)`,
		lineID, sourceLine.PurchaseSlipID, lineNumber, output.AllocatedCostJPYMinor, draft.SalePriceUSDMinor,
		nullIfEmpty(brandID), brandName, strings.TrimSpace(draft.ModelName), strings.TrimSpace(draft.ReferenceNumber),
		"パーツ: "+partName, source.SKU, output.AllocatedCostJPYMinor, strings.TrimSpace(draft.Notes), adjustmentID, now).Error; err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	if err := tx.Exec(`INSERT INTO parts(
		id,organization_id,part_code,purchase_date,purchase_staff_profile_id,supplier_role_id,purchase_tax_mode,tax_category,
		cost_amount_minor,cost_currency,fixed_cost_jpy_minor,sku,brand_id,brand_text,model_name,reference_number,
		part_name_id,part_name_text,detail_text,detail_master_type,detail_master_code,bracelet_quantity,sale_price_usd_minor,
		notes,internal_comment,status,cost_adjustment_id,purchase_slip_line_id,created_by,updated_by,created_at,updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,'JPY',?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'cost_adjustment',?,?,?,?,?,?)`,
		partID, input.OrganizationID, code, date, nullIfEmpty(source.PurchaseStaffID), nullIfEmpty(source.SupplierRoleID),
		sourceLine.PurchaseTaxMode, sourceLine.TaxCategory, output.AllocatedCostJPYMinor, output.AllocatedCostJPYMinor,
		source.SKU, nullIfEmpty(brandID), brandName, strings.TrimSpace(draft.ModelName), strings.TrimSpace(draft.ReferenceNumber),
		partNameID, partName, detailText, detailType, detailCode, quantity, draft.SalePriceUSDMinor,
		strings.TrimSpace(draft.Notes), comment, adjustmentID, lineID, input.ActorUserID, input.ActorUserID, now, now).Error; err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	if err := tx.Exec(`INSERT INTO cost_adjustment_items(
		id,cost_adjustment_id,position,item_kind,output_part_id,management_code,allocated_cost_jpy_minor,status,created_at
	) VALUES(?,?,?,'part',?,?,?,'cost_adjustment',?)`, itemID, adjustmentID, output.Position, partID, code, output.AllocatedCostJPYMinor, now).Error; err != nil {
		return CostAdjustmentOutputResult{}, err
	}
	return CostAdjustmentOutputResult{Position: output.Position, Kind: "part", ManagementCode: code, RecordID: partID, CostJPYMinor: output.AllocatedCostJPYMinor}, nil
}
