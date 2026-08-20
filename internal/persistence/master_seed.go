package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

type catalogSeed struct {
	Code string
	Name string
}

// SeedPreviewMasters creates the fixed codes used by the reference UI. Codes
// are immutable identifiers; startup seeding never overwrites edited names.
func (r *Repository) SeedPreviewMasters(ctx context.Context) error {
	if r.driver != "postgres" {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var organizationID, adminID string
		if err := tx.Raw(`SELECT id FROM organizations WHERE code='PREVIEW'`).Scan(&organizationID).Error; err != nil {
			return err
		}
		if err := tx.Raw(`SELECT id FROM users WHERE organization_id=? AND username='admin'`, organizationID).Scan(&adminID).Error; err != nil {
			return err
		}
		if organizationID == "" || adminID == "" {
			return fmt.Errorf("preview identity must be seeded before preview masters")
		}

		catalogs := map[string][]catalogSeed{
			"brands": {
				{"BRD-001", "ロレックス"}, {"BRD-002", "オメガ"}, {"BRD-003", "パテック・フィリップ"},
				{"BRD-004", "カルティエ"}, {"BRD-005", "IWC"}, {"BRD-006", "ブライトリング"},
				{"BRD-007", "タグ・ホイヤー"}, {"BRD-008", "セイコー"}, {"BRD-009", "グランドセイコー"}, {"BRD-010", "その他"},
			},
			"materials": {
				{"MAT-001", "ステンレスSS"}, {"MAT-002", "イエローゴールドYG"}, {"MAT-003", "ホワイトゴールドWG"},
				{"MAT-004", "ピンクゴールドPG"}, {"MAT-005", "プラチナPT"}, {"MAT-006", "チタンTi"},
			},
			"movements": {
				{"MOV-001", "自動巻き"}, {"MOV-002", "手巻き"}, {"MOV-003", "クオーツ"}, {"MOV-004", "電波"}, {"MOV-005", "スマート"},
			},
			"product_conditions": {
				{"CON-001", "未使用品 (N)"}, {"CON-002", "未使用展示品 (N-)"}, {"CON-003", "極美品 (S)"},
				{"CON-004", "美品 (A)"}, {"CON-005", "良品 (AB)"}, {"CON-006", "可品 (B)"}, {"CON-007", "傷あり (BC)"},
			},
			"accessories": {
				{"ACC-001", "BOX"}, {"ACC-002", "CASE"}, {"ACC-003", "GUARANTEE"},
				{"ACC-004", "BRACELET PARTS"}, {"ACC-005", "CERTIFICATE"}, {"ACC-006", "ARCHIVE"},
			},
			"auction_houses": {
				{"AUC-001", "東京オークション"}, {"AUC-002", "ワーカーオークション"},
				{"AUC-003", "APIオークション"}, {"AUC-004", "その他"},
			},
			"belt_materials": {
				{"BLT-001", "ステンレス"}, {"BLT-002", "レザー"}, {"BLT-003", "ラバー"},
				{"BLT-004", "チタン"}, {"BLT-005", "ナイロン"},
			},
			"dials": {
				{"DIA-001", "ブラック"}, {"DIA-002", "ホワイト"}, {"DIA-003", "シルバー"},
				{"DIA-004", "ブルー"}, {"DIA-005", "グリーン"},
			},
			"product_shapes": {{"TYP-001", "腕時計"}, {"TYP-002", "懐中時計"}, {"TYP-003", "置時計"}},
		}
		for table, items := range catalogs {
			for index, item := range items {
				if err := seedCatalogItem(tx, table, organizationID, adminID, item, index+1); err != nil {
					return err
				}
			}
		}
		for index, item := range []struct{ Code, Symbol, Meaning string }{
			{"MRK-001", "♡", "ハート"}, {"MRK-002", "☆", "スター"},
			{"MRK-003", "♧", "クラブ"}, {"MRK-004", "♤", "スペード"},
		} {
			now := time.Now().UTC()
			if err := tx.Exec(`INSERT INTO markings(id,organization_id,code,name,meaning,is_active,sort_order,created_by,updated_by,created_at,updated_at)
				VALUES(?,?,?,?,?,TRUE,?,?,?,?,?) ON CONFLICT (organization_id,code) DO NOTHING`,
				"master_markings_"+item.Code, organizationID, item.Code, item.Symbol, item.Meaning, index+1, adminID, adminID, now, now).Error; err != nil {
				return err
			}
		}

		partners := []struct {
			ID, Code, Name, Representative, Email, Phone, Address, Invoice string
			Roles                                                          []catalogSeed
		}{
			{"partner_cli_001", "CLI-001", "クロノス東京株式会社", "田中 正雄", "info@chronos-tokyo.co.jp", "03-9999-0000", "東京都新宿区西新宿2-1-1", "T0007777888899", []catalogSeed{{"B004", "buyer"}, {"S006", "supplier"}}},
			{"partner_cli_002", "CLI-002", "タイムレス商会有限会社", "中村 健一", "info@timeless.co.jp", "092-444-5555", "福岡県福岡市博多区博多駅前3-2-8", "T0003333444455", []catalogSeed{{"B002", "buyer"}}},
			{"partner_cli_003", "CLI-003", "ウォッチマート", "", "guest.b001@local.invalid", "03-7777-8888", "東京都渋谷区", "T0001111222233", []catalogSeed{{"B001", "buyer"}}},
			{"partner_cli_004", "CLI-004", "ラグジュアリーアイランド", "", "guest.b003@local.invalid", "098-666-7777", "沖縄県那覇市", "T0005555666677", []catalogSeed{{"B003", "buyer"}}},
			{"partner_cli_005", "CLI-005", "田中商事", "", "", "03-1234-5678", "東京都台東区", "T0001234567890", []catalogSeed{{"S001", "supplier"}}},
			{"partner_cli_006", "CLI-006", "山田時計店", "", "", "06-9876-5432", "大阪府大阪市", "T0000987654321", []catalogSeed{{"S002", "supplier"}}},
			{"partner_cli_007", "CLI-007", "ゴールデンウォッチ", "", "", "045-111-2222", "神奈川県横浜市", "T0001122334455", []catalogSeed{{"S003", "supplier"}}},
			{"partner_cli_008", "CLI-008", "プレシャスメタル", "", "", "052-333-4444", "愛知県名古屋市", "T0005566778899", []catalogSeed{{"S004", "supplier"}}},
			{"partner_cli_009", "CLI-009", "レアウォッチジャパン", "", "", "03-5555-6666", "東京都港区", "T0009988776655", []catalogSeed{{"S005", "supplier"}}},
		}
		now := time.Now().UTC()
		for _, partner := range partners {
			if err := tx.Exec(`
				INSERT INTO business_partners(
					id,organization_id,partner_code,legal_name,representative_name,email,phone,address,invoice_number,
					status,created_by,updated_by,created_at,updated_at
				) VALUES(?,?,?,?,?,?,?,?,?,'active',?,?,?,?)
				ON CONFLICT (organization_id,partner_code) DO NOTHING`,
				partner.ID, organizationID, partner.Code, partner.Name, partner.Representative, partner.Email,
				partner.Phone, partner.Address, partner.Invoice, adminID, adminID, now, now).Error; err != nil {
				return err
			}
			var partnerID string
			if err := tx.Raw(`SELECT id FROM business_partners WHERE organization_id=? AND partner_code=?`, organizationID, partner.Code).
				Scan(&partnerID).Error; err != nil {
				return err
			}
			for _, role := range partner.Roles {
				if err := tx.Exec(`
					INSERT INTO partner_roles(id,organization_id,partner_id,role_type,role_code,is_active,created_at,updated_at)
					VALUES(?,?,?,?,?,TRUE,?,?) ON CONFLICT (organization_id,role_type,role_code) DO NOTHING`,
					"partner_role_"+role.Code, organizationID, partnerID, role.Name, role.Code, now, now).Error; err != nil {
					return err
				}
			}
		}

		guestPasswordHash, err := database.HashPassword("preview-guest-2026")
		if err != nil {
			return err
		}
		for _, guest := range []struct {
			GuestCode string
			BuyerCode string
		}{
			{"G001", "B001"}, {"G002", "B002"}, {"G003", "B003"}, {"G004", "B004"},
		} {
			var buyer struct {
				RoleID      string
				CompanyName string
				Email       string
			}
			if err := tx.Table("partner_roles AS pr").
				Select("pr.id AS role_id,bp.legal_name AS company_name,bp.email").
				Joins("JOIN business_partners bp ON bp.id=pr.partner_id").
				Where("pr.organization_id=? AND pr.role_type='buyer' AND pr.role_code=?", organizationID, guest.BuyerCode).
				Take(&buyer).Error; err != nil {
				return err
			}
			userID := "usr_guest_" + guest.GuestCode
			if err := tx.Exec(`INSERT INTO users(
				id,organization_id,username,password_hash,display_name,email,role_key,is_active,created_at,updated_at
			) VALUES(?,?,?,?,?,?,'guest',TRUE,?,?) ON CONFLICT (organization_id,username) DO NOTHING`,
				userID, organizationID, guest.GuestCode, guestPasswordHash, buyer.CompanyName, buyer.Email, now, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE users SET email=?,updated_at=?
				WHERE organization_id=? AND username=? AND email=''`, buyer.Email, now, organizationID, guest.GuestCode).Error; err != nil {
				return err
			}
			if err := tx.Raw(`SELECT id FROM users WHERE organization_id=? AND username=?`, organizationID, guest.GuestCode).
				Scan(&userID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO guest_accounts(
				id,organization_id,guest_code,user_id,buyer_role_id,status,created_at,updated_at
			) VALUES(?,?,?,?,?,'active',?,?) ON CONFLICT (organization_id,guest_code) DO NOTHING`,
				"guest_account_"+guest.GuestCode, organizationID, guest.GuestCode, userID, buyer.RoleID, now, now).Error; err != nil {
				return err
			}
		}

		for index := 1; index <= 10; index++ {
			boxCode := fmt.Sprintf("BOX-%02d", index)
			boxName := fmt.Sprintf("BOX %d", index)
			switch index {
			case 1:
				boxName = "ロレックス特集"
			case 2:
				boxName = "高額品セレクト"
			case 3:
				boxName = "春の新入荷"
			}
			if err := tx.Exec(`INSERT INTO publication_boxes(
				id,organization_id,box_code,name,is_active,created_by,updated_by,created_at,updated_at
			) VALUES(?,?,?,?,TRUE,?,?,?,?) ON CONFLICT (organization_id,box_code) DO NOTHING`,
				"publication_box_"+boxCode, organizationID, boxCode, boxName, adminID, adminID, now, now).Error; err != nil {
				return err
			}
		}
		for _, buyerCode := range []string{"B001", "B002"} {
			if err := tx.Exec(`INSERT INTO publication_box_buyers(box_id,buyer_role_id,created_at)
				SELECT 'publication_box_BOX-01',id,? FROM partner_roles
				WHERE organization_id=? AND role_type='buyer' AND role_code=? ON CONFLICT DO NOTHING`,
				now, organizationID, buyerCode).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func seedCatalogItem(tx *gorm.DB, table, organizationID, actorID string, item catalogSeed, sortOrder int) error {
	allowed := map[string]bool{
		"brands": true, "materials": true, "movements": true, "product_conditions": true, "accessories": true,
		"auction_houses": true, "belt_materials": true, "dials": true, "product_shapes": true,
	}
	if !allowed[table] {
		return fmt.Errorf("unsupported catalog table %q", table)
	}
	now := time.Now().UTC()
	id := "master_" + table + "_" + item.Code
	query := fmt.Sprintf(`
		INSERT INTO %s(id,organization_id,code,name,is_active,sort_order,created_by,updated_by,created_at,updated_at)
		VALUES(?,?,?,?,TRUE,?,?,?,?,?) ON CONFLICT (organization_id,code) DO NOTHING`, table)
	return tx.Exec(query, id, organizationID, item.Code, item.Name, sortOrder, actorID, actorID, now, now).Error
}
