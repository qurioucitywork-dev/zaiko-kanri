package database

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type ShipmentRevision struct {
	ID        string
	ActorName string
	Memo      string
	CreatedAt time.Time
}

type ShipmentEditLine struct {
	LineID              string
	WholesalePriceMinor int64
}

type UpdateShipmentSlipInput struct {
	OrganizationID string
	ShipmentSlipID string
	ShipmentDate   string
	Notes          string
	Memo           string
	ActorUserID    string
	Lines          []ShipmentEditLine
}

func (s *Store) ShipmentRevisions(ctx context.Context, organizationID, shipmentSlipID string) ([]ShipmentRevision, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id,u.display_name,r.memo,r.created_at
		FROM shipment_slip_revisions r
		JOIN users u ON u.id=r.actor_user_id
		WHERE r.organization_id=? AND r.shipment_slip_id=?
		ORDER BY r.created_at DESC`, organizationID, shipmentSlipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revisions []ShipmentRevision
	for rows.Next() {
		var revision ShipmentRevision
		var createdAt string
		if err := rows.Scan(&revision.ID, &revision.ActorName, &revision.Memo, &createdAt); err != nil {
			return nil, err
		}
		revision.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (s *Store) UpdateShipmentSlip(ctx context.Context, input UpdateShipmentSlipInput) error {
	input.ShipmentDate = strings.TrimSpace(input.ShipmentDate)
	input.Memo = strings.TrimSpace(input.Memo)
	if input.ShipmentDate == "" || input.Memo == "" {
		return errors.New("出荷日と修正メモを入力してください")
	}
	if _, err := time.Parse("2006-01-02", input.ShipmentDate); err != nil {
		return errors.New("出荷日を正しく入力してください")
	}
	before, err := s.Shipment(ctx, input.OrganizationID, input.ShipmentSlipID)
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE shipment_slips SET shipment_date=?,notes=?,updated_at=?
		WHERE id=? AND organization_id=?`,
		input.ShipmentDate, strings.TrimSpace(input.Notes), now,
		input.ShipmentSlipID, input.OrganizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return errors.New("出荷伝票が見つかりません")
	}
	for _, line := range input.Lines {
		if line.WholesalePriceMinor < 0 {
			return errors.New("卸値は0以上で入力してください")
		}
		result, err = tx.ExecContext(ctx, `
			UPDATE shipment_lines SET wholesale_price_minor=?
			WHERE id=? AND organization_id=? AND shipment_slip_id=?`,
			line.WholesalePriceMinor, line.LineID, input.OrganizationID, input.ShipmentSlipID)
		if err != nil {
			return err
		}
		affected, _ = result.RowsAffected()
		if affected != 1 {
			return errors.New("修正対象の商品明細が見つかりません")
		}
	}
	afterJSON, _ := json.Marshal(input)
	revisionID, _ := NewID("srev")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO shipment_slip_revisions(
			id,organization_id,shipment_slip_id,actor_user_id,memo,before_json,after_json,created_at
		) VALUES(?,?,?,?,?,?,?,?)`,
		revisionID, input.OrganizationID, input.ShipmentSlipID, input.ActorUserID,
		input.Memo, string(beforeJSON), string(afterJSON), now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SeedShipmentWorkflowPreview(ctx context.Context) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		UPDATE shipment_slips
		SET recipient_address=CASE WHEN recipient_address='' THEN '東京都新宿区' ELSE recipient_address END,
		    recipient_phone=CASE WHEN recipient_phone='' THEN '03-9999-0000' ELSE recipient_phone END,
		    tracking_number=CASE WHEN tracking_number='' THEN 'T7777888899' ELSE tracking_number END,
		    updated_at=?
		WHERE organization_id='org_preview'`, now)
	return err
}
