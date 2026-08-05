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
	ErrUserInvalid       = errors.New("invalid user")
	ErrUserNotFound      = errors.New("user not found")
	ErrLastAdministrator = errors.New("last active administrator cannot be disabled")
)

var accountCodePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+-]{2,319}$`)
var staffCodePattern = regexp.MustCompile(`^STF-[0-9]{3,}$`)
var guestCodePattern = regexp.MustCompile(`^G[0-9]{3,}$`)

type UserRecord struct {
	ID              string     `json:"id"`
	Username        string     `json:"username"`
	DisplayName     string     `json:"displayName"`
	Email           string     `json:"email"`
	Role            string     `json:"role"`
	IsActive        bool       `json:"isActive"`
	StaffCode       string     `json:"staffCode,omitempty"`
	IsPurchaseStaff bool       `json:"isPurchaseStaff"`
	GuestCode       string     `json:"guestCode,omitempty"`
	BuyerCode       string     `json:"buyerCode,omitempty"`
	PartnerCode     string     `json:"partnerCode,omitempty"`
	CompanyName     string     `json:"companyName,omitempty"`
	LastLoginAt     *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type UserCreateInput struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	DisplayName     string `json:"displayName"`
	Email           string `json:"email"`
	Role            string `json:"role"`
	StaffCode       string `json:"staffCode"`
	IsPurchaseStaff *bool  `json:"isPurchaseStaff"`
	GuestCode       string `json:"guestCode"`
	BuyerCode       string `json:"buyerCode"`
}

type UserUpdateInput struct {
	DisplayName     *string `json:"displayName"`
	Email           *string `json:"email"`
	IsActive        *bool   `json:"isActive"`
	IsPurchaseStaff *bool   `json:"isPurchaseStaff"`
}

func (r *Repository) Users(ctx context.Context, organizationID string, includeInactive bool) ([]UserRecord, error) {
	var records []UserRecord
	query := r.db.WithContext(ctx).Table("users AS u").
		Select(`u.id,u.username,u.display_name,u.email,u.role_key AS role,u.is_active,
			COALESCE(sp.staff_code,'') AS staff_code,COALESCE(sp.is_purchase_staff,FALSE) AS is_purchase_staff,
			COALESCE(ga.guest_code,'') AS guest_code,COALESCE(br.role_code,'') AS buyer_code,
			COALESCE(bp.partner_code,'') AS partner_code,COALESCE(bp.legal_name,'') AS company_name,
			u.last_login_at,u.created_at,u.updated_at`).
		Joins("LEFT JOIN staff_profiles sp ON sp.user_id=u.id AND sp.organization_id=u.organization_id").
		Joins("LEFT JOIN guest_accounts ga ON ga.user_id=u.id AND ga.organization_id=u.organization_id").
		Joins("LEFT JOIN partner_roles br ON br.id=ga.buyer_role_id AND br.organization_id=u.organization_id").
		Joins("LEFT JOIN business_partners bp ON bp.id=br.partner_id AND bp.organization_id=u.organization_id").
		Where("u.organization_id=? AND u.deleted_at IS NULL", organizationID)
	if !includeInactive {
		query = query.Where("u.is_active")
	}
	err := query.Order("CASE u.role_key WHEN 'admin' THEN 1 WHEN 'worker' THEN 2 ELSE 3 END,u.username").Scan(&records).Error
	return records, err
}

func (r *Repository) User(ctx context.Context, organizationID, userID string) (UserRecord, error) {
	records, err := r.Users(ctx, organizationID, true)
	if err != nil {
		return UserRecord{}, err
	}
	for _, record := range records {
		if record.ID == userID {
			return record, nil
		}
	}
	return UserRecord{}, ErrUserNotFound
}

func validateUserCreate(input UserCreateInput) error {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.TrimSpace(input.Email)
	if !accountCodePattern.MatchString(input.Username) || len(input.Password) < 8 || input.DisplayName == "" || len([]rune(input.DisplayName)) > 200 {
		return ErrUserInvalid
	}
	if input.Role != database.RoleAdmin && input.Role != database.RoleWorker && input.Role != database.RoleGuest {
		return ErrUserInvalid
	}
	if input.Email != "" && (!strings.Contains(input.Email, "@") || len(input.Email) > 320) {
		return ErrUserInvalid
	}
	if input.Role == database.RoleGuest && strings.TrimSpace(input.BuyerCode) == "" {
		return ErrUserInvalid
	}
	return nil
}

func nextAccountCode(tx *gorm.DB, organizationID, kind string) (string, error) {
	var next int
	switch kind {
	case "staff":
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, organizationID+":staff_code").Error; err != nil {
			return "", err
		}
		if err := tx.Raw(`SELECT COALESCE(MAX(NULLIF(regexp_replace(staff_code,'[^0-9]','','g'),'')::INTEGER),0)+1
			FROM staff_profiles WHERE organization_id=?`, organizationID).Scan(&next).Error; err != nil {
			return "", err
		}
		return fmt.Sprintf("STF-%03d", next), nil
	case "guest":
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, organizationID+":guest_code").Error; err != nil {
			return "", err
		}
		if err := tx.Raw(`SELECT COALESCE(MAX(NULLIF(regexp_replace(guest_code,'[^0-9]','','g'),'')::INTEGER),0)+1
			FROM guest_accounts WHERE organization_id=?`, organizationID).Scan(&next).Error; err != nil {
			return "", err
		}
		return fmt.Sprintf("G%03d", next), nil
	default:
		return "", ErrUserInvalid
	}
}

func (r *Repository) CreateUser(ctx context.Context, organizationID, actorUserID string, input UserCreateInput) (UserRecord, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	input.StaffCode = strings.ToUpper(strings.TrimSpace(input.StaffCode))
	input.GuestCode = strings.ToUpper(strings.TrimSpace(input.GuestCode))
	input.BuyerCode = strings.ToUpper(strings.TrimSpace(input.BuyerCode))
	if err := validateUserCreate(input); err != nil {
		return UserRecord{}, err
	}
	hash, err := database.HashPassword(input.Password)
	if err != nil {
		return UserRecord{}, err
	}
	userID, err := database.NewID("usr")
	if err != nil {
		return UserRecord{}, err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if input.Role == database.RoleGuest {
			var buyer struct {
				RoleID, PartnerID, PartnerCode, CompanyName, Email string
			}
			result := tx.Table("partner_roles AS pr").
				Select("pr.id AS role_id,bp.id AS partner_id,bp.partner_code,bp.legal_name AS company_name,bp.email").
				Joins("JOIN business_partners bp ON bp.id=pr.partner_id AND bp.organization_id=pr.organization_id").
				Where("pr.organization_id=? AND pr.role_type='buyer' AND pr.role_code=? AND pr.is_active AND bp.status='active'", organizationID, input.BuyerCode).
				Take(&buyer)
			if result.Error != nil {
				return ErrUserInvalid
			}
			if input.GuestCode == "" {
				input.GuestCode, err = nextAccountCode(tx, organizationID, "guest")
				if err != nil {
					return err
				}
			}
			if !guestCodePattern.MatchString(input.GuestCode) {
				return ErrUserInvalid
			}
			if input.Email == "" {
				input.Email = strings.ToLower(buyer.Email)
			}
			if err := tx.Exec(`INSERT INTO users(
				id,organization_id,username,password_hash,display_name,email,role_key,is_active,created_at,updated_at
			) VALUES(?,?,?,?,?,?,'guest',TRUE,?,?)`, userID, organizationID, input.Username, hash,
				input.DisplayName, input.Email, now, now).Error; err != nil {
				return err
			}
			guestID, idErr := database.NewID("gst")
			if idErr != nil {
				return idErr
			}
			return tx.Exec(`INSERT INTO guest_accounts(
				id,organization_id,guest_code,user_id,buyer_role_id,status,created_at,updated_at
			) VALUES(?,?,?,?,?,'active',?,?)`, guestID, organizationID, input.GuestCode, userID, buyer.RoleID, now, now).Error
		}

		if input.StaffCode == "" {
			input.StaffCode, err = nextAccountCode(tx, organizationID, "staff")
			if err != nil {
				return err
			}
		}
		if !staffCodePattern.MatchString(input.StaffCode) {
			return ErrUserInvalid
		}
		if err := tx.Exec(`INSERT INTO users(
			id,organization_id,username,password_hash,display_name,email,role_key,is_active,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,TRUE,?,?)`, userID, organizationID, input.Username, hash, input.DisplayName,
			input.Email, input.Role, now, now).Error; err != nil {
			return err
		}
		staffID, idErr := database.NewID("stf")
		if idErr != nil {
			return idErr
		}
		purchaseStaff := true
		if input.IsPurchaseStaff != nil {
			purchaseStaff = *input.IsPurchaseStaff
		}
		return tx.Exec(`INSERT INTO staff_profiles(
			id,organization_id,user_id,staff_code,is_purchase_staff,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?)`, staffID, organizationID, userID, input.StaffCode, purchaseStaff, now, now).Error
	})
	if err != nil {
		return UserRecord{}, err
	}
	return r.User(ctx, organizationID, userID)
}

func (r *Repository) UpdateUser(ctx context.Context, organizationID, userID, actorUserID string, input UserUpdateInput) (UserRecord, error) {
	before, err := r.User(ctx, organizationID, userID)
	if err != nil {
		return UserRecord{}, err
	}
	if input.DisplayName == nil && input.Email == nil && input.IsActive == nil && input.IsPurchaseStaff == nil {
		return UserRecord{}, ErrUserInvalid
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		updates := map[string]any{"updated_at": now}
		if input.DisplayName != nil {
			value := strings.TrimSpace(*input.DisplayName)
			if value == "" || len([]rune(value)) > 200 {
				return ErrUserInvalid
			}
			updates["display_name"] = value
		}
		if input.Email != nil {
			value := strings.ToLower(strings.TrimSpace(*input.Email))
			if value != "" && (!strings.Contains(value, "@") || len(value) > 320) {
				return ErrUserInvalid
			}
			updates["email"] = value
		}
		if input.IsActive != nil {
			if before.Role == database.RoleAdmin && !*input.IsActive {
				var count int64
				if err := tx.Table("users").Where("organization_id=? AND role_key='admin' AND is_active AND deleted_at IS NULL AND id<>?", organizationID, userID).Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					return ErrLastAdministrator
				}
			}
			updates["is_active"] = *input.IsActive
			if before.Role == database.RoleGuest {
				status := "suspended"
				if *input.IsActive {
					status = "active"
				}
				if err := tx.Table("guest_accounts").Where("organization_id=? AND user_id=?", organizationID, userID).
					Updates(map[string]any{"status": status, "updated_at": now}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Table("users").Where("organization_id=? AND id=? AND deleted_at IS NULL", organizationID, userID).Updates(updates).Error; err != nil {
			return err
		}
		if input.IsPurchaseStaff != nil {
			if before.Role == database.RoleGuest {
				return ErrUserInvalid
			}
			if err := tx.Table("staff_profiles").Where("organization_id=? AND user_id=?", organizationID, userID).
				Updates(map[string]any{"is_purchase_staff": *input.IsPurchaseStaff, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		if before.Role == database.RoleGuest && input.Email != nil {
			return tx.Exec(`UPDATE business_partners bp SET email=?,updated_by=?,updated_at=?
				FROM partner_roles pr,guest_accounts ga
				WHERE ga.user_id=? AND ga.organization_id=? AND pr.id=ga.buyer_role_id AND bp.id=pr.partner_id`,
				strings.ToLower(strings.TrimSpace(*input.Email)), actorUserID, now, userID, organizationID).Error
		}
		return nil
	})
	if err != nil {
		return UserRecord{}, err
	}
	return r.User(ctx, organizationID, userID)
}

func (r *Repository) ChangeUserPassword(ctx context.Context, organizationID, userID, password string) error {
	if len(password) < 8 || len(password) > 256 {
		return ErrUserInvalid
	}
	hash, err := database.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Table("users").Where("organization_id=? AND id=? AND deleted_at IS NULL", organizationID, userID).
			Updates(map[string]any{"password_hash": hash, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrUserNotFound
		}
		return tx.Exec(`DELETE FROM sessions WHERE user_id=?`, userID).Error
	})
}
