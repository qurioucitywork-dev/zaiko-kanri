package persistence

import (
	"context"
	"sort"
	"time"
)

type DocumentRecord struct {
	DocumentType string     `json:"documentType"`
	ID           string     `json:"id"`
	Number       string     `json:"number"`
	Date         DateString `json:"date"`
	Status       string     `json:"status"`
	PartnerCode  string     `json:"partnerCode"`
	PartnerName  string     `json:"partnerName"`
	TotalJPY     int64      `json:"totalJpy"`
	TotalUSD     int64      `json:"totalUsd"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (r *Repository) Documents(ctx context.Context, organizationID string, limit int) ([]DocumentRecord, error) {
	if limit < 1 || limit > 1000 {
		limit = 300
	}
	var records []DocumentRecord
	var purchases []DocumentRecord
	if err := r.db.WithContext(ctx).Table("purchase_slips AS p").
		Select(`'purchase' AS document_type,p.id,p.slip_number AS number,p.purchase_date AS date,p.status,
			COALESCE(pr.role_code,'') AS partner_code,COALESCE(bp.legal_name,'') AS partner_name,
			COALESCE(SUM(CASE WHEN l.cost_currency='JPY' THEN l.unit_cost_minor*l.quantity ELSE 0 END),0) AS total_jpy,
			COALESCE(SUM(CASE WHEN l.cost_currency='USD' THEN l.unit_cost_minor*l.quantity ELSE 0 END),0) AS total_usd,p.updated_at`).
		Joins("LEFT JOIN partner_roles pr ON pr.id=p.supplier_role_id").Joins("LEFT JOIN business_partners bp ON bp.id=pr.partner_id").
		Joins("LEFT JOIN purchase_slip_lines l ON l.purchase_slip_id=p.id").Where("p.organization_id=?", organizationID).
		Group("p.id,pr.role_code,bp.legal_name").Scan(&purchases).Error; err != nil {
		return nil, err
	}
	records = append(records, purchases...)
	var sales []DocumentRecord
	if err := r.db.WithContext(ctx).Table("sales_slips AS s").
		Select(`'sale' AS document_type,s.id,s.slip_number AS number,s.sale_date AS date,s.status,
			br.role_code AS partner_code,bp.legal_name AS partner_name,
			COALESCE(SUM(CASE WHEN l.sale_currency='JPY' THEN l.total_minor ELSE 0 END),0) AS total_jpy,
			COALESCE(SUM(CASE WHEN l.sale_currency='USD' THEN l.total_minor ELSE 0 END),0) AS total_usd,s.updated_at`).
		Joins("JOIN partner_roles br ON br.id=s.buyer_role_id").Joins("JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("LEFT JOIN sales_lines l ON l.sales_slip_id=s.id").Where("s.organization_id=?", organizationID).
		Group("s.id,br.role_code,bp.legal_name").Scan(&sales).Error; err != nil {
		return nil, err
	}
	records = append(records, sales...)
	var shipments []DocumentRecord
	if err := r.db.WithContext(ctx).Table("shipment_slips AS s").
		Select(`'shipment' AS document_type,s.id,s.slip_number AS number,s.shipment_date AS date,s.status,
			br.role_code AS partner_code,bp.legal_name AS partner_name,0 AS total_jpy,0 AS total_usd,s.updated_at`).
		Joins("JOIN partner_roles br ON br.id=s.buyer_role_id").Joins("JOIN business_partners bp ON bp.id=br.partner_id").
		Where("s.organization_id=?", organizationID).Scan(&shipments).Error; err != nil {
		return nil, err
	}
	records = append(records, shipments...)
	var returns []DocumentRecord
	if err := r.db.WithContext(ctx).Table("return_slips AS r").
		Select(`CASE WHEN r.operation_type='purchase_return' THEN 'purchase_return' ELSE 'return' END AS document_type,
			r.id,r.slip_number AS number,r.transaction_date AS date,r.status,
			COALESCE(br.role_code,sr.role_code,'') AS partner_code,COALESCE(bp.legal_name,sp.legal_name,'') AS partner_name,
			0 AS total_jpy,0 AS total_usd,r.updated_at`).
		Joins("LEFT JOIN partner_roles br ON br.id=r.buyer_role_id").Joins("LEFT JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("LEFT JOIN partner_roles sr ON sr.id=r.supplier_role_id").Joins("LEFT JOIN business_partners sp ON sp.id=sr.partner_id").
		Where("r.organization_id=?", organizationID).Scan(&returns).Error; err != nil {
		return nil, err
	}
	records = append(records, returns...)
	sort.Slice(records, func(i, j int) bool {
		if string(records[i].Date) == string(records[j].Date) {
			return records[i].Number > records[j].Number
		}
		return string(records[i].Date) > string(records[j].Date)
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}
