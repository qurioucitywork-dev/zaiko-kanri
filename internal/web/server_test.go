package web

import (
	"encoding/json"
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
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/storage"
)

func TestRESTAPILoginDashboardAndProducts(t *testing.T) {
	app, _ := testServer(t)
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"preview-admin-2026"}`))
	login.Header.Set("Content-Type", "application/json")
	loginRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("api login status=%d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	var loginPayload struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(loginRecorder.Body.Bytes(), &loginPayload); err != nil || loginPayload.CSRFToken == "" {
		t.Fatalf("api login payload=%s error=%v", loginRecorder.Body.String(), err)
	}
	cookies := loginRecorder.Result().Cookies()
	if len(cookies) < 2 {
		t.Fatalf("api login cookies=%d", len(cookies))
	}
	for _, endpoint := range []string{"/api/v1/auth/me", "/api/v1/dashboard", "/api/v1/products"} {
		request := httptest.NewRequest(http.MethodGet, endpoint, nil)
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s status=%d body=%s", endpoint, recorder.Code, recorder.Body.String())
		}
	}
	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	for _, cookie := range cookies {
		logout.AddCookie(cookie)
	}
	logout.Header.Set("X-CSRF-Token", loginPayload.CSRFToken)
	logoutRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(logoutRecorder, logout)
	if logoutRecorder.Code != http.StatusNoContent {
		t.Fatalf("api logout status=%d body=%s", logoutRecorder.Code, logoutRecorder.Body.String())
	}
}

func TestAdminAccessCodeIsSharedRotatableAndDoesNotElevateWorker(t *testing.T) {
	app, _ := testServer(t)

	loginAPI := func(username, password string) ([]*http.Cookie, string) {
		t.Helper()
		body := `{"username":"` + username + `","password":"` + password + `"}`
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("api login %s status=%d body=%s", username, recorder.Code, recorder.Body.String())
		}
		var payload struct {
			CSRFToken string `json:"csrfToken"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil || payload.CSRFToken == "" {
			t.Fatalf("api login payload=%s error=%v", recorder.Body.String(), err)
		}
		return recorder.Result().Cookies(), payload.CSRFToken
	}
	requestWithSession := func(method, target, body, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, target, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			request.Header.Set("X-CSRF-Token", csrf)
		}
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		return recorder
	}

	adminCookies, adminCSRF := loginAPI("admin", "preview-admin-2026")
	if _, err := app.repository.AdminAccessCode(t.Context(), "org_preview", "usr_admin"); err != nil {
		t.Fatalf("prepare access code: %v", err)
	}
	current := requestWithSession(http.MethodGet, "/api/v1/admin-access-code", "", "", adminCookies)
	if current.Code != http.StatusOK {
		t.Fatalf("get access code status=%d body=%s", current.Code, current.Body.String())
	}
	var first persistence.AdminAccessCodeRecord
	if err := json.Unmarshal(current.Body.Bytes(), &first); err != nil || !regexp.MustCompile(`^[A-Z0-9]{6}$`).MatchString(first.Code) {
		t.Fatalf("invalid access code payload=%s error=%v", current.Body.String(), err)
	}

	rotated := requestWithSession(http.MethodPost, "/api/v1/admin-access-code/rotate", `{}`, adminCSRF, adminCookies)
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotate access code status=%d body=%s", rotated.Code, rotated.Body.String())
	}
	var second persistence.AdminAccessCodeRecord
	if err := json.Unmarshal(rotated.Body.Bytes(), &second); err != nil || second.Code == first.Code {
		t.Fatalf("access code was not rotated: first=%q second=%q error=%v", first.Code, second.Code, err)
	}

	workerCookies, workerCSRF := loginAPI("worker", "preview-worker-2026")
	workerRead := requestWithSession(http.MethodGet, "/api/v1/admin-access-code", "", "", workerCookies)
	if workerRead.Code != http.StatusForbidden {
		t.Fatalf("worker read access code status=%d, want 403", workerRead.Code)
	}
	oldCode := requestWithSession(http.MethodPost, "/api/v1/admin-access-code/verify", `{"code":"`+first.Code+`"}`, workerCSRF, workerCookies)
	if oldCode.Code != http.StatusOK || !strings.Contains(oldCode.Body.String(), `"valid":false`) {
		t.Fatalf("old code verification status=%d body=%s", oldCode.Code, oldCode.Body.String())
	}
	validCode := requestWithSession(http.MethodPost, "/api/v1/admin-access-code/verify", `{"code":"`+strings.ToLower(second.Code)+`"}`, workerCSRF, workerCookies)
	if validCode.Code != http.StatusOK || !strings.Contains(validCode.Body.String(), `"valid":true`) {
		t.Fatalf("current code verification status=%d body=%s", validCode.Code, validCode.Body.String())
	}
	me := requestWithSession(http.MethodGet, "/api/v1/auth/me", "", "", workerCookies)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"role":"worker"`) {
		t.Fatalf("verification must not elevate worker session: status=%d body=%s", me.Code, me.Body.String())
	}
	workerIssue := requestWithSession(http.MethodPost, "/api/v1/purchases/pur_example/issue", `{}`, workerCSRF, workerCookies)
	if workerIssue.Code != http.StatusForbidden || !strings.Contains(workerIssue.Body.String(), `"code":"admin_required"`) {
		t.Fatalf("worker purchase issue status=%d body=%s, want admin-only 403", workerIssue.Code, workerIssue.Body.String())
	}
	workerSaleIssue := requestWithSession(http.MethodPost, "/api/v1/sales/sale_example/issue", `{}`, workerCSRF, workerCookies)
	if workerSaleIssue.Code != http.StatusForbidden || !strings.Contains(workerSaleIssue.Body.String(), `"code":"admin_required"`) {
		t.Fatalf("worker sales issue status=%d body=%s, want admin-only 403", workerSaleIssue.Code, workerSaleIssue.Body.String())
	}
}

func TestRESTCSVExportCreatesDocumentHistory(t *testing.T) {
	app, _ := testServer(t)
	if _, err := app.repository.RecordDocumentEvent(t.Context(), "org_preview", "usr_admin", persistence.DocumentEventInput{
		DocumentType: "documents", Action: "preview", OutputFormat: "html", StorageDriver: "local",
	}); err != nil {
		t.Fatalf("prepare document event: %v", err)
	}
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"preview-admin-2026"}`))
	login.Header.Set("Content-Type", "application/json")
	loginRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("api login status=%d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	var loginPayload struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(loginRecorder.Body.Bytes(), &loginPayload); err != nil {
		t.Fatal(err)
	}
	cookies := loginRecorder.Result().Cookies()

	export := httptest.NewRequest(http.MethodGet, "/api/v1/exports/inventory.csv", nil)
	for _, cookie := range cookies {
		export.AddCookie(cookie)
	}
	exportRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(exportRecorder, export)
	if exportRecorder.Code != http.StatusOK {
		t.Fatalf("csv export status=%d body=%s", exportRecorder.Code, exportRecorder.Body.String())
	}
	if disposition := exportRecorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, "zaiko-inventory-") {
		t.Fatalf("csv content disposition=%q", disposition)
	}
	body := exportRecorder.Body.Bytes()
	if len(body) < 3 || body[0] != 0xEF || body[1] != 0xBB || body[2] != 0xBF || !strings.Contains(string(body), "管理番号") {
		t.Fatalf("csv body is missing UTF-8 BOM or headers: %q", string(body))
	}

	manual := httptest.NewRequest(http.MethodPost, "/api/v1/document-events", strings.NewReader(`{"documentType":"sale","documentNumber":"INV-TEST","action":"print","outputFormat":"html","metadata":{"source":"test"}}`))
	manual.Header.Set("Content-Type", "application/json")
	manual.Header.Set("X-CSRF-Token", loginPayload.CSRFToken)
	for _, cookie := range cookies {
		manual.AddCookie(cookie)
	}
	manualRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(manualRecorder, manual)
	if manualRecorder.Code != http.StatusCreated {
		t.Fatalf("document event status=%d body=%s", manualRecorder.Code, manualRecorder.Body.String())
	}

	history := httptest.NewRequest(http.MethodGet, "/api/v1/document-events?limit=10", nil)
	for _, cookie := range cookies {
		history.AddCookie(cookie)
	}
	historyRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(historyRecorder, history)
	if historyRecorder.Code != http.StatusOK || !strings.Contains(historyRecorder.Body.String(), "INV-TEST") || !strings.Contains(historyRecorder.Body.String(), `"documentType":"inventory"`) {
		t.Fatalf("document history status=%d body=%s", historyRecorder.Code, historyRecorder.Body.String())
	}
}

