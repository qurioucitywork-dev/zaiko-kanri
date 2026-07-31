package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type PurchaseProductLine struct {
	ProductID          string
	ProductCode        string
	SKU                string
	Brand              string
	ModelNumber        string
	CostAmountMinor    int64
	CostCurrency       string
	BaseSalePriceMinor int64
	BaseSaleCurrency   string
	InventoryStatus    string
}

type PurchaseRevision struct {
	ID           string
	ActorName    string
	ApproverName string
	Memo         string
	CreatedAt    time.Time
}

type PurchaseEditProduct struct {
	ProductID          string
	SKU                string
	CostAmountMinor    int64
	BaseSalePriceMinor int64
}

type UpdatePurchaseSlipInput struct {
	OrganizationID string
	PurchaseSlipID string
	PurchaseDate   string
	Notes          string
	Memo           string
	ActorUserID    string
	Products       []PurchaseEditProduct
}

type CreatePurchaseReturnInput struct {
	OrganizationID string
	PurchaseSlipID string
	ReturnDate     string
	Reason         string
	Notes          string
	ActorUserID    string
	ProductIDs     []string
}

func (s *Store) PurchaseProducts(ctx context.Context, organizationID, purchaseSlipID string) ([]PurchaseProductLine, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id,p.product_code,p.sku,p.brand,p.model_number,p.cost_amount_minor,p.cost_currency,
		       p.base_sale_price_minor,p.base_sale_currency,p.inventory_status
		FROM products p
		JOIN purchase_slip_lines l ON l.id=p.purchase_slip_line_id
		JOIN purchase_slips ps ON ps.id=l.purchase_slip_id
		WHERE p.organization_id=? AND ps.organization_id=? AND ps.id=? AND p.deleted_at IS NULL
		ORDER BY p.product_code`, organizationID, organizationID, purchaseSlipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []PurchaseProductLine
	for rows.Next() {
		var product PurchaseProductLine
		if err := rows.Scan(&product.ProductID, &product.ProductCode, &product.SKU, &product.Brand,
			&product.ModelNumber, &product.CostAmountMinor, &product.CostCurrency,
			&product.BaseSalePriceMinor, &product.BaseSaleCurrency,
			&product.InventoryStatus); err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

func (s *Store) PurchaseRevisions(ctx context.Context, organizationID, purchaseSlipID string) ([]PurchaseRevision, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id,u.display_name,r.memo,r.created_at
		FROM purchase_slip_revisions r
		JOIN users u ON u.id=r.actor_user_id
		WHERE r.organization_id=? AND r.purchase_slip_id=?
		ORDER BY r.created_at DESC`, organizationID, purchaseSlipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revisions []PurchaseRevision
	for rows.Next() {
		var revision PurchaseRevision
		var createdAt string
		if err := rows.Scan(&revision.ID, &revision.ActorName, &revision.Memo, &createdAt); err != nil {
			return nil, err
		}
		revision.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (s *Store) UpdatePurchaseSlip(ctx context.Context, input UpdatePurchaseSlipInput) error {
	input.PurchaseDate = strings.TrimSpace(input.PurchaseDate)
	input.Memo = strings.TrimSpace(input.Memo)
	if input.PurchaseDate == "" || input.Memo == "" {
		return errors.New("仕入日と修正メモを入力してください")
	}
	before, err := s.Purchase(ctx, input.OrganizationID, input.PurchaseSlipID)
	if err != nil {
		return err
	}
	beforeProducts, err := s.PurchaseProducts(ctx, input.OrganizationID, input.PurchaseSlipID)
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(struct {
		Slip     PurchaseSlip
		Products []PurchaseProductLine
	}{before, beforeProducts})

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE purchase_slips SET purchase_date=?,notes=?,updated_at=?
		WHERE id=? AND organization_id=?`,
		input.PurchaseDate, strings.TrimSpace(input.Notes), now, input.PurchaseSlipID, input.OrganizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return errors.New("仕入伝票が見つかりません")
	}
	for _, product := range input.Products {
		if product.CostAmountMinor < 0 || product.BaseSalePriceMinor < 0 {
			return errors.New("金額は0以上で入力してください")
		}
		result, err = tx.ExecContext(ctx, `
			UPDATE products SET sku=?,cost_amount_minor=?,base_sale_price_minor=?,purchase_date=?,updated_at=?
			WHERE id=? AND organization_id=? AND purchase_slip_line_id IN (
				SELECT id FROM purchase_slip_lines WHERE purchase_slip_id=?
			)`, strings.TrimSpace(product.SKU), product.CostAmountMinor, product.BaseSalePriceMinor,
			input.PurchaseDate, now, product.ProductID, input.OrganizationID, input.PurchaseSlipID)
		if err != nil {
			return err
		}
		affected, _ = result.RowsAffected()
		if affected != 1 {
			return errors.New("修正対象の商品が見つかりません")
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE purchase_slip_lines
			SET unit_cost_minor=?,base_sale_price_minor=?
			WHERE id=(SELECT purchase_slip_line_id FROM products WHERE id=?)`,
			product.CostAmountMinor, product.BaseSalePriceMinor, product.ProductID); err != nil {
			return err
		}
	}
	afterJSON, _ := json.Marshal(input)
	revisionID, _ := NewID("prev")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_slip_revisions(
			id,organization_id,purchase_slip_id,actor_user_id,memo,before_json,after_json,created_at
		) VALUES(?,?,?,?,?,?,?,?)`,
		revisionID, input.OrganizationID, input.PurchaseSlipID, input.ActorUserID,
		input.Memo, string(beforeJSON), string(afterJSON), now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreatePurchaseReturn(ctx context.Context, input CreatePurchaseReturnInput) (PurchaseReturnSlip, error) {
	input.ReturnDate = strings.TrimSpace(input.ReturnDate)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ReturnDate == "" || input.Reason == "" || len(input.ProductIDs) == 0 {
		return PurchaseReturnSlip{}, errors.New("返品対象商品・返品日・返品理由を入力してください")
	}
	purchase, err := s.Purchase(ctx, input.OrganizationID, input.PurchaseSlipID)
	if err != nil {
		return PurchaseReturnSlip{}, err
	}
	products, err := s.PurchaseProducts(ctx, input.OrganizationID, input.PurchaseSlipID)
	if err != nil {
		return PurchaseReturnSlip{}, err
	}
	available := make(map[string]PurchaseProductLine, len(products))
	for _, product := range products {
		available[product.ProductID] = product
	}
	selected := make([]PurchaseProductLine, 0, len(input.ProductIDs))
	seen := make(map[string]bool)
	for _, id := range input.ProductIDs {
		if seen[id] {
			continue
		}
		product, ok := available[id]
		if !ok {
			return PurchaseReturnSlip{}, errors.New("返品対象商品が仕入伝票に含まれていません")
		}
		seen[id] = true
		selected = append(selected, product)
	}
	if len(selected) == 0 {
		return PurchaseReturnSlip{}, errors.New("返品対象商品を選択してください")
	}

	amountsJPY := make(map[string]int64, len(selected))
	var usdJPYRate ExchangeRate
	rateLoaded := false
	for _, product := range selected {
		switch product.CostCurrency {
		case "", "JPY":
			amountsJPY[product.ProductID] = product.CostAmountMinor
		case "USD":
			if !rateLoaded {
				usdJPYRate, err = s.LatestExchangeRate(ctx, input.OrganizationID, "USD", "JPY")
				if err != nil {
					return PurchaseReturnSlip{}, fmt.Errorf("USD仕入商品の返品にはUSD/JPY為替レートが必要です: %w", err)
				}
				rateLoaded = true
			}
			amountsJPY[product.ProductID], err = ConvertMinor(
				product.CostAmountMinor, usdJPYRate.RateScaled, usdJPYRate.Scale, false,
			)
			if err != nil {
				return PurchaseReturnSlip{}, fmt.Errorf("仕入返品金額を日本円へ換算できません: %w", err)
			}
		default:
			return PurchaseReturnSlip{}, fmt.Errorf("未対応の仕入通貨です: %s", product.CostCurrency)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PurchaseReturnSlip{}, err
	}
	defer tx.Rollback()
	var sequence int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(CAST(SUBSTR(return_number,8) AS INTEGER)),0)+1
		FROM purchase_return_slips WHERE organization_id=?`, input.OrganizationID).Scan(&sequence); err != nil {
		return PurchaseReturnSlip{}, err
	}
	id, _ := NewID("pret")
	number := fmt.Sprintf("PR-RET-%04d", sequence)
	var total int64
	for _, product := range selected {
		total += amountsJPY[product.ProductID]
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_return_slips(
			id,organization_id,purchase_slip_id,return_number,return_date,supplier_name,item_count,
			amount_jpy,reason,status,delivery_number,created_at,updated_at,notes,created_by
		) VALUES(?,?,?,?,?,?,?,?,?,'pending','',?,?,?,?)`,
		id, input.OrganizationID, input.PurchaseSlipID, number, input.ReturnDate, purchase.SupplierName,
		len(selected), total, input.Reason, now, now, strings.TrimSpace(input.Notes), input.ActorUserID); err != nil {
		return PurchaseReturnSlip{}, err
	}
	for _, product := range selected {
		lineID, _ := NewID("pretl")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_return_lines(
				id,organization_id,purchase_return_slip_id,product_id,product_code,sku,brand,
				model_number,amount_jpy,created_at
			) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			lineID, input.OrganizationID, id, product.ProductID, product.ProductCode, product.SKU,
			product.Brand, product.ModelNumber, amountsJPY[product.ProductID], now); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return PurchaseReturnSlip{}, errors.New("選択した商品には起票済みの仕入返品伝票があります")
			}
			return PurchaseReturnSlip{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return PurchaseReturnSlip{}, err
	}
	return PurchaseReturnSlip{
		ID: id, PurchaseSlipID: input.PurchaseSlipID, ReturnNumber: number,
		ReturnDate: input.ReturnDate, SupplierName: purchase.SupplierName,
		ItemCount: len(selected), AmountJPY: total, Reason: input.Reason, Status: "pending",
	}, nil
}
