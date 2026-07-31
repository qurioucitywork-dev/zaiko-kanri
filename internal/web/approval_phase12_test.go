package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func seedPhase12Approval(t *testing.T, store *database.Store) database.ApprovalRequest {
	t.Helper()
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedApprovalPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	records, err := store.Approvals(t.Context(), "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Status == "pending" {
			return record
		}
	}
	t.Fatal("pending approval was not seeded")
	return database.ApprovalRequest{}
}

func phase12Request(t *testing.T, app *Server, method, path string, session, csrf *http.Cookie, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	request := httptest.NewRequest(method, path, body)
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if session != nil {
		request.AddCookie(session)
	}
	if csrf != nil {
		request.AddCookie(csrf)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestPhase12ApprovalListIsAdminOnlyAndMockAligned(t *testing.T) {
	app, store := testServer(t)
	pending := seedPhase12Approval(t, store)
	adminSession, _ := loginAs(t, app, "admin", "preview-admin-2026")

	response := phase12Request(t, app, http.MethodGet, "/approvals?sort=applicant", adminSession, nil, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{
		"申請ID", "申請日時", "申請者", "種別", "内容", "ステータス",
		"保留中", pending.ID, `data-approval-open="approval-` + pending.ID + `"`,
		`data-approval-return-open="return-` + pending.ID + `"`, "承認する",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("approval page missing %q: %s", expected, body)
		}
	}
	for _, removed := range []string{"requested_snapshot", "承認コメント", "却下"} {
		if strings.Contains(body, removed) {
			t.Fatalf("approval page still contains removed UI %q: %s", removed, body)
		}
	}

	workerSession, _ := loginAs(t, app, "worker", "preview-worker-2026")
	forbidden := phase12Request(t, app, http.MethodGet, "/approvals", workerSession, nil, nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("worker approval page status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
	workerPage := phase12Request(t, app, http.MethodGet, "/", workerSession, nil, nil)
	if strings.Contains(workerPage.Body.String(), `href="/approvals"`) {
		t.Fatalf("worker navigation must not expose approvals: %s", workerPage.Body.String())
	}
}

func TestPhase12ReturnResubmitApproveWorkflow(t *testing.T) {
	app, store := testServer(t)
	pending := seedPhase12Approval(t, store)
	adminSession, adminCSRF := loginAs(t, app, "admin", "preview-admin-2026")

	empty := phase12Request(t, app, http.MethodPost, "/approvals/"+pending.ID+"/return", adminSession, adminCSRF,
		url.Values{"csrf_token": {adminCSRF.Value}, "comment": {""}})
	if empty.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty return comment status=%d body=%s", empty.Code, empty.Body.String())
	}
	returnedResponse := phase12Request(t, app, http.MethodPost, "/approvals/"+pending.ID+"/return", adminSession, adminCSRF,
		url.Values{"csrf_token": {adminCSRF.Value}, "comment": {"金額と理由を再確認してください"}})
	if returnedResponse.Code != http.StatusSeeOther {
		t.Fatalf("return status=%d body=%s", returnedResponse.Code, returnedResponse.Body.String())
	}
	returned, err := store.ApprovalRequest(t.Context(), "org_preview", pending.ID)
	if err != nil || returned.Status != "returned" {
		t.Fatalf("returned=%+v err=%v", returned, err)
	}
	returnedList := phase12Request(t, app, http.MethodGet, "/approvals", adminSession, nil, nil)
	if !strings.Contains(returnedList.Body.String(), pending.ID) || !strings.Contains(returnedList.Body.String(), "差戻し") {
		t.Fatalf("returned approval must remain visible: %s", returnedList.Body.String())
	}

	resubmitted, err := store.CreateApprovalRequest(t.Context(), database.CreateApprovalInput{
		OrganizationID:  pending.OrganizationID,
		ApprovalType:    pending.ApprovalType,
		TargetType:      pending.TargetType,
		TargetID:        pending.TargetID,
		ActionKey:       pending.ActionKey,
		ApplicantUserID: pending.ApplicantUserID,
		RequestReason:   "差戻し内容を修正しました",
		ActionPayload:   map[string]string{"reason": "差戻し内容を修正しました"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resubmitted.ID != pending.ID || resubmitted.Status != "pending" || len(resubmitted.Actions) < 3 {
		t.Fatalf("resubmitted request did not retain id/history: %+v", resubmitted)
	}

	approvedResponse := phase12Request(t, app, http.MethodPost, "/approvals/"+pending.ID+"/approve", adminSession, adminCSRF,
		url.Values{"csrf_token": {adminCSRF.Value}})
	if approvedResponse.Code != http.StatusSeeOther {
		t.Fatalf("approve status=%d body=%s", approvedResponse.Code, approvedResponse.Body.String())
	}
	approved, err := store.ApprovalRequest(t.Context(), "org_preview", pending.ID)
	if err != nil || approved.Status != "approved" {
		t.Fatalf("approved=%+v err=%v", approved, err)
	}
	finalList := phase12Request(t, app, http.MethodGet, "/approvals", adminSession, nil, nil)
	if strings.Contains(finalList.Body.String(), pending.ID) {
		t.Fatalf("approved request must disappear from review list: %s", finalList.Body.String())
	}

	reject := phase12Request(t, app, http.MethodPost, "/approvals/"+pending.ID+"/reject", adminSession, adminCSRF,
		url.Values{"csrf_token": {adminCSRF.Value}, "comment": {"not available"}})
	if reject.Code != http.StatusNotFound && reject.Code != http.StatusMethodNotAllowed {
		t.Fatalf("removed reject endpoint status=%d body=%s", reject.Code, reject.Body.String())
	}
}
