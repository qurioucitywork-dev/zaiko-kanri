package dataaccess

import "time"

// Money stores an amount in the currency's smallest unit. AmountMinor must
// never be converted through a floating-point representation.
type Money struct {
	AmountMinor int64
	Currency    string
}

// Product is the provider-independent read model used by product lookup and
// search. TenantID corresponds to organization_id in the current SQLite
// schema.
type Product struct {
	ID                string
	TenantID          string
	Code              string
	SKU               string
	Brand             string
	ModelNumber       string
	SerialNumber      string
	ProductType       string
	SupplierID        string
	SupplierName      string
	BuyerID           string
	BuyerName         string
	PurchaseDate      string
	Cost              Money
	BaseSalePrice     Money
	InventoryStatus   string
	PublicationStatus string
	Condition         string
	Accessories       string
	Material          string
	Box               string
	Movement          string
	BeltMaterial      string
	Dial              string
	Features          string
	ImageCount        int
	CreatedAt         time.Time
}

// ProductSearch contains only criteria shared by the current Store and likely
// remote providers. Page and PageSize are one-based and positive.
type ProductSearch struct {
	Query            string
	Brand            string
	SupplierID       string
	InventoryStatus  string
	PurchaseDateFrom string
	PurchaseDateTo   string
	Page             int
	PageSize         int
}

// ProductPage is a stable page of tenant-scoped product results.
type ProductPage struct {
	Items      []Product
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

// ObjectStatus represents the lifecycle of object metadata. A legacy
// filesystem adapter can expose existing complete images as ObjectReady.
type ObjectStatus string

const (
	ObjectPending ObjectStatus = "pending"
	ObjectReady   ObjectStatus = "ready"
	ObjectFailed  ObjectStatus = "failed"
	ObjectDeleted ObjectStatus = "deleted"
)

// ObjectMetadata describes an object without exposing a provider locator.
// Provider, bucket, object key and provider version remain private to each
// adapter. Callers use the opaque ID when a future ObjectStore port needs to
// resolve object bytes.
type ObjectMetadata struct {
	ID             string
	TenantID       string
	ProductID      string
	ChecksumSHA256 string
	OriginalName   string
	ContentType    string
	SizeBytes      int64
	SortOrder      int
	Status         ObjectStatus
	CreatedAt      time.Time
	ReadyAt        time.Time
	DeletedAt      time.Time
}

// DiagnosticStatus is deliberately provider-neutral.
type DiagnosticStatus string

const (
	DiagnosticOK       DiagnosticStatus = "ok"
	DiagnosticDegraded DiagnosticStatus = "degraded"
	DiagnosticFailed   DiagnosticStatus = "failed"
)

type ComponentDiagnostic struct {
	Name      string
	Status    DiagnosticStatus
	Message   string
	Latency   time.Duration
	CheckedAt time.Time
}

// DiagnosticReport contains read-only connectivity and capability results.
// It must not expose credentials, DSNs, binding identifiers, or signed URLs.
type DiagnosticReport struct {
	Provider   string
	CheckedAt  time.Time
	Components []ComponentDiagnostic
}
