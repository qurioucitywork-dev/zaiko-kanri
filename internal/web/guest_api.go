package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiBoxes(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	boxes, err := s.repository.PublicationBoxes(r.Context(), user.OrganizationID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "boxes_unavailable", "BOX情報を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": boxes, "total": len(boxes)})
}

func (s *Server) apiBoxUpdate(w http.ResponseWriter, r *http.Request) {
	var input persistence.PublicationBoxInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	user, _ := currentUser(r.Context())
	box, err := s.repository.UpdatePublicationBox(r.Context(), user.OrganizationID, r.PathValue("code"), user.ID, input)
	if err != nil {
		status, code, message := http.StatusBadRequest, "box_update_failed", "BOXを更新できませんでした。"
		if errors.Is(err, persistence.ErrBoxNotFound) {
			status, code, message = http.StatusNotFound, "box_not_found", "BOXが見つかりません。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	after, _ := json.Marshal(box)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "publication_box", TargetID: box.ID,
		Action: "publication_box.updated", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, box)
}

func (s *Server) apiGuestCatalog(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleGuest {
		writeAPIError(w, http.StatusForbidden, "guest_required", "ゲストアカウントでログインしてください。")
		return
	}
	items, err := s.repository.GuestCatalog(r.Context(), user.OrganizationID, user.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "catalog_unavailable", "公開商品を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) apiGuestPurchaseRequestCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if user.Role != database.RoleGuest {
		writeAPIError(w, http.StatusForbidden, "guest_required", "ゲストアカウントでログインしてください。")
		return
	}
	var input struct {
		ProductID string `json:"productId"`
		Message   string `json:"message"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.ProductID) == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "商品を指定してください。")
		return
	}
	record, err := s.repository.CreateGuestPurchaseRequest(r.Context(), user.OrganizationID, user.ID, strings.TrimSpace(input.ProductID), input.Message)
	if err != nil {
		status, code, message := http.StatusConflict, "product_unavailable", "この商品は現在購入リクエストできません。"
		if errors.Is(err, persistence.ErrGuestAccountNotFound) {
			status, code, message = http.StatusForbidden, "guest_not_found", "ゲスト情報が見つかりません。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiPurchaseRequests(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	records, err := s.repository.PurchaseRequests(r.Context(), user.OrganizationID, user.ID, user.Role, r.URL.Query().Get("status"))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "requests_unavailable", "購入リクエストを取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiPurchaseRequestReview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Note string `json:"note"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	user, _ := currentUser(r.Context())
	record, err := s.repository.ReviewPurchaseRequest(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, r.PathValue("decision"), input.Note)
	if err != nil {
		writeAPIError(w, http.StatusConflict, "request_review_failed", "購入リクエストを処理できませんでした。商品状態を確認してください。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiNotifications(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.Notifications(r.Context(), user.OrganizationID, user.ID, user.Role, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "notifications_unavailable", "通知を取得できませんでした。")
		return
	}
	unread := 0
	for _, record := range records {
		if record.ReadAt == nil {
			unread++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records), "unread": unread})
}

func (s *Server) apiNotificationRead(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := s.repository.ReadNotification(r.Context(), user.OrganizationID, user.ID, user.Role, r.PathValue("id")); err != nil {
		writeAPIError(w, http.StatusNotFound, "notification_not_found", "通知が見つかりません。")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
