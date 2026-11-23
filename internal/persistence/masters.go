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
	ErrUnsupportedMaster = errors.New("unsupported master kind")
	ErrMasterNotFound    = errors.New("master item not found")
)

type MasterItem struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"-"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	Meaning         string    `json:"meaning,omitempty"`
	Symbol          string    `json:"symbol,omitempty"`
	Supplement      string    `json:"supplement,omitempty"`
	Reading         string    `json:"reading,omitempty"`
	InventoryDetail string    `json:"inventoryDetail,omitempty"`
	OverseasDetail  string    `json:"overseasDetail,omitempty"`
	TagDetail       string    `json:"tagDetail,omitempty"`
	DomesticDetail  string    `json:"domesticDetail,omitempty"`
	IsLegacy        bool      `json:"isLegacy,omitempty"`
	IsActive        bool      `json:"isActive"`
	SortOrder       int       `json:"sortOrder"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type MasterDisplayDetailsInput struct {
	InventoryDetail *string `json:"inventoryDetail"`
	OverseasDetail  *string `json:"overseasDetail"`
	TagDetail       *string `json:"tagDetail"`
	DomesticDetail  *string `json:"domesticDetail"`
}

func (d MasterDisplayDetailsInput) Values() map[string]*string {
	return map[string]*string{"inventory_detail": d.InventoryDetail, "overseas_detail": d.OverseasDetail, "tag_detail": d.TagDetail, "domestic_detail": d.DomesticDetail}
}
func (d MasterDisplayDetailsInput) Empty() bool {
	return d.InventoryDetail == nil && d.OverseasDetail == nil && d.TagDetail == nil && d.DomesticDetail == nil
}
func (d MasterDisplayDetailsInput) Valid() bool {
	for _, v := range d.Values() {
		if v != nil && len([]rune(strings.TrimSpace(*v))) > 200 {
			return false
		}
	}
	return true
}

func masterTable(kind string) (string, bool) {
	table, ok := map[string]string{
		"brand": "brands", "brands": "brands",
		"material": "materials", "materials": "materials",
		"movement": "movements", "movements": "movements",
		"condition": "product_conditions", "conditions": "product_conditions",
		"accessory": "accessories", "accessories": "accessories",
		"auction": "auction_houses", "auctions": "auction_houses",
		"belt": "belt_materials", "belts": "belt_materials", "belt-material": "belt_materials", "belt-materials": "belt_materials",
		"dial": "dials", "dials": "dials",
		"shape": "product_shapes", "shapes": "product_shapes", "product-shapes": "product_shapes",
		"marking": "markings", "markings": "markings",
		"part-name": "part_names", "part-names": "part_names", "partname": "part_names", "partnames": "part_names",
	}[strings.ToLower(strings.TrimSpace(kind))]
	return table, ok
}

func (r *Repository) MasterItems(ctx context.Context, organizationID, kind string, includeInactive bool) ([]MasterItem, error) {
	table, ok := masterTable(kind)
	if !ok {
		return nil, ErrUnsupportedMaster
	}
	query := r.db.WithContext(ctx).Table(table).Where("organization_id = ?", organizationID)
	if !includeInactive {
		query = query.Where("is_active")
	}
	var items []MasterItem
	if err := query.Order("sort_order, code").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) MasterItem(ctx context.Context, organizationID, kind, id string) (MasterItem, error) {
	table, ok := masterTable(kind)
	if !ok {
		return MasterItem{}, ErrUnsupportedMaster
	}
	var item MasterItem
	result := r.db.WithContext(ctx).Table(table).Where("organization_id = ? AND id = ?", organizationID, id).Take(&item)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return MasterItem{}, ErrMasterNotFound
	}
	return item, result.Error
}

func (r *Repository) CreateMasterItem(ctx context.Context, organizationID, actorID, kind, code, name, meaning, reading string, sortOrder int, symbol, supplement string, displayDetails ...MasterDisplayDetailsInput) (MasterItem, error) {
	table, ok := masterTable(kind)
	if !ok {
		return MasterItem{}, ErrUnsupportedMaster
	}
	id, err := database.NewID("mst")
	if err != nil {
		return MasterItem{}, err
	}
	now := time.Now().UTC()
	if sortOrder <= 0 {
		var maxOrder int
		if err := r.db.WithContext(ctx).Table(table).Select("COALESCE(MAX(sort_order), 0)").
			Where("organization_id = ?", organizationID).Scan(&maxOrder).Error; err != nil {
			return MasterItem{}, err
		}
		sortOrder = maxOrder + 10
	}
	columns, placeholders := "id,organization_id,code,name,is_active,sort_order,created_by,updated_by,created_at,updated_at", "?,?,?,?,TRUE,?,?,?,?,?"
	args := []any{id, organizationID, code, name, sortOrder, actorID, actorID, now, now}
	if table == "markings" || table == "materials" || table == "movements" || table == "belt_materials" || table == "accessories" || table == "product_conditions" {
		columns, placeholders = "id,organization_id,code,name,meaning,is_active,sort_order,created_by,updated_by,created_at,updated_at", "?,?,?,?,?,TRUE,?,?,?,?,?"
		args = []any{id, organizationID, code, name, strings.TrimSpace(meaning), sortOrder, actorID, actorID, now, now}
	}
	if table == "brands" {
		columns, placeholders = "id,organization_id,code,name,reading,is_active,sort_order,created_by,updated_by,created_at,updated_at", "?,?,?,?,?,TRUE,?,?,?,?,?"
		args = []any{id, organizationID, code, name, strings.TrimSpace(reading), sortOrder, actorID, actorID, now, now}
	}
	if table == "accessories" {
		columns += ",symbol,supplement"
		placeholders += ",?,?"
		args = append(args, strings.TrimSpace(symbol), strings.TrimSpace(supplement))
	}
	if (table == "belt_materials" || table == "accessories") && len(displayDetails) > 0 {
		if !displayDetails[0].Valid() {
			return MasterItem{}, ErrUserInvalid
		}
		for _, column := range []string{"inventory_detail", "overseas_detail", "tag_detail", "domestic_detail"} {
			value := ""
			if p := displayDetails[0].Values()[column]; p != nil {
				value = strings.TrimSpace(*p)
			}
			columns += "," + column
			placeholders += ",?"
			args = append(args, value)
		}
	}
	query := fmt.Sprintf("INSERT INTO %s(%s) VALUES(%s)", table, columns, placeholders)
	// A deleted material is inactive, but still owns its code. Re-register it
	// atomically with the newly entered fields while preserving existing references.
	// Active codes remain conflicts and cannot be overwritten by the create form.
	if table == "materials" {
		query += " ON CONFLICT (organization_id,code) DO UPDATE SET name=EXCLUDED.name,meaning=EXCLUDED.meaning,is_active=TRUE,sort_order=EXCLUDED.sort_order,updated_by=EXCLUDED.updated_by,updated_at=EXCLUDED.updated_at WHERE NOT materials.is_active RETURNING id"
		var savedID string
		result := r.db.WithContext(ctx).Raw(query, args...).Scan(&savedID)
		if result.Error != nil {
			return MasterItem{}, result.Error
		}
		if result.RowsAffected == 0 {
			return MasterItem{}, errors.New("active material code already exists")
		}
		return r.MasterItem(ctx, organizationID, kind, savedID)
	}

	if err := r.db.WithContext(ctx).Exec(query, args...).Error; err != nil {
		return MasterItem{}, err
	}
	return r.MasterItem(ctx, organizationID, kind, id)
}

func (r *Repository) UpdateMasterItem(ctx context.Context, organizationID, actorID, kind, id string, name, meaning, reading *string, isActive *bool, sortOrder *int, symbol, supplement *string, displayDetails ...MasterDisplayDetailsInput) (MasterItem, error) {
	table, ok := masterTable(kind)
	if !ok {
		return MasterItem{}, ErrUnsupportedMaster
	}
	updates := map[string]any{"updated_by": actorID, "updated_at": time.Now().UTC()}
	if name != nil {
		updates["name"] = *name
	}
	if meaning != nil && (table == "markings" || table == "materials" || table == "movements" || table == "belt_materials" || table == "accessories" || table == "product_conditions") {
		updates["meaning"] = strings.TrimSpace(*meaning)
	}
	if reading != nil && table == "brands" {
		updates["reading"] = strings.TrimSpace(*reading)
	}
	if symbol != nil && table == "accessories" {
		updates["symbol"] = strings.TrimSpace(*symbol)
	}
	if supplement != nil && table == "accessories" {
		updates["supplement"] = strings.TrimSpace(*supplement)
	}
	if isActive != nil {
		updates["is_active"] = *isActive
	}
	if sortOrder != nil {
		updates["sort_order"] = *sortOrder
	}
	if (table == "belt_materials" || table == "accessories") && len(displayDetails) > 0 {
		if !displayDetails[0].Valid() {
			return MasterItem{}, ErrUserInvalid
		}
		for column, value := range displayDetails[0].Values() {
			if value != nil {
				updates[column] = strings.TrimSpace(*value)
			}
		}
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Table(table).Where("organization_id = ? AND id = ?", organizationID, id).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMasterNotFound
		}
		if table == "accessories" && name != nil {
			return refreshProductAccessoryNames(tx, organizationID, id)
		}
		return nil
	})
	if err != nil {
		return MasterItem{}, err
	}
	return r.MasterItem(ctx, organizationID, kind, id)
}
