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
	ID             string    `json:"id"`
	OrganizationID string    `json:"-"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	IsActive       bool      `json:"isActive"`
	SortOrder      int       `json:"sortOrder"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func masterTable(kind string) (string, bool) {
	table, ok := map[string]string{
		"brand": "brands", "brands": "brands",
		"material": "materials", "materials": "materials",
		"movement": "movements", "movements": "movements",
		"condition": "product_conditions", "conditions": "product_conditions",
		"accessory": "accessories", "accessories": "accessories",
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

func (r *Repository) CreateMasterItem(ctx context.Context, organizationID, actorID, kind, code, name string, sortOrder int) (MasterItem, error) {
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
	query := fmt.Sprintf(`
		INSERT INTO %s(id,organization_id,code,name,is_active,sort_order,created_by,updated_by,created_at,updated_at)
		VALUES(?,?,?,?,TRUE,?,?,?,?,?)`, table)
	if err := r.db.WithContext(ctx).Exec(query, id, organizationID, code, name, sortOrder, actorID, actorID, now, now).Error; err != nil {
		return MasterItem{}, err
	}
	return r.MasterItem(ctx, organizationID, kind, id)
}

func (r *Repository) UpdateMasterItem(ctx context.Context, organizationID, actorID, kind, id string, name *string, isActive *bool, sortOrder *int) (MasterItem, error) {
	table, ok := masterTable(kind)
	if !ok {
		return MasterItem{}, ErrUnsupportedMaster
	}
	updates := map[string]any{"updated_by": actorID, "updated_at": time.Now().UTC()}
	if name != nil {
		updates["name"] = *name
	}
	if isActive != nil {
		updates["is_active"] = *isActive
	}
	if sortOrder != nil {
		updates["sort_order"] = *sortOrder
	}
	result := r.db.WithContext(ctx).Table(table).Where("organization_id = ? AND id = ?", organizationID, id).Updates(updates)
	if result.Error != nil {
		return MasterItem{}, result.Error
	}
	if result.RowsAffected == 0 {
		return MasterItem{}, ErrMasterNotFound
	}
	return r.MasterItem(ctx, organizationID, kind, id)
}
