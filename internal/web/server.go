package web

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/storage"
)

//go:embed templates/base.html templates/login.html templates/dashboard.html templates/users.html templates/settings.html templates/audit.html templates/public.html templates/products.html templates/product-new.html templates/product-detail.html templates/purchases.html templates/purchase-new.html templates/purchase-detail.html templates/market.html templates/market-import.html templates/market-import-preview.html templates/sales.html templates/sale-new.html templates/sale-detail.html templates/shipments.html templates/shipment-new.html templates/shipment-detail.html templates/purchase-requests.html templates/approvals.html static/app.css static/app.js
var assets embed.FS

const sessionCookie = "zaiko_session"
const csrfCookie = "zaiko_csrf"

type Server struct {
	cfg        config.Config
	store      *database.Store
	repository *persistence.Repository
	objects    storage.Store
	log        *slog.Logger
	templates  map[string]*template.Template
	handler    http.Handler
}

type pageData struct {
	Title            string
	Active           string
	User             database.User
	CSRF             string
	Error            string
	Notice           string
	Users            []database.User
	Settings         []database.Setting
	AuditLogs        []database.AuditEntry
	AuditQuery       string
	AuditAction      string
	AuditResult      string
	Suppliers        []database.Supplier
	Products         []database.Product
	Product          database.Product
	Purchases        []database.PurchaseSlip
	Purchase         database.PurchaseSlip
	ProductForm      productForm
	Duplicates       []database.Product
	Query            string
	Status           string
	Sort             string
	IncludeCancelled bool
	TotalProducts    int
	Page             int
	TotalPages       int
	PreviousPage     int
	NextPage         int
	HasPrevious      bool
	HasNext          bool
	Stats            database.InventoryStats
	MarketPrices     []database.MarketPrice
	ExchangeRates    []database.ExchangeRate
	ImportBatch      database.MarketImportBatch
	Sales            []database.SalesSlip
	Sale             database.SalesSlip
	Shipments        []database.ShipmentSlip
	Shipment         database.ShipmentSlip
	PublicProducts   []database.PublicProduct
	PurchaseRequests []database.PurchaseRequest
	Approvals        []database.ApprovalRequest
	SalesTotalUSD    int64
	SalesCount       int
	RequestCount     int
	PreviewMode      bool
}

