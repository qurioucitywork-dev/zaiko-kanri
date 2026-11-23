package web

import (
	"encoding/json"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"net/http"
)

func (s *Server) apiGuestInquiryCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleGuest {
		writeAPIError(w, http.StatusForbidden, "guest_required", "ゲストアカウントでログインしてください。")
		return
	}
	var input persistence.GuestInquiryInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	result, err := s.repository.CreateGuestInquiry(r.Context(), user.OrganizationID, user.ID, input)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "inquiry_failed", "お問い合わせを送信できませんでした。入力内容を確認し、もう一度お試しください。")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": result.ID, "createdAt": result.CreatedAt})
}
func (s *Server) apiGuestInquiryDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAPIAdmin(w, r, "お問い合わせの確認")
	if !ok {
		return
	}
	result, err := s.repository.GuestInquiry(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "inquiry_not_found", "お問い合わせが見つかりません。")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
