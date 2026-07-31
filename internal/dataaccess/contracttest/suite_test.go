package contracttest

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

// memoryHarness is not a production adapter. It only proves the reusable
// contract suite itself is executable without changing the current Store.
type memoryHarness struct {
	mu       sync.RWMutex
	products []dataaccess.Product
	objects  []dataaccess.ObjectMetadata
}

func newMemoryHarness(*testing.T) Harness { return &memoryHarness{} }

func (h *memoryHarness) Seed(ctx context.Context, fixture Fixture) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.products = append([]dataaccess.Product(nil), fixture.Products...)
	h.objects = append([]dataaccess.ObjectMetadata(nil), fixture.Objects...)
	return nil
}

func (h *memoryHarness) ProductReader() dataaccess.ProductReader { return h }
func (h *memoryHarness) ObjectMetadataReader() dataaccess.ObjectMetadataReader {
	return h
}
func (h *memoryHarness) DiagnosticReader() dataaccess.DiagnosticReader { return h }
func (h *memoryHarness) ForbiddenDiagnosticFragments() []string {
	return []string{
		"contract-secret-value",
		"postgres://contract-user:contract-password@database.example/contract",
		"contract-d1-binding",
		"contract-r2-bucket",
		"private/products/product-alpha-1/image.jpg",
	}
}
func (h *memoryHarness) Cleanup() {}

func (h *memoryHarness) SearchProducts(ctx context.Context, tenantID string, search dataaccess.ProductSearch) (dataaccess.ProductPage, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.ProductPage{}, err
	}
	if tenantID == "" || search.Page < 1 || search.PageSize < 1 {
		return dataaccess.ProductPage{}, dataaccess.ErrInvalidArgument
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	var matched []dataaccess.Product
	query := strings.ToLower(strings.TrimSpace(search.Query))
	for _, product := range h.products {
		if product.TenantID != tenantID {
			continue
		}
		if search.InventoryStatus != "" && product.InventoryStatus != search.InventoryStatus {
			continue
		}
		if search.Brand != "" && product.Brand != search.Brand {
			continue
		}
		if search.SupplierID != "" && product.SupplierID != search.SupplierID {
			continue
		}
		if search.PurchaseDateFrom != "" && product.PurchaseDate < search.PurchaseDateFrom {
			continue
		}
		if search.PurchaseDateTo != "" && product.PurchaseDate > search.PurchaseDateTo {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			product.Code, product.SKU, product.Brand, product.ModelNumber, product.SerialNumber,
		}, " "))
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		matched = append(matched, product)
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].PurchaseDate == matched[j].PurchaseDate {
			return matched[i].Code < matched[j].Code
		}
		return matched[i].PurchaseDate > matched[j].PurchaseDate
	})
	total := len(matched)
	totalPages := (total + search.PageSize - 1) / search.PageSize
	start := (search.Page - 1) * search.PageSize
	if start > total {
		start = total
	}
	end := start + search.PageSize
	if end > total {
		end = total
	}
	return dataaccess.ProductPage{
		Items: append([]dataaccess.Product(nil), matched[start:end]...),
		Total: total, Page: search.Page, PageSize: search.PageSize, TotalPages: totalPages,
	}, nil
}

func (h *memoryHarness) GetProduct(ctx context.Context, tenantID, productID string) (dataaccess.Product, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.Product{}, err
	}
	if tenantID == "" || productID == "" {
		return dataaccess.Product{}, dataaccess.ErrInvalidArgument
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, product := range h.products {
		if product.TenantID == tenantID && product.ID == productID {
			return product, nil
		}
	}
	return dataaccess.Product{}, dataaccess.ErrNotFound
}

func (h *memoryHarness) ListProductObjects(ctx context.Context, tenantID, productID string) ([]dataaccess.ObjectMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if tenantID == "" || productID == "" {
		return nil, dataaccess.ErrInvalidArgument
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	var matched []dataaccess.ObjectMetadata
	for _, object := range h.objects {
		if object.TenantID == tenantID && object.ProductID == productID {
			matched = append(matched, object)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].SortOrder == matched[j].SortOrder {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].SortOrder < matched[j].SortOrder
	})
	return matched, nil
}

func (h *memoryHarness) GetObjectMetadata(ctx context.Context, tenantID, objectID string) (dataaccess.ObjectMetadata, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	if tenantID == "" || objectID == "" {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrInvalidArgument
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, object := range h.objects {
		if object.TenantID == tenantID && object.ID == objectID {
			return object, nil
		}
	}
	return dataaccess.ObjectMetadata{}, dataaccess.ErrNotFound
}

func (h *memoryHarness) Diagnose(ctx context.Context) (dataaccess.DiagnosticReport, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.DiagnosticReport{}, err
	}
	now := time.Now().UTC()
	return dataaccess.DiagnosticReport{
		Provider:  "memory-contract-fixture",
		CheckedAt: now,
		Components: []dataaccess.ComponentDiagnostic{{
			Name: "memory", Status: dataaccess.DiagnosticOK, CheckedAt: now,
		}},
	}, nil
}

func TestMemoryHarnessProductReaderContract(t *testing.T) {
	RunProductReaderContract(t, newMemoryHarness)
}

func TestMemoryHarnessObjectMetadataReaderContract(t *testing.T) {
	RunObjectMetadataReaderContract(t, newMemoryHarness)
}

func TestMemoryHarnessDiagnosticReaderContract(t *testing.T) {
	RunDiagnosticReaderContract(t, newMemoryHarness)
}
