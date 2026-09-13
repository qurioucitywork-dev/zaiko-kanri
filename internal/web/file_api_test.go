package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func TestProductImageCanBeUploadedFromProductRegistration(t *testing.T) {
	app, store := testServer(t)
	cookies, csrf := loginAPIForFileTest(t, app)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatalf("seed product for image upload: %v", err)
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("find product for image upload: products=%#v error=%v", products, err)
	}
	productID := products[0].ID

	// http.DetectContentType recognizes the PNG signature. The object store does
	// not need a decodable bitmap to verify the upload/persistence contract.
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R'}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "registered-product.png")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(png); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/products/"+productID+"/files", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-CSRF-Token", csrf)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	files, err := app.repository.ProductFiles(t.Context(), "org_preview", productID)
	if err != nil || len(files) != 1 {
		t.Fatalf("uploaded file was not persisted: files=%#v error=%v", files, err)
	}
	if files[0].OriginalName != "registered-product.png" || files[0].ContentType != "image/png" {
		t.Fatalf("unexpected uploaded metadata: %#v", files[0])
	}
}

func TestProductImagesCanBeReorderedAndDeleted(t *testing.T) {
	app, _ := testServer(t)
	cookies, csrf := loginAPIForFileTest(t, app)

	for index, id := range []string{"fil_edit_1", "fil_edit_2", "fil_edit_3"} {
		key := "test/product_preview_001/" + id + ".jpg"
		if _, err := app.objects.Put(t.Context(), key, "image/jpeg", strings.NewReader("image")); err != nil {
			t.Fatalf("put test object %s: %v", id, err)
		}
		if _, err := app.repository.CreateProductFile(t.Context(), "org_preview", "usr_admin", persistence.ProductFileRecord{
			ID: id, ProductID: "product_preview_001", StorageDriver: "local", ObjectKey: key,
			OriginalName: id + ".jpg", ContentType: "image/jpeg", SizeBytes: 5, SHA256: "test",
		}); err != nil {
			t.Fatalf("create product file %d: %v", index, err)
		}
	}

	stale := authenticatedFileRequest(t, app, http.MethodPut, "/api/v1/products/product_preview_001/files/order",
		`{"fileIds":["fil_edit_1"]}`, csrf, cookies)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale reorder status=%d body=%s", stale.Code, stale.Body.String())
	}

	reordered := authenticatedFileRequest(t, app, http.MethodPut, "/api/v1/products/product_preview_001/files/order",
		`{"fileIds":["fil_edit_3","fil_edit_1","fil_edit_2"]}`, csrf, cookies)
	if reordered.Code != http.StatusOK {
		t.Fatalf("reorder status=%d body=%s", reordered.Code, reordered.Body.String())
	}
	files, err := app.repository.ProductFiles(t.Context(), "org_preview", "product_preview_001")
	if err != nil || len(files) != 3 || files[0].ID != "fil_edit_3" || files[1].ID != "fil_edit_1" || files[2].ID != "fil_edit_2" {
		t.Fatalf("unexpected reordered files=%#v error=%v", files, err)
	}

	deleted := authenticatedFileRequest(t, app, http.MethodDelete, "/api/v1/product-files/fil_edit_1", "", csrf, cookies)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	files, err = app.repository.ProductFiles(t.Context(), "org_preview", "product_preview_001")
	if err != nil || len(files) != 2 || files[0].ID != "fil_edit_3" || files[0].SortOrder != 0 || files[1].ID != "fil_edit_2" || files[1].SortOrder != 1 {
		t.Fatalf("unexpected compacted files=%#v error=%v", files, err)
	}
	missing := authenticatedFileRequest(t, app, http.MethodGet, "/api/v1/product-files/fil_edit_1", "", "", cookies)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted file remains readable: status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func loginAPIForFileTest(t *testing.T, app *Server) ([]*http.Cookie, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"preview-admin-2026"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil || payload.CSRFToken == "" {
		t.Fatalf("login payload=%s error=%v", recorder.Body.String(), err)
	}
	return recorder.Result().Cookies(), payload.CSRFToken
}

func authenticatedFileRequest(t *testing.T, app *Server, method, target, body, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}
