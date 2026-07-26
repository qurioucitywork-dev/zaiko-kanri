package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrSelfApproval       = errors.New("自分が申請した案件は承認できません")
	ErrApprovalStale      = errors.New("申請後に対象データが変更されたため、この承認申請は失効しました")
	ErrApprovalNotPending = errors.New("申請中の承認案件だけ処理できます")
)

type ApprovalRequest struct {
	ID                    string
	OrganizationID        string
	ApprovalType          string
	TargetType            string
	TargetID              string
	ActionKey             string
	ApplicantUserID       string
	ApplicantName         string
	Status                string
	RequestedSnapshot     string
	RequestedSnapshotHash string
	RequestReason         string
	ActionPayloadJSON     string
	RequestedAt           time.Time
	DecidedAt             *time.Time
	DecidedBy             string
	ExecutedAt            *time.Time
	Actions               []ApprovalAction
}

type ApprovalAction struct {
	Action    string
	ActorID   string
	ActorName string
	Comment   string
	ActedAt   time.Time
}

type CreateApprovalInput struct {
	OrganizationID  string
	ApprovalType    string
	TargetType      string
	TargetID        string
	ActionKey       string
	ApplicantUserID string
	RequestReason   string
	ActionPayload   any
}

func (s *Store) CreateApprovalRequest(ctx context.Context, input CreateApprovalInput) (ApprovalRequest, error) {
	if input.OrganizationID == "" || input.TargetID == "" || input.ApplicantUserID == "" ||
		input.TargetType == "" || input.ActionKey == "" {
		return ApprovalRequest{}, errors.New("承認申請の対象情報が不足しています")
	}
	snapshot, err := s.approvalTargetSnapshot(ctx, input.OrganizationID, input.TargetType, input.TargetID)
	if err != nil {
		return ApprovalRequest{}, err
	}
	payload, err := json.Marshal(input.ActionPayload)
	if err != nil {
		return ApprovalRequest{}, err
	}
	id, _ := NewID("apr")
	now := s.now().Format(time.RFC3339Nano)
	hash := approvalSnapshotHash(snapshot)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ApprovalRequest{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approval_requests(
			id,organization_id,approval_type,target_type,target_id,action_key,applicant_user_id,status,
			requested_snapshot,requested_snapshot_hash,request_reason,action_payload_json,requested_at,
			created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,'pending',?,?,?,?,?,?,?)`,
		id, input.OrganizationID, input.ApprovalType, input.TargetType, input.TargetID, input.ActionKey,
		input.ApplicantUserID, snapshot, hash, strings.TrimSpace(input.RequestReason), string(payload),
		now, now, now); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ApprovalRequest{}, errors.New("同じ対象・操作の承認申請がすでに存在します")
		}
		return ApprovalRequest{}, err
	}
	actionID, _ := NewID("apa")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approval_actions(id,organization_id,approval_request_id,actor_user_id,action,comment,acted_at)
		VALUES(?,?,?,?, 'requested',?,?)`,
		actionID, input.OrganizationID, id, input.ApplicantUserID, strings.TrimSpace(input.RequestReason), now); err != nil {
		return ApprovalRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return ApprovalRequest{}, err
	}
	return s.ApprovalRequest(ctx, input.OrganizationID, id)
}

