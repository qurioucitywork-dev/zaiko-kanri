package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func loginGuestAs(t *testing.T, app *Server, login string) *http.Cookie {
	t.Helper()
	get := httptest.NewRequest(http.MethodGet, "/guest/login", nil)
	getRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(getRecorder, get)
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(getRecorder.Body.String())
	if len(csrfMatch) != 2 {
		t.Fatal("guest login csrf token missing")
	}
	form := url.Values{"guest_id": {login}, "password": {"guest-preview-2026"}, "csrf_token": {csrfMatch[1]}}
	post := httptest.NewRequest(http.MethodPost, "/guest/login", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, post)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("guest login status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var session *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		switch cookie.Name {
		case guestSessionCookie:
			session = cookie
		}
	}
	if session == nil {
		t.Fatal("guest session cookies missing")
	}
	return session
}

func publishProductForGuestCompany(t *testing.T, store *database.Store, companyIndex int) (database.GuestCompany, database.Product, database.ProductImage) {
	t.Helper()
	ctx := t.Context()
	if err := store.SeedMasterPreview(ctx); err != nil {
		t.Fatal(err)
	}
	companies, err := store.GuestCompanies(ctx, "org_preview")
	if err != nil || len(companies) <= companyIndex {
		t.Fatalf("companies=%d err=%v", len(companies), err)
	}
	boxes, err := store.GuestBoxes(ctx, "org_preview")
	if err != nil || len(boxes) == 0 {
		t.Fatalf("boxes=%d err=%v", len(boxes), err)
	}
	products, err := store.Products(ctx, "org_preview", database.ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	product := products[0]
	image, err := store.AddProductImage(ctx, database.ProductImage{
		ProductID: product.ID, StoragePath: "guest-security/image.jpg", OriginalName: "image.jpg",
		ContentType: "image/jpeg", SizeBytes: 4,
	}, "org_preview", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddGuestBoxProduct(ctx, "org_preview", boxes[0].ID, product.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	for index, company := range companies {
		if err := store.SaveGuestBoxDraft(ctx, "org_preview", company.ID, boxes[0].ID, "usr_admin", index == companyIndex); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.PublishGuestBoxSnapshot(ctx, "org_preview", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	return companies[companyIndex], product, image
}

func TestPhase10PublicCatalogUsesAuthenticatedGuestCompanyNotQuery(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	company, product, _ := publishProductForGuestCompany(t, store, 0)

	anonymous := httptest.NewRecorder()
	app.Handler().ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/public/products?guest_company="+company.Code, nil))
	if anonymous.Code != http.StatusSeeOther || anonymous.Header().Get("Location") != "/guest/login" {
		t.Fatalf("anonymous status=%d location=%q", anonymous.Code, anonymous.Header().Get("Location"))
	}

	b001 := loginGuestAs(t, app, company.Code)
	allowedRequest := httptest.NewRequest(http.MethodGet, "/public/products?guest_company=B002", nil)
	allowedRequest.AddCookie(b001)
	allowed := httptest.NewRecorder()
	app.Handler().ServeHTTP(allowed, allowedRequest)
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), product.ID) ||
		!strings.Contains(allowed.Body.String(), company.Name) {
		t.Fatalf("authenticated catalog status=%d body=%s", allowed.Code, allowed.Body.String())
	}

	b002 := loginGuestAs(t, app, "B002")
	deniedRequest := httptest.NewRequest(http.MethodGet, "/public/products?guest_company="+company.Code, nil)
	deniedRequest.AddCookie(b002)
	denied := httptest.NewRecorder()
	app.Handler().ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusOK || strings.Contains(denied.Body.String(), product.ID) {
		t.Fatalf("query parameter crossed company boundary status=%d body=%s", denied.Code, denied.Body.String())
	}
}

func TestPhase10PublishedImageAndPurchaseRequestAreCompanyScoped(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	company, product, image := publishProductForGuestCompany(t, store, 0)
	fullPath := filepath.Join(app.cfg.UploadDirectory, image.StoragePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte{1, 2, 3, 4}, 0o600); err != nil {
		t.Fatal(err)
	}
	b001 := loginGuestAs(t, app, company.Code)
	imageRequest := httptest.NewRequest(http.MethodGet, "/public/companies/"+company.Code+"/product-images/"+image.ID, nil)
	imageRequest.AddCookie(b001)
	imageRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(imageRecorder, imageRequest)
	if imageRecorder.Code != http.StatusOK {
		t.Fatalf("published image status=%d body=%s", imageRecorder.Code, imageRecorder.Body.String())
	}

	b002 := loginGuestAs(t, app, "B002")
	for _, path := range []string{
		"/public/companies/" + company.Code + "/product-images/" + image.ID,
		"/public/companies/B002/product-images/" + image.ID,
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(b002)
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("image IDOR path=%s status=%d", path, recorder.Code)
		}
	}

	catalogRequest := httptest.NewRequest(http.MethodGet, "/public/products", nil)
	catalogRequest.AddCookie(b002)
	catalog := httptest.NewRecorder()
	app.Handler().ServeHTTP(catalog, catalogRequest)
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(catalog.Body.String())
	if len(csrfMatch) != 2 {
		t.Fatal("catalog csrf token missing")
	}
	form := url.Values{
		"csrf_token": {csrfMatch[1]}, "guest_name": {"B002 guest"},
		"guest_email": {"b002@example.com"}, "message": {"cross-company request"},
	}
	request := httptest.NewRequest(http.MethodPost, "/public/products/"+product.ID+"/purchase-requests", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-CSRF-Token", csrfMatch[1])
	request.AddCookie(b002)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cross-company purchase status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
