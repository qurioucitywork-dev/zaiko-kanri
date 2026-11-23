package storage

import (
	"io"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
)

func TestLocalStoreRoundTripAndTraversalProtection(t *testing.T) {
	store, err := New(t.Context(), config.Config{StorageDriver: "local", UploadDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	written, err := store.Put(t.Context(), "org/product/image.txt", "text/plain", strings.NewReader("inventory"))
	if err != nil || written != 9 {
		t.Fatalf("put written=%d error=%v", written, err)
	}
	object, err := store.Get(t.Context(), "org/product/image.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer object.Body.Close()
	content, err := io.ReadAll(object.Body)
	if err != nil || string(content) != "inventory" {
		t.Fatalf("content=%q error=%v", content, err)
	}
	if _, err := store.Put(t.Context(), "../../escape.txt", "text/plain", strings.NewReader("bad")); err == nil {
		t.Fatal("path traversal must be rejected")
	}
}
