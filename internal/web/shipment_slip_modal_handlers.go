package web

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) shipmentSlipModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.shipmentSlipModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "出荷伝票が見つかりません。")
		return
	}
	s.renderPartial(w, "shipment-slip-modal", "content", http.StatusOK, data)
}

func (s *Server) shipmentSlipEditModal(w http.ResponseWriter, r *http.Request) {
	data, err := s.shipmentSlipModalData(r)
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "出荷伝票が見つかりません。")
		return
	}
	s.renderPartial(w, "shipment-slip-edit-modal", "content", http.StatusOK, data)
}

func (s *Server) shipmentSlipModalData(r *http.Request) (pageData, error) {
	user, _ := currentUser(r.Context())
	shipment, err := s.store.Shipment(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		return pageData{}, err
	}
	revisions, err := s.store.ShipmentRevisions(r.Context(), user.OrganizationID, shipment.ID)
	if err != nil {
		return pageData{}, err
	}
	return pageData{
		Title: "出荷伝票", Active: "slips", User: user, CSRF: csrfFromRequest(r),
		Shipment: shipment, ShipmentRevisions: revisions,
	}, nil
}

func (s *Server) shipmentSlipEdit(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/slips?kind=shipments&error="+url.QueryEscape("入力内容を確認してください。"), http.StatusSeeOther)
		return
	}
	lineIDs, prices := r.Form["line_id"], r.Form["wholesale_price"]
	if len(lineIDs) != len(prices) {
		http.Redirect(w, r, "/slips?kind=shipments&error="+url.QueryEscape("商品明細の形式が正しくありません。"), http.StatusSeeOther)
		return
	}
	lines := make([]database.ShipmentEditLine, 0, len(lineIDs))
	for index := range lineIDs {
		price, err := database.ParseMinorAmount(prices[index])
		if err != nil {
			http.Redirect(w, r, "/slips?kind=shipments&error="+url.QueryEscape("卸値を数字で入力してください。"), http.StatusSeeOther)
			return
		}
		lines = append(lines, database.ShipmentEditLine{
			LineID: lineIDs[index], WholesalePriceMinor: price,
		})
	}
	before, _ := s.store.Shipment(r.Context(), user.OrganizationID, r.PathValue("id"))
	err := s.store.UpdateShipmentSlip(r.Context(), database.UpdateShipmentSlipInput{
		OrganizationID: user.OrganizationID, ShipmentSlipID: r.PathValue("id"),
		ShipmentDate: r.FormValue("shipment_date"), Notes: r.FormValue("notes"),
		Memo: r.FormValue("memo"), ActorUserID: user.ID, Lines: lines,
	})
	if err != nil {
		http.Redirect(w, r, "/slips?kind=shipments&error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	after, _ := s.store.Shipment(r.Context(), user.OrganizationID, r.PathValue("id"))
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "shipment_slip", TargetID: r.PathValue("id"), Action: "shipment.revised",
		BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), Reason: r.FormValue("memo"),
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	http.Redirect(w, r, "/slips?kind=shipments&notice="+url.QueryEscape("出荷伝票を修正し、履歴を記録しました。"), http.StatusSeeOther)
}
