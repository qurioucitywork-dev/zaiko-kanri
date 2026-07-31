// Package contracttest contains reusable behavior tests for dataaccess port
// implementations. Adapter packages provide a Harness factory and run these
// suites against SQLite, D1, AWS RDS for PostgreSQL, a filesystem, R2, or S3.
package contracttest

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

const (
	tenantAlpha = "contract-tenant-alpha"
	tenantBeta  = "contract-tenant-beta"
)

// Harness owns an isolated adapter fixture. Seed is called before Reader.
// Cleanup must be safe after partial setup.
type Harness interface {
	Seed(ctx context.Context, fixture Fixture) error
	ProductReader() dataaccess.ProductReader
	ObjectMetadataReader() dataaccess.ObjectMetadataReader
	DiagnosticReader() dataaccess.DiagnosticReader
	ForbiddenDiagnosticFragments() []string
	Cleanup()
}

// NewHarness creates one isolated fixture per contract test.
type NewHarness func(t *testing.T) Harness

type Fixture struct {
	Products []dataaccess.Product
	Objects  []dataaccess.ObjectMetadata
}

// StandardFixture includes two tenants and is suitable for verifying that
// adapters do not leak existence or rows across tenant boundaries.
func StandardFixture() Fixture {
	now := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	return Fixture{
		Products: []dataaccess.Product{
			{
				ID: "product-alpha-1", TenantID: tenantAlpha, Code: "20260301001",
				SKU: "SKU-ALPHA", Brand: "Rolex", ModelNumber: "Submariner",
				SupplierID:      "supplier-alpha",
				InventoryStatus: "in_stock", PurchaseDate: "2026-03-01",
				Cost:          dataaccess.Money{AmountMinor: 850000, Currency: "JPY"},
				BaseSalePrice: dataaccess.Money{AmountMinor: 1180000, Currency: "JPY"},
				CreatedAt:     now,
			},
			{
				ID: "product-alpha-2", TenantID: tenantAlpha, Code: "20260303001",
				SKU: "SKU-OMEGA", Brand: "Omega", ModelNumber: "Speedmaster",
				SupplierID:      "supplier-beta",
				InventoryStatus: "sold", PurchaseDate: "2026-03-03",
				Cost:          dataaccess.Money{AmountMinor: math.MaxInt64 - 1, Currency: "USD"},
				BaseSalePrice: dataaccess.Money{AmountMinor: math.MaxInt64, Currency: "USD"},
				CreatedAt:     now.Add(time.Second),
			},
			{
				ID: "product-alpha-3", TenantID: tenantAlpha, Code: "20260303002",
				SKU: "SKU-BREITLING", Brand: "Breitling", ModelNumber: "Navitimer",
				SupplierID:      "supplier-alpha",
				InventoryStatus: "in_stock", PurchaseDate: "2026-03-03",
				Cost:          dataaccess.Money{AmountMinor: 420000, Currency: "JPY"},
				BaseSalePrice: dataaccess.Money{AmountMinor: 610000, Currency: "JPY"},
				CreatedAt:     now.Add(2 * time.Second),
			},
			{
				ID: "product-beta-1", TenantID: tenantBeta, Code: "20260301001",
				SKU: "SKU-ALPHA", Brand: "Rolex", ModelNumber: "Submariner",
				InventoryStatus: "in_stock", PurchaseDate: "2026-03-01",
				Cost:          dataaccess.Money{AmountMinor: 1, Currency: "JPY"},
				BaseSalePrice: dataaccess.Money{AmountMinor: 2, Currency: "JPY"},
				CreatedAt:     now,
			},
		},
		Objects: []dataaccess.ObjectMetadata{
			{
				ID: "object-alpha-2", TenantID: tenantAlpha, ProductID: "product-alpha-1",
				OriginalName: "second.jpg",
				ContentType:  "image/jpeg", SizeBytes: 20, SortOrder: 2,
				Status: dataaccess.ObjectReady, CreatedAt: now.Add(time.Second),
			},
			{
				ID: "object-alpha-1", TenantID: tenantAlpha, ProductID: "product-alpha-1",
				OriginalName: "first.jpg",
				ContentType:  "image/jpeg", SizeBytes: 10, SortOrder: 1,
				Status: dataaccess.ObjectReady, CreatedAt: now,
			},
			{
				ID: "object-beta-1", TenantID: tenantBeta, ProductID: "product-beta-1",
				OriginalName: "first.jpg",
				ContentType:  "image/jpeg", SizeBytes: 10, SortOrder: 1,
				Status: dataaccess.ObjectReady, CreatedAt: now,
			},
		},
	}
}

