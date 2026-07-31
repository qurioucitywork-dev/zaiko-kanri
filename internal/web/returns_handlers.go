package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type returnReferenceItem struct {
	ProductCode string
	Brand       string
	ModelNumber string
	AmountJPY   int64
}

func (s *Server) returns(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	status := r.URL.Query().Get("status")
	query := r.URL.Query().Get("q")
	records, err := s.store.ReturnTakehomeSummaries(r.Context(), user.OrganizationID, status, query)
	if err != nil {
		http.Error(w, "返品／持ち帰り一覧を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "returns", http.StatusOK, pageData{
		Title: "返品/持ち帰り", Active: "returns", User: user, CSRF: csrfFromRequest(r),
		ReturnSummaries: records, Status: status, Query: query,
		Notice: r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error"),
	})
}

func (s *Server) returnDetail(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	sale, err := s.store.Sale(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "対象の売上伝票が見つかりません。", http.StatusNotFound)
		return
	}
	items, err := s.store.ReturnTakehomeItems(r.Context(), user.OrganizationID, sale.ID)
	if err != nil {
		http.Error(w, "返品／持ち帰り明細を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "return-detail", http.StatusOK, pageData{
		Title: "返品/持ち帰り詳細", Active: "returns", User: user, CSRF: csrfFromRequest(r),
		Sale: sale, ReturnItems: items,
		Notice: r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error"),
	})
}

func (s *Server) returnRestoreModal(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	sale, err := s.store.Sale(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeRequestError(w, r, http.StatusNotFound, "対象の売上伝票が見つかりません。")
		return
	}
	items, err := s.store.ReturnTakehomeItems(r.Context(), user.OrganizationID, sale.ID)
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, "返品／持ち帰り明細を取得できませんでした。")
		return
	}
	conditions, err := s.store.MasterRecords(r.Context(), user.OrganizationID, "conditions")
	if err != nil {
		writeRequestError(w, r, http.StatusInternalServerError, "コンディションを取得できませんでした。")
		return
	}
	returnLineIDs := make(map[string]bool)
	pending := make([]database.ReturnTakehomeItem, 0, len(items))
	completed := make([]database.ReturnTakehomeItem, 0, len(items))
	var returnTotal int64
	for _, item := range items {
		if item.Status == "cancelled" {
			continue
		}
		returnLineIDs[item.SalesLineID] = true
		returnTotal += item.AmountJPY
		for _, condition := range conditions {
			if item.Condition == condition.Name || strings.Contains(item.Condition, condition.Name) {
				item.RestoreCondition = condition.Name
				break
			}
		}
		if item.InventoryRestoredAt == nil {
			pending = append(pending, item)
		} else {
			completed = append(completed, item)
		}
	}
	references := make([]returnReferenceItem, 0, len(sale.Lines))
	for _, line := range sale.Lines {
		if returnLineIDs[line.ID] {
			continue
		}
		amount := line.ConvertedTotalJPY
		if amount == 0 && line.SaleCurrency == "JPY" {
			amount = line.UnitPriceMinor * int64(line.Quantity)
		}
		references = append(references, returnReferenceItem{
			ProductCode: line.ProductCode,
			Brand:       line.Brand,
			ModelNumber: line.ModelNumber,
			AmountJPY:   amount,
		})
	}
	netTotal := sale.TotalJPY - returnTotal
	if netTotal < 0 {
		netTotal = 0
	}
	s.renderPartial(w, "return-restore-modal", "content", http.StatusOK, pageData{
		Title: "返品/持ち帰り確認", Active: "returns", User: user, CSRF: csrfFromRequest(r),
		Sale: sale, ReturnPendingItems: pending, ReturnCompletedItems: completed, ReturnReferenceItems: references,
		ReturnOriginalTotal: sale.TotalJPY, ReturnPendingTotal: returnTotal, ReturnNetTotal: netTotal,
		ProductConditionOptions: conditions,
	})
}

