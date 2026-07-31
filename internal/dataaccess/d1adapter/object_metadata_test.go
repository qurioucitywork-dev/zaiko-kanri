package d1adapter

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
	_ "modernc.org/sqlite"
)

func objectScope(key string) dataaccess.CommandScope {
	return dataaccess.CommandScope{
		TenantID:       "tenant-a",
		ActorID:        "actor-a",
		IdempotencyKey: key,
		RequestedAt:    time.Date(2026, 7, 31, 1, 2, 3, 4, time.UTC),
	}
}

func verifyObjectCommand(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	if request.Method != http.MethodPost {
		t.Fatalf("method = %s", request.Method)
	}
	if request.Header.Get("x-zaiko-tenant-id") != "tenant-a" {
		t.Fatalf("tenant = %q", request.Header.Get("x-zaiko-tenant-id"))
	}
	if request.Header.Get("x-zaiko-idempotency-key") == "" {
		t.Fatal("missing idempotency key")
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if value["actor_id"] != "actor-a" ||
		value["requested_at"] != "2026-07-31T01:02:03.000000004Z" {
		t.Fatalf("scope payload = %#v", value)
	}
	operation := operationForObjectPath(t, request.URL.Path)
	expectedHash, err := canonicalObjectCommandHash(
		operation,
		dataaccess.CommandScope{
			TenantID: request.Header.Get("x-zaiko-tenant-id"),
			ActorID:  value["actor_id"].(string),
		},
		value,
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Header.Get("x-zaiko-canonical-hash") != expectedHash {
		t.Fatal("canonical hash does not bind operation, tenant, actor, and business payload")
	}
	return value
}

func operationForObjectPath(t *testing.T, path string) string {
	t.Helper()
	switch {
	case path == "/internal/v1/object-metadata/pending":
		return operationCreatePending
	case strings.HasSuffix(path, "/ready"):
		return operationMarkReady
	case strings.HasSuffix(path, "/failed"):
		return operationMarkFailed
	case strings.HasSuffix(path, "/deleted"):
		return operationMarkDeleted
	default:
		t.Fatalf("unexpected object command path %q", path)
		return ""
	}
}

func TestCanonicalObjectCommandHashIgnoresRetryTimeAndBindsCommand(t *testing.T) {
	base := objectScope("same-key")
	checksum := strings.Repeat("a", 64)
	tests := []struct {
		name      string
		operation string
		payload   func(dataaccess.CommandScope) any
		changed   func(dataaccess.CommandScope) any
	}{
		{
			name: "create pending", operation: operationCreatePending,
			payload: func(scope dataaccess.CommandScope) any {
				return createPendingRequest{
					commandEnvelope: envelope(scope),
					ObjectID:        "object-a", ProductID: "product-a",
					OriginalName: "photo.png", ContentType: "image/png", SortOrder: 1,
				}
			},
			changed: func(scope dataaccess.CommandScope) any {
				return createPendingRequest{
					commandEnvelope: envelope(scope),
					ObjectID:        "object-b", ProductID: "product-a",
					OriginalName: "photo.png", ContentType: "image/png", SortOrder: 1,
				}
			},
		},
		{
			name: "mark ready", operation: operationMarkReady,
			payload: func(scope dataaccess.CommandScope) any {
				return markReadyRequest{
					commandEnvelope: envelope(scope),
					ChecksumSHA256:  checksum, SizeBytes: 16,
					ReadyAt: "2026-07-31T02:00:00Z",
				}
			},
			changed: func(scope dataaccess.CommandScope) any {
				return markReadyRequest{
					commandEnvelope: envelope(scope),
					ChecksumSHA256:  checksum, SizeBytes: 17,
					ReadyAt: "2026-07-31T02:00:00Z",
				}
			},
		},
		{
			name: "mark failed", operation: operationMarkFailed,
			payload: func(scope dataaccess.CommandScope) any {
				return markFailedRequest{
					commandEnvelope: envelope(scope),
					ReasonCode:      "blob_upload_failed",
				}
			},
			changed: func(scope dataaccess.CommandScope) any {
				return markFailedRequest{
					commandEnvelope: envelope(scope),
					ReasonCode:      "blob_verify_failed",
				}
			},
		},
		{
			name: "mark deleted", operation: operationMarkDeleted,
			payload: func(scope dataaccess.CommandScope) any {
				return markDeletedRequest{
					commandEnvelope: envelope(scope),
					DeletedAt:       "2026-07-31T03:00:00Z",
				}
			},
			changed: func(scope dataaccess.CommandScope) any {
				return markDeletedRequest{
					commandEnvelope: envelope(scope),
					DeletedAt:       "2026-07-31T04:00:00Z",
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, err := canonicalObjectCommandHash(
				test.operation,
				base,
				test.payload(base),
			)
			if err != nil {
				t.Fatal(err)
			}

			retryScope := base
			retryScope.RequestedAt = retryScope.RequestedAt.Add(time.Hour)
			retry, err := canonicalObjectCommandHash(
				test.operation,
				retryScope,
				test.payload(retryScope),
			)
			if err != nil {
				t.Fatal(err)
			}
			if first != retry {
				t.Fatal("transport retry timestamp changed canonical hash")
			}

			otherTenant := base
			otherTenant.TenantID = "tenant-b"
			tenantHash, _ := canonicalObjectCommandHash(
				test.operation,
				otherTenant,
				test.payload(otherTenant),
			)
			otherActor := base
			otherActor.ActorID = "actor-b"
			actorHash, _ := canonicalObjectCommandHash(
				test.operation,
				otherActor,
				test.payload(otherActor),
			)
			payloadHash, _ := canonicalObjectCommandHash(
				test.operation,
				base,
				test.changed(base),
			)
			operationHash, _ := canonicalObjectCommandHash(
				test.operation+".other",
				base,
				test.payload(base),
			)
			if first == tenantHash || first == actorHash ||
				first == payloadHash || first == operationHash {
				t.Fatal("canonical hash did not bind operation, tenant, actor, and business payload")
			}
		})
	}
}

func TestCanonicalObjectCommandHashMatchesCrossLanguageJSONEscaping(t *testing.T) {
	scope := objectScope("escaping-key")
	hash, err := canonicalObjectCommandHash(
		operationCreatePending,
		scope,
		createPendingRequest{
			commandEnvelope: envelope(scope),
			ObjectID:        "object-a",
			ProductID:       "product-a",
			OriginalName:    "A&B<写真>\u2028x\u2029.png",
			ContentType:     "image/png",
			SortOrder:       1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	// This is the exact JSON.stringify-compatible byte sequence used by the
	// Worker: HTML characters stay literal and U+2028/U+2029 use Go-compatible
	// escapes.
	canonical := `{"operation":"object.create_pending","tenant_id":"tenant-a","actor_id":"actor-a","payload":{"content_type":"image/png","object_id":"object-a","original_name":"A&B<写真>\u2028x\u2029.png","product_id":"product-a","sort_order":1}}`
	sum := sha256.Sum256([]byte(canonical))
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash = %s, want cross-language canonical JSON hash", hash)
	}
}

func TestObjectMetadataWriterLifecycleAndHeaders(t *testing.T) {
	t.Parallel()
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		value := verifyObjectCommand(t, request)
		paths = append(paths, request.URL.Path)
		writer.Header().Set("content-type", "application/json")
		switch request.URL.Path {
		case "/internal/v1/object-metadata/pending":
			if value["object_id"] != "object-a" ||
				value["product_id"] != "product-a" ||
				value["content_type"] != "image/png" {
				t.Fatalf("pending payload = %#v", value)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"object":{
				"id":"object-a","organization_id":"tenant-a","product_id":"product-a",
				"checksum_sha256":"","original_name":"photo.png","content_type":"image/png",
				"size_bytes":"0","sort_order":2,"status":"pending",
				"created_at":"2026-07-31T01:02:03.000000004Z","ready_at":"","deleted_at":""
			}}`))
		case "/internal/v1/object-metadata/object-a/ready":
			if value["checksum_sha256"] != strings.Repeat("a", 64) ||
				value["size_bytes"] != float64(16) {
				t.Fatalf("ready payload = %#v", value)
			}
			_, _ = writer.Write([]byte(`{"object":{
				"id":"object-a","organization_id":"tenant-a","product_id":"product-a",
				"checksum_sha256":"` + strings.Repeat("a", 64) + `",
				"original_name":"photo.png","content_type":"image/png",
				"size_bytes":"16","sort_order":2,"status":"ready",
				"created_at":"2026-07-31T01:02:03.000000004Z",
				"ready_at":"2026-07-31T02:00:00Z","deleted_at":""
			}}`))
		case "/internal/v1/object-metadata/object-a/failed",
			"/internal/v1/object-metadata/object-a/deleted":
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	pending, err := adapter.CreatePendingObject(
		context.Background(),
		objectScope("pending-key"),
		dataaccess.PendingObjectInput{
			ObjectID:     "object-a",
			ProductID:    "product-a",
			OriginalName: "photo.png",
			ContentType:  "IMAGE/PNG",
			SortOrder:    2,
		},
	)
	if err != nil || pending.Status != dataaccess.ObjectPending ||
		pending.TenantID != "tenant-a" || pending.SizeBytes != 0 {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
	ready, err := adapter.MarkObjectReady(
		context.Background(),
		objectScope("ready-key"),
		"object-a",
		dataaccess.BlobReceipt{
			ChecksumSHA256: strings.Repeat("a", 64),
			SizeBytes:      16,
		},
		time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC),
	)
	if err != nil || ready.Status != dataaccess.ObjectReady ||
		ready.SizeBytes != 16 || ready.ReadyAt.IsZero() {
		t.Fatalf("ready = %#v, %v", ready, err)
	}
	if err := adapter.MarkObjectFailed(
		context.Background(), objectScope("failed-key"),
		"object-a", "blob_upload_failed",
	); err != nil {
		t.Fatal(err)
	}
	if err := adapter.MarkObjectDeleted(
		context.Background(), objectScope("deleted-key"),
		"object-a", time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC),
	); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 4 {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestObjectMetadataWriterErrorMappingAndNoResponseLeak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code   string
		status int
		want   error
	}{
		{"invalid_argument", 400, dataaccess.ErrInvalidArgument},
		{"not_found", 404, dataaccess.ErrNotFound},
		{"conflict", 409, dataaccess.ErrConflict},
		{"idempotency_mismatch", 409, dataaccess.ErrIdempotencyMismatch},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				_ *http.Request,
			) {
				writer.Header().Set("content-type", "application/json")
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(
					`{"error":"` + test.code +
						`","secret":"binding=DB private-worker-url"}`,
				))
			}))
			defer server.Close()
			adapter, err := New(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.CreatePendingObject(
				context.Background(),
				objectScope("error-key"),
				dataaccess.PendingObjectInput{
					ObjectID:     "object-a",
					ProductID:    "product-a",
					OriginalName: "photo.png",
					ContentType:  "image/png",
				},
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if strings.Contains(err.Error(), "binding") ||
				strings.Contains(err.Error(), "private-worker") {
				t.Fatalf("response leaked: %v", err)
			}
		})
	}
}

