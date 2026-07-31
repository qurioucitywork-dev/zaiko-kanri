package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) guestManagement(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	companies, err := s.store.GuestCompanies(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "ゲスト企業を取得できませんでした", http.StatusInternalServerError)
		return
	}
	boxes, err := s.store.GuestBoxes(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "BOXを取得できませんでした", http.StatusInternalServerError)
		return
	}
	matrix, err := s.store.GuestBoxMatrix(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "公開設定を取得できませんでした", http.StatusInternalServerError)
		return
	}
	summary, err := s.store.GuestPublicationSummary(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "公開情報を取得できませんでした", http.StatusInternalServerError)
		return
	}
	alertCount, _ := s.pendingAlerts(r, user.OrganizationID)
	s.render(w, "guest-management", http.StatusOK, pageData{
		Title: "ゲスト管理", Active: "guest-management", User: user, CSRF: csrfFromRequest(r),
		GuestCompanies: companies, GuestBoxes: boxes, GuestBoxMatrix: matrix,
		GuestPublicationSummary: summary,
		AlertCount:              alertCount, Notice: r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error"),
	})
}

func (s *Server) guestBoxDraftSave(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "公開設定を確認してください", http.StatusBadRequest)
		return
	}
	companies, err := s.store.GuestCompanies(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	boxes, err := s.store.GuestBoxes(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	selected := make(map[string]bool)
	for _, value := range r.Form["selection"] {
		selected[value] = true
	}
	for _, company := range companies {
		for _, box := range boxes {
			key := company.ID + "|" + box.ID
			if err := s.store.SaveGuestBoxDraft(r.Context(), user.OrganizationID, company.ID, box.ID,
				user.ID, selected[key]); err != nil {
				http.Redirect(w, r, "/guest-management?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
				return
			}
		}
	}
	s.auditTransaction(r, user, "guest_box_publication", user.OrganizationID,
		"guest_box.draft_saved", map[string]any{"selection": r.Form["selection"]}, "")
	http.Redirect(w, r, "/guest-management?notice="+url.QueryEscape("下書きを保存しました。"), http.StatusSeeOther)
}

func (s *Server) guestBoxPublish(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "公開設定を確認してください", http.StatusBadRequest)
		return
	}
	companies, err := s.store.GuestCompanies(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	boxes, err := s.store.GuestBoxes(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	selected := make(map[string]bool)
	for _, value := range r.Form["selection"] {
		selected[value] = true
	}
	for _, company := range companies {
		for _, box := range boxes {
			key := company.ID + "|" + box.ID
			if err := s.store.SaveGuestBoxDraft(r.Context(), user.OrganizationID, company.ID, box.ID,
				user.ID, selected[key]); err != nil {
				http.Redirect(w, r, "/guest-management?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
				return
			}
		}
	}
	if err := s.store.PublishGuestBoxSnapshot(r.Context(), user.OrganizationID, user.ID); err != nil {
		http.Redirect(w, r, "/guest-management?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	s.auditTransaction(r, user, "guest_box_publication", user.OrganizationID,
		"guest_box.snapshot_published", map[string]any{"status": "published", "selection": r.Form["selection"]}, "")
	http.Redirect(w, r, "/guest-management?notice="+url.QueryEscape("ゲスト公開情報を一括更新しました。"), http.StatusSeeOther)
}

func (s *Server) guestBoxRename(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := s.store.RenameGuestBox(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID,
		r.FormValue("box_name")); err != nil {
		http.Redirect(w, r, "/guest-management?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	s.auditTransaction(r, user, "guest_box", r.PathValue("id"),
		"guest_box.renamed", map[string]string{"box_name": r.FormValue("box_name")}, "")
	http.Redirect(w, r, "/guest-management?notice="+url.QueryEscape("BOX名を保存しました。"), http.StatusSeeOther)
}

func (s *Server) guestBoxModal(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	boxes, err := s.store.GuestBoxes(r.Context(), user.OrganizationID)
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	var selected database.GuestBox
	for _, box := range boxes {
		if box.ID == r.PathValue("id") {
			selected = box
			break
		}
	}
	if selected.ID == "" {
		writeRequestError(w, r, http.StatusNotFound, "BOXが見つかりません")
		return
	}
	lineup, err := s.store.GuestBoxProducts(r.Context(), user.OrganizationID, selected.ID)
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	publishedCompanies, err := s.store.GuestBoxPublishedCompanies(r.Context(), user.OrganizationID, selected.ID)
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderPartial(w, "guest-box-modal", "guest-box-modal", http.StatusOK, pageData{
		User: user, CSRF: csrfFromRequest(r), GuestBoxes: boxes, GuestSelectedBox: selected,
		GuestBoxProducts: lineup, GuestPublishedCompanies: publishedCompanies,
	})
}

func (s *Server) guestBoxEditModal(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	boxes, err := s.store.GuestBoxes(r.Context(), user.OrganizationID)
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	var selected database.GuestBox
	for _, box := range boxes {
		if box.ID == r.PathValue("id") {
			selected = box
			break
		}
	}
	if selected.ID == "" {
		writeRequestError(w, r, http.StatusNotFound, "BOXが見つかりません")
		return
	}
	lineup, err := s.store.GuestBoxProducts(r.Context(), user.OrganizationID, selected.ID)
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	dateFrom := strings.TrimSpace(r.FormValue("date_from"))
	dateTo := strings.TrimSpace(r.FormValue("date_to"))
	brand := strings.TrimSpace(r.FormValue("brand"))
	query := strings.TrimSpace(r.FormValue("query"))
	searched := dateFrom != "" || dateTo != "" || brand != "" || query != ""
	var candidates []database.GuestBoxProduct
	if searched {
		candidates, err = s.store.GuestBoxProductCandidates(r.Context(), user.OrganizationID, dateFrom, dateTo, brand, query)
		if err != nil {
			writeRequestError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}
	brands, err := s.store.MasterRecords(r.Context(), user.OrganizationID, "brands")
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderPartial(w, "guest-box-edit-modal", "guest-box-edit-modal", http.StatusOK, pageData{
		User: user, CSRF: csrfFromRequest(r), GuestBoxes: boxes, GuestSelectedBox: selected,
		GuestBoxProducts: lineup, GuestProductCandidates: candidates, GuestProductSearched: searched,
		GuestProductDateFrom: dateFrom, GuestProductDateTo: dateTo, GuestProductBrand: brand,
		GuestProductQuery: query, GuestBrands: brands,
	})
}

func (s *Server) guestBoxProductAdd(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		writeRequestError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	productIDs := r.Form["product_id"]
	if err := s.store.AddGuestBoxProducts(r.Context(), user.OrganizationID, r.PathValue("id"), productIDs, user.ID); err != nil {
		writeRequestError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.auditTransaction(r, user, "guest_box", r.PathValue("id"),
		"guest_box.products_added", map[string]any{"product_ids": productIDs}, "")
	s.guestBoxEditModal(w, r)
}

func (s *Server) guestBoxProductRemove(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	productID := r.PathValue("productID")
	if err := s.store.RemoveGuestBoxProduct(r.Context(), user.OrganizationID, r.PathValue("id"), productID); err != nil {
		writeRequestError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.auditTransaction(r, user, "guest_box", r.PathValue("id"),
		"guest_box.product_removed", map[string]string{"product_id": productID}, "")
	s.guestBoxEditModal(w, r)
}

func (s *Server) guestBoxProductMove(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	productID := r.PathValue("productID")
	targetID := strings.TrimSpace(r.FormValue("target_box_id"))
	if err := s.store.MoveGuestBoxProduct(r.Context(), user.OrganizationID, r.PathValue("id"), productID,
		targetID, user.ID); err != nil {
		writeRequestError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}
	after, _ := json.Marshal(map[string]string{"product_id": productID, "target_box_id": targetID})
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "guest_box",
		TargetID: r.PathValue("id"), Action: "guest_box.product_moved", AfterJSON: string(after),
		Result: "success", IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
	s.guestBoxModal(w, r)
}

func guestManagementErrorStatus(err error) int {
	if errors.Is(err, database.ErrGuestBoxNotFound) || errors.Is(err, database.ErrGuestCompanyNotFound) {
		return http.StatusNotFound
	}
	return http.StatusUnprocessableEntity
}
