package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func testServer(t *testing.T) (*Server, *database.Store) {
	t.Helper()
	store, err := database.Open("file:" + t.TempDir() + "/web.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedPreview(t.Context(), "preview-admin-2026", "preview-worker-2026"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Environment: "test", SessionTTL: time.Hour, OrganizationCode: "PREVIEW"}
	app, err := New(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return app, store
}

func loginAs(t *testing.T, app *Server, username, password string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	get := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(getRecorder, get)
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(getRecorder.Body.String())
	if len(csrfMatch) != 2 {
		t.Fatal("login csrf token missing")
	}
	form := url.Values{"username": {username}, "password": {password}, "csrf_token": {csrfMatch[1]}}
	post := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, post)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("login status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var session, csrf *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		switch cookie.Name {
		case sessionCookie:
			session = cookie
		case csrfCookie:
			csrf = cookie
		}
	}
	if session == nil || csrf == nil {
		t.Fatal("session cookies missing")
	}
	return session, csrf
}

func TestProtectedPageRedirectsToLogin(t *testing.T) {
	app, _ := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/login" {
		t.Fatalf("status=%d location=%q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestWorkerCannotOpenAdminUsersPage(t *testing.T) {
	app, _ := testServer(t)
	session, _ := loginAs(t, app, "worker", "preview-worker-2026")
	request := httptest.NewRequest(http.MethodGet, "/users", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("worker status=%d, want 403", recorder.Code)
	}
}

func TestUnsafeRequestRequiresCSRF(t *testing.T) {
	app, _ := testServer(t)
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	form := url.Values{"key": {"reservation.duration_hours"}, "value": {"24"}}
	request := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("csrf status=%d, want 403", recorder.Code)
	}
}

func TestPhase7ProductListSupportsFullPageAndPartialRequests(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	full := httptest.NewRequest(http.MethodGet, "/products?q=Rolex&sort=brand_asc", nil)
	full.AddCookie(session)
	fullRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(fullRecorder, full)
	fullBody := fullRecorder.Body.String()
	if fullRecorder.Code != http.StatusOK || !strings.Contains(fullBody, "<!doctype html>") ||
		!strings.Contains(fullBody, `method="get" action="/products"`) {
		t.Fatalf("normal GET must work without JavaScript: status=%d body=%s", fullRecorder.Code, fullBody)
	}

	partial := httptest.NewRequest(http.MethodGet, "/products?q=Rolex&sort=brand_asc", nil)
	partial.Header.Set("HX-Request", "true")
	partial.AddCookie(session)
	partialRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(partialRecorder, partial)
	partialBody := partialRecorder.Body.String()
	if partialRecorder.Code != http.StatusOK || !strings.Contains(partialBody, `id="product-results"`) ||
		strings.Contains(partialBody, "<!doctype html>") {
		t.Fatalf("partial response invalid: status=%d body=%s", partialRecorder.Code, partialBody)
	}
	if partialRecorder.Header().Get("Vary") != "HX-Request" {
		t.Fatalf("partial Vary=%q", partialRecorder.Header().Get("Vary"))
	}
}

func TestPhase7PartialRequestsKeepAuthenticationAndCSRF(t *testing.T) {
	app, _ := testServer(t)
	anonymous := httptest.NewRequest(http.MethodGet, "/products", nil)
	anonymous.Header.Set("HX-Request", "true")
	anonymousRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(anonymousRecorder, anonymous)
	if anonymousRecorder.Code != http.StatusUnauthorized ||
		anonymousRecorder.Header().Get("HX-Redirect") != "/login" {
		t.Fatalf("anonymous partial status=%d redirect=%q", anonymousRecorder.Code, anonymousRecorder.Header().Get("HX-Redirect"))
	}

	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	form := url.Values{"key": {"reservation.duration_hours"}, "value": {"24"}}
	unsafe := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	unsafe.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	unsafe.Header.Set("HX-Request", "true")
	unsafe.AddCookie(session)
	unsafeRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(unsafeRecorder, unsafe)
	if unsafeRecorder.Code != http.StatusForbidden ||
		!strings.Contains(unsafeRecorder.Body.String(), `role="alert"`) {
		t.Fatalf("partial CSRF status=%d body=%s", unsafeRecorder.Code, unsafeRecorder.Body.String())
	}
}

