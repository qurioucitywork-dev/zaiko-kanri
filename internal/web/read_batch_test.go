package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"gorm.io/gorm"
)

func readBatchLogin(t *testing.T, app *Server, username, password string) ([]*http.Cookie, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("login status=%d", w.Code)
	}
	var result struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return w.Result().Cookies(), result.CSRFToken
}
func readBatchRequest(t *testing.T, app *Server, cookies []*http.Cookie, csrf string, paths ...string) *httptest.ResponseRecorder {
	t.Helper()
	items := make([]readBatchItem, len(paths))
	for i, path := range paths {
		items[i] = readBatchItem{ID: fmt.Sprintf("r%d", i), Path: path}
	}
	body, _ := json.Marshal(map[string]any{"requests": items})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/read-batch", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", csrf)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	return w
}
func readBatchResults(t *testing.T, w *httptest.ResponseRecorder) []readBatchResult {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("batch status=%d body=%s", w.Code, w.Body.String())
	}
	var result struct {
		Responses []readBatchResult `json:"responses"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Responses
}
func readBatchDB(t *testing.T, app *Server) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(app.cfg.DatabasePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return db
}

func TestReadBatchMatchesIndividualGETsAndAuthenticatesOnlyOnce(t *testing.T) {
	app, _ := testServer(t)
	cookies, csrf := readBatchLogin(t, app, "admin", "preview-admin-2026")
	db := readBatchDB(t, app)
	for _, statement := range []string{
		`CREATE TABLE session_read_counter(n INTEGER NOT NULL)`,
		`INSERT INTO session_read_counter VALUES(0)`,
		`CREATE TRIGGER count_session_reads AFTER UPDATE OF last_seen_at ON sessions BEGIN UPDATE session_read_counter SET n=n+1; END`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{"/products?pageSize=2", "/dashboard?months=3", "/settings", "/products/does-not-exist", "/users?includeInactive=true"}
	results := readBatchResults(t, readBatchRequest(t, app, cookies, csrf, paths...))
	var reads int
	if err := db.Raw(`SELECT n FROM session_read_counter`).Scan(&reads).Error; err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("batch validated session %d times; want once", reads)
	}
	for i, path := range paths {
		if results[i].ID != fmt.Sprintf("r%d", i) {
			t.Fatal("results changed input order")
		}
		r := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		var direct, batched any
		if json.Unmarshal(w.Body.Bytes(), &direct) != nil || json.Unmarshal(results[i].Body, &batched) != nil {
			t.Fatalf("invalid JSON: %s", path)
		}
		if w.Code != results[i].Status || !reflect.DeepEqual(direct, batched) {
			t.Fatalf("GET/batch mismatch: %s direct=%d batch=%d", path, w.Code, results[i].Status)
		}
	}
}

func TestReadBatchAuthenticationCSRFAndPermissionRevocation(t *testing.T) {
	app, _ := testServer(t)
	if got := readBatchRequest(t, app, nil, "", "/products").Code; got != 401 {
		t.Fatalf("unauthenticated=%d", got)
	}
	cookies, csrf := readBatchLogin(t, app, "worker", "preview-worker-2026")
	if got := readBatchRequest(t, app, cookies, "wrong", "/products").Code; got != 403 {
		t.Fatalf("CSRF=%d", got)
	}
	results := readBatchResults(t, readBatchRequest(t, app, cookies, csrf, "/products", "/users"))
	if results[0].Status != 200 || results[1].Status != 403 {
		t.Fatalf("worker boundaries=%d,%d", results[0].Status, results[1].Status)
	}
	db := readBatchDB(t, app)
	if err := db.Exec(`INSERT INTO user_permissions(user_id,permission_key,effect) VALUES('usr_worker','inventory.read','deny')`).Error; err != nil {
		t.Fatal(err)
	}
	results = readBatchResults(t, readBatchRequest(t, app, cookies, csrf, "/products"))
	if results[0].Status != 403 {
		t.Fatal("permission revoke was not effective on next batch")
	}
	if err := db.Exec(`UPDATE users SET is_active=0 WHERE id='usr_worker'`).Error; err != nil {
		t.Fatal(err)
	}
	if got := readBatchRequest(t, app, cookies, csrf, "/products").Code; got != 401 {
		t.Fatalf("disabled user=%d", got)
	}
}

func TestReadBatchGuestCannotReadInternalDataOrForgeTrust(t *testing.T) {
	app, _ := testServer(t)
	db := readBatchDB(t, app)
	if err := db.Exec(`UPDATE users SET role_key='guest' WHERE id='usr_worker'`).Error; err != nil {
		t.Fatal(err)
	}
	cookies, csrf := readBatchLogin(t, app, "worker", "preview-worker-2026")
	results := readBatchResults(t, readBatchRequest(t, app, cookies, csrf, "/products", "/users", "/settings", "/purchases"))
	for _, result := range results {
		if result.Status != 403 {
			t.Fatalf("guest internal read=%d", result.Status)
		}
	}
	forged := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
	forged.Header.Set("X-Read-Batch-Authenticated", "admin")
	forged.Header.Set("X-User-Id", "usr_admin")
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, forged)
	if w.Code != 401 {
		t.Fatal("HTTP header bypassed session validation")
	}
}

func TestReadBatchStrictReadOnlyAllowlist(t *testing.T) {
	for _, path := range []string{
		"https://example.com/products", "//example.com/products", "/api/v1/products", "/read-batch", "/auth/me", "/auth/logout",
		"/products/../settings", "/products/%2e%2e/settings", "/products/%61", "/product-files/file_id", "/document-events/event_id/file",
		"/products/id/cost-adjustments/confirm", "/exports/inventory.csv", "/products?unknown=x", "/products?page=1&page=2",
		"/masters/unknown", "/products/", "/products#fragment", "/products?bad=%zz", "/products\\other", "/products/id/files?limit=3",
	} {
		if _, ok := readBatchURL(path); ok {
			t.Errorf("unsafe path accepted: %s", path)
		}
	}
	for _, path := range []string{"/products?q=%E6%99%82%E8%A8%88&page=2&pageSize=200", "/masters/belt-materials", "/purchases/id_1", "/products/id_1/files", "/purchases-deleted?page=1&pageSize=100"} {
		if target, ok := readBatchURL(path); !ok || !strings.HasPrefix(target.Path, "/api/v1/") {
			t.Errorf("valid path rejected: %s", path)
		}
	}
	app, _ := testServer(t)
	cookies, csrf := readBatchLogin(t, app, "admin", "preview-admin-2026")
	for _, body := range []string{
		`{"requests":[]}`, `{"requests":[{"id":"same","path":"/products"},{"id":"same","path":"/users"}]}`,
		`{"requests":[{"id":"one","path":"/products","method":"DELETE"}]}`, `{"requests":[{"id":"one","path":"/products"}]} {}`,
		`{"requests":[{"id":"one","path":"/products?q=` + strings.Repeat("x", 65536) + `"}]}`,
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/read-batch", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("invalid batch accepted: status=%d", w.Code)
		}
	}
	tooMany := make([]string, 33)
	for i := range tooMany {
		tooMany[i] = "/products"
	}
	if readBatchRequest(t, app, cookies, csrf, tooMany...).Code != 400 {
		t.Fatal("33-item batch accepted")
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/read-batch", strings.NewReader(`{"requests":[{"id":"r1","path":"/products"}]}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", csrf)
	r.Header.Set("X-Admin-Page", "master")
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("elevated batch must use individual routes")
	}
}

