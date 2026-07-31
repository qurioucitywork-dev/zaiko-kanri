package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidMasterCategory = errors.New("未対応のマスタ分類です")
	ErrMasterCodeExists      = errors.New("同じコードが既に登録されています")
	ErrMasterRecordNotFound  = errors.New("マスタデータが見つかりません")
)

type MasterRecord struct {
	ID                        string
	Category                  string
	Code                      string
	Name                      string
	Address                   string
	Contact                   string
	InvoiceRegistrationNumber string
	Details                   map[string]string
}

type SaveMasterInput struct {
	OrganizationID            string
	Category                  string
	Code                      string
	Name                      string
	Address                   string
	Contact                   string
	InvoiceRegistrationNumber string
	Details                   map[string]string
	ActorUserID               string
}

var supportedMasterCategories = map[string]bool{
	"brands": true, "suppliers": true, "buyers": true, "materials": true,
	"movements": true, "sales-destinations": true, "accessories": true,
	"conditions": true, "passwords": true, "partners": true, "company": true,
	"dashboard": true,
}

var masterCodePrefixes = map[string]string{
	"brands": "BRD", "suppliers": "SUP", "buyers": "BUY", "materials": "MAT",
	"movements": "MOV", "sales-destinations": "SAL", "accessories": "ACC",
	"conditions": "CON", "passwords": "PWD", "partners": "PTR",
	"company": "COM", "dashboard": "DSH",
}

func (s *Store) MasterRecords(ctx context.Context, organizationID, category string) ([]MasterRecord, error) {
	if !supportedMasterCategories[category] {
		return nil, ErrInvalidMasterCategory
	}
	query := `
		SELECT id,? AS category,supplier_code,name,address,contact,invoice_registration_number,'{}'
		FROM suppliers WHERE organization_id=? AND is_active=1 ORDER BY supplier_code`
	args := []any{category, organizationID}
	if category != "suppliers" {
		query = `
			SELECT id,category,record_code,name,address,contact,invoice_registration_number,details_json
			FROM master_records
			WHERE organization_id=? AND category=? AND is_active=1
			ORDER BY record_code`
		args = []any{organizationID, category}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]MasterRecord, 0)
	for rows.Next() {
		var record MasterRecord
		var details string
		if err := rows.Scan(&record.ID, &record.Category, &record.Code, &record.Name,
			&record.Address, &record.Contact, &record.InvoiceRegistrationNumber, &details); err != nil {
			return nil, err
		}
		record.Details = map[string]string{}
		_ = json.Unmarshal([]byte(details), &record.Details)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) CreateMasterRecord(ctx context.Context, input SaveMasterInput) (MasterRecord, error) {
	input = normalizeMasterInput(input)
	if input.Code == "" {
		var err error
		input.Code, err = s.nextMasterCode(ctx, input.OrganizationID, input.Category)
		if err != nil {
			return MasterRecord{}, err
		}
	}
	if err := validateMasterInput(input); err != nil {
		return MasterRecord{}, err
	}
	id, _ := NewID("mst")
	now := s.now().Format(time.RFC3339Nano)
	var err error
	if input.Category == "suppliers" {
		id, _ = NewID("sup")
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO suppliers(
				id,organization_id,supplier_code,name,address,contact,invoice_registration_number,
				is_active,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,1,?,?)`,
			id, input.OrganizationID, input.Code, input.Name, input.Address, input.Contact,
			input.InvoiceRegistrationNumber, now, now)
	} else {
		details, _ := json.Marshal(input.Details)
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO master_records(
				id,organization_id,category,record_code,name,address,contact,invoice_registration_number,
				details_json,is_active,created_by,created_at,updated_by,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,1,?,?,?,?)`,
			id, input.OrganizationID, input.Category, input.Code, input.Name, input.Address, input.Contact,
			input.InvoiceRegistrationNumber, string(details), input.ActorUserID, now, input.ActorUserID, now)
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return MasterRecord{}, ErrMasterCodeExists
		}
		return MasterRecord{}, err
	}
	return MasterRecord{
		ID: id, Category: input.Category, Code: input.Code, Name: input.Name,
		Address: input.Address, Contact: input.Contact,
		InvoiceRegistrationNumber: input.InvoiceRegistrationNumber, Details: input.Details,
	}, nil
}