func TestPhase7AdminCanFindCancelledProductsAndCSVExportIsAudited(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	cancelled := products[0]
	if err := store.CancelProduct(t.Context(), "org_preview", cancelled.ID, "usr_admin", "検索テスト"); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")

	normal := httptest.NewRequest(http.MethodGet, "/products", nil)
	normal.AddCookie(session)
	normalRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(normalRecorder, normal)
	if strings.Contains(normalRecorder.Body.String(), cancelled.ProductCode) {
		t.Fatal("cancelled product appeared in normal inventory list")
	}

	included := httptest.NewRequest(http.MethodGet, "/products?include_cancelled=1&q="+url.QueryEscape(cancelled.ProductCode), nil)
	included.AddCookie(session)
	includedRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(includedRecorder, included)
	if includedRecorder.Code != http.StatusOK || !strings.Contains(includedRecorder.Body.String(), cancelled.ProductCode) {
		t.Fatalf("admin cancelled search status=%d body=%s", includedRecorder.Code, includedRecorder.Body.String())
	}

	export := httptest.NewRequest(http.MethodGet, "/products/export.csv?include_cancelled=1", nil)
	export.AddCookie(session)
	exportRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(exportRecorder, export)
	if exportRecorder.Code != http.StatusOK || !strings.Contains(exportRecorder.Body.String(), "原価") {
		t.Fatalf("csv status=%d body=%s", exportRecorder.Code, exportRecorder.Body.String())
	}
	entries, err := store.AuditLogs(t.Context(), "org_preview", 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.Action == "inventory.cost_csv.exported" && entry.ActorUserID == "usr_admin" {
			found = true
		}
	}
	if !found {
		t.Fatal("cost CSV export audit log missing")
	}
}

func TestWorkerCancellationRoutesEnterApprovalHandler(t *testing.T) {
	app, _ := testServer(t)
	session, csrf := loginAs(t, app, "worker", "preview-worker-2026")
	for _, path := range []string{"/sales/example/cancel", "/shipments/example/cancel"} {
		form := url.Values{"csrf_token": {csrf.Value}, "reason": {"テスト"}}
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.AddCookie(session)
		request.AddCookie(csrf)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code == http.StatusForbidden {
			t.Fatalf("%s worker cancellation should be accepted for approval processing", path)
		}
	}
}

func TestPhase4SalesAndShipmentPagesRender(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMarketPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	for _, path := range []string{"/sales", "/sales/new", "/shipments", "/shipments/new"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(session)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Phase 8") {
			t.Fatalf("%s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestPublicPurchaseRequestFlowRendersWithoutCostData(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedRequestPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/public/products", nil)
	getRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(getRecorder, get)
	body := getRecorder.Body.String()
	if getRecorder.Code != http.StatusOK || !strings.Contains(body, "購入依頼を送る") {
		t.Fatalf("public catalog status=%d body=%s", getRecorder.Code, body)
	}
	if strings.Contains(body, "仕入原価") || strings.Contains(body, "粗利") {
		t.Fatal("public catalog leaked cost or profit data")
	}
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	actionMatch := regexp.MustCompile(`action="/public/products/([^/]+)/purchase-requests"`).FindStringSubmatch(body)
	if len(csrfMatch) != 2 || len(actionMatch) != 2 {
		t.Fatal("public purchase form fields missing")
	}
	form := url.Values{
		"csrf_token": {csrfMatch[1]}, "guest_name": {"公開ゲスト"},
		"guest_email": {"guest@example.com"}, "message": {"購入を希望します"},
	}
	post := httptest.NewRequest(http.MethodPost, "/public/products/"+actionMatch[1]+"/purchase-requests", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(postRecorder, post)
	if postRecorder.Code != http.StatusSeeOther {
		t.Fatalf("public request status=%d body=%s", postRecorder.Code, postRecorder.Body.String())
	}

	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	internal := httptest.NewRequest(http.MethodGet, "/purchase-requests", nil)
	internal.AddCookie(session)
	internalRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(internalRecorder, internal)
	if internalRecorder.Code != http.StatusOK || !strings.Contains(internalRecorder.Body.String(), "公開ゲスト") {
		t.Fatalf("internal request list status=%d body=%s", internalRecorder.Code, internalRecorder.Body.String())
	}
}

func TestAdminCanOpenDashboardWithoutCostDataOnPublicPage(t *testing.T) {
	app, _ := testServer(t)
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "基盤ステータス") {
		t.Fatalf("dashboard status=%d", recorder.Code)
	}

	publicRequest := httptest.NewRequest(http.MethodGet, "/public/products", nil)
	publicRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(publicRecorder, publicRequest)
	body := publicRecorder.Body.String()
	if strings.Contains(body, "仕入原価") || strings.Contains(body, "粗利率") {
		t.Fatal("public page leaked cost fields")
	}
}

func TestFormatIntegerHandlesNegativeAmounts(t *testing.T) {
	if got := formatInteger(-420000); got != "-420,000" {
		t.Fatalf("formatInteger(-420000)=%q", got)
	}
}
