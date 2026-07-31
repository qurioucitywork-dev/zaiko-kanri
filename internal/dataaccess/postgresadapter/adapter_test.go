package postgresadapter

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

func TestNewValidatesProviderPrivateMetadata(t *testing.T) {
	db := openScriptDB(t)
	for name, config := range map[string]Config{
		"empty bucket": {ObjectKeyPrefix: "objects"},
		"bad provider": {StorageProvider: "other", StorageBucket: "test-bucket", ObjectKeyPrefix: "objects"},
		"bad prefix":   {StorageBucket: "test-bucket", ObjectKeyPrefix: "../objects"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(db, config); !errors.Is(err, dataaccess.ErrInvalidArgument) {
				t.Fatalf("New error = %v, want ErrInvalidArgument", err)
			}
		})
	}
	adapter, err := New(db, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if adapter.storageProvider != "s3" {
		t.Fatalf("provider = %q, want s3", adapter.storageProvider)
	}
}

func TestProductWhereUsesTenantFirstAndPostgresPlaceholders(t *testing.T) {
	where, args := productWhere("tenant-a", dataaccess.ProductSearch{
		Query:            `50%_Rolex\`,
		Brand:            "Rolex",
		SupplierID:       "supplier-a",
		InventoryStatus:  "in_stock",
		PurchaseDateFrom: "2026-03-01",
		PurchaseDateTo:   "2026-03-31",
	})
	for _, fragment := range []string{
		"p.organization_id = $1",
		"p.product_code ILIKE $2",
		"p.serial_number ILIKE $6",
		"p.brand = $7",
		"p.supplier_id = $8",
		"p.inventory_status = $9",
		"p.purchase_date >= $10",
		"p.purchase_date <= $11",
	} {
		if !strings.Contains(where, fragment) {
			t.Fatalf("where missing %q:\n%s", fragment, where)
		}
	}
	if args[0] != "tenant-a" {
		t.Fatalf("first argument = %#v, want tenant-a", args[0])
	}
	for index := 1; index <= 5; index++ {
		if args[index] != `%50\%\_Rolex\\%` {
			t.Fatalf("query argument %d = %#v", index, args[index])
		}
	}
}

func TestSearchProductsPreservesBigintUTCAndStableOrdering(t *testing.T) {
	createdAt := time.Date(2026, 7, 31, 11, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	db := openScriptDB(t,
		expectQuery("SELECT COUNT(*)", []any{"tenant-a"}, []string{"count"}, [][]driver.Value{{int64(1)}}),
		expectQuery(
			"ORDER BY p.purchase_date DESC, p.product_code ASC LIMIT $2 OFFSET $3",
			[]any{"tenant-a", int64(20), int64(0)},
			productColumns(),
			[][]driver.Value{productRow(createdAt)},
		),
	)
	adapter := mustAdapter(t, db)
	page, err := adapter.SearchProducts(context.Background(), "tenant-a", dataaccess.ProductSearch{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page = %#v", page)
	}
	product := page.Items[0]
	if product.Cost.AmountMinor != math.MaxInt64-1 ||
		product.BaseSalePrice.AmountMinor != math.MaxInt64 {
		t.Fatalf("money changed: cost=%d sale=%d", product.Cost.AmountMinor, product.BaseSalePrice.AmountMinor)
	}
	if product.PurchaseDate != "2026-03-03" {
		t.Fatalf("purchase date = %q", product.PurchaseDate)
	}
	if product.CreatedAt.Location() != time.UTC ||
		!product.CreatedAt.Equal(createdAt) {
		t.Fatalf("created at = %s (%s)", product.CreatedAt, product.CreatedAt.Location())
	}
}

func TestGetProductCrossTenantIsNotFound(t *testing.T) {
	db := openScriptDB(t,
		expectQuery("p.organization_id = $1", []any{"tenant-a", "product-b"}, productColumns(), nil),
	)
	adapter := mustAdapter(t, db)
	_, err := adapter.GetProduct(context.Background(), "tenant-a", "product-b")
	if !errors.Is(err, dataaccess.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestListProductObjectsTenantScopeOrderingAndUTC(t *testing.T) {
	createdAt := time.Date(2026, 7, 31, 2, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	db := openScriptDB(t,
		expectQuery(
			"WHERE o.organization_id = $1 AND o.product_id = $2",
			[]any{"tenant-a", "product-a"},
			objectColumns(),
			[][]driver.Value{{
				"object-a", "tenant-a", "product-a", strings.Repeat("a", 64),
				"watch.jpg", "image/jpeg", int64(100), int64(1), "ready",
				createdAt, createdAt.Add(time.Minute), nil,
			}},
		),
	)
	adapter := mustAdapter(t, db)
	objects, err := adapter.ListProductObjects(context.Background(), "tenant-a", "product-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].Status != dataaccess.ObjectReady {
		t.Fatalf("objects = %#v", objects)
	}
	if objects[0].CreatedAt.Location() != time.UTC || objects[0].ReadyAt.Location() != time.UTC {
		t.Fatalf("timestamps were not normalized to UTC: %#v", objects[0])
	}
}

func TestCreatePendingObjectWritesIdempotencyAndAuditAtomically(t *testing.T) {
	requestedAt := time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC)
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", []any{"tenant-a", "actor-a", permissionInventoryWrite}, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", []any{
			"tenant-a", operationCreatePending, "idem-create", anyArg{}, "actor-a", requestedAt,
		}, 1),
		expectQuery("INSERT INTO zaiko.product_objects", []any{
			"tenant-a", "product-a", "object-a", "watch.jpg", "image/jpeg", int64(2),
			"s3", "test-bucket", anyArg{}, "actor-a", requestedAt,
		}, objectColumns(), [][]driver.Value{{
			"object-a", "tenant-a", "product-a", "", "watch.jpg", "image/jpeg",
			int64(0), int64(2), "pending", requestedAt, nil, nil,
		}}),
		expectExec("INSERT INTO zaiko.audit_logs", nil, 1),
		expectExec("UPDATE zaiko.idempotency_records", nil, 1),
	)
	adapter := mustAdapter(t, db)
	object, err := adapter.CreatePendingObject(
		context.Background(),
		dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "idem-create", RequestedAt: requestedAt,
		},
		dataaccess.PendingObjectInput{
			ObjectID: "object-a", ProductID: "product-a", OriginalName: "watch.jpg",
			ContentType: "image/jpeg", SortOrder: 2,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if object.Status != dataaccess.ObjectPending || object.ID != "object-a" {
		t.Fatalf("object = %#v", object)
	}
	tenantHash := "80a707af7dc77ee1228f9127180f3964835e5beb4c4ab0d812f0fe7593579b3a"
	wantKey := "objects/" + tenantHash + "/object-a"
	if got := adapter.objectKey("tenant-a", "object-a"); got != wantKey {
		t.Fatalf("object key = %q, want %q", got, wantKey)
	}
}

func TestMarkObjectReadyRejectsIdempotencyPayloadMismatch(t *testing.T) {
	requestedAt := time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC)
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", []any{"tenant-a", "actor-a", permissionInventoryWrite}, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 0),
		expectQuery("FROM zaiko.idempotency_records", nil,
			[]string{"hash", "state", "result_id", "result_number", "result_version"},
			[][]driver.Value{{strings.Repeat("0", 64), "committed", "object-a", "", int64(0)}},
		),
	)
	adapter := mustAdapter(t, db)
	_, err := adapter.MarkObjectReady(
		context.Background(),
		dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "idem-ready", RequestedAt: requestedAt,
		},
		"object-a",
		dataaccess.BlobReceipt{ChecksumSHA256: strings.Repeat("a", 64), SizeBytes: 100},
		requestedAt,
	)
	if !errors.Is(err, dataaccess.ErrIdempotencyMismatch) {
		t.Fatalf("error = %v, want ErrIdempotencyMismatch", err)
	}
}

func TestMarkObjectReadyCASDistinguishesCrossTenantFromStatusConflict(t *testing.T) {
	requestedAt := time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		exists bool
		want   error
	}{
		{name: "cross tenant or missing", exists: false, want: dataaccess.ErrNotFound},
		{name: "invalid state", exists: true, want: dataaccess.ErrConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openScriptDB(t,
				expectQuery("FROM zaiko.users", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
				expectExec("INSERT INTO zaiko.idempotency_records", nil, 1),
				expectQuery("UPDATE zaiko.product_objects", nil, objectColumns(), nil),
				expectQuery("SELECT EXISTS", nil, []string{"exists"}, [][]driver.Value{{test.exists}}),
			)
			adapter := mustAdapter(t, db)
			_, err := adapter.MarkObjectReady(
				context.Background(),
				dataaccess.CommandScope{
					TenantID: "tenant-a", ActorID: "actor-a",
					IdempotencyKey: "idem-ready", RequestedAt: requestedAt,
				},
				"object-a",
				dataaccess.BlobReceipt{ChecksumSHA256: strings.Repeat("a", 64), SizeBytes: 100},
				requestedAt,
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestCanonicalHashIgnoresRetryTimeButBindsTenantActorAndPayload(t *testing.T) {
	base := dataaccess.CommandScope{
		TenantID: "tenant-a", ActorID: "actor-a",
		IdempotencyKey: "key", RequestedAt: time.Now(),
	}
	first, err := canonicalHash("op", base, struct{ Value int }{1})
	if err != nil {
		t.Fatal(err)
	}
	base.RequestedAt = base.RequestedAt.Add(time.Hour)
	retry, _ := canonicalHash("op", base, struct{ Value int }{1})
	changed, _ := canonicalHash("op", base, struct{ Value int }{2})
	if first != retry {
		t.Fatal("retry timestamp changed canonical request hash")
	}
	if first == changed {
		t.Fatal("different payload reused canonical request hash")
	}
}

func TestWorkflowCanonicalHashesExcludeOnlyRequestedAt(t *testing.T) {
	baseScope := dataaccess.CommandScope{
		TenantID: "tenant-a", ActorID: "actor-a",
		IdempotencyKey: "same-key",
		RequestedAt:    time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
	}
	retryScope := baseScope
	retryScope.RequestedAt = retryScope.RequestedAt.Add(time.Minute)
	line := dataaccess.SlipLineAmount{
		ProductID: "product-a", Quantity: 1,
		Amount: dataaccess.Money{AmountMinor: 850000, Currency: "JPY"},
	}
	returnItem := dataaccess.RestoreReturnedInventoryItem{
		ReturnItemID: "return-a", ProductID: "product-a",
		ConditionCode: "A", Quantity: 1, ExpectedVersion: 2,
	}
	tests := []struct {
		name      string
		operation string
		first     any
		retry     any
		changed   any
	}{
		{
			name: "purchase", operation: operationConfirmPurchase,
			first: dataaccess.ConfirmPurchaseCommand{
				Scope: baseScope, PurchaseDate: "2026-08-01",
				SupplierID: "supplier-a", StaffID: "actor-a",
				Lines: []dataaccess.SlipLineAmount{line},
			},
			retry: dataaccess.ConfirmPurchaseCommand{
				Scope: retryScope, PurchaseDate: "2026-08-01",
				SupplierID: "supplier-a", StaffID: "actor-a",
				Lines: []dataaccess.SlipLineAmount{line},
			},
			changed: dataaccess.ConfirmPurchaseCommand{
				Scope: baseScope, PurchaseDate: "2026-08-01",
				SupplierID: "supplier-b", StaffID: "actor-a",
				Lines: []dataaccess.SlipLineAmount{line},
			},
		},
		{
			name: "sale", operation: operationConfirmSale,
			first: dataaccess.ConfirmSaleCommand{
				Scope: baseScope, SaleDate: "2026-08-01", BuyerID: "buyer-a",
				TaxExempt: true, Currency: "JPY",
				Lines: []dataaccess.SlipLineAmount{line},
			},
			retry: dataaccess.ConfirmSaleCommand{
				Scope: retryScope, SaleDate: "2026-08-01", BuyerID: "buyer-a",
				TaxExempt: true, Currency: "JPY",
				Lines: []dataaccess.SlipLineAmount{line},
			},
			changed: dataaccess.ConfirmSaleCommand{
				Scope: baseScope, SaleDate: "2026-08-01", BuyerID: "buyer-b",
				TaxExempt: true, Currency: "JPY",
				Lines: []dataaccess.SlipLineAmount{line},
			},
		},
		{
			name: "shipment", operation: operationConfirmShipment,
			first: dataaccess.ConfirmShipmentCommand{
				Scope: baseScope, SalesSlipID: "sale-a",
				ShipmentDate: "2026-08-01", DestinationID: "buyer-a",
				ProductIDs: []string{"product-a"}, ExpectedVersion: 4,
			},
			retry: dataaccess.ConfirmShipmentCommand{
				Scope: retryScope, SalesSlipID: "sale-a",
				ShipmentDate: "2026-08-01", DestinationID: "buyer-a",
				ProductIDs: []string{"product-a"}, ExpectedVersion: 4,
			},
			changed: dataaccess.ConfirmShipmentCommand{
				Scope: baseScope, SalesSlipID: "sale-a",
				ShipmentDate: "2026-08-01", DestinationID: "buyer-b",
				ProductIDs: []string{"product-a"}, ExpectedVersion: 4,
			},
		},
		{
			name: "restore return", operation: operationRestoreReturn,
			first: dataaccess.RestoreReturnedInventoryCommand{
				Scope: baseScope, SaleID: "sale-a",
				Items: []dataaccess.RestoreReturnedInventoryItem{returnItem},
			},
			retry: dataaccess.RestoreReturnedInventoryCommand{
				Scope: retryScope, SaleID: "sale-a",
				Items: []dataaccess.RestoreReturnedInventoryItem{returnItem},
			},
			changed: dataaccess.RestoreReturnedInventoryCommand{
				Scope: baseScope, SaleID: "sale-b",
				Items: []dataaccess.RestoreReturnedInventoryItem{returnItem},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, err := canonicalHash(test.operation, baseScope, test.first)
			if err != nil {
				t.Fatal(err)
			}
			retry, err := canonicalHash(test.operation, retryScope, test.retry)
			if err != nil {
				t.Fatal(err)
			}
			if first != retry {
				t.Fatal("RequestedAt changed workflow canonical hash")
			}

			otherTenant := baseScope
			otherTenant.TenantID = "tenant-b"
			tenantHash, _ := canonicalHash(test.operation, otherTenant, test.first)
			otherActor := baseScope
			otherActor.ActorID = "actor-b"
			actorHash, _ := canonicalHash(test.operation, otherActor, test.first)
			payloadHash, _ := canonicalHash(test.operation, baseScope, test.changed)
			if first == tenantHash || first == actorHash || first == payloadHash {
				t.Fatal("workflow hash did not bind tenant, actor, and business payload")
			}
		})
	}
}

func TestCreateProductRejectsInactiveDeletedOrUnauthorizedActor(t *testing.T) {
	for _, requiredSQL := range []string{
		"u.is_active = TRUE AND u.deleted_at IS NULL",
		"o.is_active = TRUE",
		"NOT EXISTS",
		"rp.permission_key = $3",
		"allowed.effect = 'allow'",
	} {
		t.Run(requiredSQL, func(t *testing.T) {
			db := openScriptDB(t,
				expectQuery(
					requiredSQL,
					[]any{"tenant-a", "actor-a", permissionInventoryWrite},
					[]string{"one"},
					nil,
				),
			)
			adapter := mustAdapter(t, db)
			_, err := adapter.CreateProduct(
				context.Background(),
				dataaccess.CommandScope{
					TenantID: "tenant-a", ActorID: "actor-a",
					IdempotencyKey: "authorization-check",
					RequestedAt:    time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC),
				},
				validProductDraft(),
			)
			if !errors.Is(err, dataaccess.ErrNotFound) {
				t.Fatalf("error = %v, want tenant-safe ErrNotFound", err)
			}
		})
	}
}

func TestCreateProductWritesRegistrationAtomically(t *testing.T) {
	requestedAt := time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC)
	clockAt := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	draft := validProductDraft()
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", []any{"tenant-a", "actor-a", permissionInventoryWrite}, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", []any{
			"tenant-a", operationCreateProduct, "idem-product", anyArg{}, "actor-a", requestedAt,
		}, 1),
		expectQuery("FROM zaiko.users", []any{"tenant-a", "buyer-a"}, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectQuery("FROM zaiko.suppliers", []any{"tenant-a", "supplier-a"}, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectQuery("FROM zaiko.accessory_masters", []any{"tenant-a", "CERTIFICATE", "GUARANTEE"}, []string{"count"}, [][]driver.Value{{int64(2)}}),
		expectQuery("FROM zaiko.guest_boxes", []any{"tenant-a", "box-a"}, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectQuery("ON CONFLICT (organization_id, sequence_kind, date_key)", []any{
			"tenant-a", productSequenceKind, "2026-03-03", clockAt,
		}, []string{"last_sequence"}, [][]driver.Value{{int64(7)}}),
		expectQuery("INSERT INTO zaiko.products", []any{
			"product-id", "tenant-a", "20260303007", "SKU-A", "Rolex",
			"Submariner", "SERIAL-A", "watch", "supplier-a", "2026-03-03",
			int64(850000), "JPY", int64(1180000), "JPY",
			"in_stock", "private", "A", requestedAt,
		}, []string{"version"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.product_accessories", []any{
			"tenant-a", "product-id", "CERTIFICATE", "GUARANTEE", requestedAt,
		}, 2),
		expectExec("INSERT INTO zaiko.guest_box_products", []any{
			"tenant-a", "box-a", "product-id", "actor-a", requestedAt,
		}, 1),
		expectExec("INSERT INTO zaiko.inventory_events", []any{
			"event-id", "tenant-a", "product-id", "in_stock", "actor-a", "idem-product", requestedAt,
		}, 1),
		expectExec("INSERT INTO zaiko.audit_logs", nil, 1),
		expectExec("UPDATE zaiko.idempotency_records", []any{
			"tenant-a", operationCreateProduct, "idem-product",
			"product-id", "20260303007", int64(1), clockAt,
		}, 1),
	)
	adapter := mustAdapterWithIDs(t, db, "product-id", "event-id", "audit-id")
	result, err := adapter.CreateProduct(
		context.Background(),
		dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "idem-product", RequestedAt: requestedAt,
		},
		draft,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProductID != "product-id" ||
		result.ProductCode != "20260303007" ||
		result.Version != 1 ||
		result.Replayed {
		t.Fatalf("result = %#v", result)
	}
}

func TestCreateProductReplaysStoredResult(t *testing.T) {
	requestedAt := time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC)
	draft := validProductDraft()
	normalized, _, err := normalizeProductDraft(draft)
	if err != nil {
		t.Fatal(err)
	}
	scope := dataaccess.CommandScope{
		TenantID: "tenant-a", ActorID: "actor-a",
		IdempotencyKey: "idem-product", RequestedAt: requestedAt,
	}
	hash, err := canonicalHash(operationCreateProduct, scope, normalized)
	if err != nil {
		t.Fatal(err)
	}
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", []any{"tenant-a", "actor-a", permissionInventoryWrite}, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 0),
		expectQuery("FROM zaiko.idempotency_records", nil,
			[]string{"hash", "state", "result_id", "result_number", "result_version"},
			[][]driver.Value{{hash, "committed", "product-id", "20260303007", int64(4)}},
		),
	)
	adapter := mustAdapter(t, db)
	result, err := adapter.CreateProduct(context.Background(), scope, draft)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Replayed ||
		result.ProductID != "product-id" ||
		result.ProductCode != "20260303007" ||
		result.Version != 4 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCreateProductCrossTenantSupplierIsNotFound(t *testing.T) {
	requestedAt := time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC)
	draft := validProductDraft()
	draft.BuyerID = ""
	draft.AccessoryCodes = nil
	draft.BoxID = ""
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 1),
		expectQuery("FROM zaiko.suppliers", []any{"tenant-a", "supplier-a"}, []string{"one"}, nil),
	)
	adapter := mustAdapter(t, db)
	_, err := adapter.CreateProduct(
		context.Background(),
		dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "idem-product", RequestedAt: requestedAt,
		},
		draft,
	)
	if !errors.Is(err, dataaccess.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestCreateProductSequenceExhaustionIsConflict(t *testing.T) {
	requestedAt := time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC)
	draft := validProductDraft()
	draft.BuyerID = ""
	draft.AccessoryCodes = nil
	draft.BoxID = ""
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 1),
		expectQuery("FROM zaiko.suppliers", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectQuery("WHERE zaiko.business_number_sequences.last_sequence < 999", nil, []string{"last_sequence"}, nil),
	)
	adapter := mustAdapter(t, db)
	_, err := adapter.CreateProduct(
		context.Background(),
		dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "idem-product", RequestedAt: requestedAt,
		},
		draft,
	)
	if !errors.Is(err, dataaccess.ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
}

func TestCreateProductUniqueConflictIsConflict(t *testing.T) {
	requestedAt := time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC)
	draft := validProductDraft()
	draft.ProductCode = "20260303007"
	draft.BuyerID = ""
	draft.AccessoryCodes = nil
	draft.BoxID = ""
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 1),
		expectQuery("FROM zaiko.suppliers", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectQuery("SET last_sequence = GREATEST", []any{
			"tenant-a", productSequenceKind, "2026-03-03", int64(7),
			time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		}, []string{"last_sequence"}, [][]driver.Value{{int64(7)}}),
		expectQuery("INSERT INTO zaiko.products", nil, []string{"version"}, nil),
	)
	adapter := mustAdapterWithIDs(t, db, "product-id")
	_, err := adapter.CreateProduct(
		context.Background(),
		dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "idem-product", RequestedAt: requestedAt,
		},
		draft,
	)
	if !errors.Is(err, dataaccess.ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
}

func TestCreateProductRejectsInvalidDraftBeforeTransaction(t *testing.T) {
	for name, mutate := range map[string]func(*dataaccess.ProductDraft){
		"bad status": func(draft *dataaccess.ProductDraft) {
			draft.InventoryStatus = "unknown"
		},
		"bad explicit number": func(draft *dataaccess.ProductDraft) {
			draft.ProductCode = "20260304001"
		},
		"duplicate accessory": func(draft *dataaccess.ProductDraft) {
			draft.AccessoryCodes = []string{"BOX", " BOX "}
		},
	} {
		t.Run(name, func(t *testing.T) {
			db := openScriptDB(t)
			draft := validProductDraft()
			mutate(&draft)
			adapter := mustAdapter(t, db)
			_, err := adapter.CreateProduct(
				context.Background(),
				dataaccess.CommandScope{
					TenantID: "tenant-a", ActorID: "actor-a",
					IdempotencyKey: "idem-product",
					RequestedAt:    time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC),
				},
				draft,
			)
			if !errors.Is(err, dataaccess.ErrInvalidArgument) {
				t.Fatalf("error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

func TestValidateProductSearchRejectsInvalidRangesAndOverflow(t *testing.T) {
	for _, search := range []dataaccess.ProductSearch{
		{Page: 0, PageSize: 20},
		{Page: 1, PageSize: 0},
		{Page: math.MaxInt, PageSize: math.MaxInt},
		{Page: 1, PageSize: 20, PurchaseDateFrom: "31/07/2026"},
		{Page: 1, PageSize: 20, PurchaseDateFrom: "2026-08-01", PurchaseDateTo: "2026-07-01"},
	} {
		if err := validateProductSearch(search); !errors.Is(err, dataaccess.ErrInvalidArgument) {
			t.Fatalf("search %#v error = %v", search, err)
		}
	}
}

func TestNormalizeDBErrorMapsPortablePostgresConcurrencyStates(t *testing.T) {
	for _, state := range []string{"23505", "40001", "40P01"} {
		err := normalizeDBError(context.Background(), "write", sqlStateError(state))
		if !errors.Is(err, dataaccess.ErrConflict) {
			t.Fatalf("SQLSTATE %s error = %v, want ErrConflict", state, err)
		}
	}
	plain := errors.New("database unavailable")
	if err := normalizeDBError(context.Background(), "read", plain); !errors.Is(err, plain) {
		t.Fatalf("plain error lost cause: %v", err)
	}
}

func TestConfirmPurchaseWritesSlipLinesInventoryAndAuditAtomically(t *testing.T) {
	requestedAt := time.Date(2026, 8, 1, 9, 30, 0, 0, time.FixedZone("JST", 9*60*60))
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", []any{"tenant-a", "actor-a", permissionPurchaseConfirm}, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 1),
		expectQuery("FROM zaiko.suppliers", []any{"tenant-a", "supplier-a"}, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectQuery("FROM zaiko.products", nil, workflowProductColumns(), [][]driver.Value{
			workflowProductRow("product-a", "purchasing", 3),
		}),
		expectQuery("INSERT INTO zaiko.business_number_sequences", nil, []string{"last_sequence"}, [][]driver.Value{{int64(7)}}),
		expectQuery("INSERT INTO zaiko.purchase_slips", nil, []string{"version"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.purchase_slip_lines", nil, 1),
		expectExec("UPDATE zaiko.products", nil, 1),
		expectExec("INSERT INTO zaiko.inventory_events", nil, 1),
		expectExec("INSERT INTO zaiko.audit_logs", nil, 1),
		expectExec("UPDATE zaiko.idempotency_records", nil, 1),
	)
	adapter := mustAdapterWithIDs(t, db, "purchase-a", "purchase-line-a", "event-a", "audit-a")
	result, err := adapter.ConfirmPurchase(context.Background(), dataaccess.ConfirmPurchaseCommand{
		Scope: dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "purchase-key", RequestedAt: requestedAt,
		},
		PurchaseDate: "2026-08-01", SupplierID: "supplier-a", StaffID: "actor-a",
		Lines: []dataaccess.SlipLineAmount{{
			ProductID: "product-a", Quantity: 1,
			Amount: dataaccess.Money{AmountMinor: 850000, Currency: "JPY"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "purchase-a" || result.Number != "PI-2026-0007" || result.Version != 1 || result.Replayed {
		t.Fatalf("result = %#v", result)
	}
}

func TestConfirmSaleWritesExemptJPYSlipAndTransitionsProduct(t *testing.T) {
	requestedAt := time.Date(2026, 8, 1, 0, 30, 0, 0, time.UTC)
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 1),
		expectQuery("FROM zaiko.master_records", nil,
			[]string{"name", "address", "contact", "invoice"},
			[][]driver.Value{{"Buyer A", "Tokyo", "03-0000", "T123"}}),
		expectQuery("FROM zaiko.products", nil, workflowProductColumns(), [][]driver.Value{
			workflowProductRow("product-a", "in_stock", 5),
		}),
		expectQuery("INSERT INTO zaiko.business_number_sequences", nil, []string{"last_sequence"}, [][]driver.Value{{int64(11)}}),
		expectQuery("INSERT INTO zaiko.sales_slips", nil, []string{"version"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.sales_lines", nil, 1),
		expectExec("UPDATE zaiko.products", nil, 1),
		expectExec("INSERT INTO zaiko.inventory_events", nil, 1),
		expectExec("INSERT INTO zaiko.audit_logs", nil, 1),
		expectExec("UPDATE zaiko.idempotency_records", nil, 1),
	)
	adapter := mustAdapterWithIDs(t, db, "sale-a", "sale-line-a", "event-a", "audit-a")
	result, err := adapter.ConfirmSale(context.Background(), dataaccess.ConfirmSaleCommand{
		Scope: dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "sale-key", RequestedAt: requestedAt,
		},
		SaleDate: "2026-08-01", BuyerID: "buyer-a", TaxExempt: true, Currency: "JPY",
		Lines: []dataaccess.SlipLineAmount{{
			ProductID: "product-a", Quantity: 1,
			Amount: dataaccess.Money{AmountMinor: 1180000, Currency: "JPY"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "sale-a" || result.Number != "SL-2026-0011" || result.Version != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestConfirmPurchaseReplaysCommittedWorkflowResult(t *testing.T) {
	requestedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	command := dataaccess.ConfirmPurchaseCommand{
		Scope: dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "purchase-replay", RequestedAt: requestedAt,
		},
		PurchaseDate: "2026-08-01", SupplierID: "supplier-a", StaffID: "actor-a",
		Lines: []dataaccess.SlipLineAmount{{
			ProductID: "product-a", Quantity: 1,
			Amount: dataaccess.Money{AmountMinor: 850000, Currency: "JPY"},
		}},
	}
	originalCommand := command
	hash, err := canonicalHash(
		operationConfirmPurchase,
		originalCommand.Scope,
		originalCommand,
	)
	if err != nil {
		t.Fatal(err)
	}
	command.Scope.RequestedAt = command.Scope.RequestedAt.Add(time.Minute)
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 0),
		expectQuery("FROM zaiko.idempotency_records", nil,
			[]string{"hash", "state", "result_id", "result_number", "result_version"},
			[][]driver.Value{{hash, "committed", "purchase-a", "PI-2026-0007", int64(1)}}),
	)
	adapter := mustAdapter(t, db)
	result, err := adapter.ConfirmPurchase(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Replayed || result.ID != "purchase-a" ||
		result.Number != "PI-2026-0007" || result.Version != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestConfirmShipmentWritesAllocationAndTransitionsProduct(t *testing.T) {
	requestedAt := time.Date(2026, 8, 1, 1, 30, 0, 0, time.UTC)
	shipmentRow := append([]driver.Value{"sales-line-a", int64(1), int64(1180000)},
		workflowProductRow("product-a", "sold", 6)...)
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 1),
		expectQuery("FROM zaiko.sales_slips", nil,
			[]string{"slip_number", "status", "version"},
			[][]driver.Value{{"SL-2026-0011", "confirmed", int64(4)}}),
		expectQuery("FROM zaiko.master_records", nil,
			[]string{"name", "address", "contact", "invoice"},
			[][]driver.Value{{"Buyer A", "Tokyo", "03-0000", "T123"}}),
		expectQuery("FROM zaiko.sales_lines sl", nil, append(
			[]string{"sales_line_id", "quantity", "price"}, workflowProductColumns()...,
		), [][]driver.Value{shipmentRow}),
		expectQuery("SELECT COALESCE(SUM(allocated_quantity), 0)", nil,
			[]string{"allocated"}, [][]driver.Value{{int64(0)}}),
		expectQuery("INSERT INTO zaiko.business_number_sequences", nil, []string{"last_sequence"}, [][]driver.Value{{int64(3)}}),
		expectQuery("INSERT INTO zaiko.shipment_slips", nil, []string{"version"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.shipment_lines", nil, 1),
		expectExec("INSERT INTO zaiko.sales_shipment_allocations", nil, 1),
		expectExec("UPDATE zaiko.products", nil, 1),
		expectExec("INSERT INTO zaiko.inventory_events", nil, 1),
		expectExec("INSERT INTO zaiko.audit_logs", nil, 1),
		expectExec("UPDATE zaiko.idempotency_records", nil, 1),
	)
	adapter := mustAdapterWithIDs(t, db,
		"shipment-a", "shipment-line-a", "allocation-a", "event-a", "audit-a")
	result, err := adapter.ConfirmShipment(context.Background(), dataaccess.ConfirmShipmentCommand{
		Scope: dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "shipment-key", RequestedAt: requestedAt,
		},
		SalesSlipID: "sale-a", ShipmentDate: "2026-08-01",
		DestinationID: "buyer-a", ProductIDs: []string{"product-a"}, ExpectedVersion: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "shipment-a" || result.Number != "SH-2026-0003" || result.Version != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRestoreReturnedInventoryCompletesSingleItemAndAudits(t *testing.T) {
	requestedAt := time.Date(2026, 8, 1, 2, 30, 0, 0, time.UTC)
	db := openScriptDB(t,
		expectQuery("FROM zaiko.users", nil, []string{"one"}, [][]driver.Value{{int64(1)}}),
		expectExec("INSERT INTO zaiko.idempotency_records", nil, 1),
		expectQuery("FROM zaiko.sales_slips", nil,
			[]string{"slip_number", "version"},
			[][]driver.Value{{"SL-2026-0011", int64(4)}}),
		expectQuery("FROM zaiko.return_takehome_items r", nil,
			[]string{"action", "status", "quantity", "item_version", "restored_at", "product_status", "product_version"},
			[][]driver.Value{{"return", "completed", int64(1), int64(2), nil, "shipped", int64(6)}}),
		expectExec("UPDATE zaiko.products", nil, 1),
		expectQuery("UPDATE zaiko.return_takehome_items", nil,
			[]string{"version"}, [][]driver.Value{{int64(3)}}),
		expectExec("INSERT INTO zaiko.inventory_events", nil, 1),
		expectExec("INSERT INTO zaiko.audit_logs", nil, 1),
		expectExec("UPDATE zaiko.idempotency_records", nil, 1),
	)
	adapter := mustAdapterWithIDs(t, db, "event-a", "audit-a")
	result, err := adapter.RestoreReturnedInventory(context.Background(), dataaccess.RestoreReturnedInventoryCommand{
		Scope: dataaccess.CommandScope{
			TenantID: "tenant-a", ActorID: "actor-a",
			IdempotencyKey: "restore-key", RequestedAt: requestedAt,
		},
		SaleID: "sale-a",
		Items: []dataaccess.RestoreReturnedInventoryItem{{
			ReturnItemID: "return-a", ProductID: "product-a",
			ConditionCode: "A", Quantity: 1, ExpectedVersion: 2,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "sale-a" || result.Number != "SL-2026-0011" || result.Version != 3 {
		t.Fatalf("result = %#v", result)
	}
}

func TestWorkflowRejectsUnresolvedContractsBeforeSQL(t *testing.T) {
	scope := dataaccess.CommandScope{
		TenantID: "tenant-a", ActorID: "actor-a",
		IdempotencyKey: "key", RequestedAt: time.Now(),
	}
	db := openScriptDB(t)
	adapter := mustAdapter(t, db)

	_, err := adapter.ConfirmSale(context.Background(), dataaccess.ConfirmSaleCommand{
		Scope: scope, SaleDate: "2026-08-01", BuyerID: "buyer-a",
		Currency: "USD", FXRateScaled: 150, FXRateScale: 1, TaxExempt: true,
		Lines: []dataaccess.SlipLineAmount{{
			ProductID: "product-a", Quantity: 1,
			Amount: dataaccess.Money{AmountMinor: 100, Currency: "USD"},
		}},
	})
	if !errors.Is(err, dataaccess.ErrPrecondition) {
		t.Fatalf("foreign sale error = %v, want ErrPrecondition", err)
	}
	_, err = adapter.RestoreReturnedInventory(context.Background(), dataaccess.RestoreReturnedInventoryCommand{
		Scope: scope, SaleID: "sale-a",
		Items: []dataaccess.RestoreReturnedInventoryItem{
			{ReturnItemID: "return-a", ProductID: "a", ConditionCode: "A", Quantity: 1},
			{ReturnItemID: "return-b", ProductID: "b", ConditionCode: "A", Quantity: 1},
		},
	})
	if !errors.Is(err, dataaccess.ErrPrecondition) {
		t.Fatalf("multi-item restore error = %v, want ErrPrecondition", err)
	}
}

type sqlStateError string

func (e sqlStateError) Error() string    { return "postgres error" }
func (e sqlStateError) SQLState() string { return string(e) }

func testConfig() Config {
	return Config{
		StorageBucket:   "test-bucket",
		ObjectKeyPrefix: "objects",
		Clock: func() time.Time {
			return time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
		},
		NewID: func() (string, error) { return "audit-test-id", nil },
	}
}

func mustAdapter(t *testing.T, db *sql.DB) *Adapter {
	t.Helper()
	adapter, err := New(db, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func mustAdapterWithIDs(t *testing.T, db *sql.DB, ids ...string) *Adapter {
	t.Helper()
	config := testConfig()
	var index int
	config.NewID = func() (string, error) {
		if index >= len(ids) {
			return "", fmt.Errorf("unexpected ID request %d", index)
		}
		id := ids[index]
		index++
		return id, nil
	}
	adapter, err := New(db, config)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func validProductDraft() dataaccess.ProductDraft {
	return dataaccess.ProductDraft{
		SKU: "SKU-A", Brand: "Rolex", ModelNumber: "Submariner",
		SerialNumber: "SERIAL-A", ProductType: "watch",
		SupplierID: "supplier-a", BuyerID: "buyer-a",
		PurchaseDate: "2026-03-03",
		Cost: dataaccess.Money{
			AmountMinor: 850000, Currency: "JPY",
		},
		BaseSalePrice: dataaccess.Money{
			AmountMinor: 1180000, Currency: "JPY",
		},
		InventoryStatus: "in_stock", PublicationStatus: "private",
		Condition: "A", AccessoryCodes: []string{"GUARANTEE", "CERTIFICATE"},
		BoxID: "box-a",
	}
}

func productColumns() []string {
	columns := make([]string, 29)
	for index := range columns {
		columns[index] = fmt.Sprintf("column_%d", index)
	}
	return columns
}

func productRow(createdAt time.Time) []driver.Value {
	return []driver.Value{
		"product-a", "tenant-a", "20260303001", "SKU-A", "Rolex",
		"Submariner", "SERIAL", "watch", "supplier-a", "Supplier A",
		"actor-a", "Buyer A", time.Date(2026, 3, 3, 0, 0, 0, 0, time.FixedZone("JST", 9*60*60)),
		int64(math.MaxInt64 - 1), "USD", int64(math.MaxInt64), "USD",
		"in_stock", "private", "A", "BOX", "steel", "BOX1",
		"automatic", "steel", "black", "feature", int64(2), createdAt,
	}
}

func workflowProductColumns() []string {
	return []string{
		"id", "product_code", "sku", "brand", "model_number", "product_type",
		"serial_number", "material_text", "movement_text", "condition_text",
		"belt_material_text", "dial_text", "box_text", "accessories",
		"features_text", "inventory_status", "base_sale_price_minor", "base_sale_currency", "version",
	}
}

func workflowProductRow(id, status string, version int64) []driver.Value {
	return []driver.Value{
		id, "20260801001", "SKU-A", "Rolex", "Submariner", "watch",
		"SERIAL-A", "steel", "automatic", "A", "steel", "black", "BOX1",
		"BOX,GUARANTEE", "feature", status, int64(1180000), "USD", version,
	}
}

func objectColumns() []string {
	return []string{
		"id", "organization_id", "product_id", "checksum_sha256",
		"original_name", "content_type", "size_bytes", "sort_order", "status",
		"created_at", "ready_at", "deleted_at",
	}
}

// The script driver is intentionally tiny: it verifies query order, required
// tenant arguments, and database/sql scans without adding a PostgreSQL driver.
type anyArg struct{}

type scriptStep struct {
	kind     string
	contains string
	args     []any
	columns  []string
	rows     [][]driver.Value
	affected int64
}

type scriptState struct {
	mu    sync.Mutex
	steps []scriptStep
	index int
	err   error
}

var (
	registerScriptDriver sync.Once
	scriptSequence       atomic.Uint64
	scriptRegistry       sync.Map
)

func expectQuery(contains string, args []any, columns []string, rows [][]driver.Value) scriptStep {
	return scriptStep{kind: "query", contains: contains, args: args, columns: columns, rows: rows}
}

func expectExec(contains string, args []any, affected int64) scriptStep {
	return scriptStep{kind: "exec", contains: contains, args: args, affected: affected}
}

func openScriptDB(t *testing.T, steps ...scriptStep) *sql.DB {
	t.Helper()
	registerScriptDriver.Do(func() { sql.Register("postgresadapter-script", scriptDriver{}) })
	dsn := fmt.Sprintf("script-%d", scriptSequence.Add(1))
	state := &scriptState{steps: append([]scriptStep(nil), steps...)}
	scriptRegistry.Store(dsn, state)
	db, err := sql.Open("postgresadapter-script", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		scriptRegistry.Delete(dsn)
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.err != nil {
			t.Errorf("script driver: %v", state.err)
		}
		if state.index != len(state.steps) {
			t.Errorf("script driver consumed %d/%d expectations", state.index, len(state.steps))
		}
	})
	return db
}

type scriptDriver struct{}

func (scriptDriver) Open(name string) (driver.Conn, error) {
	value, ok := scriptRegistry.Load(name)
	if !ok {
		return nil, fmt.Errorf("unknown script %q", name)
	}
	return &scriptConn{state: value.(*scriptState)}, nil
}

type scriptConn struct {
	state *scriptState
}

func (c *scriptConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("script driver does not prepare statements")
}
func (c *scriptConn) Close() error              { return nil }
func (c *scriptConn) Begin() (driver.Tx, error) { return &scriptTx{}, nil }
func (c *scriptConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &scriptTx{}, nil
}

func (c *scriptConn) QueryContext(
	_ context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Rows, error) {
	step, err := c.next("query", query, args)
	if err != nil {
		return nil, err
	}
	return &scriptRows{columns: step.columns, rows: step.rows}, nil
}

func (c *scriptConn) ExecContext(
	_ context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Result, error) {
	step, err := c.next("exec", query, args)
	if err != nil {
		return nil, err
	}
	return driver.RowsAffected(step.affected), nil
}

func (c *scriptConn) next(kind, query string, args []driver.NamedValue) (scriptStep, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.index >= len(c.state.steps) {
		err := fmt.Errorf("unexpected %s: %s", kind, compactSQL(query))
		c.state.err = err
		return scriptStep{}, err
	}
	step := c.state.steps[c.state.index]
	c.state.index++
	if step.kind != kind || !strings.Contains(compactSQL(query), compactSQL(step.contains)) {
		err := fmt.Errorf("step %d: got %s %q, want %s containing %q",
			c.state.index, kind, compactSQL(query), step.kind, compactSQL(step.contains))
		c.state.err = err
		return scriptStep{}, err
	}
	if step.args != nil {
		actual := make([]any, len(args))
		for index := range args {
			actual[index] = args[index].Value
		}
		if !matchArgs(step.args, actual) {
			err := fmt.Errorf("step %d args = %#v, want %#v", c.state.index, actual, step.args)
			c.state.err = err
			return scriptStep{}, err
		}
	}
	return step, nil
}

func compactSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func matchArgs(expected, actual []any) bool {
	if len(expected) != len(actual) {
		return false
	}
	for index := range expected {
		if _, ok := expected[index].(anyArg); ok {
			continue
		}
		if !reflect.DeepEqual(expected[index], actual[index]) {
			return false
		}
	}
	return true
}

type scriptTx struct{}

func (*scriptTx) Commit() error   { return nil }
func (*scriptTx) Rollback() error { return nil }

type scriptRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *scriptRows) Columns() []string { return r.columns }
func (r *scriptRows) Close() error      { return nil }
func (r *scriptRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}