func TestReadBatchPanicCancellationAndResponseBudget(t *testing.T) {
	app, store := testServer(t)
	user, err := store.Authenticate(t.Context(), "PREVIEW", "admin", "preview-admin-2026")
	if err != nil {
		t.Fatal(err)
	}
	session := database.Session{User: user}
	makeRequest := func(ctx context.Context) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/read-batch", strings.NewReader(`{"requests":[{"id":"r1","path":"/products"}]}`))
		r.Header.Set("Content-Type", "application/json")
		return r.WithContext(withSession(withUser(ctx, user), session))
	}
	w := httptest.NewRecorder()
	app.apiReadBatch(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("test panic") })).ServeHTTP(w, makeRequest(t.Context()))
	if results := readBatchResults(t, w); results[0].Status != 500 {
		t.Fatal("inner panic was not isolated")
	}
	w = httptest.NewRecorder()
	app.apiReadBatch(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", readBatchMaxBytes+1)))
	})).ServeHTTP(w, makeRequest(t.Context()))
	if w.Code != 413 || !json.Valid(w.Body.Bytes()) {
		t.Fatal("response overflow returned partial success")
	}
	var called atomic.Int32
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	w = httptest.NewRecorder()
	app.apiReadBatch(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called.Add(1) })).ServeHTTP(w, makeRequest(canceled))
	if called.Load() != 0 {
		t.Fatal("canceled batch executed a handler")
	}
	// A request-local authorization snapshot cannot be reused for another user.
	auth := &readBatchAuth{session: session, permissions: map[string]bool{"inventory.read": true}}
	wrong := withUser(context.WithValue(t.Context(), readBatchContextKey{}, auth), database.User{ID: "other", OrganizationID: user.OrganizationID, Role: user.Role, Active: true})
	if _, ok := readBatchAuthentication(wrong); ok {
		t.Fatal("batch trust crossed identities")
	}
}

