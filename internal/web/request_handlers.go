package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type purchaseRequestGroupView struct {
	ID            string
	DisplayNumber string
	GuestName     string
	Message       string
	RequestedAt   time.Time
	Status        string
	ItemCount     int
	PendingCount  int
	ApprovedCount int
	Total         int64
	ApprovedTotal int64
	Currency      string
	CanCreateShip bool
	Items         []purchaseRequestItemView
}

type purchaseRequestItemView struct {
	database.PurchaseRequest
	PurchasePriceJPY int64
	SalePriceUSD     int64
	SalePriceJPY     int64
	PurchaseJPYReady bool
	SaleUSDReady     bool
	SaleJPYReady     bool
}

func purchaseRequestDisplayNumber(requestNumber string) string {
	parts := strings.Split(requestNumber, "-")
	if len(parts) == 0 {
		return requestNumber
	}
	sequence, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return requestNumber
	}
	return fmt.Sprintf("PR-%03d", sequence)
}

func buildPurchaseRequestGroupViews(groups []database.PurchaseRequestGroup) ([]purchaseRequestGroupView, int) {
	return buildPurchaseRequestGroupViewsWithRate(groups, database.ExchangeRate{})
}

func buildPurchaseRequestGroupViewsWithRate(groups []database.PurchaseRequestGroup, usdJPY database.ExchangeRate) ([]purchaseRequestGroupView, int) {
	views := make([]purchaseRequestGroupView, 0, len(groups))
	pendingGroups := 0
	for _, group := range groups {
		if len(group.Items) == 0 {
			continue
		}
		first := group.Items[0]
		view := purchaseRequestGroupView{
			ID: group.ID, DisplayNumber: purchaseRequestDisplayNumber(first.RequestNumber),
			GuestName: first.GuestName, Message: first.Message, RequestedAt: first.RequestedAt,
			ItemCount: len(group.Items), Currency: "JPY",
		}
		for _, item := range group.Items {
			itemView := purchaseRequestPriceView(item, usdJPY)
			view.Items = append(view.Items, itemView)
			if itemView.SaleJPYReady {
				view.Total += itemView.SalePriceJPY
			}
			status := item.Status
			if status == "expired" {
				status = "pending"
				view.Items[len(view.Items)-1].Status = status
			}
			switch status {
			case "pending":
				view.PendingCount++
			case "approved":
				view.ApprovedCount++
				if itemView.SaleJPYReady {
					view.ApprovedTotal += itemView.SalePriceJPY
				}
			}
		}
		switch {
		case view.PendingCount > 0:
			view.Status = "pending"
			pendingGroups++
		case view.ApprovedCount > 0:
			view.Status = "approved"
		default:
			view.Status = "handled"
		}
		view.CanCreateShip = view.PendingCount == 0 && view.ApprovedCount > 0
		views = append(views, view)
	}
	return views, pendingGroups
}

func purchaseRequestPriceView(item database.PurchaseRequest, usdJPY database.ExchangeRate) purchaseRequestItemView {
	view := purchaseRequestItemView{PurchaseRequest: item}
	rateReady := usdJPY.RateScaled > 0 && usdJPY.Scale > 0
	if item.CostCurrency == "JPY" {
		view.PurchasePriceJPY, view.PurchaseJPYReady = item.CostAmountMinor, true
	} else if item.CostCurrency == "USD" && rateReady {
		view.PurchasePriceJPY, _ = database.ConvertMinor(item.CostAmountMinor, usdJPY.RateScaled, usdJPY.Scale, false)
		view.PurchaseJPYReady = true
	}
	if item.SaleCurrency == "USD" {
		view.SalePriceUSD, view.SaleUSDReady = item.SalePriceMinor, true
		if rateReady {
			view.SalePriceJPY, _ = database.ConvertMinor(item.SalePriceMinor, usdJPY.RateScaled, usdJPY.Scale, false)
			view.SaleJPYReady = true
		}
	} else if item.SaleCurrency == "JPY" {
		view.SalePriceJPY, view.SaleJPYReady = item.SalePriceMinor, true
		if rateReady {
			view.SalePriceUSD, _ = database.ConvertMinor(item.SalePriceMinor, usdJPY.RateScaled, usdJPY.Scale, true)
			view.SaleUSDReady = true
		}
	}
	return view
}