func testServer(t *testing.T) (*Server, *database.Store) {
	t.Helper()
	tempDir := t.TempDir()
	databasePath := tempDir + "/web.db"
	store, err := database.Open("file:" + databasePath)
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
	cfg := config.Config{
		Environment: "test", SessionTTL: time.Hour, OrganizationCode: "PREVIEW",
		DatabaseDriver: "sqlite", DatabasePath: databasePath,
		StorageDriver: "local", UploadDirectory: tempDir + "/uploads",
	}
	repository, err := persistence.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	objects, err := storage.New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(cfg, store, repository, objects, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestRootRedirectsToReactApplication(t *testing.T) {
	app, _ := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTemporaryRedirect || recorder.Header().Get("Location") != "/app/" {
		t.Fatalf("status=%d location=%q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestReferenceAdminIsLoadedByReactWithoutIframe(t *testing.T) {
	app, _ := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/app/admin-reference/app.html", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("reference admin status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options=%q, want DENY", got)
	}
	csp := recorder.Header().Get("Content-Security-Policy")
	for _, expected := range []string{"frame-ancestors 'none'", "'unsafe-inline'", "https://cdn.jsdelivr.net", "img-src 'self' data: blob: https://sspark.genspark.ai"} {
		if !strings.Contains(csp, expected) {
			t.Fatalf("reference CSP %q does not contain %q", csp, expected)
		}
	}
	for _, directive := range strings.Split(csp, ";") {
		if strings.HasPrefix(strings.TrimSpace(directive), "script-src") && strings.Contains(directive, "genspark.ai") {
			t.Fatalf("reference script-src must not trust Genspark inspection script: %q", csp)
		}
	}
}

func TestLegacyReactRouteRedirectsToCanonicalReferencePage(t *testing.T) {
	app, _ := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/app/market-prices?source=legacy", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusTemporaryRedirect)
	}
	location := recorder.Header().Get("Location")
	if location != "/app/?page=market&source=legacy" {
		t.Fatalf("location=%q", location)
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

func TestWorkerCannotOpenApprovalManagement(t *testing.T) {
	app, _ := testServer(t)
	session, _ := loginAs(t, app, "worker", "preview-worker-2026")

	request := httptest.NewRequest(http.MethodGet, "/approvals", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("worker approvals status=%d, want 403", recorder.Code)
	}

	dashboardRequest := httptest.NewRequest(http.MethodGet, "/app/", nil)
	dashboardRequest.AddCookie(session)
	dashboardRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(dashboardRecorder, dashboardRequest)
	if strings.Contains(dashboardRecorder.Body.String(), `href="/approvals"`) {
		t.Fatal("worker navigation must not show approval management")
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

func TestWorkerCanLoadCancelledProductsForInventoryStatusSearch(t *testing.T) {
	for _, role := range []string{database.RoleAdmin, database.RoleWorker} {
		if !canViewCancelledInventory(role, "true") {
			t.Fatalf("%s must be able to request cancelled inventory", role)
		}
	}
	if canViewCancelledInventory(database.RoleGuest, "true") {
		t.Fatal("guest must not be able to request cancelled inventory")
	}
	if canViewCancelledInventory(database.RoleWorker, "false") {
		t.Fatal("worker request without includeCancelled must not include cancelled inventory")
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
	request := httptest.NewRequest(http.MethodGet, "/legacy", nil)
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