func TestReadBatchKeepsProductOrganizationIsolation(t *testing.T) {
	app, _ := testServer(t)
	db := readBatchDB(t, app)
	// The legacy SQLite migration uses TEXT timestamps, whereas the current
	// GORM product read model needs DATETIME values when scanning real rows.
	// Use a typed, otherwise equivalent isolated fixture; production is unchanged.
	if err := db.Exec(`DROP TABLE products`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE products (
        id TEXT PRIMARY KEY, organization_id TEXT, product_code TEXT, sku TEXT DEFAULT '',
        brand TEXT, model_number TEXT DEFAULT '', serial_number TEXT DEFAULT '', product_type TEXT,
        purchase_slip_line_id TEXT, supplier_id TEXT, purchase_date TEXT,
        cost_amount_minor INTEGER, cost_currency TEXT, base_sale_price_minor INTEGER DEFAULT 0,
        base_sale_currency TEXT DEFAULT 'USD', inventory_status TEXT, publication_status TEXT DEFAULT 'private',
        deleted_at DATETIME, created_at DATETIME, updated_at DATETIME
    )`).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Exec(`INSERT INTO organizations(id,code,name,created_at,updated_at) VALUES('org_other','OTHER','Other organization',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, org := range []string{"org_preview", "org_other"} {
		for _, statement := range []string{
			`INSERT INTO suppliers(id,organization_id,supplier_code,name,created_at,updated_at) VALUES(?,?,'supplier','Supplier',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		} {
			if err := db.Exec(statement, "supplier_"+org, org).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Exec(`INSERT INTO purchase_slips(id,organization_id,slip_number,supplier_id,purchase_date,status,created_by,created_at,updated_at) VALUES(?,?,'purchase',?,'2026-11-24','confirmed','usr_admin',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, "purchase_"+org, org, "supplier_"+org).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO purchase_slip_lines(id,purchase_slip_id,line_number,quantity,unit_cost_minor,currency,brand,product_type,created_at) VALUES(?,?,1,1,100,'JPY','Test','watch',CURRENT_TIMESTAMP)`, "line_"+org, "purchase_"+org).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO products(id,organization_id,product_code,brand,product_type,purchase_slip_line_id,supplier_id,purchase_date,cost_amount_minor,cost_currency,inventory_status,created_at,updated_at) VALUES(?,?,?,'Test','watch',?,?,'2026-11-24',100,'JPY','in_stock',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, "product_"+org, org, org, "line_"+org, "supplier_"+org).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.repository.Products(t.Context(), "org_preview", persistence.ProductFilter{}); err != nil {
		t.Fatal(err)
	}
	cookies, csrf := readBatchLogin(t, app, "admin", "preview-admin-2026")
	paths := []string{"/products", "/products/product_org_preview", "/products/product_org_other", "/products?q=org_other"}
	results := readBatchResults(t, readBatchRequest(t, app, cookies, csrf, paths...))
	if results[0].Status != 200 || !strings.Contains(string(results[0].Body), "product_org_preview") || strings.Contains(string(results[0].Body), "product_org_other") {
		t.Fatalf("list did not retain organization filter: %d %s", results[0].Status, results[0].Body)
	}
	if results[1].Status != 200 || results[2].Status != 404 || strings.Contains(string(results[3].Body), "product_org_other") {
		t.Fatal("detail/search crossed organization boundary")
	}
	for i, path := range paths {
		request := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		var direct, batch any
		_ = json.Unmarshal(recorder.Body.Bytes(), &direct)
		_ = json.Unmarshal(results[i].Body, &batch)
		if recorder.Code != results[i].Status || !reflect.DeepEqual(direct, batch) {
			t.Fatalf("organization filter parity mismatch: %s", path)
		}
	}
}
