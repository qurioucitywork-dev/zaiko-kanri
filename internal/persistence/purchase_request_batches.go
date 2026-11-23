package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"sort"
	"strings"
	"time"
)

type PurchaseRequestDecision struct {
	ID       string `json:"id"`
	Decision string `json:"decision"`
}

// One transaction reserves all products and creates one notification per submission.
func (r *Repository) CreateGuestPurchaseBatch(ctx context.Context, org, user string, ids []string, message string) (PurchaseRequestRecord, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return PurchaseRequestRecord{}, ErrPurchaseRequestState
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || seen[id] {
			return PurchaseRequestRecord{}, ErrPurchaseRequestState
		}
		seen[id] = true
	}
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	var batchID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		guest, err := lookupGuestIdentity(tx, org, user)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		batchID, err = database.NewID("rqb")
		if err != nil {
			return err
		}
		seq, err := nextPurchaseRequestSequence(tx, org, now.Year(), now)
		if err != nil {
			return err
		}
		number := fmt.Sprintf("RQ-%04d-%04d", now.Year(), seq)
		if err = tx.Exec(`INSERT INTO purchase_request_batches(id,organization_id,request_number,guest_account_id,buyer_role_id,requested_at) VALUES(?,?,?,?,?,?)`, batchID, org, number, guest.AccountID, guest.BuyerRoleID, now).Error; err != nil {
			return err
		}
		for i, id := range ids {
			var p struct {
				ID, ProductCode, InventoryStatus, Brand, ModelNumber, ReferenceNumber, BaseSaleCurrency string
				BaseSalePriceMinor                                                                      int64
			}
			result := tx.Raw(`SELECT p.id,p.product_code,p.inventory_status,p.brand,p.model_number,p.reference_number,p.base_sale_currency,p.base_sale_price_minor FROM products p
    WHERE p.organization_id=? AND p.id=? AND p.deleted_at IS NULL AND p.publication_status='public' AND p.inventory_status IN ('in_stock','shipped')
    AND EXISTS(SELECT 1 FROM publication_box_products x JOIN publication_boxes b ON b.id=x.box_id AND b.organization_id=p.organization_id AND b.is_active
      JOIN publication_box_buyers xb ON xb.box_id=b.id AND xb.buyer_role_id=? WHERE x.product_id=p.id AND x.visibility_state='visible') FOR UPDATE`, org, id, guest.BuyerRoleID).Scan(&p)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrProductUnavailable
			}
			var active int64
			if err = tx.Table("purchase_requests").Where("organization_id=? AND product_id=? AND status IN ('pending','approved')", org, id).Count(&active).Error; err != nil {
				return err
			}
			if active > 0 {
				return ErrProductUnavailable
			}
			snapshot, err := json.Marshal(map[string]any{"brand": p.Brand, "model": p.ModelNumber, "reference": p.ReferenceNumber, "priceMinor": p.BaseSalePriceMinor, "currency": p.BaseSaleCurrency})
			if err != nil {
				return err
			}
			lineID, err := database.NewID("req")
			if err != nil {
				return err
			}
			if err = tx.Exec(`INSERT INTO purchase_requests(id,organization_id,request_number,guest_account_id,buyer_role_id,product_id,status,message,requested_at,created_at,updated_at,product_snapshot,batch_id)
    VALUES(?,?,?,?,?,?,'pending',?,?,?,?,?::jsonb,?)`, lineID, org, fmt.Sprintf("%s-L%03d", number, i+1), guest.AccountID, guest.BuyerRoleID, id, strings.TrimSpace(message), now, now, now, string(snapshot), batchID).Error; err != nil {
				return err
			}
			if err = tx.Exec(`UPDATE products SET inventory_status='reserved',updated_at=? WHERE id=?`, now, id).Error; err != nil {
				return err
			}
			eventID, err := database.NewID("ive")
			if err != nil {
				return err
			}
			if err = tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at) VALUES(?,?,?,'purchase_request_submitted',?,'reserved',?,?,?)`, eventID, org, id, p.InventoryStatus, "購入リクエスト "+number, user, now).Error; err != nil {
				return err
			}
		}
		return insertNotificationTx(tx, org, "", database.RoleAdmin, "purchase_request.created", "購入リクエストが届きました", fmt.Sprintf("%s / %s / %d点", guest.BuyerName, number, len(ids)), "purchase_request", batchID, now)
	})
	if err != nil {
		return PurchaseRequestRecord{}, err
	}
	return r.PurchaseBatch(ctx, org, batchID)
}

// Legacy requests remain individual records. New requests expose their immutable line snapshots together.
func (r *Repository) groupPurchaseRequests(ctx context.Context, org string, rows []PurchaseRequestRecord) ([]PurchaseRequestRecord, error) {
	if len(rows) == 0 || !r.db.Migrator().HasTable("purchase_request_batches") {
		return rows, nil
	}
	type link struct{ ID, BatchID, RequestNumber, ShipmentSlipID, ShipmentSlipNumber string }
	var links []link
	if err := r.db.WithContext(ctx).Raw(`SELECT q.id,q.batch_id,b.request_number,COALESCE(b.shipment_slip_id,'') AS shipment_slip_id,COALESCE(s.slip_number,'') AS shipment_slip_number FROM purchase_requests q
 JOIN purchase_request_batches b ON b.id=q.batch_id LEFT JOIN shipment_slips s ON s.id=b.shipment_slip_id WHERE q.organization_id=?`, org).Scan(&links).Error; err != nil {
		return nil, err
	}
	byID := map[string]link{}
	for _, l := range links {
		byID[l.ID] = l
	}
	out := []PurchaseRequestRecord{}
	indexes := map[string]int{}
	for _, row := range rows {
		l, ok := byID[row.ID]
		if !ok {
			out = append(out, row)
			continue
		}
		index, exists := indexes[l.BatchID]
		if !exists {
			header := row
			header.ID = l.BatchID
			header.RequestNumber = l.RequestNumber
			header.BatchID = l.BatchID
			header.ShipmentSlipID = l.ShipmentSlipID
			header.ShipmentSlipNumber = l.ShipmentSlipNumber
			header.Items = nil
			out = append(out, header)
			index = len(out) - 1
			indexes[l.BatchID] = index
		}
		row.BatchID = l.BatchID
		out[index].Items = append(out[index].Items, row)
	}
	for i := range out {
		if len(out[i].Items) == 0 {
			continue
		}
		sort.Slice(out[i].Items, func(a, b int) bool { return out[i].Items[a].RequestNumber < out[i].Items[b].RequestNumber })
		out[i].Status = "rejected"
		for _, line := range out[i].Items {
			if line.Status == "pending" {
				out[i].Status = "pending"
				break
			}
			if line.Status == "approved" || line.Status == "sold" {
				out[i].Status = "approved"
			}
		}
	}
	return out, nil
}
func (r *Repository) PurchaseBatch(ctx context.Context, org, id string) (PurchaseRequestRecord, error) {
	rows, err := r.PurchaseRequests(ctx, org, "", database.RoleAdmin, "")
	if err != nil {
		return PurchaseRequestRecord{}, err
	}
	for _, row := range rows {
		if row.ID == id {
			return row, nil
		}
	}
	return PurchaseRequestRecord{}, ErrPurchaseRequestState
}

// Every line is decided together, producing at most one confirmed shipment.
func (r *Repository) ReviewPurchaseBatch(ctx context.Context, org, batchID, actor string, decisions []PurchaseRequestDecision, note string) (PurchaseRequestRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch struct {
			ID, BuyerRoleID, GuestAccountID, RequestNumber string
			ReviewedAt                                     *time.Time
		}
		q := tx.Raw(`SELECT id,buyer_role_id,guest_account_id,request_number,reviewed_at FROM purchase_request_batches WHERE organization_id=? AND id=? FOR UPDATE`, org, batchID).Scan(&batch)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || batch.ReviewedAt != nil {
			return ErrPurchaseRequestState
		}
		var lines []struct {
			ID, ProductID, Status, ProductCode, InventoryStatus, Currency string
			Price                                                         int64
			DeletedAt                                                     *time.Time
		}
		if err := tx.Raw(`SELECT q.id,q.product_id,q.status,p.product_code,p.inventory_status,p.deleted_at,COALESCE(q.product_snapshot->>'currency',p.base_sale_currency) AS currency,
    COALESCE((q.product_snapshot->>'priceMinor')::bigint,p.base_sale_price_minor) AS price FROM purchase_requests q JOIN products p ON p.id=q.product_id
    WHERE q.organization_id=? AND q.batch_id=? ORDER BY p.id FOR UPDATE OF q,p`, org, batchID).Scan(&lines).Error; err != nil {
			return err
		}
		if len(lines) == 0 || len(decisions) != len(lines) {
			return ErrPurchaseRequestState
		}
		choices := map[string]string{}
		for _, d := range decisions {
			if _, ok := choices[d.ID]; ok || (d.Decision != "approved" && d.Decision != "rejected") {
				return ErrPurchaseRequestState
			}
			choices[d.ID] = d.Decision
		}
		now := time.Now().UTC()
		codes := []string{}
		prices := []ShipmentPriceInput{}
		for _, line := range lines {
			decision, ok := choices[line.ID]
			if !ok || line.Status != "pending" || line.InventoryStatus != "reserved" || line.DeletedAt != nil {
				return ErrPurchaseRequestState
			}
			if err := tx.Exec(`UPDATE purchase_requests SET status=?,reviewed_by=?,reviewed_at=?,review_note=?,updated_at=? WHERE id=?`, decision, actor, now, strings.TrimSpace(note), now, line.ID).Error; err != nil {
				return err
			}
			if decision == "approved" {
				amount := line.Price
				if line.Currency != "USD" {
					usd, err := latestFX(tx, org, "USD")
					if err != nil {
						return err
					}
					if line.Currency != "JPY" {
						source, err := latestFX(tx, org, line.Currency)
						if err != nil {
							return err
						}
						amount = mulDivRound(amount, source.RateScaled, source.Scale)
					}
					amount = mulDivRound(amount, usd.Scale, usd.RateScaled)
				}
				codes = append(codes, line.ProductCode)
				prices = append(prices, ShipmentPriceInput{ProductCode: line.ProductCode, SalePriceUSDMinor: &amount})
			} else {
				if err := tx.Exec(`UPDATE products SET inventory_status='in_stock',updated_at=? WHERE id=?`, now, line.ProductID).Error; err != nil {
					return err
				}
				eventID, err := database.NewID("ive")
				if err != nil {
					return err
				}
				if err = tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at) VALUES(?,?,?,'purchase_request_rejected','reserved','in_stock',?,?,?)`, eventID, org, line.ProductID, "購入リクエスト "+batch.RequestNumber, actor, now).Error; err != nil {
					return err
				}
			}
		}
		shipmentID := ""
		if len(codes) > 0 {
			var party struct{ BuyerCode, Name, Address string }
			if err := tx.Raw(`SELECT br.role_code AS buyer_code,bp.legal_name AS name,bp.address FROM partner_roles br JOIN business_partners bp ON bp.id=br.partner_id WHERE br.id=? AND br.organization_id=?`, batch.BuyerRoleID, org).Scan(&party).Error; err != nil {
				return err
			}
			nested := *r
			nested.db = tx
			shipment, err := nested.CreateShipment(ctx, ShipmentCreateInput{allowZeroPrice: true, OrganizationID: org, ActorUserID: actor, BuyerCode: party.BuyerCode, ShipmentDate: now.In(japanLocation).Format("2006-01-02"), RecipientName: party.Name, RecipientAddress: party.Address, DisplayCurrency: "USD", ProductCodes: codes, Items: prices, Notes: "購入リクエスト " + batch.RequestNumber + " より"})
			if err != nil {
				return err
			}
			if _, err = nested.ConfirmShipment(ctx, org, shipment.ID, actor); err != nil {
				return err
			}
			shipmentID = shipment.ID
		}
		if err := tx.Exec(`UPDATE purchase_request_batches SET reviewed_at=?,reviewed_by=?,shipment_slip_id=? WHERE id=?`, now, actor, nullIfEmpty(shipmentID), batchID).Error; err != nil {
			return err
		}
		var guestUser string
		if err := tx.Table("guest_accounts").Select("user_id").Where("id=?", batch.GuestAccountID).Scan(&guestUser).Error; err != nil {
			return err
		}
		return insertNotificationTx(tx, org, guestUser, "", "purchase_request.reviewed", "購入リクエストの確認が完了しました", fmt.Sprintf("%s / 承認%d点・棄却%d点", batch.RequestNumber, len(codes), len(lines)-len(codes)), "purchase_request", batchID, now)
	})
	if err != nil {
		return PurchaseRequestRecord{}, err
	}
	return r.PurchaseBatch(ctx, org, batchID)
}
