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
)

//go:embed templates/base.html templates/login.html templates/guest-login.html templates/dashboard.html templates/masters.html templates/guest-management.html templates/guest-box-modal.html templates/guest-box-edit-modal.html templates/stocktakes.html templates/performance.html templates/returns.html templates/return-detail.html templates/return-restore-modal.html templates/users.html templates/settings.html templates/audit.html templates/public.html templates/products.html templates/product-modal.html templates/product-edit-modal.html templates/product-new.html templates/product-detail.html templates/purchases.html templates/purchase-new.html templates/purchase-detail.html templates/purchase-slip-modal.html templates/purchase-slip-edit-modal.html templates/purchase-return-new-modal.html templates/purchase-return-slip-modal.html templates/purchase-return-invoice.html templates/shipment-slip-modal.html templates/shipment-slip-edit-modal.html templates/sales-slip-modal.html templates/sales-slip-edit-modal.html templates/sales-return-new-modal.html templates/sales-return-slip-modal.html templates/sales-return-invoice.html templates/invoice-previews.html templates/market.html templates/market-import.html templates/market-import-preview.html templates/sales.html templates/sale-new.html templates/sale-detail.html templates/shipments.html templates/shipment-new.html templates/shipment-detail.html templates/slips.html templates/purchase-requests.html templates/approvals.html static/app.css static/app.js
var assets embed.FS

const sessionCookie = "zaiko_session"
const csrfCookie = "zaiko_csrf"
const guestSessionCookie = "zaiko_guest_session"

type Server struct {
	cfg       config.Config
	store     *database.Store
	log       *slog.Logger
	templates map[string]*template.Template
	handler   http.Handler
}

