package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func publishTwoProductsForGuest(t *testing.T, store *database.Store) (database.GuestCompany, []database.Product) {
	t.Helper()
	ctx := t.Context()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedGuestCatalogPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(ctx); err != nil {
		t.Fatal(err)
	}
	companies, err := store.GuestCompanies(ctx, "org_preview")
	if err != nil || len(companies) == 0 {
		t.Fatalf("companies=%d err=%v", len(companies), err)
	}
	boxes, err := store.GuestBoxes(ctx, "org_preview")
	if err != nil || len(boxes) == 0 {
		t.Fatalf("boxes=%d err=%v", len(boxes), err)
	}
	products, err := store.Products(ctx, "org_preview", database.ProductFilter{})
	if err != nil || len(products) < 2 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	if err := store.AddGuestBoxProducts(ctx, "org_preview", boxes[0].ID,
		[]string{products[0].ID, products[1].ID}, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	for index, company := range companies {
		if err := store.SaveGuestBoxDraft(ctx, "org_preview", company.ID, boxes[0].ID, "usr_admin", index == 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.PublishGuestBoxSnapshot(ctx, "org_preview", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	return companies[0], products[:2]
}

func TestPhase14GuestCartSubmitsMultipleProductsAsOneRequestGroup(t *testing.T) {
	app, store := testServer(t)
	company, products := publishTwoProductsForGuest(t, store)
	guestSession := loginGuestAs(t, app, company.Code)

	get := httptest.NewRequest(http.MethodGet, "/public/products", nil)
	get.AddCookie(guestSession)
	getRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(getRecorder, get)
	body := getRecorder.Body.String()
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("catalog status=%d body=%s", getRecorder.Code, body)
	}
	for _, expected := range []string{
		`data-guest-filter`, `action="/public/purchase-requests"`, `data-guest-cart-items`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %q in catalog", expected)
		}
	}
	if strings.Contains(body, `type="submit">検索</button>`) {
		t.Fatal("mock-external visible search button remains")
	}
	scriptRequest := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	scriptRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(scriptRecorder, scriptRequest)
	script := scriptRecorder.Body.String()
	for _, expected := range []string{
		"let guestCart = []", "guestCart.some", "guestCart.findIndex",
		"dataset.guestCartRemove", `productID.name = "product_id"`,
		`guestFilter.querySelectorAll("select")`, "setTimeout(submitFilter, 300)",
	} {
		if scriptRecorder.Code != http.StatusOK || !strings.Contains(script, expected) {
			t.Fatalf("guest cart script missing %q status=%d", expected, scriptRecorder.Code)
		}
	}
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	if len(csrfMatch) != 2 {
		t.Fatal("purchase csrf token missing")
	}
	form := url.Values{
		"csrf_token":  {csrfMatch[1]},
		"product_id":  {products[0].ID, products[1].ID, products[0].ID},
		"guest_name":  {"ゲスト会社"},
		"guest_email": {"guest@example.com"},
		"message":     {"2点を一括購入依頼"},
	}
	post := httptest.NewRequest(http.MethodPost, "/public/purchase-requests", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(guestSession)
	postRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(postRecorder, post)
	if postRecorder.Code != http.StatusSeeOther {
		t.Fatalf("request status=%d body=%s", postRecorder.Code, postRecorder.Body.String())
	}
	groups, err := store.PurchaseRequestGroups(t.Context(), "org_preview", "pending")
	if err != nil {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
	var submitted []database.PurchaseRequest
	for _, group := range groups {
		if len(group.Items) > 0 && group.Items[0].GuestEmail == "guest@example.com" {
			submitted = group.Items
		}
	}
	if len(submitted) != 2 || submitted[0].RequestGroupID != submitted[1].RequestGroupID {
		t.Fatalf("submitted request group=%+v all=%+v", submitted, groups)
	}
}