func (s *Store) UpdateMasterRecord(ctx context.Context, id string, input SaveMasterInput) error {
	input = normalizeMasterInput(input)
	if err := validateMasterInput(input); err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	var result sql.Result
	var err error
	if input.Category == "suppliers" {
		result, err = s.db.ExecContext(ctx, `
			UPDATE suppliers
			SET supplier_code=?,name=?,address=?,contact=?,invoice_registration_number=?,updated_at=?
			WHERE id=? AND organization_id=? AND is_active=1`,
			input.Code, input.Name, input.Address, input.Contact, input.InvoiceRegistrationNumber,
			now, id, input.OrganizationID)
	} else {
		details, _ := json.Marshal(input.Details)
		result, err = s.db.ExecContext(ctx, `
			UPDATE master_records SET
				record_code=?,name=?,address=?,contact=?,invoice_registration_number=?,details_json=?,
				updated_by=?,updated_at=?
			WHERE id=? AND organization_id=? AND category=? AND is_active=1`,
			input.Code, input.Name, input.Address, input.Contact, input.InvoiceRegistrationNumber,
			string(details), input.ActorUserID, now, id, input.OrganizationID, input.Category)
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrMasterCodeExists
		}
		return err
	}
	return ensureMasterRowChanged(result)
}

func (s *Store) DeleteMasterRecord(ctx context.Context, organizationID, category, id, actorUserID string) error {
	if !supportedMasterCategories[category] {
		return ErrInvalidMasterCategory
	}
	now := s.now().Format(time.RFC3339Nano)
	var result sql.Result
	var err error
	if category == "suppliers" {
		result, err = s.db.ExecContext(ctx, `
			UPDATE suppliers SET is_active=0,updated_at=?
			WHERE id=? AND organization_id=? AND is_active=1`, now, id, organizationID)
	} else {
		result, err = s.db.ExecContext(ctx, `
			UPDATE master_records SET is_active=0,updated_by=?,updated_at=?
			WHERE id=? AND organization_id=? AND category=? AND is_active=1`,
			actorUserID, now, id, organizationID, category)
	}
	if err != nil {
		return err
	}
	return ensureMasterRowChanged(result)
}

func ensureMasterRowChanged(result sql.Result) error {
	if result == nil {
		return ErrMasterRecordNotFound
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrMasterRecordNotFound
	}
	return nil
}

func normalizeMasterInput(input SaveMasterInput) SaveMasterInput {
	input.Category = strings.TrimSpace(input.Category)
	input.Code = strings.ToUpper(strings.TrimSpace(input.Code))
	input.Name = strings.TrimSpace(input.Name)
	if input.Category == "accessories" {
		input.Name = strings.ToUpper(input.Name)
	}
	input.Address = strings.TrimSpace(input.Address)
	input.Contact = strings.TrimSpace(input.Contact)
	input.InvoiceRegistrationNumber = strings.ToUpper(strings.TrimSpace(input.InvoiceRegistrationNumber))
	if input.Details == nil {
		input.Details = map[string]string{}
	}
	for key, value := range input.Details {
		input.Details[key] = strings.TrimSpace(value)
	}
	return input
}

func validateMasterInput(input SaveMasterInput) error {
	if input.OrganizationID == "" || input.ActorUserID == "" {
		return errors.New("組織と操作者は必須です")
	}
	if !supportedMasterCategories[input.Category] {
		return ErrInvalidMasterCategory
	}
	if input.Code == "" || input.Name == "" {
		return errors.New("コードと名称は必須です")
	}
	if len([]rune(input.Code)) > 30 || len([]rune(input.Name)) > 100 ||
		len([]rune(input.Address)) > 300 || len([]rune(input.Contact)) > 200 ||
		len([]rune(input.InvoiceRegistrationNumber)) > 30 {
		return errors.New("入力文字数を確認してください")
	}
	return nil
}

func (s *Store) nextMasterCode(ctx context.Context, organizationID, category string) (string, error) {
	if !supportedMasterCategories[category] {
		return "", ErrInvalidMasterCategory
	}
	prefix := masterCodePrefixes[category]
	var codes []string
	if category == "suppliers" {
		rows, err := s.db.QueryContext(ctx, `SELECT supplier_code FROM suppliers WHERE organization_id=?`, organizationID)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		for rows.Next() {
			var code string
			if err := rows.Scan(&code); err != nil {
				return "", err
			}
			codes = append(codes, code)
		}
	} else {
		rows, err := s.db.QueryContext(ctx, `
			SELECT record_code FROM master_records WHERE organization_id=? AND category=?`, organizationID, category)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		for rows.Next() {
			var code string
			if err := rows.Scan(&code); err != nil {
				return "", err
			}
			codes = append(codes, code)
		}
	}
	maximum := 0
	for _, code := range codes {
		part := strings.TrimPrefix(strings.ToUpper(code), prefix+"-")
		if value, err := strconv.Atoi(part); err == nil && value > maximum {
			maximum = value
		}
	}
	return fmt.Sprintf("%s-%03d", prefix, maximum+1), nil
}

