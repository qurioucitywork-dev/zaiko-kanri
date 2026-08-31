package persistence

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var ErrSettingInvalid = errors.New("invalid setting")

type SettingRecord struct {
	Key          string    `json:"key"`
	Value        string    `json:"value"`
	ValueType    string    `json:"valueType"`
	IsConfigured bool      `json:"isConfigured"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type BankAccountRecord struct {
	ID            string    `json:"id"`
	BankName      string    `json:"bankName"`
	BranchName    string    `json:"branchName"`
	AccountType   string    `json:"accountType"`
	AccountNumber string    `json:"accountNumber"`
	AccountHolder string    `json:"accountHolder"`
	Currency      string    `json:"currency"`
	IsPrimary     bool      `json:"isPrimary"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type CompanyInfoRecord struct {
	OrganizationID     string              `json:"organizationId"`
	OrganizationCode   string              `json:"organizationCode"`
	CompanyName        string              `json:"companyName"`
	PostalCode         string              `json:"postalCode"`
	Address            string              `json:"address"`
	Phone              string              `json:"phone"`
	Fax                string              `json:"fax"`
	Email              string              `json:"email"`
	InvoiceNumber      string              `json:"invoiceNumber"`
	RepresentativeName string              `json:"representativeName"`
	UpdatedAt          time.Time           `json:"updatedAt"`
	BankAccounts       []BankAccountRecord `gorm:"-" json:"bankAccounts"`
}

var allowedSettings = map[string]string{
	"approval.purchase_threshold_jpy":         "integer",
	"approval.sales_threshold_jpy":            "integer",
	"approval.admin_high_value_enabled":       "boolean",
	"approval.admin_high_value_threshold_jpy": "integer",
	"reservation.duration_hours":              "integer",
	"exchange_rate.provider":                  "string",
	"csv.encoding":                            "string",
	"dashboard.sales_target_jpy":              "integer",
	"dashboard.purchase_budget_jpy":           "integer",
	"dashboard.sales_currency":                "string",
	"dashboard.purchase_currency":             "string",
}

func (r *Repository) Settings(ctx context.Context, organizationID string) ([]SettingRecord, error) {
	var records []SettingRecord
	err := r.db.WithContext(ctx).Table("organization_settings").
		Select("setting_key AS key,setting_value AS value,value_type,is_configured,updated_at").
		Where("organization_id=?", organizationID).Order("setting_key").Scan(&records).Error
	return records, err
}

func validateSetting(key, value string) (string, error) {
	valueType, ok := allowedSettings[key]
	if !ok {
		return "", ErrSettingInvalid
	}
	switch valueType {
	case "integer":
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || parsed < 0 {
			return "", ErrSettingInvalid
		}
	case "boolean":
		if value != "true" && value != "false" {
			return "", ErrSettingInvalid
		}
	case "string":
		if len(value) > 200 {
			return "", ErrSettingInvalid
		}
	}
	if strings.HasSuffix(key, "_currency") && value != "JPY" && value != "USD" {
		return "", ErrSettingInvalid
	}
	return valueType, nil
}

func (r *Repository) UpdateSetting(ctx context.Context, organizationID, actorUserID, key, value string) (SettingRecord, error) {
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)
	valueType, err := validateSetting(key, value)
	if err != nil {
		return SettingRecord{}, err
	}
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Exec(`INSERT INTO organization_settings(
		organization_id,setting_key,setting_value,value_type,is_configured,updated_by,updated_at
	) VALUES(?,?,?,?,TRUE,?,?) ON CONFLICT (organization_id,setting_key) DO UPDATE SET
		setting_value=EXCLUDED.setting_value,value_type=EXCLUDED.value_type,is_configured=TRUE,
		updated_by=EXCLUDED.updated_by,updated_at=EXCLUDED.updated_at`, organizationID, key, value, valueType,
		actorUserID, now).Error; err != nil {
		return SettingRecord{}, err
	}
	return SettingRecord{Key: key, Value: value, ValueType: valueType, IsConfigured: true, UpdatedAt: now}, nil
}

