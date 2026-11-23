package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestRequireAPIAdminRejectsWorkerAndAllowsAdmin(t *testing.T) {
	workerRequest := httptest.NewRequest(http.MethodPost, "/sensitive", nil)
	workerRequest = workerRequest.WithContext(withUser(workerRequest.Context(), database.User{
		ID: "usr_worker", OrganizationID: "org_preview", Role: database.RoleWorker,
	}))
	workerResponse := httptest.NewRecorder()
	if _, ok := requireAPIAdmin(workerResponse, workerRequest, "原価変更"); ok {
		t.Fatal("worker must not execute a sensitive operation directly")
	}
	if workerResponse.Code != http.StatusForbidden ||
		!strings.Contains(workerResponse.Body.String(), `"code":"admin_approval_required"`) {
		t.Fatalf("worker response status=%d body=%s", workerResponse.Code, workerResponse.Body.String())
	}

	adminRequest := httptest.NewRequest(http.MethodPost, "/sensitive", nil)
	adminRequest = adminRequest.WithContext(withUser(adminRequest.Context(), database.User{
		ID: "usr_admin", OrganizationID: "org_preview", Role: database.RoleAdmin,
	}))
	adminResponse := httptest.NewRecorder()
	user, ok := requireAPIAdmin(adminResponse, adminRequest, "原価変更")
	if !ok || user.ID != "usr_admin" {
		t.Fatalf("admin must be allowed: ok=%v user=%+v", ok, user)
	}
}
