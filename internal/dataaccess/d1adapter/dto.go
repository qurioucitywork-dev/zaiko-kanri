package d1adapter

import (
	"fmt"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

type productPageResponse struct {
	Items      []productDTO `json:"items"`
	Total      int          `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"page_size"`
	TotalPages int          `json:"total_pages"`
}

type productDTO struct {
	ID                 string `json:"id"`
	OrganizationID     string `json:"organization_id"`
	ProductCode        string `json:"product_code"`
	SKU                string `json:"sku"`
	Brand              string `json:"brand"`
	ModelNumber        string `json:"model_number"`
	SerialNumber       string `json:"serial_number"`
	ProductType        string `json:"product_type"`
	SupplierID         string `json:"supplier_id"`
	SupplierName       string `json:"supplier_name"`
	BuyerID            string `json:"buyer_id"`
	BuyerName          string `json:"buyer_name"`
	PurchaseDate       string `json:"purchase_date"`
	CostAmountMinor    string `json:"cost_amount_minor"`
	CostCurrency       string `json:"cost_currency"`
	BaseSalePriceMinor string `json:"base_sale_price_minor"`
	BaseSaleCurrency   string `json:"base_sale_currency"`
	InventoryStatus    string `json:"inventory_status"`
	PublicationStatus  string `json:"publication_status"`
	ConditionText      string `json:"condition_text"`
	Accessories        string `json:"accessories"`
	MaterialText       string `json:"material_text"`
	BoxText            string `json:"box_text"`
	MovementText       string `json:"movement_text"`
	BeltMaterialText   string `json:"belt_material_text"`
	DialText           string `json:"dial_text"`
	FeaturesText       string `json:"features_text"`
	ImageCount         int    `json:"image_count"`
	CreatedAt          string `json:"created_at"`
}

func (dto productDTO) product() (dataaccess.Product, error) {
	cost, err := parseInt64(dto.CostAmountMinor, "cost_amount_minor")
	if err != nil {
		return dataaccess.Product{}, err
	}
	baseSalePrice, err := parseInt64(dto.BaseSalePriceMinor, "base_sale_price_minor")
	if err != nil {
		return dataaccess.Product{}, err
	}
	createdAt, err := parseOptionalTime(dto.CreatedAt, "created_at")
	if err != nil {
		return dataaccess.Product{}, err
	}
	return dataaccess.Product{
		ID:                dto.ID,
		TenantID:          dto.OrganizationID,
		Code:              dto.ProductCode,
		SKU:               dto.SKU,
		Brand:             dto.Brand,
		ModelNumber:       dto.ModelNumber,
		SerialNumber:      dto.SerialNumber,
		ProductType:       dto.ProductType,
		SupplierID:        dto.SupplierID,
		SupplierName:      dto.SupplierName,
		BuyerID:           dto.BuyerID,
		BuyerName:         dto.BuyerName,
		PurchaseDate:      dto.PurchaseDate,
		Cost:              dataaccess.Money{AmountMinor: cost, Currency: dto.CostCurrency},
		BaseSalePrice:     dataaccess.Money{AmountMinor: baseSalePrice, Currency: dto.BaseSaleCurrency},
		InventoryStatus:   dto.InventoryStatus,
		PublicationStatus: dto.PublicationStatus,
		Condition:         dto.ConditionText,
		Accessories:       dto.Accessories,
		Material:          dto.MaterialText,
		Box:               dto.BoxText,
		Movement:          dto.MovementText,
		BeltMaterial:      dto.BeltMaterialText,
		Dial:              dto.DialText,
		Features:          dto.FeaturesText,
		ImageCount:        dto.ImageCount,
		CreatedAt:         createdAt,
	}, nil
}

type objectDTO struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	ProductID      string `json:"product_id"`
	ChecksumSHA256 string `json:"checksum_sha256"`
	OriginalName   string `json:"original_name"`
	ContentType    string `json:"content_type"`
	SizeBytes      string `json:"size_bytes"`
	SortOrder      int    `json:"sort_order"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	ReadyAt        string `json:"ready_at"`
	DeletedAt      string `json:"deleted_at"`
}

func (dto objectDTO) object() (dataaccess.ObjectMetadata, error) {
	sizeBytes, err := parseInt64(dto.SizeBytes, "size_bytes")
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	createdAt, err := parseOptionalTime(dto.CreatedAt, "created_at")
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	readyAt, err := parseOptionalTime(dto.ReadyAt, "ready_at")
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	deletedAt, err := parseOptionalTime(dto.DeletedAt, "deleted_at")
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	status := dataaccess.ObjectStatus(dto.Status)
	switch status {
	case dataaccess.ObjectPending, dataaccess.ObjectReady, dataaccess.ObjectFailed, dataaccess.ObjectDeleted:
	default:
		return dataaccess.ObjectMetadata{}, fmt.Errorf("d1adapter: invalid object status %q", dto.Status)
	}
	return dataaccess.ObjectMetadata{
		ID:             dto.ID,
		TenantID:       dto.OrganizationID,
		ProductID:      dto.ProductID,
		ChecksumSHA256: dto.ChecksumSHA256,
		OriginalName:   dto.OriginalName,
		ContentType:    dto.ContentType,
		SizeBytes:      sizeBytes,
		SortOrder:      dto.SortOrder,
		Status:         status,
		CreatedAt:      createdAt,
		ReadyAt:        readyAt,
		DeletedAt:      deletedAt,
	}, nil
}
