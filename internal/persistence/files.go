package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrProductFileLimit = errors.New("product file limit reached")

type ProductFileRecord struct {
	ID            string    `json:"id"`
	ProductID     string    `json:"productId"`
	StorageDriver string    `json:"storageDriver"`
	ObjectKey     string    `json:"-"`
	OriginalName  string    `json:"originalName"`
	ContentType   string    `json:"contentType"`
	SizeBytes     int64     `json:"sizeBytes"`
	SHA256        string    `json:"sha256"`
	SortOrder     int       `json:"sortOrder"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (r *Repository) PrepareProductFile(ctx context.Context, organizationID, productID string) (int, error) {
	var productCount int64
	if err := r.db.WithContext(ctx).Table("products").Where(
		"organization_id=? AND id=? AND deleted_at IS NULL", organizationID, productID).Count(&productCount).Error; err != nil {
		return 0, err
	}
	if productCount == 0 {
		return 0, ErrProductUnavailable
	}
	var count int64
	if err := r.db.WithContext(ctx).Table("product_files").Where(
		"organization_id=? AND product_id=?", organizationID, productID).Count(&count).Error; err != nil {
		return 0, err
	}
	if count >= 10 {
		return 0, ErrProductFileLimit
	}
	return int(count), nil
}

func (r *Repository) CreateProductFile(ctx context.Context, organizationID, actorUserID string, record ProductFileRecord) (ProductFileRecord, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("product_files").Where("organization_id=? AND product_id=?", organizationID, record.ProductID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 10 {
			return ErrProductFileLimit
		}
		record.SortOrder = int(count)
		return tx.Exec(`INSERT INTO product_files(
			id,organization_id,product_id,storage_driver,object_key,original_name,content_type,size_bytes,
			sha256,sort_order,uploaded_by,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, record.ID, organizationID, record.ProductID, record.StorageDriver,
			record.ObjectKey, record.OriginalName, record.ContentType, record.SizeBytes, record.SHA256,
			record.SortOrder, actorUserID, now).Error
	})
	if err != nil {
		return ProductFileRecord{}, err
	}
	record.CreatedAt = now
	return record, nil
}

func (r *Repository) ProductFile(ctx context.Context, organizationID, fileID string) (ProductFileRecord, error) {
	var record ProductFileRecord
	result := r.db.WithContext(ctx).Table("product_files").
		Select("id,product_id,storage_driver,object_key,original_name,content_type,size_bytes,sha256,sort_order,created_at").
		Where("organization_id=? AND id=?", organizationID, fileID).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return ProductFileRecord{}, ErrProductUnavailable
	}
	return record, result.Error
}

func (r *Repository) ProductFiles(ctx context.Context, organizationID, productID string) ([]ProductFileRecord, error) {
	var records []ProductFileRecord
	err := r.db.WithContext(ctx).Table("product_files").
		Select("id,product_id,storage_driver,object_key,original_name,content_type,size_bytes,sha256,sort_order,created_at").
		Where("organization_id=? AND product_id=?", organizationID, productID).
		Order("sort_order,id").Scan(&records).Error
	return records, err
}
