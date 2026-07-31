package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type approvalDetailRow struct {
	Label string
	Value string
}

type approvalReviewView struct {
	Request database.ApprovalRequest
	Type    string
	Content string
	Details []approvalDetailRow
}

func (s *Server) approvals(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleAdmin {
		http.Error(w, "管理者のみ利用できます。", http.StatusForbidden)
		return
	}
	records, err := s.store.Approvals(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "承認案件を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	review := make([]database.ApprovalRequest, 0, len(records))
	pending := 0
	for _, record := range records {
		if record.Status != "pending" && record.Status != "returned" {
			continue
		}
		review = append(review, record)
		if record.Status == "pending" {
			pending++
		}
	}
	sortApprovalReview(review, r.URL.Query().Get("sort"))
	views := make([]approvalReviewView, 0, len(review))
	for _, record := range review {
		views = append(views, buildApprovalReview(record))
	}
	s.render(w, "approvals", http.StatusOK, pageData{
		Title: "承認管理", Active: "approvals", User: user, Approvals: review,
		ApprovalReviewViews: views, ApprovalPendingCount: pending,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) approvalApprove(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleAdmin {
		http.Error(w, "管理者のみ利用できます。", http.StatusForbidden)
		return
	}
	approval, err := s.store.Approve(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.auditTransaction(r, user, "approval_request", approval.ID, "approval.approved", approval, "")
	http.Redirect(w, r, "/approvals?notice="+url.QueryEscape("承認し、申請された操作を実行しました。"), http.StatusSeeOther)
}

func (s *Server) approvalReturn(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleAdmin {
		http.Error(w, "管理者のみ利用できます。", http.StatusForbidden)
		return
	}
	comment := strings.TrimSpace(r.FormValue("comment"))
	if err := s.store.ReturnApproval(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, comment); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "approval_request", r.PathValue("id"), "approval.returned",
		map[string]string{"status": "returned"}, comment)
	http.Redirect(w, r, "/approvals?notice="+url.QueryEscape("承認申請を差戻しました。"), http.StatusSeeOther)
}

func sortApprovalReview(records []database.ApprovalRequest, key string) {
	sort.SliceStable(records, func(i, j int) bool {
		left, right := records[i], records[j]
		switch key {
		case "id":
			return left.ID > right.ID
		case "applicant":
			return left.ApplicantName > right.ApplicantName
		case "type":
			return approvalTypeLabel(left) > approvalTypeLabel(right)
		case "status":
			return left.Status > right.Status
		default:
			return left.RequestedAt.After(right.RequestedAt)
		}
	})
}

func approvalTypeLabel(record database.ApprovalRequest) string {
	switch record.ActionKey {
	case "sale.confirm":
		return "売上確定"
	case "sale.cancel":
		return "売上取消"
	case "shipment.cancel":
		return "出荷取消"
	case "return_takehome.restore":
		return "在庫戻し"
	case "stocktake.difference.approve":
		return "棚卸不一致"
	default:
		return "重要操作"
	}
}

func buildApprovalReview(record database.ApprovalRequest) approvalReviewView {
	content := record.TargetID
	if record.RequestReason != "" {
		content += " — " + record.RequestReason
	}
	details := []approvalDetailRow{
		{Label: "対象ID", Value: record.TargetID},
		{Label: "申請理由", Value: fallbackText(record.RequestReason, "理由記載なし")},
	}
	var snapshot map[string]any
	if json.Unmarshal([]byte(record.RequestedSnapshot), &snapshot) == nil {
		appendValue := func(label, key string) {
			if value, ok := snapshot[key]; ok {
				text := strings.TrimSpace(fmt.Sprint(value))
				if text != "" && text != "<nil>" {
					details = append(details, approvalDetailRow{Label: label, Value: text})
				}
			}
		}
		switch record.TargetType {
		case "sales_slip":
			appendValue("売上日", "Date")
			appendValue("販売先", "Customer")
		case "shipment_slip":
			appendValue("出荷日", "Date")
			appendValue("出荷先", "Recipient")
		case "return_takehome":
			if items, ok := snapshot["Items"].([]any); ok {
				details = append(details, approvalDetailRow{Label: "在庫戻し対象", Value: fmt.Sprintf("%d点（BOX振分を含む）", len(items))})
			}
		case "stocktake_line":
			appendValue("棚卸ID", "stocktake_id")
			appendValue("不一致理由", "difference_reason")
			appendValue("備考", "notes")
		}
		if lines, ok := snapshot["Lines"].([]any); ok {
			details = append(details, approvalDetailRow{Label: "対象明細", Value: fmt.Sprintf("%d点", len(lines))})
		}
	}
	return approvalReviewView{Request: record, Type: approvalTypeLabel(record), Content: content, Details: details}
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (s *Server) createOperationApproval(r *http.Request, user database.User, approvalType, targetType, targetID, actionKey, reason string) (database.ApprovalRequest, error) {
	approval, err := s.store.CreateApprovalRequest(r.Context(), database.CreateApprovalInput{
		OrganizationID: user.OrganizationID, ApprovalType: approvalType, TargetType: targetType,
		TargetID: targetID, ActionKey: actionKey, ApplicantUserID: user.ID,
		RequestReason: reason, ActionPayload: map[string]string{"reason": reason},
	})
	if err != nil {
		return database.ApprovalRequest{}, err
	}
	s.auditTransaction(r, user, "approval_request", approval.ID, "approval.requested", approval, reason)
	return approval, nil
}
