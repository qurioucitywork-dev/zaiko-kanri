package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) sales(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	records, err := s.store.Sales(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "売上伝票を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "sales", http.StatusOK, pageData{
		Title: "売上管理", Active: "sales", User: user, Sales: records,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) saleNew(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	products, err := s.store.TransactionProducts(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "商品を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	available := products[:0]
	for _, product := range products {
		if product.InventoryStatus == "in_stock" || product.InventoryStatus == "reserved" {
			available = append(available, product)
		}
	}
	s.render(w, "sale-new", http.StatusOK, pageData{
		Title: "売上伝票登録", Active: "sales", User: user, Products: available, CSRF: csrfFromRequest(r),
	})
}

func (s *Server) saleCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	quantity, err := strconv.Atoi(r.FormValue("quantity"))
	if err != nil {
		http.Error(w, "数量を正しく入力してください。", http.StatusUnprocessableEntity)
		return
	}
	price, err := database.ParseMinorAmount(r.FormValue("unit_price"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	sale, err := s.store.CreateSaleDraft(r.Context(), database.CreateSaleInput{
		OrganizationID: user.OrganizationID, SalesDate: r.FormValue("sales_date"),
		CustomerName: r.FormValue("customer_name"), Notes: r.FormValue("notes"), CreatedBy: user.ID,
		Lines: []database.SalesLineInput{{
			ProductID: r.FormValue("product_id"), Quantity: quantity,
			UnitPriceMinor: price, Currency: "USD",
		}},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "sales_slip", sale.ID, "sale.created", sale, "")
	http.Redirect(w, r, "/sales/"+sale.ID+"?notice="+url.QueryEscape("売上伝票を下書き保存しました。"), http.StatusSeeOther)
}

func (s *Server) saleDetail(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	sale, err := s.store.Sale(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "売上伝票が見つかりません。", http.StatusNotFound)
		return
	}
	s.render(w, "sale-detail", http.StatusOK, pageData{
		Title: "売上伝票詳細", Active: "sales", User: user, Sale: sale,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) saleConfirm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	needsApproval, total, err := s.store.NeedsSaleApproval(r.Context(), user.OrganizationID, r.PathValue("id"), user.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if needsApproval {
		reason := "設定金額以上の売上確定（JPY換算額: " + formatInteger(total) + "）"
		if _, err := s.createOperationApproval(r, user, "high_value_sale", "sales_slip", r.PathValue("id"), "sale.confirm", reason); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Redirect(w, r, "/sales/"+r.PathValue("id")+"?notice="+url.QueryEscape("売上確定を承認申請しました。管理者の判断をお待ちください。"), http.StatusSeeOther)
		return
	}
	sale, err := s.store.ConfirmSale(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "sales_slip", sale.ID, "sale.confirmed", sale, "")
	http.Redirect(w, r, "/sales/"+sale.ID+"?notice="+url.QueryEscape("売上を確定し、価格と為替を固定しました。"), http.StatusSeeOther)
}

func (s *Server) saleCancel(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	reason := strings.TrimSpace(r.FormValue("reason"))
	if user.Role != database.RoleAdmin {
		if reason == "" {
			http.Error(w, "取消理由は必須です", http.StatusUnprocessableEntity)
			return
		}
		if _, err := s.createOperationApproval(r, user, "important_operation", "sales_slip", r.PathValue("id"), "sale.cancel", reason); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Redirect(w, r, "/sales/"+r.PathValue("id")+"?notice="+url.QueryEscape("売上取消を承認申請しました。管理者の判断をお待ちください。"), http.StatusSeeOther)
		return
	}
	if err := s.store.CancelSale(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, reason); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "sales_slip", r.PathValue("id"), "sale.cancelled", map[string]string{"status": "cancelled"}, reason)
	http.Redirect(w, r, "/sales/"+r.PathValue("id")+"?notice="+url.QueryEscape("売上を取消し、関連状態を再計算しました。"), http.StatusSeeOther)
}

func (s *Server) shipments(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	records, err := s.store.Shipments(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "出荷伝票を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "shipments", http.StatusOK, pageData{
		Title: "出荷管理", Active: "shipments", User: user, Shipments: records,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) shipmentNew(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	products, err := s.store.TransactionProducts(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "商品を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	available := products[:0]
	for _, product := range products {
		if product.InventoryStatus == "in_stock" || product.InventoryStatus == "reserved" ||
			product.InventoryStatus == "sold" || product.InventoryStatus == "shipped" {
			available = append(available, product)
		}
	}
	s.render(w, "shipment-new", http.StatusOK, pageData{
		Title: "出荷伝票登録", Active: "shipments", User: user, Products: available, CSRF: csrfFromRequest(r),
	})
}

func (s *Server) shipmentCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	quantity, err := strconv.Atoi(r.FormValue("quantity"))
	if err != nil {
		http.Error(w, "数量を正しく入力してください。", http.StatusUnprocessableEntity)
		return
	}
	shipment, err := s.store.CreateShipmentDraft(r.Context(), database.CreateShipmentInput{
		OrganizationID: user.OrganizationID, ShipmentDate: r.FormValue("shipment_date"),
		RecipientName: r.FormValue("recipient_name"), Notes: r.FormValue("notes"), CreatedBy: user.ID,
		Lines: []database.ShipmentLineInput{{ProductID: r.FormValue("product_id"), Quantity: quantity}},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "shipment_slip", shipment.ID, "shipment.created", shipment, "")
	http.Redirect(w, r, "/shipments/"+shipment.ID+"?notice="+url.QueryEscape("出荷伝票を下書き保存しました。"), http.StatusSeeOther)
}

func (s *Server) shipmentDetail(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	shipment, err := s.store.Shipment(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		http.Error(w, "出荷伝票が見つかりません。", http.StatusNotFound)
		return
	}
	s.render(w, "shipment-detail", http.StatusOK, pageData{
		Title: "出荷伝票詳細", Active: "shipments", User: user, Shipment: shipment,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) shipmentConfirm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	shipment, err := s.store.ConfirmShipment(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "shipment_slip", shipment.ID, "shipment.confirmed", shipment, "")
	notice := "出荷を確定しました。"
	if shipment.Warning != "" {
		notice += " 売上未確定のため警告を表示しています。"
	}
	http.Redirect(w, r, "/shipments/"+shipment.ID+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

func (s *Server) shipmentCancel(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	reason := strings.TrimSpace(r.FormValue("reason"))
	if user.Role != database.RoleAdmin {
		if reason == "" {
			http.Error(w, "取消理由は必須です", http.StatusUnprocessableEntity)
			return
		}
		if _, err := s.createOperationApproval(r, user, "important_operation", "shipment_slip", r.PathValue("id"), "shipment.cancel", reason); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Redirect(w, r, "/shipments/"+r.PathValue("id")+"?notice="+url.QueryEscape("出荷取消を承認申請しました。管理者の判断をお待ちください。"), http.StatusSeeOther)
		return
	}
	if err := s.store.CancelShipment(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, reason); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "shipment_slip", r.PathValue("id"), "shipment.cancelled", map[string]string{"status": "cancelled"}, reason)
	http.Redirect(w, r, "/shipments/"+r.PathValue("id")+"?notice="+url.QueryEscape("出荷を取消し、関連状態を再計算しました。"), http.StatusSeeOther)
}

func (s *Server) auditTransaction(r *http.Request, user database.User, targetType, targetID, action string, after any, reason string) {
	afterJSON, _ := json.Marshal(after)
	_ = s.store.WriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: targetType, TargetID: targetID,
		Action: action, AfterJSON: string(afterJSON), Reason: reason, Result: "success",
		IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: requestID(r.Context()),
	})
}
