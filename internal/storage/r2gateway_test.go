package storage

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
)

func gatewayTestConfig(base string) config.Config {
	return config.Config{StorageDriver: "r2gateway", Environment: "test", R2GatewayURL: base + "/__storage", R2GatewayToken: strings.Repeat("a", 40)}
}

func TestR2GatewayRoundTripAuthenticatesAndEscapesKeys(t *testing.T) {
	const key = "org/product/photo 日本語 %?#.png"
	const payload = "private image bytes"
	var stored string
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 40) {
			t.Error("missing gateway authentication")
		}
		if r.URL.Path != "/__storage/"+key || r.URL.RawQuery != "" {
			t.Errorf("key changed: %s", r.URL.String())
		}
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("Content-Type") != "image/png" {
				t.Error("missing content type")
			}
			data, _ := io.ReadAll(r.Body)
			stored = string(data)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"size":%d}`, len(data))
		case http.MethodGet:
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, stored)
		case http.MethodDelete:
			stored = ""
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	store, err := New(t.Context(), gatewayTestConfig(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if store.Driver() != "s3" {
		t.Fatal("persisted storage driver must remain compatible")
	}
	n, err := store.Put(t.Context(), key, "image/png", strings.NewReader(payload))
	if err != nil || n != int64(len(payload)) {
		t.Fatalf("put: size=%d error=%v", n, err)
	}
	object, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(object.Body)
	object.Body.Close()
	if err != nil || string(content) != payload || object.ContentType != "image/png" {
		t.Fatalf("get: %q %s %v", content, object.ContentType, err)
	}
	if err := store.Delete(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	if stored != "" || strings.Join(methods, ",") != "PUT,GET,DELETE" {
		t.Fatalf("unexpected lifecycle: %v", methods)
	}
}

func TestR2GatewayRejectsTraversalAndUnsafeKeysBeforeRequests(t *testing.T) {
	store, err := newR2GatewayStore(gatewayTestConfig("http://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "/absolute", "../escape", "a/../b", "a/./b", "a//b", "a\\b", "a\nb", strings.Repeat("x", 1025)} {
		if _, err := store.objectURL(key); err == nil {
			t.Errorf("accepted unsafe key %q", key)
		}
	}
}

func TestR2GatewayErrorsAndNoCredentialRedirects(t *testing.T) {
	destinationCalled := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalled = true }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "missing"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "redirect"):
			http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
		case strings.HasSuffix(r.URL.Path, "malformed"):
			_, _ = io.WriteString(w, `{"size":-1}`)
		default:
			http.Error(w, "private upstream information", http.StatusForbidden)
		}
	}))
	defer server.Close()
	store, err := newR2GatewayStore(gatewayTestConfig(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing error: %v", err)
	}
	if _, err := store.Get(t.Context(), "redirect"); err == nil {
		t.Fatal("redirect must fail")
	}
	if destinationCalled {
		t.Fatal("storage credential request followed redirect")
	}
	if _, err := store.Put(t.Context(), "malformed", "text/plain", strings.NewReader("x")); err == nil {
		t.Fatal("invalid upload acknowledgement accepted")
	}
	for _, method := range []string{"get", "put", "delete"} {
		var err error
		switch method {
		case "get":
			_, err = store.Get(t.Context(), "forbidden")
		case "put":
			_, err = store.Put(t.Context(), "forbidden", "text/plain", strings.NewReader("x"))
		case "delete":
			err = store.Delete(t.Context(), "forbidden")
		}
		if err == nil || strings.Contains(err.Error(), "private upstream information") {
			t.Errorf("unsafe %s error: %v", method, err)
		}
	}
}
