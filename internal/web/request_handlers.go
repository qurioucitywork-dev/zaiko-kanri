package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) publicProducts(w http.ResponseWriter, r *http.Request) {
	organizationID, err := s.store.OrganizationIDByCode(r.Context(), s.cfg.OrganizationCode)
	if err != nil {
		http.Error(w, "公開カタログを準備できませんでした。", http.StatusServiceUnavailable)
		return
	}
	_ = s.store.ExpireReservations(r.Context(), organizationID)
	products, err := s.store.PublicProducts(r.Context(), s.cfg.OrganizationCode)
	if err != nil {
		http.Error(w, "公開商品を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	csrf, err := database.RandomToken()
	if err != nil || s.store.CreateLoginCSRF(r.Context(), csrf, 30*time.Minute) != nil {
		http.Error(w, "購入依頼画面を準備できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "public", http.StatusOK, pageData{
		Title: "公開商品", PublicProducts: products, CSRF: csrf, Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) publicPurchaseRequest(w http.ResponseWriter, r *http.Request) {
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
	requests, err := s.store.PurchaseRequests(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "購入依頼を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "purchase-requests", http.StatusOK, pageData{
		Title: "購入依頼・取置", Active: "requests", User: user, PurchaseRequests: requests,
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
