package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) salesReturnSlipModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.salesReturnModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "売上返品伝票が見つかりません。")
		return
	}
	s.renderPartial(w, "sales-return-slip-modal", "content", http.StatusOK, data)
}

func (s *Server) salesReturnInvoice(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	saleID := r.PathValue("id")
	if err := s.store.IssueSalesReturnInvoice(r.Context(), user.OrganizationID, saleID, user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.auditTransaction(r, user, "sales_return", saleID, "sales_return.invoice_printed",
		map[string]string{"invoice": "issued_and_printed"}, "売上返品請求書を発行・印刷")
	data, err := s.salesReturnModalData(r)
	if err != nil {
		http.Error(w, "請求書を表示できませんでした。", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("X-Invoice-Preview") == "true" {
		s.renderPartial(w, "sales-return-invoice", "preview", http.StatusOK, data)
		return
	}
	s.renderPartial(w, "sales-return-invoice", "document", http.StatusOK, data)
}

func (s *Server) salesReturnComplete(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	saleID := r.PathValue("id")
	if err := s.store.CompleteSalesReturn(r.Context(), user.OrganizationID, saleID, user.ID); err != nil {
		http.Redirect(w, r, "/slips?kind=sales-returns&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	s.auditTransaction(r, user, "sales_return", saleID, "sales_return.completed",
		map[string]string{"status": "completed"}, "売上返品処理を完了")
	http.Redirect(w, r, "/slips?kind=sales-returns&notice="+url.QueryEscape("売上返品伝票を完了しました。"), http.StatusSeeOther)
}

func (s *Server) salesReturnModalData(r *http.Request) (pageData, error) {
	user, _ := currentUser(r.Context())
	sale, err := s.store.Sale(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		return pageData{}, err
	}
	allItems, err := s.store.ReturnTakehomeItems(r.Context(), user.OrganizationID, sale.ID)
	if err != nil {
		return pageData{}, err
	}
	lineByID := make(map[string]database.SalesLine, len(sale.Lines))
	for _, line := range sale.Lines {
		lineByID[line.ID] = line
	}
	items := make([]database.ReturnTakehomeItem, 0, len(allItems))
	var total int64
	for _, item := range allItems {
		if item.ActionType != "return" || item.Status == "cancelled" {
			continue
		}
		line := lineByID[item.SalesLineID]
		price := line.ConvertedUnitPriceJPY
		if price == 0 && line.SaleCurrency == "JPY" {
			price = line.UnitPriceMinor
		}
		item.AmountJPY = price * int64(item.Quantity)
		total += item.AmountJPY
		items = append(items, item)
	}
	if len(items) == 0 {
		return pageData{}, database.ErrReturnNotEligible
	}
	latest := items[0]
	return pageData{
		Title: "売上返品伝票", Active: "slips", User: user, CSRF: csrfFromRequest(r),
		Sale: sale, SalesReturnItems: items,
		SalesReturnNumber: "SR-" + strings.TrimPrefix(sale.SlipNumber, "SL-"),
		SalesReturnTotal:  total, SalesReturnInvoiceReady: database.SalesReturnInvoiceReady(items),
		SalesReturnCompleted: database.SalesReturnCompleted(items), SalesReturnReason: latest.Reason,
		SalesReturnNotes: latest.Notes, SalesReturnActor: latest.RequestedByName,
		SalesReturnRequestedAt: latest.RequestedAt, InvoiceCompany: defaultInvoiceCompany(),
	}, nil
}