func New(cfg config.Config, store *database.Store, repository *persistence.Repository, objects storage.Store, logger *slog.Logger) (*Server, error) {
	s := &Server{cfg: cfg, store: store, repository: repository, objects: objects, log: logger}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/health", s.apiHealth)
	mux.HandleFunc("POST /api/v1/auth/login", s.apiLogin)
	mux.HandleFunc("POST /api/v1/auth/password-reset", s.apiPasswordResetComplete)
	mux.Handle("GET /api/v1/auth/me", s.apiAuthenticated("", http.HandlerFunc(s.apiMe)))
	mux.Handle("POST /api/v1/auth/logout", s.apiAuthenticated("", http.HandlerFunc(s.apiLogout)))
	mux.Handle("GET /api/v1/dashboard", s.apiAuthenticated("dashboard.read", http.HandlerFunc(s.apiDashboard)))
	mux.Handle("GET /api/v1/products", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiProducts)))
	mux.Handle("POST /api/v1/products", s.apiAuthenticated("inventory.write", http.HandlerFunc(s.apiProductCreate)))
	mux.Handle("GET /api/v1/products/{id}", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiProduct)))
	mux.Handle("GET /api/v1/purchases", s.apiAuthenticated("purchase.read", http.HandlerFunc(s.apiPurchases)))
	mux.Handle("GET /api/v1/purchases/{id}", s.apiAuthenticated("purchase.read", http.HandlerFunc(s.apiPurchase)))
	mux.Handle("POST /api/v1/purchases", s.apiAuthenticated("purchase.write", http.HandlerFunc(s.apiPurchaseCreate)))
	mux.Handle("POST /api/v1/purchases/{id}/confirm", s.apiAuthenticated("purchase.confirm", http.HandlerFunc(s.apiPurchaseConfirm)))
	mux.Handle("GET /api/v1/market-prices", s.apiAuthenticated("market.read", http.HandlerFunc(s.apiMarketPrices)))
	mux.Handle("POST /api/v1/market-prices", s.apiAuthenticated("market.write", http.HandlerFunc(s.apiMarketPriceCreate)))
	mux.Handle("PATCH /api/v1/market-prices/{id}", s.apiAuthenticated("market.write", http.HandlerFunc(s.apiMarketPriceUpdate)))
	mux.Handle("POST /api/v1/market-prices/imports/preview", s.apiAuthenticated("market.import", http.HandlerFunc(s.apiMarketImportPreview)))
	mux.Handle("GET /api/v1/market-prices/imports/{id}", s.apiAuthenticated("market.import", http.HandlerFunc(s.apiMarketImportDetail)))
	mux.Handle("POST /api/v1/market-prices/imports/{id}/commit", s.apiAuthenticated("market.import", http.HandlerFunc(s.apiMarketImportCommit)))
	mux.Handle("GET /api/v1/boxes", s.apiAuthenticated("inventory.publish", http.HandlerFunc(s.apiBoxes)))
	mux.Handle("PUT /api/v1/boxes/{code}", s.apiAuthenticated("inventory.publish", http.HandlerFunc(s.apiBoxUpdate)))
	mux.Handle("GET /api/v1/guest/catalog", s.apiAuthenticated("", http.HandlerFunc(s.apiGuestCatalog)))
	mux.Handle("GET /api/v1/guest/purchase-requests", s.apiAuthenticated("", http.HandlerFunc(s.apiPurchaseRequests)))
	mux.Handle("POST /api/v1/guest/purchase-requests", s.apiAuthenticated("", http.HandlerFunc(s.apiGuestPurchaseRequestCreate)))
	mux.Handle("GET /api/v1/purchase-requests", s.apiAuthenticated("request.read", http.HandlerFunc(s.apiPurchaseRequests)))
	mux.Handle("POST /api/v1/purchase-requests/{id}/{decision}", s.apiAuthenticated("request.review", http.HandlerFunc(s.apiPurchaseRequestReview)))
	mux.Handle("GET /api/v1/notifications", s.apiAuthenticated("", http.HandlerFunc(s.apiNotifications)))
	mux.Handle("POST /api/v1/notifications/{id}/read", s.apiAuthenticated("", http.HandlerFunc(s.apiNotificationRead)))
	mux.Handle("GET /api/v1/approvals", s.apiAuthenticated("approval.read", http.HandlerFunc(s.apiApprovals)))
	mux.Handle("POST /api/v1/approvals", s.apiAuthenticated("approval.request", http.HandlerFunc(s.apiApprovalCreate)))
	mux.Handle("POST /api/v1/approvals/{id}/{decision}", s.apiAuthenticated("approval.approve", http.HandlerFunc(s.apiApprovalDecision)))
	mux.Handle("GET /api/v1/sales", s.apiAuthenticated("sales.read", http.HandlerFunc(s.apiSales)))
	mux.Handle("GET /api/v1/sales/{id}", s.apiAuthenticated("sales.read", http.HandlerFunc(s.apiSale)))
	mux.Handle("POST /api/v1/sales", s.apiAuthenticated("sales.write", http.HandlerFunc(s.apiSaleCreate)))
	mux.Handle("POST /api/v1/sales/{id}/confirm", s.apiAuthenticated("sales.confirm", http.HandlerFunc(s.apiSaleConfirm)))
	mux.Handle("GET /api/v1/shipments", s.apiAuthenticated("shipment.read", http.HandlerFunc(s.apiShipments)))
	mux.Handle("GET /api/v1/shipments/{id}", s.apiAuthenticated("shipment.read", http.HandlerFunc(s.apiShipment)))
	mux.Handle("POST /api/v1/shipments", s.apiAuthenticated("shipment.write", http.HandlerFunc(s.apiShipmentCreate)))
	mux.Handle("POST /api/v1/shipments/{id}/confirm", s.apiAuthenticated("shipment.confirm", http.HandlerFunc(s.apiShipmentConfirm)))
	mux.Handle("PATCH /api/v1/shipments/{id}/tracking", s.apiAuthenticated("shipment.write", http.HandlerFunc(s.apiShipmentTrackingUpdate)))
	mux.Handle("GET /api/v1/returns", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiReturns)))
	mux.Handle("GET /api/v1/returns/{id}", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiReturn)))
	mux.Handle("POST /api/v1/returns", s.apiAuthenticated("inventory.write", http.HandlerFunc(s.apiReturnCreate)))
	mux.Handle("POST /api/v1/returns/{id}/confirm", s.apiAuthenticated("inventory.write", http.HandlerFunc(s.apiReturnConfirm)))
	mux.Handle("PATCH /api/v1/returns/{id}/tracking", s.apiAuthenticated("inventory.write", http.HandlerFunc(s.apiReturnTrackingUpdate)))
	mux.Handle("GET /api/v1/documents", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiDocuments)))
	mux.Handle("GET /api/v1/document-events", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiDocumentEvents)))
	mux.Handle("POST /api/v1/document-events", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiDocumentEventCreate)))
	mux.Handle("GET /api/v1/exports/{kind}", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiCSVExport)))
	mux.Handle("GET /api/v1/settings", s.apiAuthenticated("settings.manage", http.HandlerFunc(s.apiSettings)))
	mux.Handle("PUT /api/v1/settings/{key}", s.apiAuthenticated("settings.manage", http.HandlerFunc(s.apiSettingUpdate)))
	mux.Handle("GET /api/v1/admin-access-code", s.apiAuthenticated("settings.manage", http.HandlerFunc(s.apiAdminAccessCode)))
	mux.Handle("POST /api/v1/admin-access-code/rotate", s.apiAuthenticated("settings.manage", http.HandlerFunc(s.apiAdminAccessCodeRotate)))
	mux.Handle("POST /api/v1/admin-access-code/verify", s.apiAuthenticated("", http.HandlerFunc(s.apiAdminAccessCodeVerify)))
	mux.Handle("GET /api/v1/company", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiCompanyInfo)))
	mux.Handle("PUT /api/v1/company", s.apiAuthenticated("settings.manage", http.HandlerFunc(s.apiCompanyInfoUpdate)))
	mux.Handle("GET /api/v1/exchange-rates", s.apiAuthenticated("market.read", http.HandlerFunc(s.apiExchangeRates)))
	mux.Handle("POST /api/v1/exchange-rates", s.apiAuthenticated("market.write", http.HandlerFunc(s.apiExchangeRateCreate)))
	mux.Handle("POST /api/v1/products/{id}/files", s.apiAuthenticated("inventory.write", http.HandlerFunc(s.apiProductFileUpload)))
	mux.Handle("GET /api/v1/products/{id}/files", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiProductFiles)))
	mux.Handle("PATCH /api/v1/products/{id}", s.apiAuthenticated("inventory.write", http.HandlerFunc(s.apiProductUpdate)))
	mux.Handle("GET /api/v1/product-files/{id}", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiProductFile)))
	mux.Handle("GET /api/v1/users", s.apiAuthenticated("users.manage", http.HandlerFunc(s.apiUsers)))
	mux.Handle("GET /api/v1/purchase-staff", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiPurchaseStaff)))
	mux.Handle("POST /api/v1/users", s.apiAuthenticated("users.manage", http.HandlerFunc(s.apiUserCreate)))
	mux.Handle("PATCH /api/v1/users/{id}", s.apiAuthenticated("users.manage", http.HandlerFunc(s.apiUserUpdate)))
	mux.Handle("POST /api/v1/users/{id}/password", s.apiAuthenticated("users.manage", http.HandlerFunc(s.apiUserPassword)))
	mux.Handle("POST /api/v1/users/{id}/password-reset", s.apiAuthenticated("users.manage", http.HandlerFunc(s.apiPasswordResetRequest)))
	mux.Handle("GET /api/v1/partners", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiPartners)))
	mux.Handle("POST /api/v1/partners", s.apiAuthenticated("settings.manage", http.HandlerFunc(s.apiPartnerCreate)))
	mux.Handle("PATCH /api/v1/partners/{id}", s.apiAuthenticated("settings.manage", http.HandlerFunc(s.apiPartnerUpdate)))
	mux.Handle("GET /api/v1/audit-logs", s.apiAuthenticated("audit.read", http.HandlerFunc(s.apiAuditLogs)))
	mux.Handle("GET /api/v1/email-outbox", s.apiAuthenticated("users.manage", http.HandlerFunc(s.apiEmailOutbox)))
	mux.Handle("GET /api/v1/masters/{kind}", s.apiAuthenticated("inventory.read", http.HandlerFunc(s.apiMasterItems)))
	mux.Handle("POST /api/v1/masters/{kind}", s.apiAuthenticated("settings.manage", http.HandlerFunc(s.apiMasterCreate)))
	mux.Handle("PATCH /api/v1/masters/{kind}/{id}", s.apiAuthenticated("settings.manage", http.HandlerFunc(s.apiMasterUpdate)))
	mux.HandleFunc("GET /app", redirectToReact)
	mux.HandleFunc("GET /app/{path...}", s.reactApp)
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("GET /public/products", s.publicProducts)
	mux.HandleFunc("POST /public/products/{id}/purchase-requests", s.publicPurchaseRequest)

	mux.HandleFunc("GET /", redirectToReact)
	mux.Handle("GET /legacy", s.authenticated("dashboard.read", http.HandlerFunc(s.dashboard)))
	mux.Handle("POST /logout", s.authenticated("", http.HandlerFunc(s.logout)))
	mux.Handle("GET /products", s.authenticated("inventory.read", http.HandlerFunc(s.products)))
	mux.Handle("GET /products/export.csv", s.authenticated("inventory.read", http.HandlerFunc(s.productsCSV)))
	mux.Handle("GET /products/new", s.authenticated("inventory.write", http.HandlerFunc(s.productNew)))
	mux.Handle("POST /products", s.authenticated("inventory.write", http.HandlerFunc(s.productCreate)))
	mux.Handle("GET /products/{id}", s.authenticated("inventory.read", http.HandlerFunc(s.productDetail)))
	mux.Handle("POST /products/{id}/status", s.authenticated("inventory.write", http.HandlerFunc(s.productStatus)))
	mux.Handle("POST /products/{id}/images", s.authenticated("inventory.write", http.HandlerFunc(s.productImageUpload)))
	mux.Handle("POST /products/{id}/publication", s.authenticated("inventory.publish", http.HandlerFunc(s.productPublication)))
	mux.Handle("GET /product-images/{id}", s.authenticated("inventory.read", http.HandlerFunc(s.productImage)))
	mux.Handle("GET /purchases", s.authenticated("purchase.read", http.HandlerFunc(s.purchases)))
	mux.Handle("GET /purchases/new", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseNew)))
	mux.Handle("POST /purchases", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseCreate)))
	mux.Handle("GET /purchases/{id}", s.authenticated("purchase.read", http.HandlerFunc(s.purchaseDetail)))
	mux.Handle("POST /purchases/{id}/confirm", s.authenticated("purchase.confirm", http.HandlerFunc(s.purchaseConfirm)))
	mux.Handle("GET /market-prices", s.authenticated("market.read", http.HandlerFunc(s.marketPrices)))
	mux.Handle("POST /market-prices", s.authenticated("market.write", http.HandlerFunc(s.marketPriceCreate)))
	mux.Handle("POST /exchange-rates", s.authenticated("market.write", http.HandlerFunc(s.exchangeRateCreate)))
	mux.Handle("GET /market-prices/import", s.authenticated("market.import", http.HandlerFunc(s.marketImportPage)))
	mux.Handle("POST /market-prices/import/preview", s.authenticated("market.import", http.HandlerFunc(s.marketImportPreview)))
	mux.Handle("GET /market-prices/import/{id}", s.authenticated("market.import", http.HandlerFunc(s.marketImportDetail)))
	mux.Handle("POST /market-prices/import/{id}/commit", s.authenticated("market.import", http.HandlerFunc(s.marketImportCommit)))
	mux.Handle("GET /sales", s.authenticated("sales.read", http.HandlerFunc(s.sales)))
	mux.Handle("GET /sales/new", s.authenticated("sales.write", http.HandlerFunc(s.saleNew)))
	mux.Handle("POST /sales", s.authenticated("sales.write", http.HandlerFunc(s.saleCreate)))
	mux.Handle("GET /sales/{id}", s.authenticated("sales.read", http.HandlerFunc(s.saleDetail)))
	mux.Handle("POST /sales/{id}/confirm", s.authenticated("sales.confirm", http.HandlerFunc(s.saleConfirm)))
	mux.Handle("POST /sales/{id}/cancel", s.authenticated("sales.write", http.HandlerFunc(s.saleCancel)))
	mux.Handle("GET /shipments", s.authenticated("shipment.read", http.HandlerFunc(s.shipments)))
	mux.Handle("GET /shipments/new", s.authenticated("shipment.write", http.HandlerFunc(s.shipmentNew)))
	mux.Handle("POST /shipments", s.authenticated("shipment.write", http.HandlerFunc(s.shipmentCreate)))
	mux.Handle("GET /shipments/{id}", s.authenticated("shipment.read", http.HandlerFunc(s.shipmentDetail)))
	mux.Handle("POST /shipments/{id}/confirm", s.authenticated("shipment.confirm", http.HandlerFunc(s.shipmentConfirm)))
	mux.Handle("POST /shipments/{id}/cancel", s.authenticated("shipment.write", http.HandlerFunc(s.shipmentCancel)))
	mux.Handle("GET /purchase-requests", s.authenticated("request.read", http.HandlerFunc(s.purchaseRequests)))
	mux.Handle("POST /purchase-requests/{id}/approve", s.authenticated("request.review", http.HandlerFunc(s.purchaseRequestApprove)))
	mux.Handle("POST /purchase-requests/{id}/reject", s.authenticated("request.review", http.HandlerFunc(s.purchaseRequestReject)))
	mux.Handle("POST /purchase-requests/{id}/cancel", s.authenticated("request.review", http.HandlerFunc(s.purchaseRequestCancel)))
	mux.Handle("GET /approvals", s.authenticated("approval.approve", http.HandlerFunc(s.approvals)))
	mux.Handle("POST /approvals/{id}/approve", s.authenticated("approval.approve", http.HandlerFunc(s.approvalApprove)))
	mux.Handle("POST /approvals/{id}/return", s.authenticated("approval.approve", http.HandlerFunc(s.approvalReturn)))
	mux.Handle("POST /approvals/{id}/reject", s.authenticated("approval.approve", http.HandlerFunc(s.approvalReject)))
	mux.Handle("GET /users", s.authenticated("users.manage", http.HandlerFunc(s.users)))
	mux.Handle("GET /settings", s.authenticated("settings.manage", http.HandlerFunc(s.settings)))
	mux.Handle("POST /settings", s.authenticated("settings.manage", http.HandlerFunc(s.updateSetting)))
	mux.Handle("GET /audit", s.authenticated("audit.read", http.HandlerFunc(s.audit)))
	s.handler = s.securityHeaders(s.requestIdentity(s.recoverer(mux)))
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) parseTemplates() error {
	funcs := template.FuncMap{
		"roleName": func(role string) string {
			if role == database.RoleAdmin {
				return "管理者"
			}
			return "作業者"
		},
		"settingName": func(key string) string {
			names := map[string]string{
				"approval.purchase_threshold_jpy":         "仕入承認金額（JPY）",
				"approval.sales_threshold_jpy":            "売上承認金額（JPY）",
				"approval.admin_high_value_enabled":       "管理者高額承認モード",
				"approval.admin_high_value_threshold_jpy": "管理者高額承認金額（JPY）",
				"reservation.duration_hours":              "取置期限（時間）",
				"exchange_rate.provider":                  "為替レート取得方法",
				"csv.encoding":                            "CSV文字コード",
			}
			return names[key]
		},
		"formatTime": func(value time.Time) string {
			return value.In(time.FixedZone("JST", 9*60*60)).Format("2006/01/02 15:04:05")
		},
		"formatMoney": func(value int64, currency string) string {
			if currency == "JPY" {
				return "¥" + formatInteger(value)
			}
			return "$" + formatInteger(value)
		},
		"productStatus": func(status string) string {
			return map[string]string{
				"purchasing": "仕入中", "in_stock": "在庫中", "reserved": "取置中",
				"sold": "販売済み", "shipped": "出荷済み", "cancelled": "取消", "invalid": "無効",
				"public": "公開", "private": "非公開",
			}[status]
		},
		"purchaseStatus": func(status string) string {
			return map[string]string{"draft": "下書き", "confirmed": "確定", "cancelled": "取消"}[status]
		},
		"saleStatus": func(status string) string {
			return map[string]string{"draft": "下書き", "pending_approval": "承認待ち", "confirmed": "確定", "cancelled": "取消"}[status]
		},
		"shipmentStatus": func(status string) string {
			return map[string]string{"draft": "下書き", "confirmed": "確定", "cancelled": "取消"}[status]
		},
		"deliveryStatus": func(status string) string {
			return map[string]string{"not_confirmed": "売上未確定", "unshipped": "未出荷", "partial": "一部出荷", "complete": "出荷完了"}[status]
		},
		"requestStatus": func(status string) string {
			return map[string]string{
				"pending": "申請中", "approved": "承認済み・取置中", "rejected": "却下",
				"cancelled": "取消", "expired": "期限切れ", "sold": "売上化",
			}[status]
		},
		"approvalStatus": func(status string) string {
			return map[string]string{
				"requested": "申請", "pending": "申請中", "approved": "承認済み", "returned": "差戻し",
				"rejected": "却下", "cancelled": "取消", "expired": "失効",
			}[status]
		},
		"approvalAction": func(action string) string {
			return map[string]string{
				"sale.confirm": "売上確定", "sale.cancel": "売上取消", "shipment.cancel": "出荷取消",
			}[action]
		},
		"formatRate": func(rate, scale int64) string {
			if scale <= 0 {
				return "—"
			}
			integer := rate / scale
			fraction := rate % scale
			return fmt.Sprintf("%d.%08d", integer, fraction)
		},
		"formatPercent": func(basisPoints int64) string {
			return fmt.Sprintf("%.2f%%", float64(basisPoints)/100)
		},
	}
	base, err := template.New("base.html").Funcs(funcs).ParseFS(assets, "templates/base.html")
	if err != nil {
		return err
	}
	s.templates = make(map[string]*template.Template)
	for _, page := range []string{
		"login", "dashboard", "users", "settings", "audit", "public",
		"products", "product-new", "product-detail", "purchases", "purchase-new", "purchase-detail",
		"market", "market-import", "market-import-preview",
		"sales", "sale-new", "sale-detail", "shipments", "shipment-new", "shipment-detail",
		"purchase-requests", "approvals",
	} {
		clone, err := base.Clone()
		if err != nil {
			return err
		}
		if _, err := clone.ParseFS(assets, "templates/"+page+".html"); err != nil {
			return fmt.Errorf("parse %s: %w", page, err)
		}
		s.templates[page] = clone
	}
	return nil
}

