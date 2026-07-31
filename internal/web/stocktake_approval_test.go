package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func stocktakePost(t *testing.T, app *Server, path string, session, csrf *http.Cookie, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	values.Set("csrf_token", csrf.Value)
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(session)
	request.AddCookie(csrf)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestStocktakeHTTPApprovalReturnAndResubmitFlow(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	stocktake, err := store.CreateStocktake(t.Context(), "org_preview", "2026-07-30", "", "usr_worker")
	if err != nil {
		t.Fatal(err)
	}
	line := stocktake.Lines[0]
	workerSession, workerCSRF := loginAs(t, app, "worker", "preview-worker-2026")
	adminSession, adminCSRF := loginAs(t, app, "admin", "preview-admin-2026")

	submit := stocktakePost(t, app, "/stocktakes/"+stocktake.ID+"/lines/"+line.ID, workerSession, workerCSRF, url.Values{
		"result": {"absent"}, "difference_reason": {"商品が見つからない"}, "notes": {"倉庫を確認"},
	})
	if submit.Code != http.StatusSeeOther {
		t.Fatalf("submit status=%d body=%s", submit.Code, submit.Body.String())
	}
	approvals, err := store.Approvals(t.Context(), "org_preview")
	if err != nil || len(approvals) == 0 {
		t.Fatalf("approvals=%+v err=%v", approvals, err)
	}
	var approval database.ApprovalRequest
	for _, item := range approvals {
		if item.ActionKey == "stocktake.difference.approve" && item.TargetID == line.ID {
			approval = item
			break
		}
	}
	if approval.ID == "" {
		t.Fatal("stocktake approval was not created")
	}
	approvalPageRequest := httptest.NewRequest(http.MethodGet, "/approvals", nil)
	approvalPageRequest.AddCookie(adminSession)
	approvalPage := httptest.NewRecorder()
	app.Handler().ServeHTTP(approvalPage, approvalPageRequest)
	if approvalPage.Code != http.StatusOK || !strings.Contains(approvalPage.Body.String(), "棚卸不一致") ||
		!strings.Contains(approvalPage.Body.String(), line.ID) {
		t.Fatalf("approval management did not include stocktake request: %s", approvalPage.Body.String())
	}
	returned := stocktakePost(t, app, "/approvals/"+approval.ID+"/return", adminSession, adminCSRF, url.Values{
		"comment": {"保管場所を再確認してください"},
	})
	if returned.Code != http.StatusSeeOther {
		t.Fatalf("return status=%d body=%s", returned.Code, returned.Body.String())
	}
	pageRequest := httptest.NewRequest(http.MethodGet, "/stocktakes/"+stocktake.ID, nil)
	pageRequest.AddCookie(workerSession)
	page := httptest.NewRecorder()
	app.Handler().ServeHTTP(page, pageRequest)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "差戻し") ||
		!strings.Contains(page.Body.String(), "再申請") {
		t.Fatalf("returned page status=%d body=%s", page.Code, page.Body.String())
	}
	resubmit := stocktakePost(t, app, "/stocktakes/"+stocktake.ID+"/lines/"+line.ID, workerSession, workerCSRF, url.Values{
		"result": {"absent"}, "difference_reason": {"商品が見つからない"}, "notes": {"再確認済み"},
	})
	if resubmit.Code != http.StatusSeeOther {
		t.Fatalf("resubmit status=%d body=%s", resubmit.Code, resubmit.Body.String())
	}
	approved := stocktakePost(t, app, "/approvals/"+approval.ID+"/approve", adminSession, adminCSRF, url.Values{})
	if approved.Code != http.StatusSeeOther {
		t.Fatalf("approve status=%d body=%s", approved.Code, approved.Body.String())
	}
	final, err := store.Stocktake(t.Context(), "org_preview", stocktake.ID)
	if err != nil || final.Lines[0].ReviewStatus != "approved" {
		t.Fatalf("final=%+v err=%v", final.Lines[0], err)
	}
}

func TestStocktakePageAutoInitializesWithoutMockExternalStartUI(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "worker", "preview-worker-2026")
	request := httptest.NewRequest(http.MethodGet, "/stocktakes", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "バーコード / 商品コード入力") {
		t.Fatalf("status=%d body=%s", recorder.Code, body)
	}
	for _, forbidden := range []string{"棚卸を開始", "data-stocktake-open", "data-stocktake-close", "NEW STOCKTAKE"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("mock-external start UI remains: %q", forbidden)
		}
	}
}

func TestStocktakeConcurrentGETAutoInitializesOneDraft(t *testing.T) {
	app, store := testServer(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, _ := loginAs(t, app, "worker", "preview-worker-2026")

	const callers = 8
	start := make(chan struct{})
	statuses := make(chan int, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for range callers {
		go func() {
			defer workers.Done()
			<-start
			request := httptest.NewRequest(http.MethodGet, "/stocktakes", nil)
			request.AddCookie(session)
			recorder := httptest.NewRecorder()
			app.Handler().ServeHTTP(recorder, request)
			statuses <- recorder.Code
		}()
	}
	close(start)
	workers.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent GET status=%d, want 200", status)
		}
	}

	list, err := store.Stocktakes(t.Context(), "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	var current database.Stocktake
	var draftCount int
	for _, item := range list {
		if item.Status == "draft" {
			draftCount++
			current, err = store.Stocktake(t.Context(), "org_preview", item.ID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if draftCount != 1 {
		t.Fatalf("draft count=%d, want 1", draftCount)
	}
	if len(current.Lines) == 0 {
		t.Fatal("auto-initialized stocktake has no lines")
	}
	if err := store.MarkStocktakePresent(
		t.Context(), "org_preview", current.ID, current.Lines[0].ID, "usr_worker",
	); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/stocktakes", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "棚卸済") {
		t.Fatalf("stocktake status was not rendered: status=%d body=%s", recorder.Code, body)
	}
	if strings.Contains(body, "● 確認済") {
		t.Fatal("legacy stocktake status remains in DOM")
	}
}
