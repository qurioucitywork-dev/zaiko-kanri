package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatabaseReadinessAndCacheHeaders(t *testing.T) {
	app, _ := testServer(t)

	for _, target := range []string{"/readyz", "/api/v1/health"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", target, recorder.Code, recorder.Body.String())
		}
		if cache := recorder.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") {
			t.Fatalf("%s cache-control=%q, want no-store", target, cache)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/app/admin-reference/js/api_bridge.js", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("reference bridge status=%d", recorder.Code)
	}
	if cache := recorder.Header().Get("Cache-Control"); cache != "no-cache" {
		t.Fatalf("reference bridge cache-control=%q, want no-cache", cache)
	}
}
