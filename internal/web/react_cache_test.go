package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedAssetContentETagAndRevalidation(t *testing.T) {
	for _, asset := range []string{
		"react-dist/admin-reference/js/api_bridge.js",
		"react-dist/admin-reference/css/style.css",
		"react-dist/admin-reference/app.html",
	} {
		t.Run(asset, func(t *testing.T) {
			expected, err := reactAssets.ReadFile(asset)
			if err != nil {
				t.Fatal(err)
			}
			first := httptest.NewRecorder()
			if !serveEmbeddedAsset(first, httptest.NewRequest(http.MethodGet, "/app/asset", nil), asset) {
				t.Fatal("asset missing")
			}
			if first.Code != http.StatusOK || !bytes.Equal(first.Body.Bytes(), expected) {
				t.Fatal("initial asset response changed")
			}
			etag := first.Header().Get("ETag")
			if !strings.HasPrefix(etag, `W/"`) || !strings.HasSuffix(etag, `"`) || first.Header().Get("Cache-Control") != "no-cache" {
				t.Fatalf("invalid revalidation headers: %v", first.Header())
			}
			if first.Header().Get("Content-Type") == "" || first.Header().Get("Content-Encoding") != "" || first.Header().Get("Vary") != "" {
				t.Fatal("content type or wire encoding behavior changed")
			}

			request := httptest.NewRequest(http.MethodGet, "/app/asset", nil)
			request.Header.Set("If-None-Match", etag)
			unchanged := httptest.NewRecorder()
			serveEmbeddedAsset(unchanged, request, asset)
			if unchanged.Code != http.StatusNotModified || unchanged.Body.Len() != 0 {
				t.Fatalf("unchanged asset status=%d bytes=%d", unchanged.Code, unchanged.Body.Len())
			}
			if unchanged.Header().Get("ETag") != etag || unchanged.Header().Get("Cache-Control") != "no-cache" {
				t.Fatal("304 lost revalidation headers")
			}

			oldContent := append(append([]byte{}, expected...), []byte("\nprevious-build")...)
			oldETag := staticContentETag(oldContent)
			if oldETag == etag {
				t.Fatal("changed contents retained the same validator")
			}
			request = httptest.NewRequest(http.MethodGet, "/app/asset", nil)
			request.Header.Set("If-None-Match", oldETag)
			changed := httptest.NewRecorder()
			serveEmbeddedAsset(changed, request, asset)
			if changed.Code != http.StatusOK || !bytes.Equal(changed.Body.Bytes(), expected) || changed.Header().Get("ETag") != etag {
				t.Fatal("stale browser validator did not receive current contents")
			}
		})
	}
}

func TestEmbeddedAssetHeadAndRangesStillUseServeContent(t *testing.T) {
	const asset = "react-dist/admin-reference/js/api_bridge.js"
	content, err := reactAssets.ReadFile(asset)
	if err != nil {
		t.Fatal(err)
	}
	head := httptest.NewRecorder()
	serveEmbeddedAsset(head, httptest.NewRequest(http.MethodHead, "/app/asset", nil), asset)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" || head.Header().Get("ETag") == "" {
		t.Fatalf("HEAD changed: status=%d headers=%v", head.Code, head.Header())
	}
	request := httptest.NewRequest(http.MethodGet, "/app/asset", nil)
	request.Header.Set("Range", "bytes=0-15")
	partial := httptest.NewRecorder()
	serveEmbeddedAsset(partial, request, asset)
	if partial.Code != http.StatusPartialContent || !bytes.Equal(partial.Body.Bytes(), content[:16]) || partial.Header().Get("Content-Range") == "" {
		t.Fatalf("range changed: status=%d headers=%v", partial.Code, partial.Header())
	}
}

func TestStaticETagDoesNotCacheBusinessAPI(t *testing.T) {
	app, _ := testServer(t)
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	for _, endpoint := range []string{"/api/v1/auth/me", "/api/v1/dashboard", "/api/v1/products"} {
		request := httptest.NewRequest(http.MethodGet, endpoint, nil)
		request.AddCookie(session)
		request.Header.Set("If-None-Match", "*")
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || recorder.Body.Len() == 0 {
			t.Fatalf("business API %s response changed: %d", endpoint, recorder.Code)
		}
		if recorder.Header().Get("ETag") != "" || !strings.Contains(recorder.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("business API %s received static caching headers: %v", endpoint, recorder.Header())
		}
	}
}
