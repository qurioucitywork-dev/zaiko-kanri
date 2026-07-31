package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) purchaseSlipModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.purchaseSlipModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "仕入伝票が見つかりません。")
		return
	}
	s.renderPartial(w, "purchase-slip-modal", "content", http.StatusOK, data)
}

func (s *Server) purchaseSlipEditModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.purchaseSlipModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "仕入伝票が見つかりません。")
		return
	}
	s.renderPartial(w, "purchase-slip-edit-modal", "content", http.StatusOK, data)
}

func (s *Server) purchaseReturnNewModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.purchaseSlipModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "仕入伝票が見つかりません。")
		return
	}
	data.TodayISO = time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
	s.renderPartial(w, "purchase-return-new-modal", "content", http.StatusOK, data)
}

func (s *Server) purchaseSlipModalData(r *http.Request) (pageData, error) {
	user, _ := currentUser(r.Context())
	purchase, err := s.store.Purchase(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		return pageData{}, err
	}
	products, err := s.store.PurchaseProducts(r.Context(), user.OrganizationID, purchase.ID)
	if err != nil {
		return pageData{}, err
	}
	revisions, err := s.store.PurchaseRevisions(r.Context(), user.OrganizationID, purchase.ID)
	if err != nil {
		return pageData{}, err
	}
	return pageData{
		Title: "仕入伝票", Active: "slips", User: user, CSRF: csrfFromRequest(r),
		Purchase: purchase, PurchaseProducts: products, PurchaseRevisions: revisions,
	}, nil
}

func (s *Server) purchaseSlipEdit(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/slips?kind=purchases&error="+url.QueryEscape("入力内容を確認してください。"), http.StatusSeeOther)
		return
	}
	ids, skus := r.Form["product_id"], r.Form["sku"]
	costs, salePrices := r.Form["cost_amount"], r.Form["sale_amount"]
	if len(ids) != len(skus) || len(ids) != len(costs) || len(ids) != len(salePrices) {
		http.Redirect(w, r, "/slips?kind=purchases&error="+url.QueryEscape("商品明細の形式が正しくありません。"), http.StatusSeeOther)
		return
	}
	products := make([]database.PurchaseEditProduct, 0, len(ids))
	for index := range ids {
		cost, costErr := database.ParseMinorAmount(costs[index])
		salePrice, saleErr := database.ParseMinorAmount(salePrices[index])
		if costErr != nil || saleErr != nil {
			http.Redirect(w, r, "/slips?kind=purchases&error="+url.QueryEscape("仕入金額と売価を数字で入力してください。"), http.StatusSeeOther)
			return
		}
		products = append(products, database.PurchaseEditProduct{
			ProductID: ids[index], SKU: skus[index],
			CostAmountMinor: cost, BaseSalePriceMinor: salePrice,
		})
	}
	before, _ := s.store.Purchase(r.Context(), user.OrganizationID, r.PathValue("id"))
	err := s.store.UpdatePurchaseSlip(r.Context(), database.UpdatePurchaseSlipInput{
		OrganizationID: user.OrganizationID, PurchaseSlipID: r.PathValue("id"),
		PurchaseDate: r.FormValue("purchase_date"), Notes: r.FormValue("notes"),
		Memo: r.FormValue("memo"), ActorUserID: user.ID, Products: products,
	})
	if err != nil {
		http.Redirect(w, r, "/slips?kind=purchases&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	after, _ := s.store.Purchase(r.Context(), user.OrganizationID, r.PathValue("id"))
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "purchase_slip", TargetID: r.PathValue("id"), Action: "purchase.revised",
		BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Reason: r.FormValue("memo"),
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/slips?kind=purchases&notice="+url.QueryEscape("仕入伝票を修正し、履歴を記録しました。"), http.StatusSeeOther)
}

func (s *Server) purchaseReturnCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/slips?kind=purchases&error="+url.QueryEscape("入力内容を確認してください。"), http.StatusSeeOther)
		return
	}
	created, err := s.store.CreatePurchaseReturn(r.Context(), database.CreatePurchaseReturnInput{
		OrganizationID: user.OrganizationID, PurchaseSlipID: r.PathValue("id"),
		ReturnDate: r.FormValue("return_date"), Reason: r.FormValue("reason"),
		Notes: r.FormValue("notes"), ActorUserID: user.ID, ProductIDs: r.Form["product_id"],
	})
	if err != nil {
		http.Redirect(w, r, "/slips?kind=purchases&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	afterJSON, _ := json.Marshal(created)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "purchase_return_slip", TargetID: created.ID, Action: "purchase_return.created",
		AfterJSON: string(afterJSON), Reason: strings.TrimSpace(r.FormValue("reason")), Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/slips?kind=purchase-returns&notice="+url.QueryEscape("仕入返品伝票 "+created.ReturnNumber+" を起票しました。"), http.StatusSeeOther)
}
