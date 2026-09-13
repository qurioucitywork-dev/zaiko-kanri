package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrProductFileLimit = errors.New("product file limit reached")
var ErrProductFileOrder = errors.New("product file order is invalid")

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

// DeleteProductFile removes one product-file record and compacts the remaining
// sort order. The object itself is deleted by the storage adapter in the web
// layer so this repository remains independent of local/S3 storage.
func (r *Repository) DeleteProductFile(ctx context.Context, organizationID, fileID string) (ProductFileRecord, error) {
	var deleted ProductFileRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Table("product_files").
			Select("id,product_id,storage_driver,object_key,original_name,content_type,size_bytes,sha256,sort_order,created_at").
			Where("organization_id=? AND id=?", organizationID, fileID).Take(&deleted)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrProductUnavailable
		}
		if result.Error != nil {
			return result.Error
		}
		if err := tx.Exec("DELETE FROM product_files WHERE organization_id=? AND id=?", organizationID, fileID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE product_files SET sort_order=sort_order-1
			WHERE organization_id=? AND product_id=? AND sort_order>?`, organizationID, deleted.ProductID, deleted.SortOrder).Error
	})
	return deleted, err
}

// ReorderProductFiles persists the exact order of all images for a product.
// Requiring the complete current set prevents stale clients from silently
// dropping images while another edit is in progress.
func (r *Repository) ReorderProductFiles(ctx context.Context, organizationID, productID string, fileIDs []string) ([]ProductFileRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current []string
		if err := tx.Table("product_files").Where("organization_id=? AND product_id=?", organizationID, productID).
			Order("sort_order,id").Pluck("id", &current).Error; err != nil {
			return err
		}
		if len(current) != len(fileIDs) {
			return ErrProductFileOrder
		}
		available := make(map[string]struct{}, len(current))
		for _, id := range current {
			available[id] = struct{}{}
		}
		seen := make(map[string]struct{}, len(fileIDs))
		for index, id := range fileIDs {
			if _, ok := available[id]; !ok {
				return ErrProductFileOrder
			}
			if _, duplicate := seen[id]; duplicate {
				return ErrProductFileOrder
			}
			seen[id] = struct{}{}
			if err := tx.Table("product_files").Where("organization_id=? AND product_id=? AND id=?", organizationID, productID, id).
				Update("sort_order", index).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.ProductFiles(ctx, organizationID, productID)
}
