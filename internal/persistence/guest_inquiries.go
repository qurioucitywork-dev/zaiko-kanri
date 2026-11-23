package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"regexp"
	"strings"
	"time"
)

var ErrGuestInquiryInvalid = errors.New("inquiry input is invalid")
var inquiryRequestPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,80}$`)

type GuestInquiryInput struct {
	ClientRequestID string `json:"clientRequestId"`
	Subject         string `json:"subject"`
	ProductCode     string `json:"productCode"`
	Message         string `json:"message"`
}
type GuestInquiryRecord struct {
	ID          string    `json:"id"`
	GuestCode   string    `json:"guestCode"`
	GuestName   string    `json:"guestName"`
	CompanyName string    `json:"companyName"`
	Email       string    `json:"email"`
	Subject     string    `json:"subject"`
	ProductCode string    `json:"productCode"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (r *Repository) GuestInquiry(ctx context.Context, org, id string) (GuestInquiryRecord, error) {
	var record GuestInquiryRecord
	err := r.db.WithContext(ctx).Table("guest_inquiries").Select("id,guest_code,guest_name,company_name,email,subject,product_code,message,created_at").Where("organization_id=? AND id=?", org, id).Take(&record).Error
	return record, err
}
func (r *Repository) CreateGuestInquiry(ctx context.Context, org, user string, input GuestInquiryInput) (GuestInquiryRecord, error) {
	input.Subject = strings.TrimSpace(input.Subject)
	input.ProductCode = strings.TrimSpace(input.ProductCode)
	input.Message = strings.TrimSpace(input.Message)
	if !inquiryRequestPattern.MatchString(input.ClientRequestID) || input.Subject == "" || len([]rune(input.Subject)) > 200 || len([]rune(input.ProductCode)) > 100 || input.Message == "" || len([]rune(input.Message)) > 5000 {
		return GuestInquiryRecord{}, ErrGuestInquiryInvalid
	}
	var saved GuestInquiryRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		guest, err := lookupGuestIdentity(tx, org, user)
		if err != nil {
			return err
		}
		var account struct{ DisplayName, Email string }
		if err := tx.Table("users").Select("display_name,COALESCE(email,'') AS email").Where("organization_id=? AND id=? AND is_active AND deleted_at IS NULL", org, user).Take(&account).Error; err != nil {
			return err
		}
		id, err := database.NewID("inq")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		result := tx.Exec(`INSERT INTO guest_inquiries(id,organization_id,guest_user_id,client_request_id,guest_code,guest_name,company_name,email,subject,product_code,message,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(organization_id,guest_user_id,client_request_id) DO NOTHING`, id, org, user, input.ClientRequestID, guest.GuestCode, account.DisplayName, guest.BuyerName, account.Email, input.Subject, input.ProductCode, input.Message, now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var existing string
			if err := tx.Table("guest_inquiries").Select("id").Where("organization_id=? AND guest_user_id=? AND client_request_id=?", org, user, input.ClientRequestID).Scan(&existing).Error; err != nil {
				return err
			}
			id = existing
		}
		repo := &Repository{db: tx, driver: r.driver}
		saved, err = repo.GuestInquiry(ctx, org, id)
		if err != nil {
			return err
		}
		if saved.Subject != input.Subject || saved.ProductCode != input.ProductCode || saved.Message != input.Message {
			return ErrGuestInquiryInvalid
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return insertNotificationTx(tx, org, "", database.RoleAdmin, "guest_inquiry.created", "ゲストからお問い合わせが届きました", account.DisplayName+" / "+input.Subject, "guest_inquiry", id, now)
	})
	return saved, err
}
