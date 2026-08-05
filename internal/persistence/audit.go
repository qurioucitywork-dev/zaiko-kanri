package persistence

import (
	"context"
	"encoding/json"
	"time"
)

type AuditLogRecord struct {
	ID          string          `json:"id"`
	ActorUserID string          `json:"actorUserId,omitempty"`
	ActorName   string          `json:"actorName"`
	TargetType  string          `json:"targetType"`
	TargetID    string          `json:"targetId"`
	Action      string          `json:"action"`
	Before      json.RawMessage `gorm:"column:before_json" json:"before"`
	After       json.RawMessage `gorm:"column:after_json" json:"after"`
	Reason      string          `json:"reason"`
	Comment     string          `json:"comment"`
	IPAddress   string          `json:"ipAddress"`
	RequestID   string          `json:"requestId"`
	Result      string          `json:"result"`
	CreatedAt   time.Time       `json:"createdAt"`
}

func (r *Repository) AuditLogs(ctx context.Context, organizationID, action, targetType string, limit int) ([]AuditLogRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var records []AuditLogRecord
	query := r.db.WithContext(ctx).Table("audit_logs AS a").
		Select(`a.id,COALESCE(a.actor_user_id,'') AS actor_user_id,COALESCE(u.display_name,'システム') AS actor_name,
			a.target_type,a.target_id,a.action,a.before_json,a.after_json,a.reason,a.comment,a.ip_address,
			a.request_id,a.result,a.created_at`).
		Joins("LEFT JOIN users u ON u.id=a.actor_user_id").Where("a.organization_id=?", organizationID)
	if action != "" {
		query = query.Where("a.action=?", action)
	}
	if targetType != "" {
		query = query.Where("a.target_type=?", targetType)
	}
	err := query.Order("a.created_at DESC,a.id DESC").Limit(limit).Scan(&records).Error
	return records, err
}
