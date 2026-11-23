package web

import (
	"encoding/json"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"net/http"
)

func (s *Server) apiReturnedApprovalDocument(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleWorker {
		writeAPIError(w, http.StatusForbidden, "worker_required", "申請した作業者のみ修正できます。")
		return
	}
	result, err := s.repository.ReturnedApprovalDocument(r.Context(), user.OrganizationID, user.ID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusConflict, "returned_approval_unavailable", persistence.ErrReturnedApprovalState.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) apiApprovalResubmit(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleWorker {
		writeAPIError(w, http.StatusForbidden, "worker_required", "申請した作業者のみ再申請できます。")
		return
	}
	var input persistence.ApprovalResubmitInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	result, err := s.repository.ResubmitApproval(r.Context(), user.OrganizationID, user.ID, r.PathValue("id"), input)
	if err != nil {
		writeAPIError(w, http.StatusConflict, "approval_resubmit_failed", approvalResubmitErrorMessage(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func approvalResubmitErrorMessage(err error) string {
	switch {
	case errors.Is(err, persistence.ErrSaleBuyerLocked):
		return persistence.ErrSaleBuyerLocked.Error()
	case errors.Is(err, persistence.ErrReturnedApprovalState):
		return "申請または伝票が更新されています。入力内容を控えて、最新の通知から開き直してください。"
	case errors.Is(err, persistence.ErrProductConflict):
		return "選択した商品が他の取引先・伝票に紐付いているため再申請できません。追加・変更した商品の管理番号と販売先を確認してください。"
	case errors.Is(err, persistence.ErrProductUnavailable):
		return "選択した商品の現在の状態では再申請できません。在庫状態と管理番号を確認してください。"
	case errors.Is(err, persistence.ErrBuyerNotFound):
		return "販売先が見つかりません。販売先を選び直してください。"
	case errors.Is(err, persistence.ErrExchangeRate):
		return "選択した通貨の為替レートが未設定です。マスタの為替レートを確認してください。"
	default:
		return "修正・再申請できませんでした。入力内容と伝票の状態を確認してください。"
	}
}
