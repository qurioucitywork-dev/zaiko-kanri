package web

import (
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuestLoginSaveAccess(t *testing.T) {
	for _, role := range []string{database.RoleWorker, database.RoleGuest} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/guest-logins", strings.NewReader(`{}`))
		r = r.WithContext(withUser(r.Context(), database.User{ID: "actor", OrganizationID: "org", Role: role}))
		w := httptest.NewRecorder()
		(&Server{}).apiGuestLoginSave(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d", role, w.Code)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/guest-logins", strings.NewReader(`{"role":"admin"}`))
	r = r.WithContext(withUser(r.Context(), database.User{ID: "actor", OrganizationID: "org", Role: database.RoleAdmin}))
	w := httptest.NewRecorder()
	(&Server{}).apiGuestLoginSave(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown privilege field status=%d", w.Code)
	}
}
