package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"strings"
	"time"
)

// PurchaseStaffRecord is a buyer master entry, independent of a login account.
type PurchaseStaffRecord struct {
	ID              string `json:"id"`
	StaffProfileID  string `json:"staffProfileId"`
	StaffCode       string `json:"staffCode"`
	DisplayName     string `json:"displayName"`
	InventoryDetail string `json:"inventoryDetail"`
	OverseasDetail  string `json:"overseasDetail"`
	TagDetail       string `json:"tagDetail"`
	DomesticDetail  string `json:"domesticDetail"`
	IsActive        bool   `json:"isActive"`
}

type PurchaseStaffInput struct {
	StaffCode       string  `json:"staffCode"`
	DisplayName     *string `json:"displayName"`
	InventoryDetail *string `json:"inventoryDetail"`
	OverseasDetail  *string `json:"overseasDetail"`
	TagDetail       *string `json:"tagDetail"`
	DomesticDetail  *string `json:"domesticDetail"`
	IsActive        *bool   `json:"isActive"`
}

func (r *Repository) PurchaseStaff(ctx context.Context, org string) ([]PurchaseStaffRecord, error) {
	records := []PurchaseStaffRecord{}
	err := r.db.WithContext(ctx).Table("staff_profiles").
		Select("id,id AS staff_profile_id,staff_code,display_name,inventory_detail,overseas_detail,tag_detail,domestic_detail,is_active").
		Where("organization_id=? AND is_purchase_staff AND is_active", org).Order("staff_code").Scan(&records).Error
	return records, err
}

func (r *Repository) SavePurchaseStaff(ctx context.Context, org, id string, input PurchaseStaffInput) (PurchaseStaffRecord, error) {
	code := normalizeStaffCode(input.StaffCode)
	if input.DisplayName != nil {
		name := strings.TrimSpace(*input.DisplayName)
		if name == "" || len([]rune(name)) > 200 {
			return PurchaseStaffRecord{}, ErrUserInvalid
		}
		input.DisplayName = &name
	}
	details := map[string]*string{"inventory_detail": input.InventoryDetail, "overseas_detail": input.OverseasDetail, "tag_detail": input.TagDetail, "domestic_detail": input.DomesticDetail}
	for _, value := range details {
		if value != nil {
			*value = strings.TrimSpace(*value)
			if len([]rune(*value)) > 200 {
				return PurchaseStaffRecord{}, ErrUserInvalid
			}
		}
	}
	detailValue := func(value *string) string {
		if value == nil {
			return ""
		}
		return *value
	}
	now := time.Now().UTC()
	if id == "" {
		if !staffCodePattern.MatchString(code) || input.DisplayName == nil {
			return PurchaseStaffRecord{}, ErrUserInvalid
		}
		var err error
		id, err = database.NewID("stf")
		if err != nil {
			return PurchaseStaffRecord{}, err
		}
		err = r.db.WithContext(ctx).Exec(`INSERT INTO staff_profiles(id,organization_id,staff_code,display_name,inventory_detail,overseas_detail,tag_detail,domestic_detail,is_purchase_staff,is_active,created_at,updated_at)
   VALUES(?,?,?,?,?,?,?,?,TRUE,TRUE,?,?)`, id, org, code, *input.DisplayName, detailValue(input.InventoryDetail), detailValue(input.OverseasDetail), detailValue(input.TagDetail), detailValue(input.DomesticDetail), now, now).Error
		if err != nil {
			return PurchaseStaffRecord{}, err
		}
	} else {
		var old PurchaseStaffRecord
		if err := r.db.WithContext(ctx).Table("staff_profiles").Where("organization_id=? AND id=?", org, id).Take(&old).Error; err != nil {
			return PurchaseStaffRecord{}, ErrUserNotFound
		}
		// Codes are stable identifiers referenced by stored products and slips.
		if code != "" && code != old.StaffCode {
			return PurchaseStaffRecord{}, ErrUserInvalid
		}
		updates := map[string]any{"updated_at": now}
		if input.DisplayName != nil {
			updates["display_name"] = *input.DisplayName
		}
		if input.IsActive != nil {
			updates["is_active"] = *input.IsActive
		}
		for column, value := range details {
			if value != nil {
				updates[column] = *value
			}
		}
		if len(updates) == 1 {
			return PurchaseStaffRecord{}, ErrUserInvalid
		}
		if err := r.db.WithContext(ctx).Table("staff_profiles").Where("organization_id=? AND id=?", org, id).Updates(updates).Error; err != nil {
			return PurchaseStaffRecord{}, err
		}
	}
	var record PurchaseStaffRecord
	err := r.db.WithContext(ctx).Table("staff_profiles").Select("id,id AS staff_profile_id,staff_code,display_name,inventory_detail,overseas_detail,tag_detail,domestic_detail,is_active").Where("organization_id=? AND id=?", org, id).Take(&record).Error
	return record, err
}
