package persistence

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var (
	ErrPartnerInvalid  = errors.New("invalid partner")
	ErrPartnerNotFound = errors.New("partner not found")
)

var partnerCodePattern = regexp.MustCompile(`^CLI-[0-9]{3,}$`)
var buyerCodePattern = regexp.MustCompile(`^B[0-9]{3,}$`)
var supplierCodePattern = regexp.MustCompile(`^S[0-9]{3,}$`)

type PartnerRoleRecord struct {
	ID        string `json:"id"`
	RoleType  string `json:"roleType"`
	RoleCode  string `json:"roleCode"`
	IsActive  bool   `json:"isActive"`
	GuestCode string `json:"guestCode,omitempty"`
}

type PartnerRecord struct {
	ID                 string              `json:"id"`
	PartnerCode        string              `json:"partnerCode"`
	LegalName          string              `json:"legalName"`
	RepresentativeName string              `json:"representativeName"`
	Email              string              `json:"email"`
	Phone              string              `json:"phone"`
	PostalCode         string              `json:"postalCode"`
	Address            string              `json:"address"`
	InvoiceNumber      string              `json:"invoiceNumber"`
	Notes              string              `json:"notes"`
	Status             string              `json:"status"`
	Roles              []PartnerRoleRecord `gorm:"-" json:"roles"`
	CreatedAt          time.Time           `json:"createdAt"`
	UpdatedAt          time.Time           `json:"updatedAt"`
}

type PartnerRoleInput struct {
	RoleType string `json:"roleType"`
	RoleCode string `json:"roleCode"`
	IsActive *bool  `json:"isActive"`
}

type PartnerCreateInput struct {
	PartnerCode        string             `json:"partnerCode"`
	LegalName          string             `json:"legalName"`
	RepresentativeName string             `json:"representativeName"`
	Email              string             `json:"email"`
	Phone              string             `json:"phone"`
	PostalCode         string             `json:"postalCode"`
	Address            string             `json:"address"`
	InvoiceNumber      string             `json:"invoiceNumber"`
	Notes              string             `json:"notes"`
	Roles              []PartnerRoleInput `json:"roles"`
}

type PartnerUpdateInput struct {
	LegalName          *string             `json:"legalName"`
	RepresentativeName *string             `json:"representativeName"`
	Email              *string             `json:"email"`
	Phone              *string             `json:"phone"`
	PostalCode         *string             `json:"postalCode"`
	Address            *string             `json:"address"`
	InvoiceNumber      *string             `json:"invoiceNumber"`
	Notes              *string             `json:"notes"`
	Status             *string             `json:"status"`
	Roles              *[]PartnerRoleInput `json:"roles"`
}

func (r *Repository) Partners(ctx context.Context, organizationID string, includeInactive bool) ([]PartnerRecord, error) {
	var records []PartnerRecord
	query := r.db.WithContext(ctx).Table("business_partners").
		Select(`id,partner_code,legal_name,representative_name,email,phone,postal_code,address,
			invoice_number,notes,status,created_at,updated_at`).Where("organization_id=?", organizationID)
	if !includeInactive {
		query = query.Where("status='active'")
	}
	if err := query.Order("partner_code").Scan(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return records, nil
	}
	var roles []struct {
		PartnerID string
		PartnerRoleRecord
	}
	roleQuery := r.db.WithContext(ctx).Table("partner_roles AS pr").
		Select(`pr.partner_id,pr.id,pr.role_type,pr.role_code,pr.is_active,COALESCE(ga.guest_code,'') AS guest_code`).
		Joins("LEFT JOIN guest_accounts ga ON ga.buyer_role_id=pr.id AND ga.organization_id=pr.organization_id").
		Where("pr.organization_id=?", organizationID)
	if !includeInactive {
		roleQuery = roleQuery.Where("pr.is_active")
	}
	if err := roleQuery.Order("pr.role_type,pr.role_code").Scan(&roles).Error; err != nil {
		return nil, err
	}
	byPartner := make(map[string][]PartnerRoleRecord, len(records))
	for _, role := range roles {
		byPartner[role.PartnerID] = append(byPartner[role.PartnerID], role.PartnerRoleRecord)
	}
	for index := range records {
		records[index].Roles = byPartner[records[index].ID]
		if records[index].Roles == nil {
			records[index].Roles = []PartnerRoleRecord{}
		}
	}
	return records, nil
}

func (r *Repository) Partner(ctx context.Context, organizationID, partnerID string) (PartnerRecord, error) {
	records, err := r.Partners(ctx, organizationID, true)
	if err != nil {
		return PartnerRecord{}, err
	}
	for _, record := range records {
		if record.ID == partnerID {
			return record, nil
		}
	}
	return PartnerRecord{}, ErrPartnerNotFound
}

