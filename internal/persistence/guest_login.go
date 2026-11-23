package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"strings"
)

// GuestLoginInput saves the login and its customer details as one operation.
// Omitted customer fields leave existing master data intact.
type GuestLoginInput struct {
	RegionType    *string `json:"regionType"`
	UserID        string  `json:"userId"`
	GuestCode     string  `json:"guestCode"`
	BuyerCode     string  `json:"buyerCode"`
	DisplayName   string  `json:"displayName"`
	Company       string  `json:"company"`
	Email         string  `json:"email"`
	Password      string  `json:"password"`
	IsActive      bool    `json:"isActive"`
	Address       *string `json:"address"`
	Contact       *string `json:"contact"`
	InvoiceNumber *string `json:"invoiceNumber"`
}

// GuestLoginValidationError carries a safe, field-specific message, never input values.
type GuestLoginValidationError string

func (e GuestLoginValidationError) Error() string { return string(e) }

func (r *Repository) SaveGuestLogin(ctx context.Context, org, actor string, input GuestLoginInput) (UserRecord, error) {
	var saved UserRecord
	input.GuestCode = strings.ToUpper(strings.TrimSpace(input.GuestCode))
	input.BuyerCode = strings.ToUpper(strings.TrimSpace(input.BuyerCode))
	input.Company = strings.TrimSpace(input.Company)
	if input.UserID == "" || input.Password != "" {
		if len([]rune(input.Password)) < 8 || len(input.Password) > 72 {
			return saved, database.ErrGuestPasswordInvalid
		}
	}
	if input.Company == "" || len([]rune(input.Company)) > 200 {
		return saved, GuestLoginValidationError("会社名は1〜200文字で入力してください。")
	}
	if strings.TrimSpace(input.DisplayName) == "" || len([]rune(strings.TrimSpace(input.DisplayName))) > 200 {
		return saved, GuestLoginValidationError("氏名は1〜200文字で入力してください。")
	}
	if input.GuestCode != "" && !guestCodePattern.MatchString(input.GuestCode) {
		return saved, GuestLoginValidationError("ゲストIDはG001の形式で入力してください。")
	}
	if input.BuyerCode != "" && !buyerCodePattern.MatchString(input.BuyerCode) {
		return saved, GuestLoginValidationError("顧客コードはB007の形式で入力してください。")
	}
	if input.Address != nil && strings.TrimSpace(*input.Address) == "" {
		return saved, GuestLoginValidationError("住所を入力してください。")
	}
	if input.RegionType != nil {
		region := strings.ToLower(strings.TrimSpace(*input.RegionType))
		if region != "domestic" && region != "overseas" {
			return saved, GuestLoginValidationError("国内・海外を選択してください。")
		}
		input.RegionType = &region
	}
	if input.InvoiceNumber != nil {
		invoice := strings.ToUpper(strings.TrimSpace(*input.InvoiceNumber))
		if invoice != "" && !invoiceNumberPattern.MatchString(invoice) {
			return saved, GuestLoginValidationError("インボイス登録番号はTと13桁の数字で入力してください。未登録の場合は空欄にしてください。")
		}
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, org+":guest_login").Error; err != nil {
			return err
		}
		repo := &Repository{db: tx, driver: r.driver}
		if strings.TrimSpace(input.Email) != "" {
			var duplicates int64
			if err := tx.Table("users").Where("organization_id=? AND lower(email)=? AND id<>? AND deleted_at IS NULL", org, strings.ToLower(strings.TrimSpace(input.Email)), input.UserID).Count(&duplicates).Error; err != nil {
				return err
			}
			if duplicates > 0 {
				return GuestLoginValidationError("このメールアドレスは登録済みです。別のメールアドレスを入力してください。")
			}
		}
		if input.UserID == "" && input.GuestCode != "" {
			var duplicates int64
			if err := tx.Table("users").Where("organization_id=? AND upper(username)=?", org, input.GuestCode).Count(&duplicates).Error; err != nil {
				return err
			}
			if duplicates > 0 {
				return GuestLoginValidationError("このゲストIDは登録済みです。別のゲストIDを入力してください。")
			}
		}
		if input.UserID != "" {
			before, err := repo.User(ctx, org, input.UserID)
			if err != nil {
				return err
			}
			if before.Role != database.RoleGuest || before.GuestCode != input.GuestCode || before.BuyerCode != input.BuyerCode {
				return ErrUserInvalid
			}
		}
		var partnerID string
		if err := tx.Table("partner_roles").Select("partner_id").Where("organization_id=? AND role_type='buyer' AND role_code=? AND is_active", org, input.BuyerCode).Scan(&partnerID).Error; err != nil {
			return err
		}
		if partnerID == "" {
			if input.UserID != "" {
				return ErrUserInvalid
			}
			address := ""
			if input.Address != nil {
				address = *input.Address
			}
			partner, err := repo.CreatePartner(ctx, org, actor, PartnerCreateInput{LegalName: input.Company, Address: address, Email: input.Email, Roles: []PartnerRoleInput{{RoleType: "buyer", RoleCode: input.BuyerCode}}})
			if err != nil {
				return err
			}
			partnerID = partner.ID
			for _, role := range partner.Roles {
				if role.RoleType == "buyer" {
					input.BuyerCode = role.RoleCode
				}
			}
		}
		var count int64
		if err := tx.Table("guest_accounts AS ga").Joins("JOIN partner_roles pr ON pr.id=ga.buyer_role_id AND pr.organization_id=ga.organization_id").Where("ga.organization_id=? AND pr.role_code=? AND ga.user_id<>?", org, input.BuyerCode, input.UserID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return GuestLoginValidationError("この顧客コードにはゲストログインが登録済みです。既存のアカウントを編集してください。")
		}
		if _, err := repo.UpdatePartner(ctx, org, partnerID, actor, PartnerUpdateInput{LegalName: &input.Company, Email: &input.Email, Address: input.Address, Phone: input.Contact, InvoiceNumber: input.InvoiceNumber, RegionType: input.RegionType}); err != nil {
			return err
		}
		if input.UserID == "" {
			if input.GuestCode == "" {
				code, err := nextAccountCode(tx, org, "guest")
				if err != nil {
					return err
				}
				input.GuestCode = code
			}
			user, err := repo.CreateUser(ctx, org, actor, UserCreateInput{Username: input.GuestCode, GuestCode: input.GuestCode, Password: input.Password, DisplayName: input.DisplayName, Email: input.Email, Role: database.RoleGuest, BuyerCode: input.BuyerCode})
			if err != nil {
				return err
			}
			input.UserID = user.ID
		} else if input.Password != "" {
			if err := repo.ChangeUserPassword(ctx, org, input.UserID, input.Password); err != nil {
				return err
			}
		}
		var err error
		saved, err = repo.UpdateUser(ctx, org, input.UserID, actor, UserUpdateInput{DisplayName: &input.DisplayName, Email: &input.Email, IsActive: &input.IsActive})
		return err
	})
	if err != nil {
		return UserRecord{}, err
	}
	return saved, nil
}

// Existing accounts may have an internal username different from the guest ID
// displayed in the directory. Resolve that ID only within the same organization.
func (r *Repository) guestLoginUsername(ctx context.Context, orgCode, code string) (string, error) {
	var username string
	result := r.db.WithContext(ctx).Table("guest_accounts ga").Select("u.username").Joins("JOIN users u ON u.id=ga.user_id AND u.organization_id=ga.organization_id").Joins("JOIN organizations o ON o.id=u.organization_id").Where("o.code=? AND o.is_active AND ga.guest_code=? AND ga.status='active' AND u.role_key='guest' AND u.is_active AND u.deleted_at IS NULL", orgCode, strings.ToUpper(strings.TrimSpace(code))).Scan(&username)
	if result.Error != nil {
		return "", result.Error
	}
	if username == "" {
		return "", errors.New("invalid credentials")
	}
	return username, nil
}
