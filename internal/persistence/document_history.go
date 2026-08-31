package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var ErrDocumentEventInvalid = errors.New("invalid document generation event")

type DocumentEventInput struct {
	DocumentType   string          `json:"documentType"`
	DocumentID     string          `json:"documentId"`
	DocumentNumber string          `json:"documentNumber"`
	Action         string          `json:"action"`
	OutputFormat   string          `json:"outputFormat"`
	FileName       string          `json:"fileName"`
	StorageDriver  string          `json:"storageDriver"`
	ObjectKey      string          `json:"objectKey"`
	Metadata       json.RawMessage `json:"metadata"`
}

// RawJSON accepts both PostgreSQL JSONB byte values and SQLite TEXT values.
// Keeping the transport shape identical lets local/test storage and RDS use the
// same repository contract.
type RawJSON json.RawMessage

func (value *RawJSON) Scan(source any) error {
	var raw []byte
	switch source := source.(type) {
	case nil:
		raw = []byte(`{}`)
	case []byte:
		raw = source
	case string:
		raw = []byte(source)
	default:
		return ErrDocumentEventInvalid
	}
	if !json.Valid(raw) {
		return ErrDocumentEventInvalid
	}
	*value = append((*value)[:0], raw...)
	return nil
}

func (value RawJSON) MarshalJSON() ([]byte, error) {
	if len(value) == 0 {
		return []byte(`{}`), nil
	}
	if !json.Valid(value) {
		return nil, ErrDocumentEventInvalid
	}
	return value, nil
}

type DocumentEventRecord struct {
	ID             string    `json:"id"`
	DocumentType   string    `json:"documentType"`
	DocumentID     string    `json:"documentId"`
	DocumentNumber string    `json:"documentNumber"`
	Action         string    `json:"action"`
	OutputFormat   string    `json:"outputFormat"`
	FileName       string    `json:"fileName"`
	StorageDriver  string    `json:"storageDriver"`
	ObjectKey      string    `json:"objectKey"`
	Metadata       RawJSON   `gorm:"column:metadata_json" json:"metadata"`
	CreatedBy      string    `json:"createdBy"`
	CreatedByName  string    `json:"createdByName"`
	CreatedAt      time.Time `json:"createdAt"`
}

// OfficialDocumentRef points to an immutable PDF stored at issuance time.
// The URL is API-relative so it remains valid when local storage is replaced
// by S3 without exposing the underlying object key.
type OfficialDocumentRef struct {
	EventID     string    `json:"eventId"`
	Version     int       `json:"version"`
	FileName    string    `json:"fileName"`
	DownloadURL string    `json:"downloadUrl"`
	SHA256      string    `json:"sha256"`
	SizeBytes   int64     `json:"sizeBytes"`
	IssuedAt    time.Time `json:"issuedAt"`
}

func normalizeDocumentEvent(input DocumentEventInput) (DocumentEventInput, error) {
	input.DocumentType = strings.ToLower(strings.TrimSpace(input.DocumentType))
	input.DocumentID = strings.TrimSpace(input.DocumentID)
	input.DocumentNumber = strings.TrimSpace(input.DocumentNumber)
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	input.OutputFormat = strings.ToLower(strings.TrimSpace(input.OutputFormat))
	input.FileName = strings.TrimSpace(input.FileName)
	input.StorageDriver = strings.ToLower(strings.TrimSpace(input.StorageDriver))
	input.ObjectKey = strings.TrimSpace(input.ObjectKey)
	validTypes := map[string]bool{
		"purchase": true, "sale": true, "shipment": true, "return": true,
		"purchase_return": true, "inventory": true, "market": true, "documents": true, "stocktake": true,
		"consignment": true,
	}
	validActions := map[string]bool{"preview": true, "print": true, "download": true}
	validFormats := map[string]bool{"html": true, "pdf": true, "csv": true}
	if !validTypes[input.DocumentType] || !validActions[input.Action] || !validFormats[input.OutputFormat] ||
		len(input.DocumentNumber) > 100 || len(input.FileName) > 255 || len(input.ObjectKey) > 1000 {
		return DocumentEventInput{}, ErrDocumentEventInvalid
	}
	if input.StorageDriver == "" {
		input.StorageDriver = "local"
	}
	if input.StorageDriver != "local" && input.StorageDriver != "s3" {
		return DocumentEventInput{}, ErrDocumentEventInvalid
	}
	if len(input.Metadata) == 0 || !json.Valid(input.Metadata) {
		input.Metadata = json.RawMessage(`{}`)
	}
	return input, nil
}

func (r *Repository) RecordDocumentEvent(ctx context.Context, organizationID, actorUserID string, input DocumentEventInput) (DocumentEventRecord, error) {
	input, err := normalizeDocumentEvent(input)
	if err != nil {
		return DocumentEventRecord{}, err
	}
	id, err := database.NewID("dhe")
	if err != nil {
		return DocumentEventRecord{}, err
	}
	now := time.Now().UTC()
	query := `INSERT INTO document_generation_events(
		id,organization_id,document_type,document_id,document_number,action,output_format,file_name,
		storage_driver,object_key,metadata_json,created_by,created_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,CAST(? AS JSONB),?,?)`
	if r.driver == "sqlite" {
		query = strings.Replace(query, "CAST(? AS JSONB)", "?", 1)
	}
	if err := r.db.WithContext(ctx).Exec(query, id, organizationID, input.DocumentType, input.DocumentID,
		input.DocumentNumber, input.Action, input.OutputFormat, input.FileName, input.StorageDriver,
		input.ObjectKey, string(input.Metadata), actorUserID, now).Error; err != nil {
		return DocumentEventRecord{}, err
	}
	return r.DocumentEvent(ctx, organizationID, id)
}

func (r *Repository) DocumentEvent(ctx context.Context, organizationID, id string) (DocumentEventRecord, error) {
	var record DocumentEventRecord
	result := r.db.WithContext(ctx).Table("document_generation_events AS e").
		Select(`e.id,e.document_type,e.document_id,e.document_number,e.action,e.output_format,e.file_name,
			e.storage_driver,e.object_key,e.metadata_json,e.created_by,u.display_name AS created_by_name,e.created_at`).
		Joins("JOIN users u ON u.id=e.created_by").Where("e.organization_id=? AND e.id=?", organizationID, id).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return DocumentEventRecord{}, ErrDocumentEventInvalid
	}
	return record, result.Error
}

func (r *Repository) DocumentEvents(ctx context.Context, organizationID, documentType, documentID string, limit int) ([]DocumentEventRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	query := r.db.WithContext(ctx).Table("document_generation_events AS e").
		Select(`e.id,e.document_type,e.document_id,e.document_number,e.action,e.output_format,e.file_name,
			e.storage_driver,e.object_key,e.metadata_json,e.created_by,u.display_name AS created_by_name,e.created_at`).
		Joins("JOIN users u ON u.id=e.created_by").Where("e.organization_id=?", organizationID)
	if value := strings.TrimSpace(documentType); value != "" {
		query = query.Where("e.document_type=?", strings.ToLower(value))
	}
	if value := strings.TrimSpace(documentID); value != "" {
		query = query.Where("e.document_id=?", value)
	}
	var records []DocumentEventRecord
	err := query.Order("e.created_at DESC,e.id DESC").Limit(limit).Scan(&records).Error
	return records, err
}
