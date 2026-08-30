package persistence

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var platformPermissions = map[string]string{
	"dashboard.read": "ダッシュボード閲覧", "inventory.read": "在庫閲覧", "inventory.write": "在庫更新",
	"purchase.read": "仕入閲覧", "purchase.write": "仕入更新", "purchase.confirm": "仕入確定",
	"sales.read": "売上閲覧", "sales.write": "売上更新", "sales.confirm": "売上確定", "sales.cancel": "売上取消",
	"shipment.read": "出荷閲覧", "shipment.write": "出荷更新", "shipment.confirm": "出荷確定", "shipment.cancel": "出荷取消",
	"market.read": "相場閲覧", "market.write": "相場更新", "market.import": "相場CSV取込",
	"inventory.publish": "商品公開管理", "request.read": "購入依頼閲覧", "request.review": "購入依頼審査",
	"approval.request": "承認申請", "approval.read": "承認閲覧", "approval.approve": "承認実行",
	"audit.read": "監査ログ閲覧", "users.manage": "利用者管理", "settings.manage": "設定管理",
}

var workerDeniedPermissions = map[string]bool{
	"approval.approve": true, "audit.read": true, "users.manage": true, "settings.manage": true,
	"purchase.confirm": true, "sales.confirm": true, "shipment.confirm": true,
	"sales.cancel": true, "shipment.cancel": true, "market.write": true,
}

type previewIdentitySeed struct {
	ID        string
	Username  string
	Display   string
	Email     string
	Role      string
	StaffID   string
	StaffCode string
}

var previewIdentitySeeds = []previewIdentitySeed{
	{ID: "usr_admin", Username: "admin", Display: "管理者", Email: "admin@watch-premium.example", Role: database.RoleAdmin, StaffID: "staff_admin", StaffCode: "BUY-000"},
	{ID: "usr_worker", Username: "worker", Display: "山本 太郎", Email: "worker@watch-premium.example", Role: database.RoleWorker, StaffID: "staff_worker", StaffCode: "BUY-001"},
	{ID: "usr_worker2", Username: "worker2", Display: "佐藤 花子", Email: "worker2@watch-premium.example", Role: database.RoleWorker, StaffID: "staff_worker2", StaffCode: "BUY-002"},
	{ID: "usr_worker3", Username: "worker3", Display: "鈴木 一郎", Email: "worker3@watch-premium.example", Role: database.RoleWorker, StaffID: "staff_worker3", StaffCode: "BUY-003"},
	{ID: "usr_worker4", Username: "worker4", Display: "田中 美香", Email: "worker4@watch-premium.example", Role: database.RoleWorker, StaffID: "staff_worker4", StaffCode: "BUY-004"},
	{ID: "usr_worker5", Username: "worker5", Display: "伊藤 健司", Email: "worker5@watch-premium.example", Role: database.RoleWorker, StaffID: "staff_worker5", StaffCode: "BUY-005"},
}