func (s *Store) Approvals(ctx context.Context, organizationID string) ([]ApprovalRequest, error) {
	rows, err := s.db.QueryContext(ctx, approvalSelect+`
		WHERE a.organization_id=? ORDER BY
		CASE a.status WHEN 'pending' THEN 0 ELSE 1 END,a.requested_at DESC LIMIT 500`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var approvals []ApprovalRequest
	for rows.Next() {
		approval, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, approval)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range approvals {
		actions, err := s.ApprovalActions(ctx, organizationID, approvals[index].ID)
		if err != nil {
			return nil, err
		}
		approvals[index].Actions = actions
	}
	return approvals, nil
}

func (s *Store) ApprovalRequest(ctx context.Context, organizationID, approvalID string) (ApprovalRequest, error) {
	approval, err := scanApproval(s.db.QueryRowContext(ctx, approvalSelect+`
		WHERE a.organization_id=? AND a.id=?`, organizationID, approvalID))
	if err != nil {
		return ApprovalRequest{}, err
	}
	approval.Actions, err = s.ApprovalActions(ctx, organizationID, approvalID)
	return approval, err
}

const approvalSelect = `
	SELECT a.id,a.organization_id,a.approval_type,a.target_type,a.target_id,a.action_key,
	       a.applicant_user_id,u.display_name,a.status,a.requested_snapshot,a.requested_snapshot_hash,
	       a.request_reason,a.action_payload_json,a.requested_at,a.decided_at,COALESCE(a.decided_by,''),a.executed_at
	FROM approval_requests a JOIN users u ON u.id=a.applicant_user_id AND u.organization_id=a.organization_id
`

func scanApproval(row rowScanner) (ApprovalRequest, error) {
	var approval ApprovalRequest
	var requested string
	var decided, executed sql.NullString
	err := row.Scan(&approval.ID, &approval.OrganizationID, &approval.ApprovalType, &approval.TargetType,
		&approval.TargetID, &approval.ActionKey, &approval.ApplicantUserID, &approval.ApplicantName,
		&approval.Status, &approval.RequestedSnapshot, &approval.RequestedSnapshotHash,
		&approval.RequestReason, &approval.ActionPayloadJSON, &requested, &decided,
		&approval.DecidedBy, &executed)
	if err != nil {
		return ApprovalRequest{}, err
	}
	approval.RequestedAt, _ = time.Parse(time.RFC3339Nano, requested)
	if decided.Valid {
		value, _ := time.Parse(time.RFC3339Nano, decided.String)
		approval.DecidedAt = &value
	}
	if executed.Valid {
		value, _ := time.Parse(time.RFC3339Nano, executed.String)
		approval.ExecutedAt = &value
	}
	return approval, nil
}

func (s *Store) ApprovalActions(ctx context.Context, organizationID, approvalID string) ([]ApprovalAction, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.action,a.actor_user_id,u.display_name,a.comment,a.acted_at
		FROM approval_actions a JOIN users u ON u.id=a.actor_user_id AND u.organization_id=a.organization_id
		WHERE a.organization_id=? AND a.approval_request_id=? ORDER BY a.acted_at`,
		organizationID, approvalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actions []ApprovalAction
	for rows.Next() {
		var action ApprovalAction
		var acted string
		if err := rows.Scan(&action.Action, &action.ActorID, &action.ActorName, &action.Comment, &acted); err != nil {
			return nil, err
		}
		action.ActedAt, _ = time.Parse(time.RFC3339Nano, acted)
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

func (s *Store) Approve(ctx context.Context, organizationID, approvalID, approverID, comment string) (ApprovalRequest, error) {
	approval, err := s.ApprovalRequest(ctx, organizationID, approvalID)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if approval.Status != "pending" {
		return ApprovalRequest{}, ErrApprovalNotPending
	}
	if approval.ApplicantUserID == approverID {
		return ApprovalRequest{}, ErrSelfApproval
	}
	current, err := s.approvalTargetSnapshot(ctx, organizationID, approval.TargetType, approval.TargetID)
	if err != nil || approvalSnapshotHash(current) != approval.RequestedSnapshotHash {
		if expireErr := s.finishApproval(ctx, approval, approverID, "expired", "対象データが変更されました", false); expireErr != nil {
			return ApprovalRequest{}, expireErr
		}
		return ApprovalRequest{}, ErrApprovalStale
	}
	if err := s.executeApprovedAction(ctx, approval, approverID); err != nil {
		return ApprovalRequest{}, err
	}
	if err := s.finishApproval(ctx, approval, approverID, "approved", strings.TrimSpace(comment), true); err != nil {
		return ApprovalRequest{}, err
	}
	return s.ApprovalRequest(ctx, organizationID, approvalID)
}

func (s *Store) ReturnApproval(ctx context.Context, organizationID, approvalID, actorID, comment string) error {
	if strings.TrimSpace(comment) == "" {
		return errors.New("差戻しコメントは必須です")
	}
	approval, err := s.ApprovalRequest(ctx, organizationID, approvalID)
	if err != nil {
		return err
	}
	if approval.Status != "pending" {
		return ErrApprovalNotPending
	}
	if approval.ApplicantUserID == actorID {
		return ErrSelfApproval
	}
	return s.finishApproval(ctx, approval, actorID, "returned", strings.TrimSpace(comment), false)
}

func (s *Store) RejectApproval(ctx context.Context, organizationID, approvalID, actorID, comment string) error {
	approval, err := s.ApprovalRequest(ctx, organizationID, approvalID)
	if err != nil {
		return err
	}
	if approval.Status != "pending" {
		return ErrApprovalNotPending
	}
	if approval.ApplicantUserID == actorID {
		return ErrSelfApproval
	}
	return s.finishApproval(ctx, approval, actorID, "rejected", strings.TrimSpace(comment), false)
}

func (s *Store) finishApproval(ctx context.Context, approval ApprovalRequest, actorID, status, comment string, executed bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now().Format(time.RFC3339Nano)
	var executedAt any
	if executed {
		executedAt = now
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE approval_requests SET status=?,decided_at=?,decided_by=?,executed_at=?,updated_at=?
		WHERE id=? AND organization_id=? AND status='pending'`,
		status, now, actorID, executedAt, now, approval.ID, approval.OrganizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return ErrApprovalNotPending
	}
	actionID, _ := NewID("apa")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approval_actions(id,organization_id,approval_request_id,actor_user_id,action,comment,acted_at)
		VALUES(?,?,?,?,?,?,?)`,
		actionID, approval.OrganizationID, approval.ID, actorID, status, comment, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) executeApprovedAction(ctx context.Context, approval ApprovalRequest, actorID string) error {
	var payload map[string]any
	_ = json.Unmarshal([]byte(approval.ActionPayloadJSON), &payload)
	reason, _ := payload["reason"].(string)
	switch approval.ActionKey {
	case "sale.confirm":
		_, err := s.ConfirmSale(ctx, approval.OrganizationID, approval.TargetID, actorID)
		return err
	case "sale.cancel":
		return s.CancelSale(ctx, approval.OrganizationID, approval.TargetID, actorID, reason)
	case "shipment.cancel":
		return s.CancelShipment(ctx, approval.OrganizationID, approval.TargetID, actorID, reason)
	default:
		return fmt.Errorf("未対応の承認操作です: %s", approval.ActionKey)
	}
}

func (s *Store) approvalTargetSnapshot(ctx context.Context, organizationID, targetType, targetID string) (string, error) {
	var value any
	switch targetType {
	case "sales_slip":
		sale, err := s.Sale(ctx, organizationID, targetID)
		if err != nil {
			return "", err
		}
		value = struct {
			ID       string
			Status   string
			Date     string
			Customer string
			Lines    []SalesLine
		}{sale.ID, sale.Status, sale.SalesDate, sale.CustomerName, sale.Lines}
	case "shipment_slip":
		shipment, err := s.Shipment(ctx, organizationID, targetID)
		if err != nil {
			return "", err
		}
		value = struct {
			ID        string
			Status    string
			Date      string
			Recipient string
			Lines     []ShipmentLine
		}{shipment.ID, shipment.Status, shipment.ShipmentDate, shipment.RecipientName, shipment.Lines}
	default:
		return "", errors.New("承認対象の種類が正しくありません")
	}
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

func approvalSnapshotHash(snapshot string) string {
	sum := sha256.Sum256([]byte(snapshot))
	return hex.EncodeToString(sum[:])
}

func (s *Store) SaleAmountJPY(ctx context.Context, organizationID, saleID string) (int64, error) {
	sale, err := s.Sale(ctx, organizationID, saleID)
	if err != nil {
		return 0, err
	}
	var total int64
	var rate ExchangeRate
	for _, line := range sale.Lines {
		unit := line.UnitPriceMinor
		if line.SaleCurrency == "USD" {
			if rate.ID == "" {
				rate, err = s.LatestExchangeRate(ctx, organizationID, "USD", "JPY")
				if err != nil {
					return 0, errors.New("USD売上の承認判定には為替レートが必要です")
				}
			}
			unit, err = ConvertMinor(unit, rate.RateScaled, rate.Scale, false)
			if err != nil {
				return 0, err
			}
		}
		if line.Quantity < 1 || unit > (1<<63-1)/int64(line.Quantity) {
			return 0, errors.New("売上金額が大きすぎます")
		}
		lineTotal := unit * int64(line.Quantity)
		if total > (1<<63-1)-lineTotal {
			return 0, errors.New("売上金額が大きすぎます")
		}
		total += lineTotal
	}
	return total, nil
}

func (s *Store) NeedsSaleApproval(ctx context.Context, organizationID, saleID, role string) (bool, int64, error) {
	total, err := s.SaleAmountJPY(ctx, organizationID, saleID)
	if err != nil {
		return false, 0, err
	}
	if role == RoleAdmin {
		enabled, err := s.settingBool(ctx, organizationID, "approval.admin_high_value_enabled")
		if err != nil || !enabled {
			return false, total, err
		}
		threshold, configured, err := s.settingInt(ctx, organizationID, "approval.admin_high_value_threshold_jpy")
		return configured && total >= threshold, total, err
	}
	threshold, configured, err := s.settingInt(ctx, organizationID, "approval.sales_threshold_jpy")
	return configured && total >= threshold, total, err
}

func (s *Store) settingInt(ctx context.Context, organizationID, key string) (int64, bool, error) {
	var raw string
	var configured bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT setting_value,is_configured FROM organization_settings WHERE organization_id=? AND setting_key=?`,
		organizationID, key).Scan(&raw, &configured); err != nil {
		return 0, false, err
	}
	if !configured || strings.TrimSpace(raw) == "" {
		return 0, false, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	return value, true, err
}

