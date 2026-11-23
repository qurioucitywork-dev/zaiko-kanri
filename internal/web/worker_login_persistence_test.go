package web

import (
	"encoding/json"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/storage"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkerLoginPersistencePostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_WORKER_LOGIN_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_worker_login_test") {
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
	admin, csrf := login("admin", "worker-admin-test-password", 200)
	w := request("POST", "/users", csrf, admin, map[string]any{"username": "test-worker", "displayName": "メールなし作業員", "email": "", "role": "worker", "password": "initial-password"}, 201)
	var created persistence.UserRecord
	must(json.Unmarshal(w.Body.Bytes(), &created))
	if created.ID == "" || created.Email != "" || created.Role != "worker" {
		t.Fatal("bad worker record")
	}
	worker, _ := login("test-worker", "initial-password", 200)
	request("GET", "/auth/me", "", worker, nil, 200)
	request("GET", "/products", "", worker, nil, 200)
	request("GET", "/users", "", worker, nil, 403)
	// A fresh server/repository reads persisted records, not browser memory.
	fresh, err := persistence.Open(cfg)
	must(err)
	defer fresh.Close()
	saved, err := fresh.User(t.Context(), createdOrganization(t, fresh), created.ID)
	must(err)
	if saved.Username != "test-worker" {
		t.Fatal("save lost")
	}
	request("PATCH", "/users/"+created.ID, csrf, admin, map[string]any{"username": "renamed-worker", "displayName": "更新した作業員", "email": "", "isActive": true, "password": "updated-password"}, 200)
	login("test-worker", "initial-password", 401)
	login("renamed-worker", "initial-password", 401)
	worker, _ = login("renamed-worker", "updated-password", 200)
	request("GET", "/auth/me", "", worker, nil, 200)
	// Invalid credentials must roll back the accompanying profile change.
	request("PATCH", "/users/"+created.ID, csrf, admin, map[string]any{"displayName": "must-not-save", "password": "short"}, 400)
	w = request("GET", "/users?includeInactive=true", "", admin, nil, 200)
	if strings.Contains(w.Body.String(), "must-not-save") || strings.Contains(w.Body.String(), "updated-password") || !strings.Contains(w.Body.String(), "更新した作業員") {
		t.Fatal("partial save or password disclosure")
	}
	// Master PW button preserves literal whitespace and revokes old sessions.
	request("POST", "/users/"+created.ID+"/password", csrf, admin, map[string]string{"password": " last-password "}, 204)
	request("GET", "/auth/me", "", worker, nil, 401)
	worker, _ = login("renamed-worker", " last-password ", 200)
	request("GET", "/products", "", worker, nil, 200)
	request("PATCH", "/users/"+created.ID, csrf, admin, map[string]any{"isActive": false}, 200)
	login("renamed-worker", " last-password ", 401)
	request("PATCH", "/users/"+created.ID, csrf, admin, map[string]any{"isActive": true}, 200)
	login("renamed-worker", " last-password ", 200)
}

func createdOrganization(t *testing.T, repo *persistence.Repository) string {
	t.Helper()
	u, e := repo.Authenticate(t.Context(), "PREVIEW", "admin", "worker-admin-test-password")
	if e != nil {
		t.Fatal(e)
	}
	return u.OrganizationID
}