type pageData struct {
	Title                              string
	Active                             string
	User                               database.User
	CSRF                               string
	Error                              string
	Notice                             string
	Users                              []database.User
	Settings                           []database.Setting
	AuditLogs                          []database.AuditEntry
	AuditQuery                         string
	AuditAction                        string
	AuditResult                        string
	Suppliers                          []database.Supplier
	Products                           []database.Product
	Product                            database.Product
	ProductBrands                      []string
	Purchases                          []database.PurchaseSlip
	Purchase                           database.PurchaseSlip
	PurchaseProducts                   []database.PurchaseProductLine
	PurchaseRevisions                  []database.PurchaseRevision
	NextPurchaseNumber                 string
	ProductForm                        productForm
	Duplicates                         []database.Product
	ProductBrandOptions                []database.MasterRecord
	ProductMaterialOptions             []database.MasterRecord
	ProductMovementOptions             []database.MasterRecord
	ProductConditionOptions            []database.MasterRecord
	ProductAccessoryOptions            []database.MasterRecord
	NextProductCode                    string
	Query                              string
	Brand                              string
	ModelNumber                        string
	SerialNumber                       string
	SKU                                string
	SupplierID                         string
	BuyerID                            string
	Box                                string
	Accessory                          string
	PurchaseDateFrom                   string
	PurchaseDateTo                     string
	Status                             string
	Sort                               string
	IncludeCancelled                   bool
	InventorySearchRequested           bool
	TotalProducts                      int
	Page                               int
	TotalPages                         int
	PreviousPage                       int
	NextPage                           int
	CurrentURL                         string
	HasPrevious                        bool
	HasNext                            bool
	Stats                              database.InventoryStats
	MarketPrices                       []database.MarketPrice
	MarketProductPrices                []database.ProductMarketPrice
	MarketSearchRequested              bool
	MarketProductPrice                 database.ProductMarketPrice
	ExchangeRates                      []database.ExchangeRate
	USDJPYRate                         database.ExchangeRate
	USDJPYRateAvailable                bool
	ImportBatch                        database.MarketImportBatch
	Sales                              []database.SalesSlip
	Sale                               database.SalesSlip
	SalesRevisions                     []database.SalesRevision
	SalesDestinationOptions            []database.MasterRecord
	NextSalesNumber                    string
	NextShipmentNumber                 string
	Shipments                          []database.ShipmentSlip
	Shipment                           database.ShipmentSlip
	ShipmentRevisions                  []database.ShipmentRevision
	PublicProducts                     []database.PublicProduct
	PublicQuery                        string
	PublicBrand                        string
	PublicCondition                    string
	PurchaseRequests                   []database.PurchaseRequest
	PurchaseRequestGroups              []purchaseRequestGroupView
	PurchaseRequestTotal               int64
	PurchaseRequestPending             int
	ShipmentPrefillCodes               []string
	ShipmentPrefillRecipient           string
	ShipmentPurchaseRequestGroup       string
	Approvals                          []database.ApprovalRequest
	ApprovalReviewViews                []approvalReviewView
	ApprovalPendingCount               int
	SalesTotalJPY                      int64
	SalesCount                         int
	RequestCount                       int
	PurchaseTotalJPY                   int64
	PurchaseCount                      int
	AlertCount                         int
	ApprovalAlertCount                 int
	PurchaseAlertCount                 int
	NotificationCountsSet              bool
	SalesTrendText                     string
	DashboardSuppliers                 []dashboardSupplierSlice
	DashboardMonths                    []dashboardMonthBar
	DashboardGridlines                 []dashboardGridline
	DashboardRequests                  []dashboardRequestCard
	DashboardSalesTarget               int64
	DashboardPurchaseBudget            int64
	MasterCategories                   []masterCategory
	MasterRecords                      []database.MasterRecord
	MasterCategory                     masterCategory
	MasterUsers                        []database.User
	MasterGuestCredentials             []database.GuestCredential
	MasterSettings                     map[string]string
	MasterExchangeRates                []masterExchangeRateCard
	GuestCompanies                     []database.GuestCompany
	GuestBoxes                         []database.GuestBox
	GuestBoxMatrix                     []database.GuestBoxMatrixCell
	GuestBoxProducts                   []database.GuestBoxProduct
	GuestSelectedBox                   database.GuestBox
	GuestPublicationSummary            database.GuestPublicationSummary
	GuestPublishedCompanies            []database.GuestCompany
	GuestProductCandidates             []database.GuestBoxProduct
	GuestProductSearched               bool
	GuestProductDateFrom               string
	GuestProductDateTo                 string
	GuestProductBrand                  string
	GuestProductQuery                  string
	GuestBrands                        []database.MasterRecord
	Stocktakes                         []database.Stocktake
	Stocktake                          database.Stocktake
	StocktakeCanComplete               bool
	StocktakeInStockCount              int
	StocktakeShippedCount              int
	StocktakeInStockCountedCount       int
	StocktakeShippedCountedCount       int
	StocktakeInStockTotal              int64
	StocktakeShippedTotal              int64
	StocktakeInStockCountedTotal       int64
	StocktakeShippedCountedTotal       int64
	StocktakeCountedTotal              int64
	StocktakeDifferenceTotal           int64
	StocktakePendingCount              int
	TodayISO                           string
	PerformanceRows                    []performanceRow
	PerformanceMode                    performanceMode
	PerformanceFrom                    string
	PerformanceTo                      string
	PerformanceTotal                   int64
	PerformanceCount                   int
	BuyerPerformanceRows               []buyerPerformanceRow
	BuyerPerformanceSummary            buyerPerformanceSummary
	SalesDestinationPerformanceRows    []salesDestinationPerformanceRow
	SalesDestinationPerformanceSummary salesDestinationPerformanceSummary
	ReturnSummaries                    []database.ReturnTakehomeSummary
	ReturnItems                        []database.ReturnTakehomeItem
	ReturnPendingItems                 []database.ReturnTakehomeItem
	ReturnCompletedItems               []database.ReturnTakehomeItem
	ReturnReferenceItems               []returnReferenceItem
	ReturnOriginalTotal                int64
	ReturnPendingTotal                 int64
	ReturnNetTotal                     int64
	SalesReturnItems                   []database.ReturnTakehomeItem
	SalesReturnNumber                  string
	SalesReturnTotal                   int64
	SalesReturnInvoiceReady            bool
	SalesReturnCompleted               bool
	SalesReturnReason                  string
	SalesReturnNotes                   string
	SalesReturnActor                   string
	SalesReturnRequestedAt             time.Time
	PurchaseReturn                     database.PurchaseReturnSlip
	PurchaseReturnLines                []database.PurchaseReturnLine
	PurchaseReturnTotal                int64
	PurchaseReturnCompleted            bool
	InvoiceCompany                     invoiceCompany
	SalesInvoiceGroups                 []salesInvoiceGroup
	PurchaseInvoiceGroups              []purchaseReturnInvoiceGroup
	InvoiceSelectedCount               int
	InvoicePartnerCount                int
	SlipRows                           []slipListRow
	SlipTabs                           []slipListTab
	SlipKind                           string
	SlipDateFrom                       string
	SlipDateTo                         string
	SlipPartner                        string
	SlipPartners                       []string
	SlipSummary                        slipListSummary
	SlipSearchPerformed                bool
	ApprovalOnly                       bool
	PreviewMode                        bool
	LoginAdminPassword                 string
	LoginWorkerPassword                string
	GuestCompanyCode                   string
	GuestCompanyName                   string
	LoginGuestPassword                 string
}

