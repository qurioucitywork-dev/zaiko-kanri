package web

import (
	"encoding/json"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGuestInquiryPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_GUEST_INQUIRY_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_guest_inquiry_test") {
		t.Fatal("non-test database rejected")
	}
	cfg := config.Config{Environment: "test", DatabaseDriver: "postgres", DatabaseURL: dsn, OrganizationCode: "PREVIEW", SessionTTL: time.Hour, StorageDriver: "local", UploadDirectory: t.TempDir()}
	repo, err := persistence.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(repo.Migrate(t.Context()))
	must(repo.SeedPreviewIdentity(t.Context(), "worker-admin-test-password", "worker-initial-test-password"))
	must(repo.SeedPreviewMasters(t.Context()))
	objects, err := storage.New(t.Context(), cfg)
	must(err)
	app, err := New(cfg, nil, repo, objects, slog.New(slog.NewTextHandler(io.Discard, nil)))
	must(err)
	request := func(method, path, csrf string, cookies []*http.Cookie, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(payload)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	login := func(username, password string, want int) ([]*http.Cookie, string) {
		t.Helper()
		w := request("POST", "/auth/login", "", nil, map[string]string{"username": username, "password": password}, want)
		var p struct {
			CSRFToken string `json:"csrfToken"`
		}
		must(json.Unmarshal(w.Body.Bytes(), &p))
		return w.Result().Cookies(), p.CSRFToken
	}

	adminCookies, csrf := login("admin", "worker-admin-test-password", 200)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	must(err)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	var org, admin, brand string
	must(db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	must(db.Raw("SELECT id FROM users WHERE username='admin'").Scan(&admin).Error)
	must(db.Raw("SELECT code FROM brands ORDER BY code LIMIT 1").Scan(&brand).Error)
	for _, code := range []string{"G901", "G902"} {
		request("POST", "/guest-logins", csrf, adminCookies, map[string]any{"guestCode": code, "buyerCode": "B" + code[1:], "displayName": code, "company": "Image Test", "address": "Test address", "email": code + "@example.invalid", "password": "image-test-password", "isActive": true}, 200)
	}
	guest, gcsrf := login("G901", "image-test-password", 200)
	other, ocsrf := login("G902", "image-test-password", 200)
	worker, wcsrf := login("worker", "worker-initial-test-password", 200)

	input := map[string]string{"clientRequestId": "inquiry-browser-request-001", "subject": " 商品について ", "productCode": "1809260001", "message": "状態を教えてください。\n<script>not executable</script>"}
	request("POST", "/guest/inquiries", wcsrf, worker, input, 403)
	request("POST", "/guest/inquiries", csrf, adminCookies, input, 403)
	request("POST", "/guest/inquiries", "", nil, input, 401)
	request("POST", "/guest/inquiries", gcsrf, guest, map[string]string{"clientRequestId": "invalid-message", "subject": " ", "message": " "}, 400)
	request("POST", "/guest/inquiries", gcsrf, guest, map[string]string{"clientRequestId": "invalid-message", "subject": "件名", "message": strings.Repeat("x", 5001)}, 400)
	w := request("POST", "/guest/inquiries", gcsrf, guest, input, 201)
	var created struct {
		ID string `json:"id"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &created))
	if created.ID == "" {
		t.Fatal("missing inquiry id")
	}
	w = request("POST", "/guest/inquiries", gcsrf, guest, input, 201)
	var retry struct {
		ID string `json:"id"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &retry))
	if retry.ID != created.ID {
		t.Fatal("duplicate inquiry created")
	}
	changed := map[string]string{"clientRequestId": input["clientRequestId"], "subject": "different", "message": "changed"}
	request("POST", "/guest/inquiries", gcsrf, guest, changed, 400)
	for _, cookies := range [][]*http.Cookie{guest, other, worker} {
		request("GET", "/inquiries/"+created.ID, "", cookies, nil, 403)
	}
	w = request("GET", "/inquiries/"+created.ID, "", adminCookies, nil, 200)
	var saved persistence.GuestInquiryRecord
	must(json.Unmarshal(w.Body.Bytes(), &saved))
	if saved.Subject != "商品について" || saved.Message != input["message"] || saved.GuestCode != "G901" || saved.Email != "G901@example.invalid" && saved.Email != "g901@example.invalid" {
		t.Fatalf("incorrect saved inquiry %+v", saved)
	}
	var count int64
	must(db.Table("guest_inquiries").Where("id=?", created.ID).Count(&count).Error)
	if count != 1 {
		t.Fatal("inquiry not persisted")
	}
	must(db.Table("notifications").Where("target_type='guest_inquiry' AND target_id=?", created.ID).Count(&count).Error)
	if count != 1 {
		t.Fatal("duplicate/missing admin notification")
	}
	w = request("GET", "/notifications?unread=true&limit=200", "", adminCookies, nil, 200)
	if !strings.Contains(w.Body.String(), created.ID) {
		t.Fatal("admin notification missing")
	}
	w = request("GET", "/notifications?unread=true&limit=200", "", worker, nil, 200)
	if strings.Contains(w.Body.String(), created.ID) {
		t.Fatal("inquiry notification leaked to worker")
	}
	// A different guest may reuse a client key without learning another guest's inquiry.
	w = request("POST", "/guest/inquiries", ocsrf, other, input, 201)
	must(json.Unmarshal(w.Body.Bytes(), &retry))
	if retry.ID == created.ID {
		t.Fatal("guest idempotency scope leaked")
	}
	reloaded, e := persistence.Open(cfg)
	must(e)
	defer reloaded.Close()
	stored, e := reloaded.GuestInquiry(t.Context(), org, created.ID)
	must(e)
	if stored.Message != input["message"] {
		t.Fatal("inquiry lost on reload")
	}
	if _, e := reloaded.GuestInquiry(t.Context(), "different-org", created.ID); e == nil {
		t.Fatal("organization isolation failed")
	}
}
