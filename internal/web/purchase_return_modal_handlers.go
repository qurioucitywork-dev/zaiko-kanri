package web

import (
	"net/http"
	"net/url"
)

func (s *Server) purchaseReturnSlipModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.purchaseReturnModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "仕入返品伝票が見つかりません。")
		return
	}
	s.renderPartial(w, "purchase-return-slip-modal", "content", http.StatusOK, data)
}

func (s *Server) purchaseReturnInvoice(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	id := r.PathValue("id")
	if err := s.store.IssuePurchaseReturnInvoice(r.Context(), user.OrganizationID, id, user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.auditTransaction(r, user, "purchase_return", id, "purchase_return.invoice_printed",
		map[string]string{"invoice": "issued_and_printed"}, "仕入返品請求書を発行・印刷")
	data, err := s.purchaseReturnModalData(r)
	if err != nil {
		http.Error(w, "請求書を表示できませんでした。", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("X-Invoice-Preview") == "true" {
		s.renderPartial(w, "purchase-return-invoice", "preview", http.StatusOK, data)
		return
	}
	s.renderPartial(w, "purchase-return-invoice", "document", http.StatusOK, data)
}

func (s *Server) purchaseReturnComplete(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	id := r.PathValue("id")
	if err := s.store.CompletePurchaseReturn(r.Context(), user.OrganizationID, id); err != nil {
		http.Redirect(w, r, "/slips?kind=purchase-returns&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	s.auditTransaction(r, user, "purchase_return", id, "purchase_return.completed",
		map[string]string{"status": "completed"}, "仕入返品処理を完了")
	http.Redirect(w, r, "/slips?kind=purchase-returns&notice="+url.QueryEscape("仕入返品伝票を完了しました。"), http.StatusSeeOther)
}

func (s *Server) purchaseReturnModalData(r *http.Request) (pageData, error) {
	user, _ := currentUser(r.Context())
	item, err := s.store.PurchaseReturn(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		return pageData{}, err
	}
	lines, err := s.store.PurchaseReturnLines(r.Context(), user.OrganizationID, item)
	if err != nil {
		return pageData{}, err
	}
	var total int64
	for _, line := range lines {
		total += line.AmountJPY
	}
	if total == 0 {
		total = item.AmountJPY
	}
	return pageData{
		Title: "仕入返品伝票", Active: "slips", User: user, CSRF: csrfFromRequest(r),
		PurchaseReturn: item, PurchaseReturnLines: lines, PurchaseReturnTotal: total,
		PurchaseReturnCompleted: item.Status == "completed", InvoiceCompany: defaultInvoiceCompany(),
	}, nil
}
