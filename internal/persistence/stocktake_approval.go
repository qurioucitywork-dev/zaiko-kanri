package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

func (r *Repository) loadStocktakeApprovalStates(ctx context.Context, org string, lines []StocktakeLine) error {
	if len(lines) == 0 || !r.db.Migrator().HasTable("approval_requests") {
		return nil
	}
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		ids = append(ids, line.ID)
	}
	var rows []struct{ TargetID, Status string }
	if err := r.db.WithContext(ctx).Table("approval_requests").Select("target_id,status").Where("organization_id=? AND target_type='stocktake_line' AND target_id IN ?", org, ids).Order("requested_at,id").Scan(&rows).Error; err != nil {
		return err
	}
	states := map[string]string{}
	for _, row := range rows {
		states[row.TargetID] = row.Status
	}
	for i := range lines {
		lines[i].ApprovalStatus = states[lines[i].ID]
	}
	return nil
}

func createStocktakeApproval(tx *gorm.DB, org, actor string, line StocktakeLine, now time.Time) error {
	id, err := database.NewID("apr")
	if err != nil {
		return err
	}
	snapshot, _ := json.Marshal(line)
	if err := tx.Exec(`INSERT INTO approval_requests(id,organization_id,approval_type,target_type,target_id,requested_action,status,requested_by,requested_at,snapshot_json,created_at,updated_at)
 VALUES(?,?,'棚卸不一致','stocktake_line',?,'stocktake.mismatch','pending',?,?,CAST(? AS JSONB),?,?)`, id, org, line.ID, actor, now, string(snapshot), now, now).Error; err != nil {
		return err
	}
	action, _ := database.NewID("apa")
	if err := tx.Exec(`INSERT INTO approval_actions(id,approval_request_id,actor_user_id,action,note,created_at) VALUES(?,?,?,'requested','',?)`, action, id, actor, now).Error; err != nil {
		return err
	}
	return insertNotificationTx(tx, org, "", database.RoleAdmin, "approval.requested", "棚卸不一致の承認申請", line.ProductCode+"："+line.Reason, "approval_request", id, now)
}

func (r *Repository) SaveStocktakeWithActor(ctx context.Context, org, sessionID, actor string, admin bool, updates map[string]struct{ Reason, Note string }, resolvedIDs []string) (StocktakeSession, error) {
	if !admin && len(resolvedIDs) > 0 {
		return StocktakeSession{}, ErrApprovalInvalid
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session StocktakeSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND organization_id=? AND status='in_progress'", sessionID, org).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrStocktakeNotFound
			}
			return err
		}
		now := time.Now().UTC()
		for id, update := range updates {
			var line StocktakeLine
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND session_id=? AND organization_id=?", id, sessionID, org).First(&line).Error; err != nil {
				return err
			}
			reason, note := strings.TrimSpace(update.Reason), strings.TrimSpace(update.Note)
			changed := reason != line.Reason || note != line.Note
			var approvalStatus string
			if err := tx.Table("approval_requests").Select("status").Where("organization_id=? AND target_type='stocktake_line' AND target_id=?", org, id).Order("requested_at DESC,id DESC").Limit(1).Scan(&approvalStatus).Error; err != nil {
				return err
			}
			if approvalStatus == "pending" {
				if changed {
					return fmt.Errorf("承認待ちの理由は変更できません")
				}
				continue
			}
			if !changed && (approvalStatus == "approved" || reason == "") {
				continue
			}
			line.Reason, line.Note = reason, note
			var resolved any
			if admin && reason != "" {
				resolved = now
			}
			if err := tx.Model(&StocktakeLine{}).Where("id=?", id).Updates(map[string]any{"reason": reason, "note": note, "resolved_at": resolved, "updated_at": now}).Error; err != nil {
				return err
			}
			if !admin && reason != "" {
				if line.ResultStatus == "verified" {
					return ErrApprovalInvalid
				}
				if err := createStocktakeApproval(tx, org, actor, line, now); err != nil {
					return err
				}
			}
		}
		if len(resolvedIDs) > 0 {
			// Pending requests must be decided through approval, never bypassed by a save.
			var pending int64
			if err := tx.Table("approval_requests").Where("organization_id=? AND target_type='stocktake_line' AND target_id IN ? AND status='pending'", org, resolvedIDs).Count(&pending).Error; err != nil {
				return err
			}
			if pending > 0 {
				return ErrApprovalInvalid
			}
			if err := tx.Model(&StocktakeLine{}).Where("organization_id=? AND session_id=? AND id IN ? AND reason<>''", org, sessionID, resolvedIDs).Updates(map[string]any{"resolved_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&StocktakeSession{}).Where("id=?", sessionID).Updates(map[string]any{"saved_at": now, "updated_at": now}).Error
	})
	if err != nil {
		return StocktakeSession{}, err
	}
	return r.CurrentStocktake(ctx, org)
}

// Approval and the resolution of its exact line are committed together.
func decideStocktakeLine(tx *gorm.DB, org string, record ApprovalRequestRecord, decision string, now time.Time) error {
	var line StocktakeLine
	if err := tx.Where("organization_id=? AND id=?", org, record.TargetID).First(&line).Error; err != nil {
		return err
	}
	var session StocktakeSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id=? AND id=?", org, line.SessionID).First(&session).Error; err != nil {
		return err
	}
	if session.Status != "in_progress" {
		return ErrStocktakeNotFound
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id=? AND id=?", org, record.TargetID).First(&line).Error; err != nil {
		return err
	}
	var snapshot StocktakeLine
	if err := json.Unmarshal([]byte(record.SnapshotJSON), &snapshot); err != nil {
		return err
	}
	if line.Reason != snapshot.Reason || line.Note != snapshot.Note {
		return ErrApprovalInvalid
	}
	if decision == "approved" {
		return tx.Model(&StocktakeLine{}).Where("id=?", line.ID).Updates(map[string]any{"resolved_at": now, "updated_at": now}).Error
	}
	return nil
}