func nextPartnerCode(tx *gorm.DB, organizationID, kind string) (string, error) {
	var next int
	var query, prefix string
	switch kind {
	case "partner":
		query, prefix = `SELECT COALESCE(MAX(NULLIF(regexp_replace(partner_code,'[^0-9]','','g'),'')::INTEGER),0)+1 FROM business_partners WHERE organization_id=?`, "CLI-"
	case "buyer":
		query, prefix = `SELECT COALESCE(MAX(NULLIF(regexp_replace(role_code,'[^0-9]','','g'),'')::INTEGER),0)+1 FROM partner_roles WHERE organization_id=? AND role_type='buyer'`, "B"
	case "supplier":
		query, prefix = `SELECT COALESCE(MAX(NULLIF(regexp_replace(role_code,'[^0-9]','','g'),'')::INTEGER),0)+1 FROM partner_roles WHERE organization_id=? AND role_type='supplier'`, "S"
	default:
		return "", ErrPartnerInvalid
	}
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, organizationID+":"+kind+"_code").Error; err != nil {
		return "", err
	}
	if err := tx.Raw(query, organizationID).Scan(&next).Error; err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%03d", prefix, next), nil
}

func normalizePartnerRole(input PartnerRoleInput) (PartnerRoleInput, error) {
	input.RoleType = strings.ToLower(strings.TrimSpace(input.RoleType))
	input.RoleCode = strings.ToUpper(strings.TrimSpace(input.RoleCode))
	if input.RoleType != "buyer" && input.RoleType != "supplier" {
		return PartnerRoleInput{}, ErrPartnerInvalid
	}
	pattern := buyerCodePattern
	if input.RoleType == "supplier" {
		pattern = supplierCodePattern
	}
	if input.RoleCode != "" && !pattern.MatchString(input.RoleCode) {
		return PartnerRoleInput{}, ErrPartnerInvalid
	}
	return input, nil
}

func validatePartnerFields(name, email, invoice string, roles []PartnerRoleInput) error {
	if strings.TrimSpace(name) == "" || len([]rune(strings.TrimSpace(name))) > 200 || len(roles) == 0 || len(roles) > 2 {
		return ErrPartnerInvalid
	}
	if email != "" && (!strings.Contains(email, "@") || len(email) > 320) {
		return ErrPartnerInvalid
	}
	if len(invoice) > 30 {
		return ErrPartnerInvalid
	}
	seen := map[string]bool{}
	for _, role := range roles {
		if seen[role.RoleType] {
			return ErrPartnerInvalid
		}
		seen[role.RoleType] = true
	}
	return nil
}

