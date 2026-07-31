package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPhase8MasterDialogsAndFXFormsAreWired(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	for _, path := range []string{
		"/masters?category=brands",
		"/masters?category=suppliers",
		"/masters?category=partners",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(session)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		body := recorder.Body.String()
		if recorder.Code != http.StatusOK || !strings.Contains(body, "data-master-open") {
			t.Fatalf("%s status=%d master opener missing", path, recorder.Code)
		}
		if !strings.Contains(body, `data-master-category-name="`) {
			t.Fatalf("%s master category name data is missing", path)
		}
		if strings.Contains(path, "brands") &&
			!strings.Contains(body, `<input type="hidden" name="code" data-master-code>`) {
			t.Fatalf("%s auto-code input must be hidden", path)
		}
		if strings.Contains(path, "suppliers") &&
			!strings.Contains(body, `data-master-extra="invoice_registration_number"`) {
			t.Fatalf("%s supplier extra fields missing", path)
		}
		if strings.Contains(path, "partners") &&
			!strings.Contains(body, `data-master-extra="representative_name"`) {
			t.Fatalf("%s partner extra fields missing", path)
		}
	}

	form := url.Values{
		"csrf_token": {csrf.Value}, "currency": {"EUR"}, "rate": {"166.25"},
	}
	request := httptest.NewRequest(http.MethodPost, "/masters/exchange-rates", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(session)
	request.AddCookie(csrf)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("FX save status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	rate, err := store.LatestExchangeRate(t.Context(), "org_preview", "EUR", "JPY")
	if err != nil || rate.BaseCurrency != "EUR" {
		t.Fatalf("saved FX=%+v err=%v", rate, err)
	}
}

func TestPhase8GuestBoxRenameUsesHandlerFieldNameAndPersists(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")
	boxes, err := store.GuestBoxes(t.Context(), "org_preview")
	if err != nil || len(boxes) != 10 {
		t.Fatalf("boxes=%d err=%v", len(boxes), err)
	}

	request := httptest.NewRequest(http.MethodGet, "/guest-management", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Body.String(), `name="box_name"`) {
		t.Fatalf("guest management status=%d rename field missing", recorder.Code)
	}

	form := url.Values{"csrf_token": {csrf.Value}, "box_name": {"監査済みBOX"}}
	recorder = postPhase8Form(t, app, session, csrf,
		"/guest-management/boxes/"+boxes[0].ID, form)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("box rename status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	boxes, err = store.GuestBoxes(t.Context(), "org_preview")
	if err != nil || boxes[0].Name != "監査済みBOX" {
		t.Fatalf("renamed box=%+v err=%v", boxes[0], err)
	}
}

func TestPhase8SalesAndShipmentFallbacksUseUnifiedDestinations(t *testing.T) {
	destinations := defaultSalesDestinations()
	wantCodes := []string{"B001", "B002", "B003", "B004"}
	wantNames := []string{"ウォッチマート", "タイムレス商会", "ラグジュアリーアイランド", "クロノス東京"}
	if len(destinations) != 4 {
		t.Fatalf("fallback destinations=%+v", destinations)
	}
	for index, destination := range destinations {
		if destination.Code != wantCodes[index] || destination.Name != wantNames[index] {
			t.Fatalf("fallback[%d]=%+v", index, destination)
		}
	}
}

func TestPhase8PasswordManagementRoutesPerformOperations(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	form := url.Values{
		"csrf_token": {csrf.Value}, "display_name": {"Phase 8 User"},
		"username": {"phase8-web@example.jp"}, "role": {"worker"},
		"password": {"phase8-web-password"},
	}
	recorder := postPhase8Form(t, app, session, csrf, "/masters/passwords/users", form)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("user create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	users, err := store.Users(t.Context(), "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	var userID string
	for _, user := range users {
		if user.Username == "phase8-web@example.jp" {
			userID = user.ID
		}
	}
	if userID == "" {
		t.Fatal("password management did not create user")
	}

	form = url.Values{"csrf_token": {csrf.Value}, "password": {"phase8-changed-password"}}
	recorder = postPhase8Form(t, app, session, csrf,
		"/masters/passwords/users/"+userID+"/password", form)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("password change status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := store.Authenticate(t.Context(), "PREVIEW", "phase8-web@example.jp", "phase8-changed-password"); err != nil {
		t.Fatalf("changed user credential does not work: %v", err)
	}

	form = url.Values{"csrf_token": {csrf.Value}}
	recorder = postPhase8Form(t, app, session, csrf, "/masters/passwords/guest-bulk", form)
	if recorder.Code != http.StatusSeeOther ||
		!strings.Contains(recorder.Header().Get("Location"), "category=passwords") {
		t.Fatalf("guest bulk status=%d location=%q", recorder.Code, recorder.Header().Get("Location"))
	}
	recorder = postPhase8Form(t, app, session, csrf, "/masters/passwords/notify", form)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("notification status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func postPhase8Form(t *testing.T, app *Server, session, csrf *http.Cookie, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(session)
	request.AddCookie(csrf)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}