func TestObjectMetadataWriterRejectsInvalidInputBeforeHTTP(t *testing.T) {
	t.Parallel()
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(
		http.ResponseWriter,
		*http.Request,
	) {
		requests++
	}))
	defer server.Close()
	adapter, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CreatePendingObject(
		context.Background(),
		objectScope("invalid-key"),
		dataaccess.PendingObjectInput{
			ObjectID:     "../object",
			ProductID:    "product-a",
			OriginalName: "photo.png",
			ContentType:  "image/png",
		},
	); !errors.Is(err, dataaccess.ErrInvalidArgument) {
		t.Fatalf("pending error = %v", err)
	}
	if _, err := adapter.MarkObjectReady(
		context.Background(),
		objectScope("invalid-ready"),
		"object-a",
		dataaccess.BlobReceipt{ChecksumSHA256: "bad", SizeBytes: 1},
		time.Now(),
	); !errors.Is(err, dataaccess.ErrInvalidArgument) {
		t.Fatalf("ready error = %v", err)
	}
	if err := adapter.MarkObjectFailed(
		context.Background(), objectScope("invalid-failed"),
		"object-a", "../secret",
	); !errors.Is(err, dataaccess.ErrInvalidArgument) {
		t.Fatalf("failed error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("invalid commands made %d request(s)", requests)
	}
}

func TestObjectMetadataWriterHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(
		http.ResponseWriter,
		*http.Request,
	) {
		requests++
	}))
	defer server.Close()
	adapter, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = adapter.CreatePendingObject(
		ctx,
		objectScope("cancel-key"),
		dataaccess.PendingObjectInput{
			ObjectID:     "object-a",
			ProductID:    "product-a",
			OriginalName: "photo.png",
			ContentType:  "image/png",
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if requests != 0 {
		t.Fatalf("canceled command made %d request(s)", requests)
	}
}

func TestWorkerScopedLookupBindingContract(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(
		"../../../deploy/cloudflare/d1-service/src/object-metadata.ts",
	)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "async function scopedObjectAndActor(")
	end := strings.Index(text, "export async function createPendingObject(")
	if start < 0 || end <= start {
		t.Fatal("scopedObjectAndActor source block not found")
	}
	block := text[start:end]
	if count := strings.Count(block, "?"); count != 4 {
		t.Fatalf("scoped lookup placeholder count = %d, want 4", count)
	}
	if !strings.Contains(
		block,
		".bind(tenantID, actorID, tenantID, objectID)",
	) {
		t.Fatal("scoped lookup bind order must be tenant, actor, tenant, object")
	}
}

func TestWorkerObjectMetadataWritePermissionContract(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(
		"../../../deploy/cloudflare/d1-service/src/object-metadata.ts",
	)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"JOIN organizations o ON o.id = u.organization_id",
		"o.is_active = 1",
		"u.is_active = 1",
		"u.deleted_at IS NULL",
		"FROM user_permissions up",
		"up.permission_key = 'inventory.write'",
		"up.effect = 'allow'",
		"NOT EXISTS(",
		"FROM role_permissions rp",
		"rp.permission_key = 'inventory.write'",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("worker authorization contract missing %q", required)
		}
	}
	if count := strings.Count(
		text,
		"JOIN organizations o ON o.id = u.organization_id",
	); count != 2 {
		t.Fatalf("organization authorization join count = %d, want 2", count)
	}
	if count := strings.Count(text, "o.is_active = 1"); count != 2 {
		t.Fatalf("organization active check count = %d, want 2", count)
	}
	functionChecks := []struct {
		start string
		end   string
		check string
	}{
		{
			"export async function createPendingObject(",
			"async function transitionObject",
			"actorAndProductExist(",
		},
		{
			"export async function markObjectReady(",
			"export async function markObjectFailed(",
			"scopedObjectAndActor(",
		},
		{
			"export async function markObjectFailed(",
			"export async function markObjectDeleted(",
			"scopedObjectAndActor(",
		},
		{
			"export async function markObjectDeleted(",
			"// handleObjectMetadataRequest",
			"scopedObjectAndActor(",
		},
	}
	for _, check := range functionChecks {
		start := strings.Index(text, check.start)
		end := strings.Index(text, check.end)
		if start < 0 || end <= start {
			t.Fatalf("worker function block %q not found", check.start)
		}
		block := text[start:end]
		authorization := strings.Index(block, check.check)
		replay := strings.Index(block, "const replayed = await replay(")
		if authorization < 0 || replay < 0 || authorization > replay {
			t.Fatalf(
				"%s must authorize actor before idempotency replay",
				check.start,
			)
		}
	}
}

