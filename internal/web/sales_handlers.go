package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func defaultSalesDestinations() []database.MasterRecord {
	return []database.MasterRecord{
		{Code: "B001", Name: "ウォッチマート"},
		{Code: "B002", Name: "タイムレス商会"},
		{Code: "B003", Name: "ラグジュアリーアイランド"},
		{Code: "B004", Name: "クロノス東京"},
	}
}

func (s *Server) sales(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	products, err := s.store.TransactionProducts(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "商品を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	var sourceShipment database.ShipmentSlip
	shipmentID := strings.TrimSpace(r.URL.Query().Get("shipment_id"))
	if shipmentID != "" {
		sourceShipment, err = s.store.Shipment(r.Context(), user.OrganizationID, shipmentID)
		if err != nil || sourceShipment.Status != "confirmed" {
			http.Error(w, "売上登録元の出荷伝票が見つかりません。", http.StatusNotFound)
			return
		}
		if sourceShipment.LinkedSalesSlipID != "" {
			http.Error(w, database.ErrShipmentAlreadyUsed.Error(), http.StatusConflict)
			return
		}
	}
	sourceProductIDs := make(map[string]bool, len(sourceShipment.Lines))
	for _, line := range sourceShipment.Lines {
		sourceProductIDs[line.ProductID] = true
	}
	available := products[:0]
	for _, product := range products {
		if product.InventoryStatus == "in_stock" || product.InventoryStatus == "reserved" ||
			sourceProductIDs[product.ID] {
			available = append(available, product)
		}
	}
	destinations, err := s.store.MasterRecords(r.Context(), user.OrganizationID, "sales-destinations")
	if err != nil {
		http.Error(w, "販売先を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	if len(destinations) == 0 {
		destinations = defaultSalesDestinations()
	}
	if sourceShipment.ID != "" {
		foundRecipient := false
		for _, destination := range destinations {
			if destination.Name == sourceShipment.RecipientName {
				foundRecipient = true
				break
			}
		}
		if !foundRecipient {
			destinations = append(destinations, database.MasterRecord{Name: sourceShipment.RecipientName})
		}
	}
	todayISO := time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
	nextNumber, err := s.store.NextSalesSlipNumber(r.Context(), user.OrganizationID, todayISO)
	if err != nil {
		http.Error(w, "伝票番号を採番できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "sales", http.StatusOK, pageData{
		Title: "売上登録", Active: "sales", User: user, Products: available,
		SalesDestinationOptions: destinations, TodayISO: todayISO, NextSalesNumber: nextNumber,
		Shipment: sourceShipment,
		CSRF:     csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) saleNew(w http.ResponseWriter, r *http.Request) {
	s.sales(w, r)
}

func (s *Server) salesNextNumber(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	salesDate := strings.TrimSpace(r.URL.Query().Get("date"))
	if _, err := time.Parse("2006-01-02", salesDate); err != nil {
		http.Error(w, "売上日を正しく入力してください", http.StatusUnprocessableEntity)
		return
	}
	nextNumber, err := s.store.NextSalesSlipNumber(r.Context(), user.OrganizationID, salesDate)
	if err != nil {
		http.Error(w, "売上伝票番号を採番できませんでした", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{"sales_slip_number": nextNumber})
}

func (s *Server) salesShipmentPrefill(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	number := strings.TrimSpace(r.URL.Query().Get("number"))
	shipment, err := s.store.ShipmentByNumber(r.Context(), user.OrganizationID, number)
	if err != nil {
		http.Error(w, "指定された出荷伝票が見つかりません", http.StatusNotFound)
		return
	}
	if shipment.Status != "confirmed" {
		http.Error(w, "確定済みの出荷伝票だけを売上伝票へ取り込めます", http.StatusConflict)
		return
	}
	if shipment.LinkedSalesSlipID != "" {
		http.Error(w, database.ErrShipmentAlreadyUsed.Error(), http.StatusConflict)
		return
	}
	salesDate := time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
	nextNumber, err := s.store.NextSalesSlipNumber(r.Context(), user.OrganizationID, salesDate)
	if err != nil {
		http.Error(w, "売上伝票番号を採番できませんでした", http.StatusInternalServerError)
		return
	}
	type prefillLine struct {
		ProductID   string `json:"product_id"`
		ProductCode string `json:"product_code"`
		Brand       string `json:"brand"`
		Model       string `json:"model"`
		Reference   string `json:"reference"`
		Serial      string `json:"serial"`
		Accessories string `json:"accessories"`
		Quantity    int    `json:"quantity"`
		Price       int64  `json:"price"`
	}
	products, err := s.store.TransactionProducts(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "商品情報を取得できませんでした", http.StatusInternalServerError)
		return
	}
	productByID := make(map[string]database.Product, len(products))
	for _, product := range products {
		productByID[product.ID] = product
	}
	lines := make([]prefillLine, 0, len(shipment.Lines))
	for _, line := range shipment.Lines {
		item := prefillLine{
			ProductID: line.ProductID, ProductCode: line.ProductCode,
			Brand: line.Brand, Model: line.ModelNumber, Reference: line.ModelNumber,
			Quantity: line.Quantity, Price: line.WholesalePriceMinor,
		}
		if product, ok := productByID[line.ProductID]; ok {
			item.Brand = product.Brand
			item.Model = product.ProductType
			item.Reference = product.ModelNumber
			item.Serial = product.SerialNumber
			item.Accessories = product.Accessories
		}
		lines = append(lines, item)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"shipment_id": shipment.ID, "shipment_number": shipment.ShipmentNumber,
		"sales_slip_number": nextNumber, "recipient_name": shipment.RecipientName,
		"recipient_address": shipment.RecipientAddress, "recipient_phone": shipment.RecipientPhone,
		"notes": shipment.Notes, "lines": lines,
	})
}

func (s *Server) saleCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "入力内容を読み取れませんでした。", http.StatusBadRequest)
		return
	}
	taxMode := r.FormValue("tax_mode")
	if taxMode == "" {
		taxMode = "normal"
	}
	if taxMode != "normal" && taxMode != "exempt" {
		http.Error(w, "通常または免税を選択してください。", http.StatusUnprocessableEntity)
		return
	}
	shipmentID := strings.TrimSpace(r.FormValue("shipment_id"))
	enteredNumber := strings.ToUpper(strings.TrimSpace(r.FormValue("slip_number")))
	autoNumber := r.FormValue("auto_number") == "1"
	if shipmentID == "" && strings.HasPrefix(enteredNumber, "SH-") {
		shipment, err := s.store.ShipmentByNumber(r.Context(), user.OrganizationID, enteredNumber)
		if err != nil {
			http.Error(w, "指定された出荷伝票が見つかりません", http.StatusUnprocessableEntity)
			return
		}
		shipmentID = shipment.ID
	}
	if autoNumber && shipmentID == "" {
		// The number shown in the form is only a preview. Generate the final
		// number inside CreateSaleDraft's transaction to avoid stale-number
		// collisions with sales registered by another request.
		enteredNumber = ""
	}
	var lines []database.SalesLineInput
	if shipmentID == "" {
		productIDs, quantities, prices := r.Form["product_id"], r.Form["quantity"], r.Form["unit_price"]
		if len(productIDs) == 0 || len(productIDs) != len(quantities) || len(productIDs) != len(prices) {
			http.Error(w, "売上明細を1件以上入力してください。", http.StatusUnprocessableEntity)
			return
		}
		lines = make([]database.SalesLineInput, 0, len(productIDs))
		for index, productID := range productIDs {
			quantity, err := strconv.Atoi(quantities[index])
			if err != nil || quantity < 1 {
				http.Error(w, "数量を正しく入力してください。", http.StatusUnprocessableEntity)
				return
			}
			price, err := database.ParseMinorAmount(prices[index])
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
				return
			}
			lines = append(lines, database.SalesLineInput{
				ProductID: productID, Quantity: quantity, UnitPriceMinor: price, Currency: "JPY",
			})
		}
	}
	sale, err := s.store.CreateSaleDraft(r.Context(), database.CreateSaleInput{
		OrganizationID: user.OrganizationID, SlipNumber: enteredNumber,
		SourceShipmentID: shipmentID,
		SalesDate:        r.FormValue("sales_date"),
		CustomerName:     r.FormValue("customer_name"), Notes: r.FormValue("notes"), CreatedBy: user.ID,
		Lines: lines,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "sales_slip", sale.ID, "sale.created", sale, "")
	http.Redirect(w, r, "/sales/"+sale.ID+"?notice="+url.QueryEscape("売上伝票を登録しました。続けて売上を確定してください。"), http.StatusSeeOther)
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
		http.Redirect(w, r, "/approvals?notice="+url.QueryEscape("売上確定を承認申請しました。"), http.StatusSeeOther)
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
		http.Redirect(w, r, "/approvals?notice="+url.QueryEscape("売上取消を承認申請しました。"), http.StatusSeeOther)
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
	requestGroupID := strings.TrimSpace(r.URL.Query().Get("request_group_id"))
	var prefillCodes []string
	var prefillRecipient string
	if requestGroupID != "" {
		group, groupErr := s.store.PurchaseRequestGroup(r.Context(), user.OrganizationID, requestGroupID)
		if groupErr != nil {
			http.Error(w, "購入リクエストが見つかりません。", http.StatusNotFound)
			return
		}
		pending := 0
		for _, request := range group.Items {
			if prefillRecipient == "" {
				prefillRecipient = request.GuestName
			}
			if request.Status == "pending" {
				pending++
			}
			if request.Status == "approved" {
				prefillCodes = append(prefillCodes, request.ProductCode)
			}
		}
		if pending > 0 || len(prefillCodes) == 0 {
			http.Error(w, "全商品を判定し、1点以上承認してから出荷登録へ進んでください。", http.StatusConflict)
			return
		}
	}
	products, err := s.store.TransactionProducts(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "商品を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	available := products[:0]
	for _, product := range products {
		if product.InventoryStatus == "in_stock" || product.InventoryStatus == "reserved" ||
			product.InventoryStatus == "sold" {
			available = append(available, product)
		}
	}
	destinations, err := s.store.MasterRecords(r.Context(), user.OrganizationID, "sales-destinations")
	if err != nil {
		http.Error(w, "出荷先を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	if len(destinations) == 0 {
		destinations = defaultSalesDestinations()
	}
	todayISO := time.Now().In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
	nextNumber, err := s.store.NextShipmentSlipNumber(r.Context(), user.OrganizationID, todayISO)
	if err != nil {
		http.Error(w, "伝票番号を採番できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "shipments", http.StatusOK, pageData{
		Title: "出荷登録", Active: "shipments", User: user, Products: available,
		SalesDestinationOptions: destinations, TodayISO: todayISO, NextShipmentNumber: nextNumber,
		ShipmentPrefillCodes: prefillCodes, ShipmentPrefillRecipient: prefillRecipient,
		ShipmentPurchaseRequestGroup: requestGroupID,
		CSRF:                         csrfFromRequest(r), Notice: r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error"),
	})
}

func (s *Server) shipmentNew(w http.ResponseWriter, r *http.Request) {
	s.shipments(w, r)
}

func (s *Server) shipmentCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "入力内容を確認してください。", http.StatusBadRequest)
		return
	}
	productIDs, quantities, wholesalePrices := r.Form["product_id"], r.Form["quantity"], r.Form["wholesale_price"]
	if len(productIDs) == 0 || len(productIDs) != len(quantities) || len(productIDs) != len(wholesalePrices) {
		http.Error(w, "出荷明細を1件以上入力してください。", http.StatusUnprocessableEntity)
		return
	}
	lines := make([]database.ShipmentLineInput, 0, len(productIDs))
	for index, productID := range productIDs {
		quantity, err := strconv.Atoi(quantities[index])
		if err != nil || quantity < 1 {
			http.Error(w, "数量を正しく入力してください。", http.StatusUnprocessableEntity)
			return
		}
		wholesalePrice, err := database.ParseMinorAmount(wholesalePrices[index])
		if err != nil {
			http.Error(w, "卸値を正しく入力してください。", http.StatusUnprocessableEntity)
			return
		}
		lines = append(lines, database.ShipmentLineInput{
			ProductID: productID, Quantity: quantity, WholesalePriceMinor: wholesalePrice,
		})
	}
	shipment, err := s.store.CreateShipmentDraft(r.Context(), database.CreateShipmentInput{
		OrganizationID: user.OrganizationID, ShipmentDate: r.FormValue("shipment_date"),
		RecipientName: r.FormValue("recipient_name"), Notes: r.FormValue("notes"), CreatedBy: user.ID,
		PurchaseRequestGroupID: r.FormValue("purchase_request_group_id"), Lines: lines,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	shipment, err = s.store.ConfirmShipment(r.Context(), user.OrganizationID, shipment.ID, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "shipment_slip", shipment.ID, "shipment.created", shipment, "")
	http.Redirect(w, r, "/sales?shipment_id="+url.QueryEscape(shipment.ID)+
		"&notice="+url.QueryEscape("出荷を確定しました。続けて売上登録を行ってください。"), http.StatusSeeOther)
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
		http.Redirect(w, r, "/approvals?notice="+url.QueryEscape("出荷取消を承認申請しました。"), http.StatusSeeOther)
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
