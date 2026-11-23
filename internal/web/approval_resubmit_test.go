package web

import (
	"encoding/json"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReturnedApprovalResubmitPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_RETURNED_APPROVAL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_returned_approval_test") {
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

	adminCookies, csrf := login("admin", "worker-admin-test-password", 200)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	must(err)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	var org, admin, brand string
	must(db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	must(db.Raw("SELECT id FROM users WHERE username='admin'").Scan(&admin).Error)
	must(db.Raw("SELECT code FROM brands ORDER BY code LIMIT 1").Scan(&brand).Error)

	worker, wcsrf := login("worker", "worker-initial-test-password", 200)
	other, ocsrf := login("worker2", "worker-initial-test-password", 200)
	var accessory string
	must(db.Table("accessories").Select("code").Where("organization_id=?", org).Order("code").Limit(1).Scan(&accessory).Error)
	must(db.Table("accessories").Where("organization_id=? AND code=?", org, accessory).Update("supplement", "月").Error)
	var workerID string
	must(db.Raw("SELECT id FROM users WHERE username='worker'").Scan(&workerID).Error)
	_, err = repo.CreateExchangeRate(t.Context(), org, admin, "USD", "155", "test", time.Now())
	must(err)
	product := func(status string) persistence.SingleProductResult {
		p, e := repo.CreateSingleProduct(t.Context(), persistence.SingleProductInput{OrganizationID: org, ActorUserID: admin, StaffCode: "BUY-001", SupplierCode: "S001", PurchaseDate: "2026-09-18", BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 100, BaseSaleCurrency: "USD"})
		must(e)
		must(db.Table("products").Where("id=?", p.Product.ID).Update("inventory_status", status).Error)
		return p
	}
	purchase, err := repo.CreatePurchase(t.Context(), persistence.PurchaseCreateInput{OrganizationID: org, ActorUserID: workerID, SupplierCode: "S001", StaffCode: "BUY-001", PurchaseDate: "2026-09-18", PaymentMethod: "bank_transfer", Lines: []persistence.PurchaseLineInput{{Quantity: 1, BrandCode: brand, ModelNumber: "Before", UnitCostMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 100, BaseSaleCurrency: "USD"}}})
	must(err)
	p1 := product("in_stock")
	sale, err := repo.CreateSale(t.Context(), persistence.SaleCreateInput{OrganizationID: org, ActorUserID: workerID, BuyerCode: "B001", SaleDate: "2026-09-18", DisplayCurrency: "USD", TaxMode: "out_of_scope", Lines: []persistence.SaleLineInput{{ProductCode: p1.Product.ProductCode, UnitPriceMinor: 100}}})
	must(err)
	p2 := product("in_stock")
	shipment, err := repo.CreateShipment(t.Context(), persistence.ShipmentCreateInput{OrganizationID: org, ActorUserID: workerID, BuyerCode: "B001", ShipmentDate: "2026-09-18", DisplayCurrency: "USD", ProductCodes: []string{p2.Product.ProductCode}})
	must(err)
	replacement := product("in_stock")
	p3 := product("sold")
	saleReturn, err := repo.CreateReturn(t.Context(), persistence.ReturnCreateInput{OrganizationID: org, ActorUserID: workerID, OperationType: "return", BuyerCode: "B001", TransactionDate: "2026-09-18", Reason: "before", Notes: "return", ProductCodes: []string{p3.Product.ProductCode}})
	must(err)
	p4 := product("in_stock")
	var purchaseNo string
	must(db.Raw("SELECT s.slip_number FROM purchase_slips s JOIN purchase_slip_lines l ON l.purchase_slip_id=s.id JOIN products p ON p.purchase_slip_line_id=l.id WHERE p.id=?", p4.Product.ID).Scan(&purchaseNo).Error)
	purchaseReturn, err := repo.CreateReturn(t.Context(), persistence.ReturnCreateInput{OrganizationID: org, ActorUserID: workerID, OperationType: "purchase_return", SupplierCode: "S001", PurchaseSlipNo: purchaseNo, TransactionDate: "2026-09-18", Reason: "before", Notes: "return", ProductCodes: []string{p4.Product.ProductCode}})
	must(err)

	t.Run("full draft rebuild keeps identity and audit", func(t *testing.T) {
		type fixture struct {
			kind, id, number, table string
			rebuild                 persistence.ReturnedDraftInput
			oldProduct              string
		}
		var fixtures []fixture
		pl := persistence.PurchaseLineInput{Quantity: 1, BrandCode: brand, ModelNumber: "before", UnitCostMinor: 500, CostCurrency: "USD", BaseSalePriceMinor: 900, BaseSaleCurrency: "USD"}
		pi := persistence.PurchaseCreateInput{OrganizationID: org, ActorUserID: workerID, SupplierCode: "S001", StaffCode: "BUY-001", PurchaseDate: "2026-09-18", Lines: []persistence.PurchaseLineInput{pl, pl}}
		pd, e := repo.CreatePurchase(t.Context(), pi)
		must(e)
		pi.PurchaseDate = "2026-09-19"
		pi.PaymentMethod = "cash"
		pi.Notes = "full correction"
		pi.Lines[0].ModelNumber = "after"
		pi.Lines[0].WeightGrams = "123.45"
		pi.Lines[0].AccessoryCodes = []string{accessory}
		pi.Lines[0].AccessorySupplements = persistence.AccessorySupplementValues{accessory: "10"}
		pi.Lines = append(pi.Lines, pl)
		fixtures = append(fixtures, fixture{"purchase_slip", pd.ID, pd.SlipNumber, "purchase_slips", persistence.ReturnedDraftInput{Reason: "replace all editable content", OriginalLineIDs: []string{pd.Lines[0].ID, pd.Lines[1].ID, ""}, Purchase: &pi}, ""})
		for _, kind := range []string{"sales_slip", "shipment_slip", "consignment_slip", "return_slip", "purchase_return"} {
			status := "in_stock"
			if kind == "return_slip" {
				status = "sold"
			}
			old, newProduct, extra := product(status), product(status), product(status)
			codes := []string{old.Product.ProductCode}
			newCodes := []string{newProduct.Product.ProductCode, extra.Product.ProductCode}
			rebuild := persistence.ReturnedDraftInput{Reason: "wrong product and date", OriginalLineIDs: []string{"", ""}}
			var id, number, lineID, table string
			switch kind {
			case "sales_slip":
				in := persistence.SaleCreateInput{OrganizationID: org, ActorUserID: workerID, BuyerCode: "B001", SaleDate: "2026-09-18", DisplayCurrency: "USD", TaxMode: "out_of_scope", Lines: []persistence.SaleLineInput{{ProductCode: codes[0], UnitPriceMinor: 100}}}
				d, e := repo.CreateSale(t.Context(), in)
				must(e)
				id, number, lineID, table = d.ID, d.SlipNumber, d.Lines[0].ID, "sales_slips"
				in.SaleDate = "2026-09-19"
				in.Notes = "full sale"
				in.Lines = []persistence.SaleLineInput{{ProductCode: newCodes[0], UnitPriceMinor: 200}, {ProductCode: newCodes[1], UnitPriceMinor: 300}}
				rebuild.Sale = &in
			case "shipment_slip":
				in := persistence.ShipmentCreateInput{OrganizationID: org, ActorUserID: workerID, BuyerCode: "B001", ShipmentDate: "2026-09-18", DisplayCurrency: "USD", ProductCodes: codes}
				d, e := repo.CreateShipment(t.Context(), in)
				must(e)
				id, number, lineID, table = d.ID, d.SlipNumber, d.Lines[0].ID, "shipment_slips"
				in.ShipmentDate = "2026-09-19"
				in.RecipientAddress = "changed address"
				in.Carrier = "changed carrier"
				in.ProductCodes = newCodes
				rebuild.Shipment = &in
			case "consignment_slip":
				in := persistence.ConsignmentCreateInput{OrganizationID: org, ActorUserID: workerID, ConsigneeCode: "B001", ConsignmentDate: "2026-09-18", ProductCodes: codes}
				d, e := repo.RequestConsignmentApproval(t.Context(), in)
				must(e)
				id, number, lineID, table = d.ID, d.SlipNumber, d.Lines[0].ID, "consignment_slips"
				in.ConsignmentDate = "2026-09-19"
				in.Notes = "full consignment"
				in.ProductCodes = newCodes
				rebuild.Consignment = &in
			default:
				in := persistence.ReturnCreateInput{OrganizationID: org, ActorUserID: workerID, OperationType: "return", BuyerCode: "B001", TransactionDate: "2026-09-18", Reason: "before", Notes: "return", ProductCodes: codes}
				if kind == "purchase_return" {
					in.OperationType = "purchase_return"
					in.BuyerCode = ""
					in.SupplierCode = "S001"
					must(db.Raw("SELECT s.slip_number FROM purchase_slips s JOIN purchase_slip_lines l ON l.purchase_slip_id=s.id JOIN products p ON p.purchase_slip_line_id=l.id WHERE p.id=?", old.Product.ID).Scan(&in.PurchaseSlipNo).Error)
				}
				d, e := repo.CreateReturn(t.Context(), in)
				must(e)
				id, number, lineID, table = d.ID, d.SlipNumber, d.Lines[0].ID, "return_slips"
				in.TransactionDate = "2026-09-19"
				in.Reason = "corrected return"
				in.Notes = "full return"
				in.ProductCodes = newCodes
				if kind == "purchase_return" {
					// A new purchase source can be selected, with a replacement product belonging to it.
					in.ProductCodes = newCodes[:1]
					rebuild.OriginalLineIDs = rebuild.OriginalLineIDs[:1]
					must(db.Raw("SELECT s.slip_number FROM purchase_slips s JOIN purchase_slip_lines l ON l.purchase_slip_id=s.id JOIN products p ON p.purchase_slip_line_id=l.id WHERE p.id=?", newProduct.Product.ID).Scan(&in.PurchaseSlipNo).Error)
				}
				rebuild.Return = &in
			}
			rebuild.OriginalLineIDs[0] = lineID
			if kind == "purchase_return" {
				kind = "return_slip"
			}
			fixtures = append(fixtures, fixture{kind, id, number, table, rebuild, old.Product.ID})
		}
		_, e = repo.CreateExchangeRate(t.Context(), org, admin, "USD", "999", "changed after registration", time.Now().Add(time.Hour))
		must(e)
		for _, f := range fixtures {
			t.Run(f.kind+f.number, func(t *testing.T) {
				action := map[string]string{"purchase_slip": "purchase.confirm", "sales_slip": "sale.confirm", "shipment_slip": "shipment.confirm", "consignment_slip": "consignment.confirm", "return_slip": "return.confirm"}[f.kind]
				var approval persistence.ApprovalRequestRecord
				if f.kind == "consignment_slip" {
					list, e := repo.ApprovalRequests(t.Context(), org, workerID, "worker", "")
					must(e)
					for _, a := range list {
						if a.TargetID == f.id {
							approval = a
						}
					}
				} else {
					var e error
					approval, e = repo.CreateApproval(t.Context(), org, workerID, persistence.ApprovalCreateInput{ApprovalType: f.kind, TargetType: f.kind, TargetID: f.id, RequestedAction: action})
					must(e)
				}
				path := "/approvals/" + approval.ID
				request("POST", path+"/returned", csrf, adminCookies, map[string]string{"note": "rebuild the draft"}, 200)
				var loaded struct {
					Approval persistence.ApprovalRequestRecord `json:"approval"`
					Document struct {
						UpdatedAt time.Time `json:"updatedAt"`
					} `json:"document"`
				}
				w := request("GET", path+"/returned-document", "", worker, nil, 200)
				must(json.Unmarshal(w.Body.Bytes(), &loaded))
				input := persistence.ApprovalResubmitInput{ExpectedApprovalUpdatedAt: loaded.Approval.UpdatedAt, ExpectedDocumentUpdatedAt: loaded.Document.UpdatedAt, Rebuild: &f.rebuild}
				invalid := f.rebuild
				invalid.OriginalLineIDs = append([]string(nil), f.rebuild.OriginalLineIDs...)
				invalid.OriginalLineIDs[0] = "foreign-line"
				bad := input
				bad.Rebuild = &invalid
				request("POST", path+"/resubmit", wcsrf, worker, bad, 409)
				request("POST", path+"/resubmit", ocsrf, other, input, 409)
				encoded, _ := json.Marshal(f.rebuild)
				var invalidBusiness persistence.ReturnedDraftInput
				must(json.Unmarshal(encoded, &invalidBusiness))
				if invalidBusiness.Purchase != nil {
					invalidBusiness.Purchase.Lines[0].BrandCode = "missing-brand"
				}
				if invalidBusiness.Sale != nil {
					invalidBusiness.Sale.Lines[0].ProductCode = "missing-product"
				}
				if invalidBusiness.Shipment != nil {
					invalidBusiness.Shipment.ProductCodes[0] = "missing-product"
				}
				if invalidBusiness.Consignment != nil {
					invalidBusiness.Consignment.ProductCodes[0] = "missing-product"
				}
				if invalidBusiness.Return != nil {
					invalidBusiness.Return.ProductCodes[0] = "missing-product"
				}
				bad.Rebuild = &invalidBusiness
				request("POST", path+"/resubmit", wcsrf, worker, bad, 409)
				var sequenceBefore, sequenceAfter int64
				must(db.Raw("SELECT COALESCE(SUM(last_sequence),0) FROM daily_document_sequences WHERE organization_id=?", org).Scan(&sequenceBefore).Error)
				w = request("POST", path+"/resubmit", wcsrf, worker, input, 200)
				must(json.Unmarshal(w.Body.Bytes(), &approval))
				if approval.ID == "" || approval.Status != "pending" {
					t.Fatal("not pending")
				}
				var snapshot struct {
					ID, SlipNumber, Status string
					Lines                  []struct{ ID string }
					FXRateScaled           int64
					FXScale                int64
				}
				must(json.Unmarshal([]byte(approval.SnapshotJSON), &snapshot))
				if snapshot.ID != f.id || snapshot.SlipNumber != f.number || snapshot.Status != "draft" || len(snapshot.Lines) != len(f.rebuild.OriginalLineIDs) {
					t.Fatalf("identity or lines changed incorrectly: %+v", snapshot)
				}
				must(db.Raw("SELECT COALESCE(SUM(last_sequence),0) FROM daily_document_sequences WHERE organization_id=?", org).Scan(&sequenceAfter).Error)
				if sequenceBefore != sequenceAfter {
					t.Fatal("resubmit consumed a document number")
				}
				if (f.kind == "shipment_slip" || f.kind == "consignment_slip" || f.kind == "sales_slip") && snapshot.FXRateScaled != 155*snapshot.FXScale {
					t.Fatalf("fixed rate changed: %d", snapshot.FXRateScaled)
				}
				if snapshot.Lines[0].ID != f.rebuild.OriginalLineIDs[0] {
					t.Fatal("original line identity lost")
				}
				var history struct {
					PreviousDocument struct {
						SlipNumber string
						Lines      []any
					}
				}
				must(json.Unmarshal([]byte(approval.ResubmissionJSON), &history))
				if history.PreviousDocument.SlipNumber != f.number {
					t.Fatal("comparison baseline missing")
				}
				if f.oldProduct != "" && f.kind != "return_slip" {
					var state string
					must(db.Table("products").Select("inventory_status").Where("id=?", f.oldProduct).Scan(&state).Error)
					if state != "in_stock" {
						t.Fatalf("old reservation not released: %s", state)
					}
				}
				// A second return removes a newly added line and compares against the last revision.
				if f.rebuild.Purchase != nil {
					request("POST", path+"/returned", csrf, adminCookies, map[string]string{"note": "remove an extra row"}, 200)
					w = request("GET", path+"/returned-document", "", worker, nil, 200)
					must(json.Unmarshal(w.Body.Bytes(), &loaded))
					f.rebuild.Purchase.Lines = f.rebuild.Purchase.Lines[:1]
					f.rebuild.OriginalLineIDs = f.rebuild.OriginalLineIDs[:1]
					input.Rebuild = &f.rebuild
					input.ExpectedApprovalUpdatedAt = loaded.Approval.UpdatedAt
					input.ExpectedDocumentUpdatedAt = loaded.Document.UpdatedAt
					w = request("POST", path+"/resubmit", wcsrf, worker, input, 200)
					must(json.Unmarshal(w.Body.Bytes(), &approval))
					must(json.Unmarshal([]byte(approval.ResubmissionJSON), &history))
					if len(history.PreviousDocument.Lines) != 3 {
						t.Fatal("second baseline did not use latest revision")
					}
				}
				request("POST", path+"/approved", csrf, adminCookies, map[string]string{"note": "checked full correction"}, 200)
				var status string
				must(db.Table(f.table).Select("status").Where("id=?", f.id).Scan(&status).Error)
				if status != "confirmed" {
					t.Fatal("approval did not confirm rebuilt draft")
				}
			})
		}
	})

	cases := []struct {
		kind, id, action, table string
		input                   persistence.ApprovalResubmitInput
	}{
		{"purchase_slip", purchase.ID, "purchase.confirm", "purchase_slips", persistence.ApprovalResubmitInput{Purchase: &persistence.PurchaseRevisionInput{PurchaseDate: "2026-09-18", SupplierCode: "S001", StaffCode: "BUY-001", Notes: "corrected", Reason: "cost correction", Lines: []persistence.PurchaseRevisionLine{{ID: purchase.Lines[0].ID, SKU: "NEW", UnitCostMinor: 1200, BaseSalePriceMinor: 200}}}}},
		{"sales_slip", sale.ID, "sale.confirm", "sales_slips", persistence.ApprovalResubmitInput{Sale: &persistence.SaleRevisionInput{SaleDate: "2026-09-18", BuyerCode: "B001", Notes: "corrected", Reason: "price correction", Lines: []persistence.SaleRevisionLine{{ID: sale.Lines[0].ID, UnitPriceMinor: 90}}}}},
		{"shipment_slip", shipment.ID, "shipment.confirm", "shipment_slips", persistence.ApprovalResubmitInput{Shipment: &persistence.ShipmentRevisionInput{ShipmentDate: "2026-09-18", BuyerCode: "B001", Notes: "corrected", Reason: "price correction", Lines: []persistence.ShipmentRevisionLine{{ID: shipment.Lines[0].ID, UnitPriceMinor: 90}}}}},
		{"return_slip", saleReturn.ID, "return.confirm", "return_slips", persistence.ApprovalResubmitInput{Return: &persistence.PurchaseReturnRevisionInput{TransactionDate: "2026-09-18", ReturnReason: "corrected reason", Notes: "corrected", Reason: "return correction"}}},
		{"return_slip", purchaseReturn.ID, "return.confirm", "return_slips", persistence.ApprovalResubmitInput{Return: &persistence.PurchaseReturnRevisionInput{TransactionDate: "2026-09-18", ReturnReason: "corrected reason", Notes: "corrected", Reason: "return correction"}}},
	}
	for _, tc := range cases {
		t.Run(tc.kind+tc.id, func(t *testing.T) {
			w := request("POST", "/approvals", wcsrf, worker, map[string]string{"approvalType": tc.kind, "targetType": tc.kind, "targetId": tc.id, "requestedAction": tc.action}, 201)
			var approval persistence.ApprovalRequestRecord
			must(json.Unmarshal(w.Body.Bytes(), &approval))
			path := "/approvals/" + approval.ID
			request("GET", path+"/returned-document", "", worker, nil, 409)
			request("POST", path+"/returned", csrf, adminCookies, map[string]string{"note": "金額・備考を確認してください"}, 200)
			request("GET", path+"/returned-document", "", other, nil, 409)
			request("GET", path+"/returned-document", "", adminCookies, nil, 403)
			w = request("GET", path+"/returned-document", "", worker, nil, 200)
			var loaded struct {
				Approval persistence.ApprovalRequestRecord `json:"approval"`
				Document struct {
					UpdatedAt  time.Time `json:"updatedAt"`
					SlipNumber string    `json:"slipNumber"`
				} `json:"document"`
			}
			must(json.Unmarshal(w.Body.Bytes(), &loaded))
			input := tc.input
			if tc.kind == "purchase_slip" {
				input.PurchaseProducts = map[string]persistence.PurchaseLineInput{purchase.Lines[0].ID: {BrandCode: brand, ModelNumber: "Corrected model", ReferenceNumber: "REF0011", SerialNumber: "SER0011", WeightGrams: "125.50", DialText: "Blue", BeltText: "Leather", Notes: "corrected product note", BoxCode: "BOX-02", AccessoryCodes: []string{accessory}, AccessorySupplements: persistence.AccessorySupplementValues{accessory: "10"}}}
			}
			if tc.kind == "shipment_slip" {
				input.ShipmentProducts = map[string]string{shipment.Lines[0].ID: replacement.Product.ProductCode}
			}
			input.ExpectedApprovalUpdatedAt = loaded.Approval.UpdatedAt
			input.ExpectedDocumentUpdatedAt = loaded.Document.UpdatedAt
			stale := input
			stale.ExpectedDocumentUpdatedAt = stale.ExpectedDocumentUpdatedAt.Add(-time.Second)
			request("POST", path+"/resubmit", wcsrf, worker, stale, 409)
			request("POST", path+"/resubmit", ocsrf, other, input, 409)
			// Invalid line input must roll back notes, amounts, and the approval state together.
			if input.Purchase != nil {
				invalid := input
				copy := *input.Purchase
				copy.Lines = append([]persistence.PurchaseRevisionLine(nil), copy.Lines...)
				copy.Lines[0].UnitCostMinor = -1
				invalid.Purchase = &copy
				request("POST", path+"/resubmit", wcsrf, worker, invalid, 409)
			}
			if input.PurchaseProducts != nil {
				invalid := input
				invalid.PurchaseProducts = map[string]persistence.PurchaseLineInput{"not-this-slip": {ModelNumber: "Bad"}}
				request("POST", path+"/resubmit", wcsrf, worker, invalid, 409)
				invalid.PurchaseProducts = map[string]persistence.PurchaseLineInput{purchase.Lines[0].ID: {BrandCode: "missing-brand", ModelNumber: "Bad"}}
				request("POST", path+"/resubmit", wcsrf, worker, invalid, 409)
				invalid.PurchaseProducts = map[string]persistence.PurchaseLineInput{purchase.Lines[0].ID: {BrandCode: brand, WeightGrams: "-1"}}
				request("POST", path+"/resubmit", wcsrf, worker, invalid, 409)
				unchanged, e := repo.PurchaseSlip(t.Context(), org, tc.id)
				must(e)
				if unchanged.Lines[0].ModelNumber != "Before" || unchanged.Lines[0].UnitCostMinor != 1000 {
					t.Fatal("invalid product edit was not rolled back")
				}
			}
			if input.ShipmentProducts != nil {
				for _, bad := range []map[string]string{
					{"foreign-line": replacement.Product.ProductCode},
					{shipment.Lines[0].ID: "MISSING"},
					{shipment.Lines[0].ID: p1.Product.ProductCode},
					{shipment.Lines[0].ID: p3.Product.ProductCode},
				} {
					invalid := input
					invalid.ShipmentProducts = bad
					request("POST", path+"/resubmit", wcsrf, worker, invalid, 409)
				}
				unchanged, e := repo.ShipmentSlip(t.Context(), org, tc.id)
				must(e)
				if unchanged.Lines[0].ProductID != p2.Product.ID || unchanged.Lines[0].SalePriceUSDMinor != 100 {
					t.Fatal("invalid replacement did not roll back")
				}
			}
			w = request("POST", path+"/resubmit", wcsrf, worker, input, 200)
			var pending persistence.ApprovalRequestRecord
			must(json.Unmarshal(w.Body.Bytes(), &pending))
			if pending.ID != approval.ID || pending.Status != "pending" || pending.DecidedAt != nil || !strings.Contains(pending.SnapshotJSON, "corrected") {
				t.Fatalf("invalid resubmission %+v", pending)
			}
			if tc.kind == "shipment_slip" {
				var oldStatus, newStatus string
				must(db.Table("products").Select("inventory_status").Where("id=?", p2.Product.ID).Scan(&oldStatus).Error)
				must(db.Table("products").Select("inventory_status").Where("id=?", replacement.Product.ID).Scan(&newStatus).Error)
				if oldStatus != "in_stock" || newStatus != "reserved" {
					t.Fatalf("wrong reservation transition %s %s", oldStatus, newStatus)
				}
				if !strings.Contains(pending.SnapshotJSON, replacement.Product.ProductCode) || strings.Contains(pending.SnapshotJSON, p2.Product.ProductCode) {
					t.Fatal("replacement approval snapshot incorrect")
				}
			}
			var state struct{ Status, SlipNumber string }
			must(db.Table(tc.table).Select("status,slip_number").Where("id=?", tc.id).Take(&state).Error)
			if state.Status != "draft" || state.SlipNumber != loaded.Document.SlipNumber {
				t.Fatal("document confirmed or renumbered too early")
			}
			request("POST", path+"/resubmit", wcsrf, worker, input, 409)
			request("GET", path+"/returned-document", "", worker, nil, 409)
			var history string
			must(db.Table("approval_actions").Select("note").Where("approval_request_id=? AND action='requested'", approval.ID).Order("created_at DESC").Limit(1).Scan(&history).Error)
			if !strings.Contains(history, "previousDocument") || !strings.Contains(history, "金額・備考") {
				t.Fatal("return history missing")
			}
			if tc.kind == "purchase_slip" {
				revised, e := repo.PurchaseSlip(t.Context(), org, tc.id)
				must(e)
				if revised.Lines[0].ModelNumber != "Corrected model" || revised.Lines[0].WeightGrams != "125.5" || revised.Lines[0].ReferenceNumber != "REF0011" || revised.Lines[0].SerialNumber != "SER0011" || revised.Lines[0].ProductID != "" {
					t.Fatalf("draft specifications wrong: %+v", revised.Lines[0])
				}
				if !strings.Contains(pending.SnapshotJSON, "Corrected model") {
					t.Fatal("administrator snapshot not refreshed")
				}
				if revised.Lines[0].UnitCostMinor != 1200 || revised.Lines[0].ConvertedTotalJPY != 1200 {
					t.Fatal("draft cost totals incorrect")
				}
			}
			if tc.kind == "purchase_slip" {
				// A second correction changes only product specifications, through rejection restoration.
				request("POST", path+"/rejected", csrf, adminCookies, map[string]string{"note": "商品情報を再確認"}, 200)
				w = request("GET", path+"/returned-document", "", worker, nil, 200)
				var next struct {
					Approval persistence.ApprovalRequestRecord
					Document persistence.PurchaseSlipRecord
				}
				must(json.Unmarshal(w.Body.Bytes(), &next))
				input.ExpectedApprovalUpdatedAt = next.Approval.UpdatedAt
				input.ExpectedDocumentUpdatedAt = next.Document.UpdatedAt
				input.Purchase.Notes = next.Document.Notes
				input.Purchase.Reason = "product specifications only"
				detail := input.PurchaseProducts[purchase.Lines[0].ID]
				detail.ModelNumber = "Corrected model final"
				input.PurchaseProducts[purchase.Lines[0].ID] = detail
				request("POST", path+"/resubmit", wcsrf, worker, input, 200)
			}
			request("POST", path+"/approved", csrf, adminCookies, map[string]string{"note": "再確認済み"}, 200)
			must(db.Table(tc.table).Select("status,slip_number").Where("id=?", tc.id).Take(&state).Error)
			if tc.kind == "purchase_slip" {
				var product struct {
					ModelNumber, ReferenceNumber, SerialNumber, WeightGrams, DialText, BeltText string
					AccessorySupplements                                                        persistence.AccessorySupplementValues
				}
				must(db.Table("products").Where("purchase_slip_line_id=?", purchase.Lines[0].ID).Take(&product).Error)
				if product.AccessorySupplements[accessory] != "10" {
					t.Fatal("accessory supplement was not carried into approved product")
				}
				if product.ModelNumber != "Corrected model final" || product.ReferenceNumber != "REF0011" || product.SerialNumber != "SER0011" || product.WeightGrams != "125.5" || product.DialText != "Blue" || product.BeltText != "Leather" {
					t.Fatalf("approved product specifications wrong: %+v", product)
				}
			}
			if tc.kind == "shipment_slip" {
				var oldStatus, newStatus string
				must(db.Table("products").Select("inventory_status").Where("id=?", p2.Product.ID).Scan(&oldStatus).Error)
				must(db.Table("products").Select("inventory_status").Where("id=?", replacement.Product.ID).Scan(&newStatus).Error)
				if oldStatus != "in_stock" || newStatus != "shipped" {
					t.Fatalf("approval shipped wrong product %s %s", oldStatus, newStatus)
				}
			}
			if state.Status != "confirmed" {
				t.Fatal("corrected document not confirmed")
			}
			request("POST", path+"/resubmit", wcsrf, worker, input, 409)
		})
	}
}