func TestWorkerCanonicalHashExcludesOnlyRequestedAt(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(
		"../../../deploy/cloudflare/d1-service/src/object-metadata.ts",
	)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "function canonicalObjectCommand(")
	end := strings.Index(text, "async function verifiedCommand")
	if start < 0 || end <= start {
		t.Fatal("canonicalObjectCommand source block not found")
	}
	block := text[start:end]
	for _, required := range []string{
		`key !== "actor_id" && key !== "requested_at"`,
		"operation: operationName",
		"tenant_id: tenantID.trim()",
		"actor_id: body.actor_id.trim()",
		"payload: businessPayload",
	} {
		if !strings.Contains(block, required) {
			t.Fatalf("worker canonical hash contract missing %q", required)
		}
	}
	for _, operation := range []string{
		"object.create_pending",
		"object.mark_ready",
		"object.mark_failed",
		"object.mark_deleted",
	} {
		if !strings.Contains(text, `"`+operation+`"`) {
			t.Fatalf("worker canonical hash operation missing %q", operation)
		}
	}
}

func TestObjectMetadataMigrationTransitions(t *testing.T) {
	t.Parallel()
	migration, err := os.ReadFile(
		"../../../deploy/cloudflare/d1-service/migrations/0002_object_metadata.sql",
	)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE organizations (id TEXT PRIMARY KEY);
		CREATE TABLE products (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL REFERENCES organizations(id)
		);
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL REFERENCES organizations(id)
		);
		INSERT INTO organizations(id) VALUES ('tenant'), ('other-tenant');
		INSERT INTO products(id, organization_id)
			VALUES ('product', 'tenant'), ('other-product', 'other-tenant');
		INSERT INTO users(id, organization_id)
			VALUES ('actor', 'tenant'), ('other-actor', 'other-tenant');
	`); err != nil {
		t.Fatalf("create migration parents: %v", err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	insert := `INSERT INTO object_metadata (
		organization_id, id, product_id, original_name, content_type,
		sort_order, created_by, created_at, updated_at
	) VALUES (?, ?, ?, 'photo.png', 'image/png', 0,
	          ?, '2026-07-31T00:00:00Z', '2026-07-31T00:00:00Z')`
	if _, err := db.Exec(
		insert, "tenant", "cross-product", "other-product", "actor",
	); err == nil {
		t.Fatal("cross-tenant product reference unexpectedly succeeded")
	}
	if _, err := db.Exec(
		insert, "tenant", "cross-actor", "product", "other-actor",
	); err == nil {
		t.Fatal("cross-tenant created_by reference unexpectedly succeeded")
	}
	if _, err := db.Exec(`
		INSERT INTO object_metadata_idempotency (
			organization_id, operation_name, idempotency_key, canonical_hash,
			object_id, actor_id, response_json, created_at
		) VALUES (
			'tenant', 'object.create_pending', 'cross-actor',
			?, 'object', 'other-actor', '{}', '2026-07-31T00:00:00Z'
		)`,
		strings.Repeat("a", 64),
	); err == nil {
		t.Fatal("cross-tenant idempotency actor reference unexpectedly succeeded")
	}

	if _, err := db.Exec(
		insert, "tenant", "ready-object", "product", "actor",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`UPDATE object_metadata
		    SET status='ready', checksum_sha256=?, size_bytes=16,
		        ready_at='2026-07-31T01:00:00Z'
		  WHERE organization_id='tenant' AND id='ready-object'`,
		strings.Repeat("a", 64),
	); err != nil {
		t.Fatalf("pending -> ready: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE object_metadata
		    SET status='deleted', deleted_at='2026-07-31T02:00:00Z'
		  WHERE organization_id='tenant' AND id='ready-object'`,
	); err != nil {
		t.Fatalf("ready -> deleted: %v", err)
	}
	var readyAt string
	if err := db.QueryRow(
		`SELECT ready_at FROM object_metadata
		  WHERE organization_id='tenant' AND id='ready-object'`,
	).Scan(&readyAt); err != nil || readyAt != "2026-07-31T01:00:00Z" {
		t.Fatalf("ready_at = %q, %v", readyAt, err)
	}

	if _, err := db.Exec(
		insert, "tenant", "failed-object", "product", "actor",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`UPDATE object_metadata
		    SET status='failed', failure_reason_code='blob_upload_failed'
		  WHERE organization_id='tenant' AND id='failed-object'`,
	); err != nil {
		t.Fatalf("pending -> failed: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE object_metadata
		    SET status='deleted', deleted_at='2026-07-31T03:00:00Z'
		  WHERE organization_id='tenant' AND id='failed-object'`,
	); err != nil {
		t.Fatalf("failed -> deleted: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE object_metadata SET status='ready'
		  WHERE organization_id='tenant' AND id='failed-object'`,
	); err == nil {
		t.Fatal("deleted -> ready unexpectedly succeeded")
	}
}