func New(cfg config.Config, store *database.Store, logger *slog.Logger) (*Server, error) {
	s := &Server{cfg: cfg, store: store, log: logger}
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
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("GET /guest/login", s.guestLoginPage)
	mux.HandleFunc("POST /guest/login", s.guestLogin)
	mux.Handle("GET /public/products", s.guestAuthenticated(http.HandlerFunc(s.publicProducts)))
	mux.Handle("GET /public/companies/{companyCode}/product-images/{id}", s.guestAuthenticated(http.HandlerFunc(s.publicProductImage)))
	mux.Handle("POST /public/products/{id}/purchase-requests", s.guestAuthenticated(http.HandlerFunc(s.publicPurchaseRequest)))
	mux.Handle("POST /public/purchase-requests", s.guestAuthenticated(http.HandlerFunc(s.publicPurchaseRequests)))

	mux.Handle("GET /", s.authenticated("dashboard.read", http.HandlerFunc(s.dashboard)))
	mux.Handle("POST /logout", s.authenticated("", http.HandlerFunc(s.logout)))
	mux.Handle("GET /products", s.authenticated("inventory.read", http.HandlerFunc(s.products)))
	mux.Handle("GET /products/export.csv", s.authenticated("inventory.read", http.HandlerFunc(s.productsCSV)))
	mux.Handle("GET /products/new", s.authenticated("inventory.write", http.HandlerFunc(s.productNew)))
	mux.Handle("GET /products/next-code", s.authenticated("inventory.write", http.HandlerFunc(s.productNextCode)))
	mux.Handle("POST /products", s.authenticated("inventory.write", http.HandlerFunc(s.productCreate)))
	mux.Handle("GET /products/{id}", s.authenticated("inventory.read", http.HandlerFunc(s.productDetail)))
	mux.Handle("GET /products/{id}/modal", s.authenticated("inventory.read", http.HandlerFunc(s.productModal)))
	mux.Handle("GET /products/{id}/edit", s.authenticated("inventory.write", http.HandlerFunc(s.productEditModal)))
	mux.Handle("POST /products/{id}/edit", s.authenticated("inventory.write", http.HandlerFunc(s.productUpdate)))
	mux.Handle("POST /products/{id}/status", s.authenticated("inventory.write", http.HandlerFunc(s.productStatus)))
	mux.Handle("POST /products/{id}/images", s.authenticated("inventory.write", http.HandlerFunc(s.productImageUpload)))
	mux.Handle("POST /products/{id}/publication", s.authenticated("inventory.publish", http.HandlerFunc(s.productPublication)))
	mux.Handle("GET /product-images/{id}", s.authenticated("inventory.read", http.HandlerFunc(s.productImage)))
	mux.Handle("GET /purchases", s.authenticated("purchase.read", http.HandlerFunc(s.purchases)))
	mux.Handle("GET /purchases/export.csv", s.authenticated("purchase.read", http.HandlerFunc(s.purchasesCSV)))
	mux.Handle("GET /purchases/new", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseNew)))
	mux.Handle("POST /purchases", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseCreate)))
	mux.Handle("GET /purchases/{id}", s.authenticated("purchase.read", http.HandlerFunc(s.purchaseDetail)))
	mux.Handle("POST /purchases/{id}/confirm", s.authenticated("purchase.confirm", http.HandlerFunc(s.purchaseConfirm)))
	mux.Handle("GET /market-prices", s.authenticated("market.read", http.HandlerFunc(s.marketPrices)))
	mux.Handle("GET /market-prices/export.csv", s.authenticated("market.read", http.HandlerFunc(s.marketPricesCSV)))
	mux.Handle("POST /market-prices/import.csv", s.authenticated("market.import", http.HandlerFunc(s.marketPricesCSVImport)))
	mux.Handle("GET /market-prices/products/{id}/modal", s.authenticated("market.read", http.HandlerFunc(s.marketPriceModal)))
	mux.Handle("POST /market-prices/products/{id}/edit", s.authenticated("market.write", http.HandlerFunc(s.marketPriceUpdate)))
	mux.Handle("GET /sales", s.authenticated("sales.read", http.HandlerFunc(s.sales)))
	mux.Handle("GET /sales/next-number", s.authenticated("sales.write", http.HandlerFunc(s.salesNextNumber)))
	mux.Handle("GET /sales/shipment-prefill", s.authenticated("sales.write", http.HandlerFunc(s.salesShipmentPrefill)))
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
	mux.Handle("GET /returns", s.authenticated("sales.read", http.HandlerFunc(s.returns)))
	mux.Handle("GET /returns/{id}", s.authenticated("sales.read", http.HandlerFunc(s.returnDetail)))
	mux.Handle("GET /returns/{id}/modal", s.authenticated("sales.read", http.HandlerFunc(s.returnRestoreModal)))
	mux.Handle("POST /returns/{id}/restore", s.authenticated("sales.write", http.HandlerFunc(s.returnRestore)))
	mux.Handle("POST /returns/{id}/items", s.authenticated("sales.write", http.HandlerFunc(s.returnCreate)))
	mux.Handle("POST /returns/{id}/items/{itemID}/complete", s.authenticated("sales.write", http.HandlerFunc(s.returnComplete)))
	mux.Handle("GET /slips", s.authenticated("dashboard.read", http.HandlerFunc(s.slips)))
	mux.Handle("GET /slips/export.csv", s.authenticated("dashboard.read", http.HandlerFunc(s.slipsCSV)))
	mux.Handle("GET /slips/purchases/{id}/modal", s.authenticated("purchase.read", http.HandlerFunc(s.purchaseSlipModal)))
	mux.Handle("GET /slips/purchases/{id}/edit", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseSlipEditModal)))
	mux.Handle("POST /slips/purchases/{id}/edit", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseSlipEdit)))
	mux.Handle("GET /slips/purchases/{id}/return", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseReturnNewModal)))
	mux.Handle("POST /slips/purchases/{id}/return", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseReturnCreate)))
	mux.Handle("GET /slips/shipments/{id}/modal", s.authenticated("shipment.read", http.HandlerFunc(s.shipmentSlipModal)))
	mux.Handle("GET /slips/shipments/{id}/edit", s.authenticated("shipment.write", http.HandlerFunc(s.shipmentSlipEditModal)))
	mux.Handle("POST /slips/shipments/{id}/edit", s.authenticated("shipment.write", http.HandlerFunc(s.shipmentSlipEdit)))
	mux.Handle("GET /slips/sales/{id}/modal", s.authenticated("sales.read", http.HandlerFunc(s.salesSlipModal)))
	mux.Handle("GET /slips/sales/{id}/edit", s.authenticated("sales.write", http.HandlerFunc(s.salesSlipEditModal)))
	mux.Handle("POST /slips/sales/{id}/edit", s.authenticated("sales.write", http.HandlerFunc(s.salesSlipEdit)))
	mux.Handle("GET /slips/sales/{id}/return", s.authenticated("sales.write", http.HandlerFunc(s.salesReturnNewModal)))
	mux.Handle("POST /slips/sales/{id}/return", s.authenticated("sales.write", http.HandlerFunc(s.salesReturnCreate)))
	mux.Handle("GET /slips/sales-returns/{id}/modal", s.authenticated("sales.read", http.HandlerFunc(s.salesReturnSlipModal)))
	mux.Handle("POST /slips/sales-returns/{id}/invoice", s.authenticated("sales.write", http.HandlerFunc(s.salesReturnInvoice)))
	mux.Handle("POST /slips/sales-returns/{id}/complete", s.authenticated("sales.write", http.HandlerFunc(s.salesReturnComplete)))
	mux.Handle("GET /slips/purchase-returns/{id}/modal", s.authenticated("purchase.read", http.HandlerFunc(s.purchaseReturnSlipModal)))
	mux.Handle("POST /slips/purchase-returns/{id}/invoice", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseReturnInvoice)))
	mux.Handle("POST /slips/purchase-returns/{id}/complete", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseReturnComplete)))
	mux.Handle("POST /slips/purchase-returns/{id}/delivery", s.authenticated("purchase.write", http.HandlerFunc(s.purchaseReturnDelivery)))
	mux.Handle("POST /slips/sales/invoices/preview", s.authenticated("sales.read", http.HandlerFunc(s.salesInvoicesPreview)))
	mux.Handle("POST /slips/purchase-returns/invoices/preview", s.authenticated("purchase.read", http.HandlerFunc(s.purchaseReturnInvoicesPreview)))
	mux.Handle("POST /slips/purchase-returns/invoices.csv", s.authenticated("purchase.read", http.HandlerFunc(s.purchaseReturnInvoicesCSV)))
	mux.Handle("GET /purchase-requests", s.authenticated("request.read", http.HandlerFunc(s.purchaseRequests)))
	mux.Handle("POST /purchase-requests/{id}/approve", s.authenticated("request.review", http.HandlerFunc(s.purchaseRequestApprove)))
	mux.Handle("POST /purchase-requests/{id}/reject", s.authenticated("request.review", http.HandlerFunc(s.purchaseRequestReject)))
	mux.Handle("POST /purchase-requests/{id}/cancel", s.authenticated("request.review", http.HandlerFunc(s.purchaseRequestCancel)))
	mux.Handle("GET /approvals", s.authenticated("approval.read", http.HandlerFunc(s.approvals)))
	mux.Handle("POST /approvals/{id}/approve", s.authenticated("approval.approve", http.HandlerFunc(s.approvalApprove)))
	mux.Handle("POST /approvals/{id}/return", s.authenticated("approval.approve", http.HandlerFunc(s.approvalReturn)))
	mux.Handle("GET /masters", s.authenticated("settings.manage", http.HandlerFunc(s.masters)))
	mux.Handle("POST /masters", s.authenticated("settings.manage", http.HandlerFunc(s.masterCreate)))
	mux.Handle("POST /masters/{id}/update", s.authenticated("settings.manage", http.HandlerFunc(s.masterUpdate)))
	mux.Handle("POST /masters/{id}/delete", s.authenticated("settings.manage", http.HandlerFunc(s.masterDelete)))
	mux.Handle("POST /masters/exchange-rates", s.authenticated("settings.manage", http.HandlerFunc(s.masterExchangeRateUpdate)))
	mux.Handle("POST /masters/passwords/users", s.authenticated("users.manage", http.HandlerFunc(s.masterPasswordUserCreate)))
	mux.Handle("POST /masters/passwords/users/{id}/password", s.authenticated("users.manage", http.HandlerFunc(s.masterPasswordUserChange)))
	mux.Handle("POST /masters/passwords/users/{id}/delete", s.authenticated("users.manage", http.HandlerFunc(s.masterPasswordUserDelete)))
	mux.Handle("POST /masters/passwords/guests/{id}/password", s.authenticated("users.manage", http.HandlerFunc(s.masterGuestPasswordChange)))
	mux.Handle("POST /masters/passwords/guests/{id}/delete", s.authenticated("users.manage", http.HandlerFunc(s.masterGuestPasswordDelete)))
	mux.Handle("POST /masters/passwords/guest-bulk", s.authenticated("users.manage", http.HandlerFunc(s.masterGuestPasswordsBulk)))
	mux.Handle("POST /masters/passwords/notify", s.authenticated("users.manage", http.HandlerFunc(s.masterPasswordNotify)))
	mux.Handle("GET /guest-management", s.authenticated("settings.manage", http.HandlerFunc(s.guestManagement)))
	mux.Handle("POST /guest-management/draft", s.authenticated("settings.manage", http.HandlerFunc(s.guestBoxDraftSave)))
	mux.Handle("POST /guest-management/publish", s.authenticated("settings.manage", http.HandlerFunc(s.guestBoxPublish)))
	mux.Handle("POST /guest-management/boxes/{id}", s.authenticated("settings.manage", http.HandlerFunc(s.guestBoxRename)))
	mux.Handle("GET /guest-management/boxes/{id}/modal", s.authenticated("settings.manage", http.HandlerFunc(s.guestBoxModal)))
	mux.Handle("GET /guest-management/boxes/{id}/edit-modal", s.authenticated("settings.manage", http.HandlerFunc(s.guestBoxEditModal)))
	mux.Handle("POST /guest-management/boxes/{id}/products", s.authenticated("settings.manage", http.HandlerFunc(s.guestBoxProductAdd)))
	mux.Handle("POST /guest-management/boxes/{id}/products/{productID}/remove", s.authenticated("settings.manage", http.HandlerFunc(s.guestBoxProductRemove)))
	mux.Handle("POST /guest-management/boxes/{id}/products/{productID}/move", s.authenticated("settings.manage", http.HandlerFunc(s.guestBoxProductMove)))
	mux.Handle("GET /stocktakes", s.authenticated("inventory.read", http.HandlerFunc(s.stocktakes)))
	mux.Handle("POST /stocktakes", s.authenticated("inventory.write", http.HandlerFunc(s.stocktakeCreate)))
	mux.Handle("GET /stocktakes/{id}", s.authenticated("inventory.read", http.HandlerFunc(s.stocktakeDetail)))
	mux.Handle("POST /stocktakes/{id}/lines/{lineID}", s.authenticated("inventory.write", http.HandlerFunc(s.stocktakeLineUpdate)))
	mux.Handle("POST /stocktakes/{id}/save", s.authenticated("inventory.write", http.HandlerFunc(s.stocktakeSave)))
	mux.Handle("POST /stocktakes/{id}/complete", s.authenticated("inventory.write", http.HandlerFunc(s.stocktakeComplete)))
	mux.Handle("GET /performance", s.authenticated("dashboard.read", http.HandlerFunc(s.performance)))
	mux.Handle("GET /performance/export.csv", s.authenticated("dashboard.read", http.HandlerFunc(s.performanceCSV)))
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
				"sold": "売上済", "shipped": "出荷済", "cancelled": "取消", "invalid": "無効",
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
				"cancelled": "取消", "sold": "売上化",
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
				"return_takehome.restore": "返品／持ち帰り在庫戻し",
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
		"stocktakePresent": func(value *bool) bool {
			return value != nil && *value
		},
		"stocktakeAbsent": func(value *bool) bool {
			return value != nil && !*value
		},
		"contains": strings.Contains,
		"masterHas": func(records []database.MasterRecord, name string) bool {
			for _, record := range records {
				if record.Name == name {
					return true
				}
			}
			return false
		},
		"masterDetail": func(record database.MasterRecord, key string) string {
			if record.Details == nil {
				return ""
			}
			return record.Details[key]
		},
		"guestMatrixCell": func(cells []database.GuestBoxMatrixCell, companyID, boxID string) database.GuestBoxMatrixCell {
			for _, cell := range cells {
				if cell.CompanyID == companyID && cell.BoxID == boxID {
					return cell
				}
			}
			return database.GuestBoxMatrixCell{CompanyID: companyID, BoxID: boxID}
		},
		"add":   func(left, right int) int { return left + right },
		"add64": func(left, right int64) int64 { return left + right },
		"sequence": func(count int) []int {
			values := make([]int, count)
			for index := range values {
				values[index] = index + 1
			}
			return values
		},
		"productAccessories": func() []string {
			return []string{"BOX", "CASE", "GUARANTEE", "BRACELET PARTS", "CERTIFICATE", "ARCHIVE"}
		},
		"accessoryList": func(value string) []string {
			items := make([]string, 0)
			for _, item := range strings.Split(value, ",") {
				if item = strings.TrimSpace(item); item != "" {
					items = append(items, item)
				}
			}
			return items
		},
		"formatAccessories": func(value string) string {
			items := make([]string, 0)
			for _, item := range strings.Split(value, ",") {
				if item = strings.TrimSpace(item); item != "" {
					items = append(items, item)
				}
			}
			return strings.Join(items, "・")
		},
		"today": func() string {
			return time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006年1月2日")
		},
	}
	base, err := template.New("base.html").Funcs(funcs).ParseFS(assets, "templates/base.html")
	if err != nil {
		return err
	}
	s.templates = make(map[string]*template.Template)
	for _, page := range []string{
		"login", "guest-login", "dashboard", "masters", "guest-management", "guest-box-modal", "guest-box-edit-modal", "stocktakes", "performance", "returns", "return-detail", "return-restore-modal", "users", "settings", "audit", "public",
		"products", "product-modal", "product-edit-modal", "product-new", "product-detail", "purchases", "purchase-new", "purchase-detail",
		"purchase-slip-modal", "purchase-slip-edit-modal", "purchase-return-new-modal",
		"purchase-return-slip-modal", "purchase-return-invoice",
		"shipment-slip-modal", "shipment-slip-edit-modal",
		"sales-slip-modal", "sales-slip-edit-modal", "sales-return-new-modal",
		"sales-return-slip-modal", "sales-return-invoice", "invoice-previews",
		"market", "market-import", "market-import-preview",
		"sales", "sale-new", "sale-detail", "shipments", "shipment-new", "shipment-detail", "slips",
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
	if data.User.ID != "" && !data.NotificationCountsSet {
		if approvals, err := s.store.Approvals(context.Background(), data.User.OrganizationID); err == nil {
			data.ApprovalAlertCount = 0
			for _, approval := range approvals {
				if approval.Status == "pending" {
					data.ApprovalAlertCount++
				}
			}
			data.AlertCount = data.ApprovalAlertCount
		}
		if requestGroups, err := s.store.PurchaseRequestGroups(
			context.Background(), data.User.OrganizationID, "pending",
		); err == nil {
			data.PurchaseAlertCount = len(requestGroups)
		}
	}
	if data.PreviewMode && name == "login" {
		data.LoginAdminPassword = s.cfg.AdminPassword
		data.LoginWorkerPassword = s.cfg.WorkerPassword
	}
	if data.PreviewMode && name == "guest-login" {
		data.LoginGuestPassword = "guest-preview-2026"
	}
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

func (s *Server) guestLoginPage(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(guestSessionCookie); err == nil {
		if _, err := s.store.GuestSession(r.Context(), s.cfg.OrganizationCode, cookie.Value); err == nil {
			http.Redirect(w, r, "/public/products", http.StatusSeeOther)
			return
		}
	}
	csrf, err := database.RandomToken()
	if err != nil || s.store.CreateLoginCSRF(r.Context(), csrf, 10*time.Minute) != nil {
		http.Error(w, "ゲストログイン画面を準備できませんでした", http.StatusInternalServerError)
		return
	}
	s.render(w, "guest-login", http.StatusOK, pageData{
		Title: "ゲストログイン", CSRF: csrf, Error: r.URL.Query().Get("error"),
	})
}

func (s *Server) guestLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "入力を確認してください", http.StatusBadRequest)
		return
	}
	if !s.store.ConsumeLoginCSRF(r.Context(), r.FormValue("csrf_token")) {
		http.Redirect(w, r, "/guest/login?error="+url.QueryEscape("画面の有効期限が切れました。"), http.StatusSeeOther)
		return
	}
	loginID := strings.TrimSpace(r.FormValue("guest_id"))
	guest, err := s.store.AuthenticateGuest(r.Context(), s.cfg.OrganizationCode, loginID, r.FormValue("password"))
	if err != nil {
		_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
			TargetType: "guest_session", TargetID: loginID, Action: "guest.login.failed",
			Result: "denied", Reason: "invalid credentials", IPAddress: clientIP(r),
			UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
		})
		http.Redirect(w, r, "/guest/login?error="+url.QueryEscape("ゲストIDまたはパスワードが違います。"), http.StatusSeeOther)
		return
	}
	random, err := database.RandomToken()
	if err != nil {
		http.Error(w, "ゲストセッションを作成できませんでした", http.StatusInternalServerError)
		return
	}
	token := guest.CompanyID + "." + random
	if err := s.store.CreateGuestSession(r.Context(), guest, token, s.cfg.SessionTTL); err != nil {
		http.Error(w, "ゲストセッションを作成できませんでした", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: guestSessionCookie, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: s.cfg.CookieSecure, MaxAge: int(s.cfg.SessionTTL.Seconds()),
	})
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: guest.OrganizationID, TargetType: "guest_session", TargetID: guest.CompanyID,
		Action: "guest.login.succeeded", Result: "success", IPAddress: clientIP(r),
		UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/public/products", http.StatusSeeOther)
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
					r.Body = http.MaxBytesReader(w, r.Body, 82<<20)
					_ = r.ParseMultipartForm(16 << 20)
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

func (s *Server) guestAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(guestSessionCookie)
		if err != nil {
			http.Redirect(w, r, "/guest/login", http.StatusSeeOther)
			return
		}
		guest, err := s.store.GuestSession(r.Context(), s.cfg.OrganizationCode, cookie.Value)
		if err != nil {
			http.SetCookie(w, &http.Cookie{
				Name: guestSessionCookie, Value: "", Path: "/", HttpOnly: true,
				MaxAge: -1, SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/guest/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(withGuest(r.Context(), guest)))
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
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