func seededHarness(t *testing.T, factory NewHarness) Harness {
	t.Helper()
	harness := factory(t)
	if harness == nil {
		t.Fatal("contracttest: factory returned a nil harness")
	}
	t.Cleanup(harness.Cleanup)
	if err := harness.Seed(context.Background(), StandardFixture()); err != nil {
		t.Fatalf("contracttest: seed fixture: %v", err)
	}
	return harness
}

// RunProductReaderContract verifies the minimum cross-provider behavior.
func RunProductReaderContract(t *testing.T, factory NewHarness) {
	t.Helper()

	t.Run("rejects empty tenant", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		if _, err := reader.SearchProducts(context.Background(), "", dataaccess.ProductSearch{Page: 1, PageSize: 20}); !errors.Is(err, dataaccess.ErrInvalidArgument) {
			t.Fatalf("SearchProducts error = %v, want ErrInvalidArgument", err)
		}
		if _, err := reader.GetProduct(context.Background(), "", "product-alpha-1"); !errors.Is(err, dataaccess.ErrInvalidArgument) {
			t.Fatalf("GetProduct error = %v, want ErrInvalidArgument", err)
		}
	})

	t.Run("scopes search to tenant", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		page, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{Page: 1, PageSize: 20})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 3 || len(page.Items) != 3 {
			t.Fatalf("result count = total %d/items %d, want 3/3", page.Total, len(page.Items))
		}
		for _, product := range page.Items {
			if product.TenantID != tenantAlpha {
				t.Fatalf("search leaked tenant %q", product.TenantID)
			}
		}
	})

	t.Run("applies shared filters and pagination", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		page, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{
			Query: "submariner", InventoryStatus: "in_stock", Page: 1, PageSize: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "product-alpha-1" {
			t.Fatalf("filtered page = %#v", page)
		}
		if page.Page != 1 || page.PageSize != 1 || page.TotalPages != 1 {
			t.Fatalf("page metadata = page %d/size %d/pages %d", page.Page, page.PageSize, page.TotalPages)
		}
	})

	t.Run("preserves integer money and currency", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		jpy, err := reader.GetProduct(context.Background(), tenantAlpha, "product-alpha-1")
		if err != nil {
			t.Fatal(err)
		}
		if jpy.Cost.AmountMinor != 850000 || jpy.Cost.Currency != "JPY" ||
			jpy.BaseSalePrice.AmountMinor != 1180000 || jpy.BaseSalePrice.Currency != "JPY" {
			t.Fatalf("JPY money changed: cost=%#v sale=%#v", jpy.Cost, jpy.BaseSalePrice)
		}
		usd, err := reader.GetProduct(context.Background(), tenantAlpha, "product-alpha-2")
		if err != nil {
			t.Fatal(err)
		}
		if usd.Cost.AmountMinor != math.MaxInt64-1 || usd.Cost.Currency != "USD" ||
			usd.BaseSalePrice.AmountMinor != math.MaxInt64 || usd.BaseSalePrice.Currency != "USD" {
			t.Fatalf("USD boundary money changed: cost=%#v sale=%#v", usd.Cost, usd.BaseSalePrice)
		}
		page, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{
			Query: "SKU-OMEGA", Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 ||
			page.Items[0].Cost.AmountMinor != math.MaxInt64-1 || page.Items[0].Cost.Currency != "USD" ||
			page.Items[0].BaseSalePrice.AmountMinor != math.MaxInt64 || page.Items[0].BaseSalePrice.Currency != "USD" {
			t.Fatalf("search money changed: %#v", page.Items)
		}
	})

	t.Run("paginates deterministically", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		first, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{Page: 1, PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		repeated, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{Page: 1, PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		second, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{Page: 2, PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(first.Items) != 1 || len(repeated.Items) != 1 || first.Items[0].ID != repeated.Items[0].ID {
			t.Fatalf("page 1 is not deterministic: first=%#v repeated=%#v", first.Items, repeated.Items)
		}
		third, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{Page: 3, PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		beyond, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{Page: 4, PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		if first.Items[0].ID != "product-alpha-2" ||
			len(second.Items) != 1 || second.Items[0].ID != "product-alpha-3" ||
			len(third.Items) != 1 || third.Items[0].ID != "product-alpha-1" {
			t.Fatalf("standard order differs: first=%#v second=%#v third=%#v", first.Items, second.Items, third.Items)
		}
		if len(beyond.Items) != 0 || beyond.Total != 3 || beyond.TotalPages != 3 {
			t.Fatalf("out-of-range page metadata = %#v", beyond)
		}
	})

	t.Run("applies brand filter independently", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		page, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{
			Brand: "Omega", Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "product-alpha-2" {
			t.Fatalf("brand filter returned %#v", page)
		}
		none, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{
			Brand: "Missing Brand", Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		if none.Total != 0 || len(none.Items) != 0 {
			t.Fatalf("brand mismatch returned %#v", none)
		}
		if none.Page != 1 || none.PageSize != 20 || none.TotalPages != 0 {
			t.Fatalf("empty page metadata = page %d/size %d/pages %d, want 1/20/0",
				none.Page, none.PageSize, none.TotalPages)
		}
	})

	t.Run("applies supplier filter independently", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		page, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{
			SupplierID: "supplier-alpha", Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 2 || len(page.Items) != 2 ||
			page.Items[0].ID != "product-alpha-3" || page.Items[1].ID != "product-alpha-1" {
			t.Fatalf("supplier filter returned %#v", page)
		}
	})

	t.Run("applies inclusive date boundaries independently", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		from, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{
			PurchaseDateFrom: "2026-03-03", Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		if from.Total != 2 || len(from.Items) != 2 {
			t.Fatalf("date-from boundary returned %#v", from)
		}
		to, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{
			PurchaseDateTo: "2026-03-01", Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		if to.Total != 1 || len(to.Items) != 1 || to.Items[0].ID != "product-alpha-1" {
			t.Fatalf("date-to boundary returned %#v", to)
		}
		exact, err := reader.SearchProducts(context.Background(), tenantAlpha, dataaccess.ProductSearch{
			PurchaseDateFrom: "2026-03-03", PurchaseDateTo: "2026-03-03",
			Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		if exact.Total != 2 || len(exact.Items) != 2 {
			t.Fatalf("exact date boundary returned %#v", exact)
		}
	})

	t.Run("does not disclose cross tenant product", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		if _, err := reader.GetProduct(context.Background(), tenantAlpha, "product-beta-1"); !errors.Is(err, dataaccess.ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})

	t.Run("honors canceled context", func(t *testing.T) {
		reader := seededHarness(t, factory).ProductReader()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := reader.SearchProducts(ctx, tenantAlpha, dataaccess.ProductSearch{Page: 1, PageSize: 20}); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})
}

// RunObjectMetadataReaderContract verifies metadata isolation and ordering.
func RunObjectMetadataReaderContract(t *testing.T, factory NewHarness) {
	t.Helper()

	t.Run("rejects empty tenant", func(t *testing.T) {
		reader := seededHarness(t, factory).ObjectMetadataReader()
		if _, err := reader.ListProductObjects(context.Background(), "", "product-alpha-1"); !errors.Is(err, dataaccess.ErrInvalidArgument) {
			t.Fatalf("ListProductObjects error = %v, want ErrInvalidArgument", err)
		}
		if _, err := reader.GetObjectMetadata(context.Background(), "", "object-alpha-1"); !errors.Is(err, dataaccess.ErrInvalidArgument) {
			t.Fatalf("GetObjectMetadata error = %v, want ErrInvalidArgument", err)
		}
	})

	t.Run("orders product objects and scopes tenant", func(t *testing.T) {
		reader := seededHarness(t, factory).ObjectMetadataReader()
		objects, err := reader.ListProductObjects(context.Background(), tenantAlpha, "product-alpha-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(objects) != 2 || objects[0].ID != "object-alpha-1" || objects[1].ID != "object-alpha-2" {
			t.Fatalf("objects = %#v, want sort_order then id", objects)
		}
		for _, object := range objects {
			if object.TenantID != tenantAlpha {
				t.Fatalf("metadata leaked tenant %q", object.TenantID)
			}
		}
	})

	t.Run("does not disclose cross tenant object", func(t *testing.T) {
		reader := seededHarness(t, factory).ObjectMetadataReader()
		if _, err := reader.GetObjectMetadata(context.Background(), tenantAlpha, "object-beta-1"); !errors.Is(err, dataaccess.ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})

	t.Run("does not disclose cross tenant product objects", func(t *testing.T) {
		reader := seededHarness(t, factory).ObjectMetadataReader()
		crossTenant, crossErr := reader.ListProductObjects(context.Background(), tenantAlpha, "product-beta-1")
		missing, missingErr := reader.ListProductObjects(context.Background(), tenantAlpha, "missing-product")
		switch {
		case crossErr == nil && missingErr == nil:
			if len(crossTenant) != 0 || len(missing) != 0 {
				t.Fatalf("cross-tenant or missing product returned objects: cross=%#v missing=%#v", crossTenant, missing)
			}
		case errors.Is(crossErr, dataaccess.ErrNotFound) && errors.Is(missingErr, dataaccess.ErrNotFound):
			if len(crossTenant) != 0 || len(missing) != 0 {
				t.Fatalf("not-found response returned objects: cross=%#v missing=%#v", crossTenant, missing)
			}
		default:
			t.Fatalf("cross-tenant and missing behavior differs: cross=%v missing=%v", crossErr, missingErr)
		}
	})

	t.Run("honors canceled context", func(t *testing.T) {
		reader := seededHarness(t, factory).ObjectMetadataReader()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := reader.ListProductObjects(ctx, tenantAlpha, "product-alpha-1"); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})
}

// RunDiagnosticReaderContract verifies that diagnostics stay read-only at the
// port boundary and return provider-neutral status. Whether an implementation
// writes is reviewed by its adapter-specific tests.
func RunDiagnosticReaderContract(t *testing.T, factory NewHarness) {
	t.Helper()

	t.Run("reports provider and components", func(t *testing.T) {
		harness := seededHarness(t, factory)
		reader := harness.DiagnosticReader()
		report, err := reader.Diagnose(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if report.Provider == "" || report.CheckedAt.IsZero() || len(report.Components) == 0 {
			t.Fatalf("incomplete diagnostic report: %#v", report)
		}
		allText := []string{report.Provider}
		for _, component := range report.Components {
			if component.Name == "" || component.CheckedAt.IsZero() {
				t.Fatalf("incomplete component diagnostic: %#v", component)
			}
			switch component.Status {
			case dataaccess.DiagnosticOK, dataaccess.DiagnosticDegraded, dataaccess.DiagnosticFailed:
			default:
				t.Fatalf("unsupported diagnostic status %q", component.Status)
			}
			allText = append(allText, component.Name, component.Message)
		}
		diagnosticText := strings.ToLower(strings.Join(allText, "\n"))
		for _, forbidden := range append([]string{
			"password=", "password:", "token=", "token:", "secret=", "secret:",
			"dsn=", "dsn:", "binding=", "binding:", "postgres://",
			"x-amz-signature", "x-amz-credential",
		}, harness.ForbiddenDiagnosticFragments()...) {
			if forbidden != "" && strings.Contains(diagnosticText, strings.ToLower(forbidden)) {
				t.Fatalf("diagnostic report exposes secret or provider locator fragment %q", forbidden)
			}
		}
	})

	t.Run("honors canceled context", func(t *testing.T) {
		reader := seededHarness(t, factory).DiagnosticReader()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := reader.Diagnose(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})
}
