package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestPhase10GuestManagementUsesSingleBatchPublishAndFixedMatrix(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "admin", "preview-admin-2026")
	request := httptest.NewRequest(http.MethodGet, "/guest-management", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{
		"ゲスト公開情報を一括更新", "最終更新", "公開BOX数", "公開商品数",
		"B001", "B002", "B003", "B004", "BOX1", "BOX10", "商品なし",
		`data-guest-box-editor-dialog`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"公開設定を下書き保存", "公開プレビュー", "ゲスト企業を追加", "BOX11",
		`name="target_box_id"`, `name="company_name"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("unexpected legacy control %q", forbidden)
		}
	}
	if strings.Count(body, `action="/guest-management/publish"`) != 1 {
		t.Errorf("batch publish forms=%d", strings.Count(body, `action="/guest-management/publish"`))
	}
}

func TestPhase10GuestBoxReadAndSearchEditModals(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMasterPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	boxes, err := store.GuestBoxes(t.Context(), "org_preview")
	if err != nil || len(boxes) != 10 {
		t.Fatalf("boxes=%d err=%v", len(boxes), err)
	}
	products, err := store.Products(t.Context(), "org_preview", database.ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	targetBox := boxes[9]
	targetProduct := products[0]
	if err := store.AddGuestBoxProduct(t.Context(), "org_preview", targetBox.ID, targetProduct.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	session, csrf := loginAs(t, app, "admin", "preview-admin-2026")

	readRequest := httptest.NewRequest(http.MethodGet, "/guest-management/boxes/"+targetBox.ID+"/modal", nil)
	readRequest.Header.Set("HX-Request", "true")
	readRequest.AddCookie(session)
	readRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(readRecorder, readRequest)
	if readRecorder.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}
	readBody := readRecorder.Body.String()
	for _, want := range []string{"商品ラインナップ", "公開企業", "商品コード", "販売価格", "商品を編集", targetProduct.ProductCode} {
		if !strings.Contains(readBody, want) {
			t.Errorf("read modal missing %q", want)
		}
	}
	if strings.Contains(readBody, "<select") || strings.Contains(readBody, "一括移動") {
		t.Error("read modal contains edit controls")
	}

	unsearchedRequest := httptest.NewRequest(http.MethodGet, "/guest-management/boxes/"+targetBox.ID+"/edit-modal", nil)
	unsearchedRequest.Header.Set("HX-Request", "true")
	unsearchedRequest.AddCookie(session)
	unsearchedRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(unsearchedRecorder, unsearchedRequest)
	if unsearchedRecorder.Code != http.StatusOK {
		t.Fatalf("edit status=%d body=%s", unsearchedRecorder.Code, unsearchedRecorder.Body.String())
	}
	if strings.Contains(unsearchedRecorder.Body.String(), "data-guest-candidate") ||
		!strings.Contains(unsearchedRecorder.Body.String(), "検索条件を入力して") {
		t.Error("edit modal must not show candidates before search")
	}

	searchURL := "/guest-management/boxes/" + targetBox.ID + "/edit-modal?query=" + url.QueryEscape(targetProduct.Brand)
	searchRequest := httptest.NewRequest(http.MethodGet, searchURL, nil)
	searchRequest.Header.Set("HX-Request", "true")
	searchRequest.AddCookie(session)
	searchRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(searchRecorder, searchRequest)
	if searchRecorder.Code != http.StatusOK || !strings.Contains(searchRecorder.Body.String(), "data-guest-candidate") {
		t.Fatalf("search status=%d body=%s", searchRecorder.Code, searchRecorder.Body.String())
	}

	addForm := url.Values{
		"csrf_token": {csrf.Value},
		"product_id": {targetProduct.ID},
	}
	addRequest := httptest.NewRequest(http.MethodPost, "/guest-management/boxes/"+boxes[8].ID+"/products",
		strings.NewReader(addForm.Encode()))
	addRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addRequest.Header.Set("HX-Request", "true")
	addRequest.AddCookie(session)
	addRequest.AddCookie(csrf)
	addRecorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(addRecorder, addRequest)
	if addRecorder.Code != http.StatusOK {
		t.Fatalf("bulk add status=%d body=%s", addRecorder.Code, addRecorder.Body.String())
	}
	oldLineup, _ := store.GuestBoxProducts(t.Context(), "org_preview", targetBox.ID)
	newLineup, _ := store.GuestBoxProducts(t.Context(), "org_preview", boxes[8].ID)
	if containsGuestProduct(oldLineup, targetProduct.ID) || !containsGuestProduct(newLineup, targetProduct.ID) {
		t.Fatalf("auto move failed old=%+v new=%+v", oldLineup, newLineup)
	}
}

func TestPhase10GuestEditorJavaScriptWiresSearchAndMultiSelect(t *testing.T) {
	app, _ := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, want := range []string{
		"loadGuestBoxEditor", "data-guest-product-search-form", "data-guest-editor-form",
		"data-guest-candidate", "data-guest-bulk-add-button", "count === 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("app.js missing %q", want)
		}
	}
}

func containsGuestProduct(products []database.GuestBoxProduct, productID string) bool {
	for _, product := range products {
		if product.ProductID == productID {
			return true
		}
	}
	return false
}
