package web

import (
	"encoding/json"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/storage"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestApprovedPurchaseManagementCodesPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_PURCHASE_CODES_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_purchase_codes_test") {
		t.Fatal("non-test database rejected")
	}
	cfg := config.Config{Environment: "test", DatabaseDriver: "postgres", DatabaseURL: dsn, OrganizationCode: "PREVIEW", SessionTTL: time.Hour, StorageDriver: "local", UploadDirectory: t.TempDir()}
	repo, err := persistence.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(repo.Migrate(t.Context()))
	must(repo.SeedPreviewIdentity(t.Context(), "worker-admin-test-password", "worker-initial-test-password"))
	must(repo.SeedPreviewMasters(t.Context()))
	objects, err := storage.New(t.Context(), cfg)
	must(err)
	app, err := New(cfg, nil, repo, objects, slog.New(slog.NewTextHandler(io.Discard, nil)))
	must(err)
	request := func(method, path, csrf string, cookies []*http.Cookie, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(payload)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	login := func(username, password string, want int) ([]*http.Cookie, string) {
		t.Helper()
		w := request("POST", "/auth/login", "", nil, map[string]string{"username": username, "password": password}, want)
		var p struct {
			CSRFToken string `json:"csrfToken"`
		}
		must(json.Unmarshal(w.Body.Bytes(), &p))
		return w.Result().Cookies(), p.CSRFToken
	}

	admin, csrf := login("admin", "worker-admin-test-password", 200)
	worker, workerCSRF := login("worker", "worker-initial-test-password", 200)
	line := map[string]any{"quantity": 1, "modelNumber": "Approval code test", "costCurrency": "JPY", "unitCostMinor": 1000, "baseSaleCurrency": "USD", "baseSalePriceMinor": 100}
	w := request("POST", "/purchases", workerCSRF, worker, map[string]any{"supplierCode": "S001", "staffCode": "BUY-001", "purchaseDate": "2026-09-17", "lines": []any{line, line}}, 201)
	var slip persistence.PurchaseSlipRecord
	must(json.Unmarshal(w.Body.Bytes(), &slip))
	if len(slip.Lines) != 2 || slip.Lines[0].ProductCode != "" {
		t.Fatal("draft should not have assigned product codes")
	}
	w = request("POST", "/approvals", workerCSRF, worker, map[string]string{"approvalType": "仕入登録", "targetType": "purchase_slip", "targetId": slip.ID, "requestedAction": "purchase.confirm"}, 201)
	var approval persistence.ApprovalRequestRecord
	must(json.Unmarshal(w.Body.Bytes(), &approval))
	request("POST", "/approvals/"+approval.ID+"/approved", csrf, admin, map[string]string{"note": "test approval"}, 200)
	w = request("GET", "/purchases/"+slip.ID, "", worker, nil, 200)
	must(json.Unmarshal(w.Body.Bytes(), &slip))
	if slip.Status != "confirmed" || len(slip.Lines) != 2 {
		t.Fatal("approved purchase missing")
	}
	seen := map[string]bool{}
	for _, l := range slip.Lines {
		if l.ProductCode == "" || seen[l.ProductCode] {
			t.Fatal("missing or repeated code")
		}
		seen[l.ProductCode] = true
	}
	doc := purchasePDF(slip)
	for i, l := range slip.Lines {
		if doc.Lines[i].ProductCode != l.ProductCode {
			t.Fatal("PDF lost code", l.ProductCode)
		}
	}
	w = request("POST", "/purchases/"+slip.ID+"/issue", csrf, admin, map[string]any{}, 200)
	must(json.Unmarshal(w.Body.Bytes(), &slip))
	if slip.OfficialPDF == nil {
		t.Fatal("missing official PDF")
	}
	// A worker can read the generated official document after admin approval/issue.
	pdf := request("GET", strings.TrimPrefix(slip.OfficialPDF.DownloadURL, "/api/v1"), "", worker, nil, 200)
	if !strings.HasPrefix(pdf.Body.String(), "%PDF-") {
		t.Fatal("invalid PDF")
	}
	if path := os.Getenv("ZAIKO_PURCHASE_CODES_PDF"); path != "" {
		must(os.WriteFile(path, pdf.Body.Bytes(), 0600))
	}
}

func TestPurchasePDFIncludesManagementCodes(t *testing.T) {
	doc := purchasePDF(persistence.PurchaseSlipRecord{Lines: []persistence.PurchaseLineRecord{{ProductCode: "1709260001"}, {PartCode: "PART-0001"}}})
	if doc.Lines[0].ProductCode != "1709260001" || doc.Lines[1].ProductCode != "PART-0001" {
		t.Fatal("PDF management numbers missing", doc.Lines)
	}
}
