package persistence

import (
	"fmt"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

func validateRegistrationBox(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimPrefix(code, "BOX-"))
	if err != nil || n < 1 || n > 10 || code != fmt.Sprintf("BOX-%02d", n) {
		return ErrBoxNotFound
	}
	return nil
}

// Called inside the product-creation transaction; it never replaces other members.
func assignCreatedProductBox(tx *gorm.DB, org, productID, code, actor string, now time.Time) error {
	code = strings.TrimSpace(code)
	if err := validateRegistrationBox(code); err != nil {
		return err
	}
	if code == "" {
		return nil
	}
	id, err := database.NewID("box")
	if err != nil {
		return err
	}
	if err := tx.Exec(`INSERT INTO publication_boxes(id,organization_id,box_code,name,is_active,created_by,updated_by,created_at,updated_at)
 VALUES(?,?,?,?,TRUE,?,?,?,?) ON CONFLICT(organization_id,box_code) DO NOTHING`, id, org, code, code, actor, actor, now, now).Error; err != nil {
		return err
	}
	var box struct {
		ID       string
		IsActive bool
	}
	if err := tx.Raw(`SELECT id,is_active FROM publication_boxes WHERE organization_id=? AND box_code=? FOR UPDATE`, org, code).Scan(&box).Error; err != nil {
		return err
	}
	if box.ID == "" {
		return ErrBoxNotFound
	}
	if err := tx.Exec(`INSERT INTO publication_box_products(box_id,product_id,created_at) VALUES(?,?,?)`, box.ID, productID, now).Error; err != nil {
		return err
	}
	visibility := "private"
	if box.IsActive {
		visibility = "public"
	}
	return tx.Table("products").Where("organization_id=? AND id=?", org, productID).Update("publication_status", visibility).Error
}
