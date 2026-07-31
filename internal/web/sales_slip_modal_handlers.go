package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) salesSlipModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.salesSlipModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "売上伝票が見つかりません。")
		return
	}
	s.renderPartial(w, "sales-slip-modal", "content", http.StatusOK, data)
}

func (s *Server) salesSlipEditModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.salesSlipModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "売上伝票が見つかりません。")
		return
	}
	s.renderPartial(w, "sales-slip-edit-modal", "content", http.StatusOK, data)
}

func (s *Server) salesReturnNewModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.salesSlipModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "売上伝票が見つかりません。")
		return
	}
	data.TodayISO = time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
	s.renderPartial(w, "sales-return-new-modal", "content", http.StatusOK, data)
}

func (s *Server) salesSlipModalData(r *http.Request) (pageData, error) {
	user, _ := currentUser(r.Context())
	sale, err := s.store.Sale(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		return pageData{}, err
	}
	revisions, err := s.store.SalesRevisions(r.Context(), user.OrganizationID, sale.ID)
	if err != nil {
		return pageData{}, err
	}
	return pageData{
		Title: "売上伝票", Active: "slips", User: user, CSRF: csrfFromRequest(r),
		Sale: sale, SalesRevisions: revisions,
	}, nil
}

func (s *Server) salesSlipEdit(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/slips?kind=sales&error="+url.QueryEscape("入力内容を確認してください。"), http.StatusSeeOther)
		return
	}
	lineIDs, prices := r.Form["line_id"], r.Form["sale_price"]
	if len(lineIDs) != len(prices) {
		http.Redirect(w, r, "/slips?kind=sales&error="+url.QueryEscape("商品明細の形式が正しくありません。"), http.StatusSeeOther)
		return
	}
	lines := make([]database.SalesEditLine, 0, len(lineIDs))
	for index := range lineIDs {
		price, err := database.ParseMinorAmount(prices[index])
		if err != nil {
			http.Redirect(w, r, "/slips?kind=sales&error="+url.QueryEscape("販売金額を数字で入力してください。"), http.StatusSeeOther)
			return
		}
		lines = append(lines, database.SalesEditLine{LineID: lineIDs[index], SalePriceMinor: price})
	}
	before, _ := s.store.Sale(r.Context(), user.OrganizationID, r.PathValue("id"))
	err := s.store.UpdateSalesSlip(r.Context(), database.UpdateSalesSlipInput{
		OrganizationID: user.OrganizationID, SalesSlipID: r.PathValue("id"),
		SalesDate: r.FormValue("sales_date"), Notes: r.FormValue("notes"),
		Memo: r.FormValue("memo"), ActorUserID: user.ID, Lines: lines,
	})
	if err != nil {
		http.Redirect(w, r, "/slips?kind=sales&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	after, _ := s.store.Sale(r.Context(), user.OrganizationID, r.PathValue("id"))
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "sales_slip", TargetID: r.PathValue("id"), Action: "sale.revised",
		BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Reason: r.FormValue("memo"),
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/slips?kind=sales&notice="+url.QueryEscape("売上伝票を修正し、履歴を記録しました。"), http.StatusSeeOther)
}

func (s *Server) salesReturnCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/slips?kind=sales&error="+url.QueryEscape("入力内容を確認してください。"), http.StatusSeeOther)
		return
	}
	ids, err := s.store.CreateSalesReturn(r.Context(), database.CreateSalesReturnInput{
		OrganizationID: user.OrganizationID, SalesSlipID: r.PathValue("id"),
		SalesLineIDs: r.Form["sales_line_id"], ReturnDate: r.FormValue("return_date"),
		Reason: r.FormValue("reason"), Notes: r.FormValue("notes"), ActorUserID: user.ID,
	})
	if err != nil {
		http.Redirect(w, r, "/slips?kind=sales&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	for _, id := range ids {
		s.auditTransaction(r, user, "return_takehome", id, "sales_return.created",
			map[string]string{"sales_slip_id": r.PathValue("id")}, r.FormValue("reason"))
	}
	http.Redirect(w, r, "/slips?kind=sales-returns&notice="+url.QueryEscape("売上返品伝票を起票しました。"), http.StatusSeeOther)
}