func (s *Store) SeedMasterPreview(ctx context.Context) error {
	now := s.now().Format(time.RFC3339Nano)
	for _, supplier := range []Supplier{
		{ID: "sup_004", Code: "S004", Name: "プレシャスメタル"},
		{ID: "sup_005", Code: "S005", Name: "レアウォッチジャパン"},
	} {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO suppliers(id,organization_id,supplier_code,name,created_at,updated_at)
			VALUES(?,'org_preview',?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET supplier_code=excluded.supplier_code,name=excluded.name,
				is_active=1,updated_at=excluded.updated_at`,
			supplier.ID, supplier.Code, supplier.Name, now, now); err != nil {
			return err
		}
	}
	type previewMaster struct{ code, name string }
	seed := map[string][]previewMaster{
		"brands": {
			{"BRD-001", "ロレックス"}, {"BRD-002", "オメガ"}, {"BRD-003", "パテック・フィリップ"},
			{"BRD-004", "カルティエ"}, {"BRD-005", "IWC"}, {"BRD-006", "ブライトリング"},
			{"BRD-007", "タグ・ホイヤー"}, {"BRD-008", "セイコー"}, {"BRD-009", "グランドセイコー"}, {"BRD-010", "その他"},
		},
		"materials": {
			{"M01", "ステンレスSS"}, {"M02", "イエローゴールドYG"}, {"M03", "ホワイトゴールドWG"},
			{"M04", "ピンクゴールドPG"}, {"M05", "プラチナPT"}, {"M06", "チタンTi"},
		},
		"movements": {
			{"D01", "自動巻き"}, {"D02", "手巻き"}, {"D03", "クオーツ"}, {"D04", "電波"}, {"D05", "スマート"},
		},
		"conditions": {
			{"C01", "未使用品 (N)"}, {"C02", "未使用展示品 (N-)"}, {"C03", "極美品 (S)"},
			{"C04", "美品 (A)"}, {"C05", "良品 (AB)"}, {"C06", "可品 (B)"}, {"C07", "傷あり (BC)"},
		},
		"buyers":             {{"BUY-001", "田中 一郎"}, {"BUY-002", "佐藤 美香"}, {"BUY-003", "鈴木 健太"}, {"BUY-004", "山本 綾"}, {"BUY-005", "高橋 翼"}},
		"sales-destinations": {{"B001", "ウォッチマート"}, {"B002", "タイムレス商会"}, {"B003", "ラグジュアリーアイランド"}, {"B004", "クロノス東京"}},
		"accessories":        {{"ACC-001", "BOX"}, {"ACC-002", "CASE"}, {"ACC-003", "GUARANTEE"}, {"ACC-004", "BRACELET PARTS"}, {"ACC-005", "CERTIFICATE"}, {"ACC-006", "ARCHIVE"}},
		"passwords":          {{"PWD-001", "管理者ポリシー"}, {"PWD-002", "作業員ポリシー"}, {"PWD-003", "ゲストポリシー"}},
		"partners":           {{"PTR-001", "国内取引先"}, {"PTR-002", "海外取引先"}},
		"company":            {{"COM-001", "本社情報"}},
		"dashboard":          {{"DSH-001", "標準ダッシュボード"}},
	}
	for category, records := range seed {
		for index, record := range records {
			id := fmt.Sprintf("mst_preview_%s_%03d", strings.ReplaceAll(category, "-", "_"), index+1)
			if _, err := s.db.ExecContext(ctx, `
				INSERT INTO master_records(
					id,organization_id,category,record_code,name,is_active,created_by,created_at,updated_by,updated_at
				) VALUES(?,'org_preview',?,?,?,1,'usr_admin',?,'usr_admin',?)
				ON CONFLICT(id) DO UPDATE SET record_code=excluded.record_code,name=excluded.name,
					is_active=1,updated_by=excluded.updated_by,updated_at=excluded.updated_at`,
				id, category, record.code, record.name, now, now); err != nil {
				return err
			}
		}
	}
	for currency, value := range map[string]string{
		"USD": "155", "EUR": "165", "HKD": "19.8", "CHF": "172",
	} {
		var count int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM exchange_rate_snapshots
			WHERE organization_id='org_preview' AND base_currency=? AND quote_currency='JPY'`,
			currency).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			scaled, err := ParseRate(value)
			if err != nil {
				return err
			}
			if _, err := s.AddExchangeRate(ctx, "org_preview", currency, "JPY", scaled,
				"手動設定", "2026-03-24T09:00", "usr_admin"); err != nil {
				return err
			}
		}
	}
	return s.SeedGuestManagementPreview(ctx)
}