func (s *Server) returnRestore(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	saleID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/returns?error="+url.QueryEscape("入力内容を確認してください。"), http.StatusSeeOther)
		return
	}
	conditionRecords, err := s.store.MasterRecords(r.Context(), user.OrganizationID, "conditions")
	if err != nil {
		http.Redirect(w, r, "/returns?error="+url.QueryEscape("コンディションを確認できませんでした。"), http.StatusSeeOther)
		return
	}
	allowedConditions := make(map[string]bool, len(conditionRecords))
	for _, record := range conditionRecords {
		allowedConditions[record.Name] = true
	}
	selected := r.Form["item_id"]
	restoreItems := make([]database.ReturnRestoreItemInput, 0, len(selected))
	for _, itemID := range selected {
		itemID = strings.TrimSpace(itemID)
		condition := strings.TrimSpace(r.FormValue("condition_" + itemID))
		if condition != "" && !allowedConditions[condition] {
			http.Redirect(w, r, "/returns?error="+url.QueryEscape(database.ErrReturnRestoreCondition.Error()), http.StatusSeeOther)
			return
		}
		quantity, parseErr := strconv.Atoi(r.FormValue("quantity_" + itemID))
		if parseErr != nil {
			http.Redirect(w, r, "/returns?error="+url.QueryEscape(database.ErrReturnInvalidQuantity.Error()), http.StatusSeeOther)
			return
		}
		restoreItems = append(restoreItems, database.ReturnRestoreItemInput{
			ItemID: itemID, Condition: condition, Quantity: quantity,
			Box: strings.TrimSpace(r.FormValue("box_" + itemID)),
		})
	}
	if len(restoreItems) == 0 {
		http.Redirect(w, r, "/returns?error="+url.QueryEscape(database.ErrReturnRestoreSelection.Error()), http.StatusSeeOther)
		return
	}
	comment := strings.TrimSpace(r.FormValue("admin_comment"))
	if user.Role != database.RoleAdmin {
		approval, createErr := s.store.CreateApprovalRequest(r.Context(), database.CreateApprovalInput{
			OrganizationID:  user.OrganizationID,
			ApprovalType:    "inventory",
			TargetType:      "return_takehome",
			TargetID:        saleID,
			ActionKey:       "return_takehome.restore",
			ApplicantUserID: user.ID,
			RequestReason:   "返品／持ち帰り商品の在庫戻し",
			ActionPayload: database.RestoreReturnTakehomeInput{
				OrganizationID: user.OrganizationID, SaleID: saleID, ActorID: user.ID,
				Comment: comment, Items: restoreItems,
			},
		})
		if createErr != nil {
			http.Redirect(w, r, "/returns?error="+url.QueryEscape(returnErrorMessage(createErr)), http.StatusSeeOther)
			return
		}
		s.auditTransaction(r, user, "approval_request", approval.ID, "approval.requested",
			map[string]any{"action": "return_takehome.restore", "sale_id": saleID}, "在庫戻しを承認申請")
		http.Redirect(w, r, "/returns?notice="+url.QueryEscape("在庫戻しの承認を申請しました。"), http.StatusSeeOther)
		return
	}
	if r.FormValue("confirmed") != "1" {
		http.Redirect(w, r, "/returns?error="+url.QueryEscape("BOX確認後に在庫戻しを確定してください。"), http.StatusSeeOther)
		return
	}
	if err := s.store.RestoreReturnTakehomeItems(r.Context(), database.RestoreReturnTakehomeInput{
		OrganizationID: user.OrganizationID,
		SaleID:         saleID,
		ActorID:        user.ID,
		Comment:        comment,
		Items:          restoreItems,
	}); err != nil {
		http.Redirect(w, r, "/returns?error="+url.QueryEscape(returnErrorMessage(err)), http.StatusSeeOther)
		return
	}
	s.auditTransaction(r, user, "return_takehome", saleID, "return_takehome.inventory_restored",
		map[string]any{"status": "completed", "restored_items": len(restoreItems)}, "返品／持ち帰り商品を在庫に戻す")
	http.Redirect(w, r, "/returns?notice="+url.QueryEscape(strconv.Itoa(len(restoreItems))+"点の商品を在庫に戻しました。"), http.StatusSeeOther)
}

func (s *Server) returnCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	saleID := r.PathValue("id")
	quantity, err := strconv.Atoi(r.FormValue("quantity"))
	if err != nil {
		http.Redirect(w, r, "/returns/"+saleID+"?error="+url.QueryEscape("対象数量を正しく入力してください。"), http.StatusSeeOther)
		return
	}
	item, err := s.store.CreateReturnTakehome(r.Context(), user.OrganizationID, saleID,
		r.FormValue("sales_line_id"), r.FormValue("action_type"), quantity, r.FormValue("reason"), user.ID)
	if err != nil {
		http.Redirect(w, r, "/returns/"+saleID+"?error="+url.QueryEscape(returnErrorMessage(err)), http.StatusSeeOther)
		return
	}
	s.auditTransaction(r, user, "return_takehome", item.ID, "return_takehome.created", item, item.Reason)
	http.Redirect(w, r, "/returns/"+saleID+"?notice="+url.QueryEscape("返品／持ち帰り対象を受け付けました。"), http.StatusSeeOther)
}

func (s *Server) returnComplete(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	saleID := r.PathValue("id")
	itemID := r.PathValue("itemID")
	if err := s.store.CompleteReturnTakehome(r.Context(), user.OrganizationID, saleID, itemID, r.FormValue("notes"), user.ID); err != nil {
		http.Redirect(w, r, "/returns/"+saleID+"?error="+url.QueryEscape(returnErrorMessage(err)), http.StatusSeeOther)
		return
	}
	s.auditTransaction(r, user, "return_takehome", itemID, "return_takehome.completed", map[string]string{"status": "completed"}, r.FormValue("notes"))
	http.Redirect(w, r, "/returns/"+saleID+"?notice="+url.QueryEscape("処理完了として記録しました。"), http.StatusSeeOther)
}

func returnErrorMessage(err error) string {
	switch {
	case errors.Is(err, database.ErrReturnInvalidQuantity),
		errors.Is(err, database.ErrReturnNotEligible),
		errors.Is(err, database.ErrReturnAlreadyPending),
		errors.Is(err, database.ErrReturnAlreadyHandled),
		errors.Is(err, database.ErrReturnRestoreSelection),
		errors.Is(err, database.ErrReturnRestoreCondition):
		return err.Error()
	default:
		return "処理を完了できませんでした。"
	}
}