func (s *Server) publicProducts(w http.ResponseWriter, r *http.Request) {
	guest, ok := currentGuest(r.Context())
	if !ok {
		http.Redirect(w, r, "/guest/login", http.StatusSeeOther)
		return
	}
	organizationID, err := s.store.OrganizationIDByCode(r.Context(), s.cfg.OrganizationCode)
	if err != nil {
		http.Error(w, "公開カタログを準備できませんでした。", http.StatusServiceUnavailable)
		return
	}
	_ = s.store.ExpireReservations(r.Context(), organizationID)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	brand := strings.TrimSpace(r.URL.Query().Get("brand"))
	condition := strings.TrimSpace(r.URL.Query().Get("condition"))
	filter := database.PublicProductFilter{
		Query: query, Brand: brand, Condition: condition,
	}
	products, err := s.store.PublicProductsForGuest(r.Context(), s.cfg.OrganizationCode, guest.CompanyCode, filter)
	if err != nil {
		http.Error(w, "公開商品を検索できませんでした。", http.StatusInternalServerError)
		return
	}
	brandSeen := make(map[string]bool)
	var brands []string
	for _, product := range products {
		if !brandSeen[product.Brand] {
			brandSeen[product.Brand] = true
			brands = append(brands, product.Brand)
		}
	}
	csrf, err := database.RandomToken()
	if err != nil || s.store.CreateLoginCSRF(r.Context(), csrf, 30*time.Minute) != nil {
		http.Error(w, "購入依頼画面を準備できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "public", http.StatusOK, pageData{
		Title: "公開商品", PublicProducts: products, ProductBrands: brands,
		PublicQuery: query, PublicBrand: brand, PublicCondition: condition,
		GuestCompanyCode: guest.CompanyCode, GuestCompanyName: guest.CompanyName,
		CSRF: csrf, Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) publicProductImage(w http.ResponseWriter, r *http.Request) {
	guest, ok := currentGuest(r.Context())
	if !ok || !strings.EqualFold(r.PathValue("companyCode"), guest.CompanyCode) {
		http.NotFound(w, r)
		return
	}
	image, err := s.store.GuestPublishedProductImage(r.Context(), guest.OrganizationID, guest.CompanyID, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	base, err := filepath.Abs(s.cfg.UploadDirectory)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	target, err := filepath.Abs(filepath.Join(base, image.StoragePath))
	if err != nil || (!strings.HasPrefix(target, base+string(os.PathSeparator)) && target != base) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", image.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="`+url.PathEscape(image.OriginalName)+`"`)
	http.ServeFile(w, r, target)
}

func (s *Server) publicPurchaseRequests(w http.ResponseWriter, r *http.Request) {
	guest, ok := currentGuest(r.Context())
	if !ok {
		http.Redirect(w, r, "/guest/login", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "入力を確認してください。", http.StatusBadRequest)
		return
	}
	if !s.store.ConsumeLoginCSRF(r.Context(), r.FormValue("csrf_token")) {
		http.Error(w, "画面の有効期限が切れました。読み込み直してください。", http.StatusForbidden)
		return
	}
	group, err := s.store.CreatePurchaseRequestGroup(r.Context(), database.PurchaseRequestGroupInput{
		OrganizationCode: s.cfg.OrganizationCode,
		GuestCompanyID:   guest.CompanyID,
		ProductIDs:       r.Form["product_id"],
		GuestName:        r.FormValue("guest_name"),
		GuestEmail:       r.FormValue("guest_email"),
		GuestPhone:       r.FormValue("guest_phone"),
		Message:          r.FormValue("message"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	for _, request := range group.Items {
		after, _ := json.Marshal(request)
		_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
			OrganizationID: request.OrganizationID, TargetType: "purchase_request", TargetID: request.ID,
			Action: "purchase_request.submitted", AfterJSON: string(after), Result: "success",
			IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
		})
	}
	http.Redirect(w, r, "/public/products?notice="+url.QueryEscape(
		strconv.Itoa(len(group.Items))+"点の購入依頼を受け付けました。担当者からの連絡をお待ちください。"), http.StatusSeeOther)
}

func (s *Server) publicPurchaseRequest(w http.ResponseWriter, r *http.Request) {
	guest, ok := currentGuest(r.Context())
	if !ok {
		http.Redirect(w, r, "/guest/login", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "入力を確認してください。", http.StatusBadRequest)
		return
	}
	if !s.store.ConsumeLoginCSRF(r.Context(), r.FormValue("csrf_token")) {
		http.Error(w, "画面の有効期限が切れました。再読み込みしてください。", http.StatusForbidden)
		return
	}
	request, err := s.store.CreatePurchaseRequest(r.Context(), database.PurchaseRequestInput{
		OrganizationCode: s.cfg.OrganizationCode,
		GuestCompanyID:   guest.CompanyID,
		ProductID:        r.PathValue("id"), GuestName: r.FormValue("guest_name"),
		GuestEmail: r.FormValue("guest_email"), GuestPhone: r.FormValue("guest_phone"),
		Message: r.FormValue("message"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	after, _ := json.Marshal(request)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: request.OrganizationID, TargetType: "purchase_request", TargetID: request.ID,
		Action: "purchase_request.submitted", AfterJSON: string(after), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/public/products?notice="+url.QueryEscape(
		"購入依頼を受け付けました（"+request.RequestNumber+"）。この時点では在庫は確保されません。"), http.StatusSeeOther)
}

func (s *Server) purchaseRequests(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	groups, err := s.store.PurchaseRequestGroups(r.Context(), user.OrganizationID, "")
	if err != nil {
		http.Error(w, "購入依頼を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	usdJPY, rateErr := s.store.LatestExchangeRate(r.Context(), user.OrganizationID, "USD", "JPY")
	if rateErr != nil && !errors.Is(rateErr, sql.ErrNoRows) {
		http.Error(w, "為替レートを取得できませんでした。", http.StatusInternalServerError)
		return
	}
	groupViews, pendingCount := buildPurchaseRequestGroupViewsWithRate(groups, usdJPY)
	var total int64
	for _, group := range groupViews {
		total += group.Total
	}
	s.render(w, "purchase-requests", http.StatusOK, pageData{
		Title: "購入一覧", Active: "requests", User: user, PurchaseRequestGroups: groupViews,
		PurchaseRequestTotal: total, PurchaseRequestPending: pendingCount,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) purchaseRequestApprove(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	request, err := s.store.ApprovePurchaseRequest(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.auditTransaction(r, user, "purchase_request", request.ID, "purchase_request.approved", request, "")
	http.Redirect(w, r, "/purchase-requests?notice="+url.QueryEscape(
		"購入依頼を承認し、商品を取置中にしました。"), http.StatusSeeOther)
}

func (s *Server) purchaseRequestReject(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	reason := strings.TrimSpace(r.FormValue("reason"))
	if err := s.store.RejectPurchaseRequest(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, reason); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "purchase_request", r.PathValue("id"), "purchase_request.rejected",
		map[string]string{"status": "rejected"}, reason)
	http.Redirect(w, r, "/purchase-requests?notice="+url.QueryEscape("購入依頼を却下しました。"), http.StatusSeeOther)
}

func (s *Server) purchaseRequestCancel(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	reason := strings.TrimSpace(r.FormValue("reason"))
	if err := s.store.CancelPurchaseRequest(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, reason); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "purchase_request", r.PathValue("id"), "purchase_request.cancelled",
		map[string]string{"status": "cancelled"}, reason)
	http.Redirect(w, r, "/purchase-requests?notice="+url.QueryEscape("購入依頼を取消し、取置を解除しました。"), http.StatusSeeOther)
}

func (s *Server) productPublication(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	status := r.FormValue("publication_status")
	if err := s.store.SetProductPublication(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, status); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "product", r.PathValue("id"), "product.publication_changed",
		map[string]string{"publication_status": status}, "")
	notice := "商品を非公開にしました。"
	if status == "public" {
		notice = "商品を公開しました。"
	}
	http.Redirect(w, r, "/products/"+r.PathValue("id")+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}