func (r *Repository) CreatePartner(ctx context.Context, organizationID, actorUserID string, input PartnerCreateInput) (PartnerRecord, error) {
	input.PartnerCode = strings.ToUpper(strings.TrimSpace(input.PartnerCode))
	input.LegalName = strings.TrimSpace(input.LegalName)
	input.RepresentativeName = strings.TrimSpace(input.RepresentativeName)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Phone = strings.TrimSpace(input.Phone)
	input.PostalCode = strings.TrimSpace(input.PostalCode)
	input.Address = strings.TrimSpace(input.Address)
	input.InvoiceNumber = strings.TrimSpace(input.InvoiceNumber)
	input.Notes = strings.TrimSpace(input.Notes)
	for index := range input.Roles {
		role, err := normalizePartnerRole(input.Roles[index])
		if err != nil {
			return PartnerRecord{}, err
		}
		input.Roles[index] = role
	}
	if err := validatePartnerFields(input.LegalName, input.Email, input.InvoiceNumber, input.Roles); err != nil {
		return PartnerRecord{}, err
	}
	partnerID, err := database.NewID("ptn")
	if err != nil {
		return PartnerRecord{}, err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if input.PartnerCode == "" {
			input.PartnerCode, err = nextPartnerCode(tx, organizationID, "partner")
			if err != nil {
				return err
			}
		}
		if !partnerCodePattern.MatchString(input.PartnerCode) {
			return ErrPartnerInvalid
		}
		if err := tx.Exec(`INSERT INTO business_partners(
			id,organization_id,partner_code,legal_name,representative_name,email,phone,postal_code,address,
			invoice_number,notes,status,created_by,updated_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,'active',?,?,?,?)`, partnerID, organizationID, input.PartnerCode,
			input.LegalName, input.RepresentativeName, input.Email, input.Phone, input.PostalCode, input.Address,
			input.InvoiceNumber, input.Notes, actorUserID, actorUserID, now, now).Error; err != nil {
			return err
		}
		for _, role := range input.Roles {
			if role.RoleCode == "" {
				role.RoleCode, err = nextPartnerCode(tx, organizationID, role.RoleType)
				if err != nil {
					return err
				}
			}
			roleID, idErr := database.NewID("prl")
			if idErr != nil {
				return idErr
			}
			active := true
			if role.IsActive != nil {
				active = *role.IsActive
			}
			if err := tx.Exec(`INSERT INTO partner_roles(
				id,organization_id,partner_id,role_type,role_code,is_active,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?)`, roleID, organizationID, partnerID, role.RoleType, role.RoleCode, active, now, now).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return PartnerRecord{}, err
	}
	return r.Partner(ctx, organizationID, partnerID)
}

func (r *Repository) UpdatePartner(ctx context.Context, organizationID, partnerID, actorUserID string, input PartnerUpdateInput) (PartnerRecord, error) {
	if _, err := r.Partner(ctx, organizationID, partnerID); err != nil {
		return PartnerRecord{}, err
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		updates := map[string]any{"updated_by": actorUserID, "updated_at": now}
		stringFields := []struct {
			Value  *string
			Column string
		}{
			{input.LegalName, "legal_name"}, {input.RepresentativeName, "representative_name"},
			{input.Email, "email"}, {input.Phone, "phone"}, {input.PostalCode, "postal_code"},
			{input.Address, "address"}, {input.InvoiceNumber, "invoice_number"}, {input.Notes, "notes"},
		}
		for _, field := range stringFields {
			if field.Value == nil {
				continue
			}
			value := strings.TrimSpace(*field.Value)
			if field.Column == "legal_name" && value == "" {
				return ErrPartnerInvalid
			}
			if field.Column == "email" {
				value = strings.ToLower(value)
				if value != "" && (!strings.Contains(value, "@") || len(value) > 320) {
					return ErrPartnerInvalid
				}
			}
			updates[field.Column] = value
		}
		if input.Status != nil {
			status := strings.ToLower(strings.TrimSpace(*input.Status))
			if status != "active" && status != "inactive" {
				return ErrPartnerInvalid
			}
			updates["status"] = status
		}
		if len(updates) > 2 {
			if err := tx.Table("business_partners").Where("organization_id=? AND id=?", organizationID, partnerID).Updates(updates).Error; err != nil {
				return err
			}
			if input.Email != nil {
				if err := tx.Exec(`UPDATE users u SET email=?,updated_at=?
					FROM guest_accounts ga,partner_roles pr
					WHERE ga.user_id=u.id AND pr.id=ga.buyer_role_id AND pr.partner_id=?
					AND u.organization_id=?`, strings.ToLower(strings.TrimSpace(*input.Email)), now, partnerID, organizationID).Error; err != nil {
					return err
				}
			}
			if input.LegalName != nil {
				if err := tx.Exec(`UPDATE users u SET display_name=?,updated_at=?
					FROM guest_accounts ga,partner_roles pr
					WHERE ga.user_id=u.id AND pr.id=ga.buyer_role_id AND pr.partner_id=?
					AND u.organization_id=?`, strings.TrimSpace(*input.LegalName), now, partnerID, organizationID).Error; err != nil {
					return err
				}
			}
		}
		if input.Roles != nil {
			seen := map[string]bool{}
			for _, raw := range *input.Roles {
				role, roleErr := normalizePartnerRole(raw)
				if roleErr != nil || seen[role.RoleType] {
					return ErrPartnerInvalid
				}
				seen[role.RoleType] = true
				var existing struct{ ID, RoleCode string }
				result := tx.Table("partner_roles").Select("id,role_code").Where(
					"organization_id=? AND partner_id=? AND role_type=?", organizationID, partnerID, role.RoleType).Take(&existing)
				active := true
				if role.IsActive != nil {
					active = *role.IsActive
				}
				if result.Error == nil {
					if role.RoleCode != "" && role.RoleCode != existing.RoleCode {
						return ErrPartnerInvalid
					}
					if err := tx.Table("partner_roles").Where("id=?", existing.ID).
						Updates(map[string]any{"is_active": active, "updated_at": now}).Error; err != nil {
						return err
					}
					continue
				}
				if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
					return result.Error
				}
				if role.RoleCode == "" {
					role.RoleCode, roleErr = nextPartnerCode(tx, organizationID, role.RoleType)
					if roleErr != nil {
						return roleErr
					}
				}
				roleID, idErr := database.NewID("prl")
				if idErr != nil {
					return idErr
				}
				if err := tx.Exec(`INSERT INTO partner_roles(
					id,organization_id,partner_id,role_type,role_code,is_active,created_at,updated_at
				) VALUES(?,?,?,?,?,?,?,?)`, roleID, organizationID, partnerID, role.RoleType, role.RoleCode, active, now, now).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return PartnerRecord{}, err
	}
	return r.Partner(ctx, organizationID, partnerID)
}