// SeedPreviewIdentity seeds only development identities. It never overwrites
// existing passwords or organization settings.
func (r *Repository) SeedPreviewIdentity(ctx context.Context, adminPassword, workerPassword string) error {
	if r.driver != "postgres" {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		for _, role := range []struct{ Key, Name, Description string }{
			{database.RoleAdmin, "管理者", "すべての業務機能と設定を管理"},
			{database.RoleWorker, "作業者", "仕入・在庫・売上・出荷を担当"},
			{database.RoleGuest, "ゲスト", "公開された商品を閲覧し購入リクエストを送信"},
		} {
			if err := tx.Exec(`INSERT INTO roles(role_key,name,description) VALUES(?,?,?) ON CONFLICT (role_key) DO NOTHING`,
				role.Key, role.Name, role.Description).Error; err != nil {
				return err
			}
		}

		keys := make([]string, 0, len(platformPermissions))
		for key := range platformPermissions {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := tx.Exec(`INSERT INTO permissions(permission_key,description) VALUES(?,?) ON CONFLICT (permission_key) DO NOTHING`,
				key, platformPermissions[key]).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO role_permissions(role_key,permission_key) VALUES(?,?) ON CONFLICT DO NOTHING`,
				database.RoleAdmin, key).Error; err != nil {
				return err
			}
			if !workerDeniedPermissions[key] {
				if err := tx.Exec(`INSERT INTO role_permissions(role_key,permission_key) VALUES(?,?) ON CONFLICT DO NOTHING`,
					database.RoleWorker, key).Error; err != nil {
					return err
				}
			}
		}
		// Security policy changes must also be applied to organizations that were
		// seeded before the permission was made administrator-only.
		for key := range workerDeniedPermissions {
			if err := tx.Exec(`DELETE FROM role_permissions WHERE role_key=? AND permission_key=?`,
				database.RoleWorker, key).Error; err != nil {
				return err
			}
		}

		if err := tx.Exec(`
			INSERT INTO organizations(id,code,name,is_active,created_at,updated_at)
			VALUES('org_preview','PREVIEW','株式会社ウォッチプレミアム',TRUE,?,?)
			ON CONFLICT (code) DO NOTHING`, now, now).Error; err != nil {
			return err
		}
		var organizationID string
		if err := tx.Raw(`SELECT id FROM organizations WHERE code='PREVIEW'`).Scan(&organizationID).Error; err != nil {
			return err
		}
		if organizationID == "" {
			return fmt.Errorf("preview organization was not created")
		}
		if err := tx.Exec(`INSERT INTO organization_profiles(
			organization_id,postal_code,address,phone,fax,email,invoice_number,representative_name,updated_at
		) VALUES(?,'160-0023','東京都新宿区西新宿2-1-1','03-1234-5678','','info@watch-premium.example',
			'T1234567890123','代表取締役 山田 太郎',?) ON CONFLICT (organization_id) DO NOTHING`, organizationID, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO organization_bank_accounts(
			id,organization_id,bank_name,branch_name,account_type,account_number,account_holder,currency,is_primary,created_at,updated_at
		) VALUES('bank_preview_jpy',?,'サンプル銀行','本店','普通','1234567','カ）ウォッチプレミアム','JPY',TRUE,?,?)
		ON CONFLICT (id) DO NOTHING`, organizationID, now, now).Error; err != nil {
			return err
		}

		for _, seed := range previewIdentitySeeds {
			var count int64
			if err := tx.Raw(`SELECT COUNT(*) FROM users WHERE organization_id=? AND username=?`, organizationID, seed.Username).
				Scan(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				password := workerPassword
				if seed.Role == database.RoleAdmin {
					password = adminPassword
				}
				hash, err := database.HashPassword(password)
				if err != nil {
					return fmt.Errorf("hash preview password: %w", err)
				}
				if err := tx.Exec(`
					INSERT INTO users(id,organization_id,username,password_hash,display_name,email,role_key,is_active,created_at,updated_at)
					VALUES(?,?,?,?,?,?,?,TRUE,?,?)`,
					seed.ID, organizationID, seed.Username, hash, seed.Display, seed.Email, seed.Role, now, now).Error; err != nil {
					return err
				}
			}
			if err := tx.Exec(`UPDATE users SET email=?,updated_at=?
				WHERE organization_id=? AND username=? AND email=''`, seed.Email, now, organizationID, seed.Username).Error; err != nil {
				return err
			}
			var userID string
			if err := tx.Raw(`SELECT id FROM users WHERE organization_id=? AND username=?`, organizationID, seed.Username).
				Scan(&userID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				INSERT INTO staff_profiles(id,organization_id,user_id,staff_code,is_purchase_staff,created_at,updated_at)
				VALUES(?,?,?,?,TRUE,?,?) ON CONFLICT (user_id) DO NOTHING`,
				seed.StaffID, organizationID, userID, seed.StaffCode, now, now).Error; err != nil {
				return err
			}
		}

		for _, setting := range []struct{ Key, Value, Type string }{
			{"approval.purchase_threshold_jpy", "", "integer"},
			{"approval.sales_threshold_jpy", "", "integer"},
			{"approval.admin_high_value_enabled", "false", "boolean"},
			{"approval.admin_high_value_threshold_jpy", "", "integer"},
			{"reservation.duration_hours", "", "integer"},
			{"exchange_rate.provider", "manual", "string"},
			{"csv.encoding", "UTF-8-BOM", "string"},
			{"dashboard.sales_target_jpy", "0", "integer"},
			{"dashboard.purchase_budget_jpy", "0", "integer"},
			{"dashboard.sales_currency", "USD", "string"},
			{"dashboard.purchase_currency", "JPY", "string"},
			{"security.admin_access_code", "", "string"},
		} {
			if err := tx.Exec(`
				INSERT INTO organization_settings(organization_id,setting_key,setting_value,value_type,is_configured,updated_at)
				VALUES(?,?,?,?,FALSE,?) ON CONFLICT (organization_id,setting_key) DO NOTHING`,
				organizationID, setting.Key, setting.Value, setting.Type, now).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