func (s *Server) render(w http.ResponseWriter, name string, status int, data pageData) {
	data.PreviewMode = s.cfg.Environment == "development"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.templates[name].ExecuteTemplate(w, "base", data); err != nil {
		s.log.Error("render template", "template", name, "error", err)
	}
}

func (s *Server) renderPartial(w http.ResponseWriter, name, definition string, status int, data pageData) {
	data.PreviewMode = s.cfg.Environment == "development"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "HX-Request")
	w.WriteHeader(status)
	if err := s.templates[name].ExecuteTemplate(w, definition, data); err != nil {
		s.log.Error("render partial template", "template", name, "definition", definition, "error", err)
	}
}

func isHXRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("HX-Request")), "true")
}

func writeRequestError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if isHXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Vary", "HX-Request")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `<div class="alert error" role="alert">%s</div>`, template.HTMLEscapeString(message))
		return
	}
	http.Error(w, message, status)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "phase": "8"})
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if _, err := s.store.Session(r.Context(), cookie.Value); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	csrf, err := database.RandomToken()
	if err != nil || s.store.CreateLoginCSRF(r.Context(), csrf, 10*time.Minute) != nil {
		http.Error(w, "ログイン画面を準備できませんでした", http.StatusInternalServerError)
		return
	}
	s.render(w, "login", http.StatusOK, pageData{Title: "ログイン", CSRF: csrf, Notice: r.URL.Query().Get("notice")})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "入力を確認してください", http.StatusBadRequest)
		return
	}
	csrf := r.FormValue("csrf_token")
	if !s.store.ConsumeLoginCSRF(r.Context(), csrf) {
		freshCSRF, _ := database.RandomToken()
		_ = s.store.CreateLoginCSRF(r.Context(), freshCSRF, 10*time.Minute)
		s.render(w, "login", http.StatusForbidden, pageData{Title: "ログイン", CSRF: freshCSRF, Error: "画面の有効期限が切れました。再読み込みしてください。"})
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	user, err := s.store.Authenticate(r.Context(), s.cfg.OrganizationCode, username, r.FormValue("password"))
	if err != nil {
		_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
			TargetType: "session", TargetID: username, Action: "login.failed", Result: "denied",
			Reason: "invalid credentials", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
		})
		csrf, _ := database.RandomToken()
		_ = s.store.CreateLoginCSRF(r.Context(), csrf, 10*time.Minute)
		s.render(w, "login", http.StatusUnauthorized, pageData{Title: "ログイン", CSRF: csrf, Error: "ユーザーIDまたはパスワードが違います。"})
		return
	}
	token, tokenErr := database.RandomToken()
	sessionCSRF, csrfErr := database.RandomToken()
	if tokenErr != nil || csrfErr != nil || s.store.CreateSession(r.Context(), user, token, sessionCSRF, clientIP(r), r.UserAgent(), s.cfg.SessionTTL) != nil {
		http.Error(w, "セッションを作成できませんでした", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.CookieSecure, MaxAge: int(s.cfg.SessionTTL.Seconds())})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: sessionCSRF, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.cfg.CookieSecure, MaxAge: int(s.cfg.SessionTTL.Seconds())})
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "session", TargetID: user.ID,
		Action: "login.succeeded", Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	cookie, _ := r.Cookie(sessionCookie)
	if cookie != nil {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "session", TargetID: user.ID,
		Action: "logout", Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/login?notice="+url.QueryEscape("ログアウトしました。"), http.StatusSeeOther)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	stats, _ := s.store.InventoryStats(r.Context(), user.OrganizationID)
	sales, _ := s.store.Sales(r.Context(), user.OrganizationID)
	var salesTotal int64
	var salesCount int
	for _, sale := range sales {
		if sale.Status == "confirmed" {
			salesTotal += sale.TotalUSD
			salesCount++
		}
	}
	requests, _ := s.store.PurchaseRequests(r.Context(), user.OrganizationID)
	var requestCount int
	for _, request := range requests {
		if request.Status == "pending" {
			requestCount++
		}
	}
	s.render(w, "dashboard", http.StatusOK, pageData{
		Title: "ダッシュボード", Active: "dashboard", User: user, CSRF: csrfFromRequest(r), Stats: stats,
		SalesTotalUSD: salesTotal, SalesCount: salesCount, RequestCount: requestCount,
	})
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	users, err := s.store.Users(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "利用者を取得できませんでした", http.StatusInternalServerError)
		return
	}
	s.render(w, "users", http.StatusOK, pageData{Title: "利用者・権限", Active: "users", User: user, Users: users, CSRF: csrfFromRequest(r)})
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	settings, err := s.store.Settings(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "設定を取得できませんでした", http.StatusInternalServerError)
		return
	}
	s.render(w, "settings", http.StatusOK, pageData{Title: "組織設定", Active: "settings", User: user, Settings: settings, CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice")})
}

func (s *Server) updateSetting(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "入力を確認してください", http.StatusBadRequest)
		return
	}
	before, after, err := s.store.UpdateSetting(r.Context(), user.OrganizationID, user.ID, r.FormValue("key"), r.FormValue("value"))
	if err != nil {
		http.Error(w, "設定を更新できませんでした", http.StatusBadRequest)
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "organization_setting", TargetID: after.Key,
		Action: "setting.updated", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/settings?notice="+url.QueryEscape("設定を保存しました。"), http.StatusSeeOther)
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	entries, err := s.store.AuditLogs(r.Context(), user.OrganizationID, 500)
	if err != nil {
		http.Error(w, "監査ログを取得できませんでした", http.StatusInternalServerError)
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	result := strings.TrimSpace(r.URL.Query().Get("result"))
	filtered := entries[:0]
	for _, entry := range entries {
		if action != "" && entry.Action != action {
			continue
		}
		if result != "" && entry.Result != result {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(
			entry.ActorName+" "+entry.TargetType+" "+entry.TargetID+" "+entry.Action+" "+entry.RequestID,
		), query) {
			continue
		}
		filtered = append(filtered, entry)
	}
	s.render(w, "audit", http.StatusOK, pageData{
		Title: "監査ログ", Active: "audit", User: user, AuditLogs: filtered,
		AuditQuery: query, AuditAction: action, AuditResult: result, CSRF: csrfFromRequest(r),
	})
}

func (s *Server) authenticated(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			if isHXRequest(r) {
				w.Header().Set("HX-Redirect", "/login")
				writeRequestError(w, r, http.StatusUnauthorized, "ログインが必要です。")
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		session, err := s.store.Session(r.Context(), cookie.Value)
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
			if isHXRequest(r) {
				w.Header().Set("HX-Redirect", "/login")
				writeRequestError(w, r, http.StatusUnauthorized, "セッションの有効期限が切れました。")
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if isUnsafe(r.Method) {
			formToken := r.Header.Get("X-CSRF-Token")
			if formToken == "" {
				if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
					r.Body = http.MaxBytesReader(w, r.Body, 9<<20)
					_ = r.ParseMultipartForm(8 << 20)
				} else {
					_ = r.ParseForm()
				}
				if r.Form != nil {
					formToken = r.FormValue("csrf_token")
				}
			}
			if subtle.ConstantTimeCompare([]byte(database.TokenHash(formToken)), []byte(session.CSRFTokenHash)) != 1 {
				writeRequestError(w, r, http.StatusForbidden, "不正な操作を拒否しました。画面を再読み込みしてください。")
				return
			}
		}
		if permission != "" && !s.store.HasPermission(r.Context(), session.User, permission) {
			writeRequestError(w, r, http.StatusForbidden, "この操作を行う権限がありません。")
			return
		}
		ctx := withSession(withUser(r.Context(), session.User), session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requestIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := database.NewID("req")
		if err != nil {
			http.Error(w, "request id unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app" || strings.HasPrefix(r.URL.Path, "/app/") {
			// The reference design is the canonical application UI. It uses inline
			// styles and handlers plus Font Awesome and Chart.js. Business data still
			// travels through the same-origin Go REST API; product photos may use the
			// reference image CDN until they are replaced by the local/S3 adapter.
			w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; font-src 'self' data: https://cdn.jsdelivr.net; img-src 'self' data: blob: https://sspark.genspark.ai; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
			w.Header().Set("X-Frame-Options", "DENY")
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
			w.Header().Set("X-Frame-Options", "DENY")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("panic recovered", "error", recovered, "request_id", requestID(r.Context()))
				http.Error(w, "処理中にエラーが発生しました。", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func isUnsafe(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func csrfFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(csrfCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func formatInteger(value int64) string {
	negative := value < 0
	text := fmt.Sprintf("%d", value)
	if negative {
		text = strings.TrimPrefix(text, "-")
	}
	for index := len(text) - 3; index > 0; index -= 3 {
		text = text[:index] + "," + text[index:]
	}
	if negative {
		return "-" + text
	}
	return text
}
