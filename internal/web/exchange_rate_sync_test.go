package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestSMBCSyncAdminOnly(t *testing.T) {
	for _, role := range []string{database.RoleWorker, "guest", ""} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/exchange-rates/sync", nil)
		request = request.WithContext(withUser(request.Context(), database.User{ID: "user", OrganizationID: "org", Role: role}))
		response := httptest.NewRecorder()
		s := &Server{}
		s.apiExchangeRateSync(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: %d", role, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/exchange-rates/sync", nil)
	request = request.WithContext(withUser(request.Context(), database.User{ID: "admin", OrganizationID: "org", Role: database.RoleAdmin}))
	response := httptest.NewRecorder()
	s := &Server{}
	s.apiExchangeRateSync(response, request)
	if response.Code != http.StatusConflict {
		t.Fatal(response.Code)
	}
}
