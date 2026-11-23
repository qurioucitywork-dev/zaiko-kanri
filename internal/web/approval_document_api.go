package web

import (
	"encoding/json"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/reportpdf"
	"net/http"
	"strings"
)

// Render the approved snapshot, not potentially changed live document values.
func (s *Server) apiApprovedDocument(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	a, err := s.repository.ApprovalRequest(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil || (user.Role != database.RoleAdmin && a.RequestedBy != user.ID) {
		writeAPIError(w, 404, "approval_not_found", "申請が見つかりません。")
		return
	}
	if a.Status != "approved" || a.TargetType != "document_change" || !strings.HasSuffix(a.RequestedAction, ".issue") {
		writeAPIError(w, 409, "approval_required", "発行には管理者承認が必要です。")
		return
	}
	var snap struct {
		After       json.RawMessage                  `json:"after"`
		OfficialPDF *persistence.OfficialDocumentRef `json:"officialPDF"`
	}
	if json.Unmarshal([]byte(a.SnapshotJSON), &snap) != nil {
		writeAPIError(w, 409, "snapshot_invalid", "承認内容を確認できません。")
		return
	}
	if snap.OfficialPDF != nil {
		writeJSON(w, 200, snap.OfficialPDF)
		return
	}
	kind := strings.TrimSuffix(a.RequestedAction, ".issue")
	var doc reportpdf.Document
	var number string
	switch kind {
	case "purchase":
		var v persistence.PurchaseSlipRecord
		err = json.Unmarshal(snap.After, &v)
		doc = purchasePDF(v)
		number = v.SlipNumber
	case "sale":
		var v persistence.SaleSlipRecord
		err = json.Unmarshal(snap.After, &v)
		doc = salePDF(v)
		number = v.SlipNumber
	case "consignment":
		var v persistence.ConsignmentSlipRecord
		err = json.Unmarshal(snap.After, &v)
		doc = consignmentPDF(v)
		number = v.SlipNumber
	case "shipment":
		var v persistence.ShipmentSlipRecord
		err = json.Unmarshal(snap.After, &v)
		number = v.SlipNumber
		doc = reportpdf.Document{Title: "出荷伝票", Number: number, TransactionDate: string(v.ShipmentDate), PartnerLabel: "出荷先", PartnerName: v.BuyerName, Currency: v.DisplayCurrency, Notes: v.Notes}
		for i, line := range v.Lines {
			value := line.SalePriceUSDMinor
			if v.DisplayCurrency == "JPY" {
				value = line.ConvertedSalePriceJPY
			}
			doc.Lines = append(doc.Lines, reportpdf.Line{Number: i + 1, ProductCode: line.ProductCode, Description: line.Brand + " / " + line.ModelNumber, Amount: amount(v.DisplayCurrency, value)})
		}
	case "return":
		var v persistence.ReturnSlipRecord
		err = json.Unmarshal(snap.After, &v)
		number = v.SlipNumber
		title, partner := "売上返品伝票", v.BuyerName
		if v.OperationType == "purchase_return" {
			title, partner = "仕入返品伝票", v.SupplierName
		}
		doc = reportpdf.Document{Title: title, Number: number, TransactionDate: string(v.TransactionDate), PartnerLabel: "取引先", PartnerName: partner, Notes: v.Reason + "\n" + v.Notes}
		for i, line := range v.Lines {
			doc.Lines = append(doc.Lines, reportpdf.Line{Number: i + 1, ProductCode: line.ProductCode, Description: line.Brand + " / " + line.ModelNumber})
		}
	default:
		writeAPIError(w, 400, "invalid_document", "帳票種別を確認してください。")
		return
	}
	if err != nil {
		writeAPIError(w, 409, "snapshot_invalid", "承認内容を確認できません。")
		return
	}
	ref, err := s.storeOfficialPDF(r, kind, a.TargetID, number, doc, json.RawMessage(snap.After))
	if err == nil {
		err = s.repository.SaveApprovalPDF(r.Context(), user.OrganizationID, a.ID, ref)
	}
	if err != nil {
		writeAPIError(w, 500, "pdf_failed", "承認は完了しています。PDF作成を再試行してください。")
		return
	}
	writeJSON(w, 200, ref)
}
