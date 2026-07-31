package d1adapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

func TestAdapterProductMoneyRoundTripAndTenantHeader(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-zaiko-tenant-id"); got != "tenant-a" {
			t.Fatalf("tenant header = %q", got)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"product":{
			"id":"product-1","organization_id":"tenant-a","product_code":"20260731001",
			"sku":"SKU-1","brand":"Brand","model_number":"Model","serial_number":"Serial",
			"product_type":"watch","supplier_id":"supplier-1","supplier_name":"Supplier",
			"buyer_id":"","buyer_name":"","purchase_date":"2026-07-31",
			"cost_amount_minor":"9223372036854775807","cost_currency":"JPY",
			"base_sale_price_minor":"9007199254740993","base_sale_currency":"USD",
			"inventory_status":"in_stock","publication_status":"private",
			"condition_text":"A","accessories":"BOX","material_text":"SS","box_text":"BOX1",
			"movement_text":"automatic","belt_material_text":"SS","dial_text":"black",
			"features_text":"","image_count":1,"created_at":"2026-07-31T00:00:00Z"
		}}`))
	}))
	defer server.Close()

	adapter, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	product, err := adapter.GetProduct(context.Background(), "tenant-a", "product-1")
	if err != nil {
		t.Fatal(err)
	}
	if product.Cost.AmountMinor != int64(9223372036854775807) {
		t.Fatalf("cost = %d", product.Cost.AmountMinor)
	}
	if product.BaseSalePrice.AmountMinor != int64(9007199254740993) {
		t.Fatalf("sale = %d", product.BaseSalePrice.AmountMinor)
	}
}

func TestAdapterErrorMappingAndNoBodyLeak(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "binding=DB secret=do-not-expose", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	adapter, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.GetProduct(context.Background(), "tenant-a", "product-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "binding") {
		t.Fatalf("provider response leaked: %v", err)
	}
}

func TestAdapterNotFoundAndInvalidArgument(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	adapter, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetProduct(context.Background(), "tenant-a", "missing"); !errors.Is(err, dataaccess.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
	if _, err := adapter.GetProduct(context.Background(), "", "missing"); !errors.Is(err, dataaccess.ErrInvalidArgument) {
		t.Fatalf("error = %v", err)
	}
}
