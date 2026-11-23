package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPurchaseReturnRevisionInvalid = errors.New("invalid purchase return revision")

type PurchaseReturnRevisionInput struct {
	OrganizationID  string `json:"-"`
	ActorUserID     string `json:"-"`
	TransactionDate string `json:"transactionDate"`
	ReturnReason    string `json:"returnReason"`
	Notes           string `json:"notes"`
	Reason          string `json:"reason"`
}

// RevisePurchaseReturn edits document information without reopening shipment tracking
// or changing the products and source purchase attached to the return.
func (r *Repository) RevisePurchaseReturn(ctx context.Context, returnID string, input PurchaseReturnRevisionInput) (ReturnSlipRecord, error) {
	date, err := time.Parse("2006-01-02", input.TransactionDate)
	notes, returnReason, reason := strings.TrimSpace(input.Notes), strings.TrimSpace(input.ReturnReason), strings.TrimSpace(input.Reason)
	if err != nil || notes == "" || reason == "" || len([]rune(notes)) > 10000 || len([]rune(returnReason)) > 2000 || len([]rune(reason)) > 2000 {
		return ReturnSlipRecord{}, ErrPurchaseReturnRevisionInvalid
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var stored struct {
			OperationType   string
			Status          string
			TransactionDate DateString
			Reason          string
			Notes           string
		}
		result := tx.Table("return_slips").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("operation_type,status,transaction_date,reason,notes").
			Where("organization_id=? AND id=?", input.OrganizationID, returnID).Take(&stored)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrReturnNotFound
		}
		if result.Error != nil {
			return result.Error
		}
		if stored.OperationType != "purchase_return" || stored.Status == "cancelled" {
			return ErrReturnState
		}
		if string(stored.TransactionDate) == input.TransactionDate && strings.TrimSpace(stored.Notes) == notes && strings.TrimSpace(stored.Reason) == returnReason {
			return ErrSlipRevisionUnchanged
		}
		return tx.Table("return_slips").Where("organization_id=? AND id=?", input.OrganizationID, returnID).
			Updates(map[string]any{"transaction_date": date, "reason": returnReason, "notes": notes + "\n修正理由: " + reason, "updated_at": time.Now().UTC()}).Error
	})
	if err != nil {
		return ReturnSlipRecord{}, err
	}
	return r.ReturnSlip(ctx, input.OrganizationID, returnID)
}