func (r *Repository) CompanyInfo(ctx context.Context, organizationID string) (CompanyInfoRecord, error) {
	var record CompanyInfoRecord
	result := r.db.WithContext(ctx).Table("organizations AS o").
		Select(`o.id AS organization_id,o.code AS organization_code,o.name AS company_name,
			COALESCE(p.postal_code,'') AS postal_code,COALESCE(p.address,'') AS address,
			COALESCE(p.phone,'') AS phone,COALESCE(p.fax,'') AS fax,COALESCE(p.email,'') AS email,
			COALESCE(p.invoice_number,'') AS invoice_number,COALESCE(p.representative_name,'') AS representative_name,
			COALESCE(p.updated_at,o.updated_at) AS updated_at`).
		Joins("LEFT JOIN organization_profiles p ON p.organization_id=o.id").Where("o.id=?", organizationID).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return CompanyInfoRecord{}, ErrSettingInvalid
	}
	if result.Error != nil {
		return CompanyInfoRecord{}, result.Error
	}
	if err := r.db.WithContext(ctx).Table("organization_bank_accounts").
		Select("id,bank_name,branch_name,account_type,account_number,account_holder,currency,is_primary,updated_at").
		Where("organization_id=?", organizationID).Order("currency,is_primary DESC,id").Scan(&record.BankAccounts).Error; err != nil {
		return CompanyInfoRecord{}, err
	}
	return record, nil
}

func (r *Repository) UpdateCompanyInfo(ctx context.Context, organizationID, actorUserID string, input CompanyInfoRecord) (CompanyInfoRecord, error) {
	if strings.TrimSpace(input.CompanyName) == "" || len(input.BankAccounts) > 10 {
		return CompanyInfoRecord{}, ErrSettingInvalid
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Exec(`UPDATE organizations SET name=?,updated_at=? WHERE id=?`, strings.TrimSpace(input.CompanyName), now, organizationID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO organization_profiles(
			organization_id,postal_code,address,phone,fax,email,invoice_number,representative_name,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT (organization_id) DO UPDATE SET
			postal_code=EXCLUDED.postal_code,address=EXCLUDED.address,phone=EXCLUDED.phone,fax=EXCLUDED.fax,
			email=EXCLUDED.email,invoice_number=EXCLUDED.invoice_number,
			representative_name=EXCLUDED.representative_name,updated_at=EXCLUDED.updated_at`, organizationID,
			strings.TrimSpace(input.PostalCode), strings.TrimSpace(input.Address), strings.TrimSpace(input.Phone),
			strings.TrimSpace(input.Fax), strings.TrimSpace(input.Email), strings.TrimSpace(input.InvoiceNumber),
			strings.TrimSpace(input.RepresentativeName), now).Error; err != nil {
			return err
		}
		for _, account := range input.BankAccounts {
			currency := strings.ToUpper(strings.TrimSpace(account.Currency))
			if strings.TrimSpace(account.BankName) == "" || strings.TrimSpace(account.AccountNumber) == "" ||
				strings.TrimSpace(account.AccountHolder) == "" || (currency != "JPY" && currency != "USD") {
				return ErrSettingInvalid
			}
			if account.IsPrimary {
				if err := tx.Exec(`UPDATE organization_bank_accounts SET is_primary=FALSE,updated_at=?
					WHERE organization_id=? AND currency=?`, now, organizationID, currency).Error; err != nil {
					return err
				}
			}
			id := strings.TrimSpace(account.ID)
			if id == "" {
				id, _ = database.NewID("bnk")
			} else {
				var owned int64
				if err := tx.Table("organization_bank_accounts").Where("organization_id=? AND id=?", organizationID, id).Count(&owned).Error; err != nil {
					return err
				}
				if owned == 0 {
					return ErrSettingInvalid
				}
			}
			if err := tx.Exec(`INSERT INTO organization_bank_accounts(
				id,organization_id,bank_name,branch_name,account_type,account_number,account_holder,currency,is_primary,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT (id) DO UPDATE SET
				bank_name=EXCLUDED.bank_name,branch_name=EXCLUDED.branch_name,account_type=EXCLUDED.account_type,
				account_number=EXCLUDED.account_number,account_holder=EXCLUDED.account_holder,currency=EXCLUDED.currency,
				is_primary=EXCLUDED.is_primary,updated_at=EXCLUDED.updated_at`, id, organizationID,
				strings.TrimSpace(account.BankName), strings.TrimSpace(account.BranchName), strings.TrimSpace(account.AccountType),
				strings.TrimSpace(account.AccountNumber), strings.TrimSpace(account.AccountHolder), currency, account.IsPrimary, now, now).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return CompanyInfoRecord{}, err
	}
	return r.CompanyInfo(ctx, organizationID)
}
