package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPartRoutesRequireAuthentication(t *testing.T) {
	app, _ := testServer(t)
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/parts"}, {"POST", "/api/v1/parts"}, {"PATCH", "/api/v1/parts/test-part"}} {
		request := httptest.NewRequest(route.method, route.path, nil)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s returned %d, want 401 (registered and protected)", route.method, route.path, response.Code)
		}
	}
}