func (s *Store) settingBool(ctx context.Context, organizationID, key string) (bool, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `
		SELECT setting_value FROM organization_settings WHERE organization_id=? AND setting_key=?`,
		organizationID, key).Scan(&raw); err != nil {
		return false, err
	}
	return raw == "true", nil
}

// SeedApprovalPreview creates one pending request so the development preview
// demonstrates the reviewer workflow without requiring data entry first.
func (s *Store) SeedApprovalPreview(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM approval_requests
		WHERE organization_id='org_preview' AND status='pending'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var workerID, shipmentID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id FROM users
		WHERE organization_id='org_preview' AND role_key='worker' AND is_active=1
		ORDER BY created_at LIMIT 1`).Scan(&workerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT id FROM shipment_slips
		WHERE organization_id='org_preview' AND status='confirmed'
		ORDER BY shipment_date,id LIMIT 1`).Scan(&shipmentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	_, err := s.CreateApprovalRequest(ctx, CreateApprovalInput{
		OrganizationID:  "org_preview",
		ApprovalType:    "important_operation",
		TargetType:      "shipment_slip",
		TargetID:        shipmentID,
		ActionKey:       "shipment.cancel",
		ApplicantUserID: workerID,
		RequestReason:   "配送先の変更依頼を受けたため、出荷取消の承認をお願いします。",
		ActionPayload:   map[string]string{"reason": "配送先変更のため"},
	})
	return err
}
