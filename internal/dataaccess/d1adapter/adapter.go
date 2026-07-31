// Package d1adapter implements provider-neutral read contracts over the
// application-specific internal D1 Worker API. It never accepts SQL from a
// caller and does not fall back to SQLite.
package d1adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

const defaultTimeout = 10 * time.Second

// Adapter is safe for concurrent use when the supplied HTTP client is safe
// for concurrent use.
type Adapter struct {
	baseURL *url.URL
	client  *http.Client
}

// New constructs a D1 adapter. In Cloudflare Containers, baseURL is the
// outbound-handler virtual host http://d1.internal. Tests may use an
// httptest.Server URL.
func New(baseURL string, client *http.Client) (*Adapter, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: invalid D1 service URL", dataaccess.ErrInvalidArgument)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: invalid D1 service URL scheme", dataaccess.ErrInvalidArgument)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	return &Adapter{baseURL: parsed, client: client}, nil
}

func (a *Adapter) SearchProducts(
	ctx context.Context,
	tenantID string,
	search dataaccess.ProductSearch,
) (dataaccess.ProductPage, error) {
	if strings.TrimSpace(tenantID) == "" || search.Page < 1 || search.PageSize < 1 {
		return dataaccess.ProductPage{}, dataaccess.ErrInvalidArgument
	}

	query := make(url.Values)
	query.Set("page", strconv.Itoa(search.Page))
	query.Set("page_size", strconv.Itoa(search.PageSize))
	addOptional(query, "query", search.Query)
	addOptional(query, "brand", search.Brand)
	addOptional(query, "supplier_id", search.SupplierID)
	addOptional(query, "inventory_status", search.InventoryStatus)
	addOptional(query, "purchase_date_from", search.PurchaseDateFrom)
	addOptional(query, "purchase_date_to", search.PurchaseDateTo)

	var response productPageResponse
	if err := a.get(ctx, tenantID, "/internal/v1/products", query, &response); err != nil {
		return dataaccess.ProductPage{}, err
	}
	items := make([]dataaccess.Product, 0, len(response.Items))
	for _, item := range response.Items {
		product, err := item.product()
		if err != nil {
			return dataaccess.ProductPage{}, err
		}
		items = append(items, product)
	}
	return dataaccess.ProductPage{
		Items:      items,
		Total:      response.Total,
		Page:       response.Page,
		PageSize:   response.PageSize,
		TotalPages: response.TotalPages,
	}, nil
}

func (a *Adapter) GetProduct(
	ctx context.Context,
	tenantID string,
	productID string,
) (dataaccess.Product, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(productID) == "" {
		return dataaccess.Product{}, dataaccess.ErrInvalidArgument
	}
	var response struct {
		Product productDTO `json:"product"`
	}
	path := "/internal/v1/products/" + url.PathEscape(productID)
	if err := a.get(ctx, tenantID, path, nil, &response); err != nil {
		return dataaccess.Product{}, err
	}
	return response.Product.product()
}

func (a *Adapter) ListProductObjects(
	ctx context.Context,
	tenantID string,
	productID string,
) ([]dataaccess.ObjectMetadata, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(productID) == "" {
		return nil, dataaccess.ErrInvalidArgument
	}
	var response struct {
		Objects []objectDTO `json:"objects"`
	}
	path := "/internal/v1/products/" + url.PathEscape(productID) + "/objects"
	if err := a.get(ctx, tenantID, path, nil, &response); err != nil {
		return nil, err
	}
	objects := make([]dataaccess.ObjectMetadata, 0, len(response.Objects))
	for _, item := range response.Objects {
		object, err := item.object()
		if err != nil {
			return nil, err
		}
		objects = append(objects, object)
	}
	return objects, nil
}

func (a *Adapter) GetObjectMetadata(
	ctx context.Context,
	tenantID string,
	objectID string,
) (dataaccess.ObjectMetadata, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(objectID) == "" {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrInvalidArgument
	}
	var response struct {
		Object objectDTO `json:"object"`
	}
	path := "/internal/v1/objects/" + url.PathEscape(objectID)
	if err := a.get(ctx, tenantID, path, nil, &response); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	return response.Object.object()
}

func (a *Adapter) Diagnose(ctx context.Context) (dataaccess.DiagnosticReport, error) {
	started := time.Now()
	var response struct {
		Provider string `json:"provider"`
		Status   string `json:"status"`
	}
	if err := a.get(ctx, "", "/internal/v1/diagnostics", nil, &response); err != nil {
		return dataaccess.DiagnosticReport{}, err
	}
	status := dataaccess.DiagnosticFailed
	if response.Status == "ok" {
		status = dataaccess.DiagnosticOK
	}
	checkedAt := time.Now().UTC()
	return dataaccess.DiagnosticReport{
		Provider:  "d1",
		CheckedAt: checkedAt,
		Components: []dataaccess.ComponentDiagnostic{{
			Name:      "database",
			Status:    status,
			Message:   "read-only D1 connectivity check",
			Latency:   time.Since(started),
			CheckedAt: checkedAt,
		}},
	}, nil
}

func (a *Adapter) get(
	ctx context.Context,
	tenantID string,
	path string,
	query url.Values,
	destination any,
) error {
	endpoint := *a.baseURL
	endpoint.Path += path
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("d1adapter: create request: %w", err)
	}
	request.Header.Set("accept", "application/json")
	if tenantID != "" {
		request.Header.Set("x-zaiko-tenant-id", tenantID)
	}
	response, err := a.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("d1adapter: request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, response.Body)
		return dataaccess.ErrNotFound
	}
	if response.StatusCode == http.StatusBadRequest {
		_, _ = io.Copy(io.Discard, response.Body)
		return dataaccess.ErrInvalidArgument
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return fmt.Errorf("d1adapter: service status %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("d1adapter: decode response: %w", err)
	}
	return nil
}

func addOptional(values url.Values, key, value string) {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		values.Set(key, trimmed)
	}
}

func parseInt64(raw string, field string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("d1adapter: invalid %s: %w", field, err)
	}
	return value, nil
}

func parseOptionalTime(raw string, field string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("d1adapter: invalid %s: %w", field, err)
	}
	return value, nil
}

var (
	_ dataaccess.DiagnosticReader     = (*Adapter)(nil)
	_ dataaccess.ProductReader        = (*Adapter)(nil)
	_ dataaccess.ObjectMetadataReader = (*Adapter)(nil)
)
