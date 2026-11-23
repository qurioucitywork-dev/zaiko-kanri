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

func TestWorkerApprovalNotificationsPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_WORKER_NOTIFICATION_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_worker_notification_test") {
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
	worker, workerCSRF := login("worker", "worker-initial-test-password", 200)

	other, _ := login("worker2", "worker-initial-test-password", 200)
	for _, decision := range []string{"approved", "returned", "rejected"} {
		w := request("POST", "/approvals", workerCSRF, worker, map[string]string{"approvalType": "閲覧申請", "targetType": "access_request", "targetId": "notification-test-" + decision, "requestedAction": "access.approve"}, 201)
		var approval persistence.ApprovalRequestRecord
		must(json.Unmarshal(w.Body.Bytes(), &approval))
		request("POST", "/approvals/"+approval.ID+"/"+decision, csrf, admin, map[string]string{"note": "管理者コメント " + decision}, 200)
		var notifications struct {
			Items []persistence.NotificationRecord `json:"items"`
		}
		w = request("GET", "/notifications?unread=true&limit=200", "", worker, nil, 200)
		must(json.Unmarshal(w.Body.Bytes(), &notifications))
		var notificationID string
		for _, n := range notifications.Items {
			if n.TargetID == approval.ID && n.EventKey == "approval."+decision {
				notificationID = n.ID
			}
		}
		if notificationID == "" {
			t.Fatalf("missing %s notification", decision)
		}
		w = request("GET", "/notifications?unread=true&limit=200", "", other, nil, 200)
		if strings.Contains(w.Body.String(), notificationID) {
			t.Fatal("worker notification leaked")
		}
		request("POST", "/notifications/"+notificationID+"/read", workerCSRF, worker, map[string]any{}, 204)
		w = request("GET", "/notifications?unread=true&limit=200", "", worker, nil, 200)
		if strings.Contains(w.Body.String(), notificationID) {
			t.Fatal("read did not persist")
		}
		var history struct {
			Items []persistence.ApprovalRequestRecord `json:"items"`
		}
		w = request("GET", "/approvals", "", worker, nil, 200)
		must(json.Unmarshal(w.Body.Bytes(), &history))
		found := false
		for _, a := range history.Items {
			if a.ID == approval.ID {
				found = true
				if a.Status != decision || a.DecisionNote != "管理者コメント "+decision || a.DecidedAt == nil || a.DeciderName == "" {
					t.Fatalf("incomplete result %+v", a)
				}
			}
		}
		if !found {
			t.Fatal("read result disappeared from history")
		}
		w = request("GET", "/approvals", "", other, nil, 200)
		if strings.Contains(w.Body.String(), approval.ID) {
			t.Fatal("worker approval leaked")
		}
	}
}
