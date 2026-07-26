package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func (s *Server) approvals(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	records, err := s.store.Approvals(r.Context(), user.OrganizationID)
	if err != nil {
		http.Error(w, "承認案件を取得できませんでした。", http.StatusInternalServerError)
		return
	}
	s.render(w, "approvals", http.StatusOK, pageData{
		Title: "承認管理", Active: "approvals", User: user, Approvals: records,
		CSRF: csrfFromRequest(r), Notice: r.URL.Query().Get("notice"),
	})
}

func (s *Server) approvalApprove(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	approval, err := s.store.Approve(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, r.FormValue("comment"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.auditTransaction(r, user, "approval_request", approval.ID, "approval.approved", approval, "")
	http.Redirect(w, r, "/approvals?notice="+url.QueryEscape("承認し、申請された操作を実行しました。"), http.StatusSeeOther)
}

func (s *Server) approvalReturn(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	comment := strings.TrimSpace(r.FormValue("comment"))
	if err := s.store.ReturnApproval(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, comment); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "approval_request", r.PathValue("id"), "approval.returned",
		map[string]string{"status": "returned"}, comment)
	http.Redirect(w, r, "/approvals?notice="+url.QueryEscape("承認申請を差戻しました。"), http.StatusSeeOther)
}

func (s *Server) approvalReject(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	comment := strings.TrimSpace(r.FormValue("comment"))
	if err := s.store.RejectApproval(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, comment); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	s.auditTransaction(r, user, "approval_request", r.PathValue("id"), "approval.rejected",
		map[string]string{"status": "rejected"}, comment)
	http.Redirect(w, r, "/approvals?notice="+url.QueryEscape("承認申請を却下しました。"), http.StatusSeeOther)
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
