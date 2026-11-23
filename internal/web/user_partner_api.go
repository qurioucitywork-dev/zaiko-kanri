package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiUsers(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "利用者管理はPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	records, err := s.repository.Users(r.Context(), user.OrganizationID, r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "users_unavailable", "利用者情報を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

// apiPurchaseStaff exposes only the fields required by purchase and product
// entry screens. Workers can use this endpoint without gaining access to login
// IDs, email addresses, guest accounts, or other password-management data.
func (s *Server) apiPurchaseStaff(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "仕入担当者はPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	records, err := s.repository.PurchaseStaff(r.Context(), user.OrganizationID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "purchase_staff_unavailable", "バイヤーを取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiUserCreate(w http.ResponseWriter, r *http.Request) {
	var input persistence.UserCreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	actor, _ := currentUser(r.Context())
	record, err := s.repository.CreateUser(r.Context(), actor.OrganizationID, actor.ID, input)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, persistence.ErrUserInvalid) {
			status = http.StatusBadRequest
		}
		writeAPIError(w, status, "user_create_failed", "ログインID、メール、12文字以上のパスワードを確認してください。")
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: actor.OrganizationID, ActorUserID: actor.ID,
		TargetType: "user", TargetID: record.ID, Action: "user.created", AfterJSON: string(after), Result: "success",
		RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiUserUpdate(w http.ResponseWriter, r *http.Request) {
	var input persistence.UserUpdateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	actor, _ := currentUser(r.Context())
	before, err := s.repository.User(r.Context(), actor.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "user_not_found", "利用者が見つかりません。")
		return
	}
	record, err := s.repository.UpdateUser(r.Context(), actor.OrganizationID, r.PathValue("id"), actor.ID, input)
	if err != nil {
		status := http.StatusBadRequest
		code := "user_update_failed"
		if errors.Is(err, persistence.ErrLastAdministrator) {
			status, code = http.StatusConflict, "last_administrator"
		}
		writeAPIError(w, status, code, "利用者情報を更新できませんでした。ログインIDの重複・形式と、変更するパスワードの文字数を確認してください。")
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: actor.OrganizationID, ActorUserID: actor.ID,
		TargetType: "user", TargetID: record.ID, Action: "user.updated", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON),
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiUserPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	actor, _ := currentUser(r.Context())
	if err := s.repository.ChangeUserPassword(r.Context(), actor.OrganizationID, r.PathValue("id"), input.Password); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, persistence.ErrUserNotFound) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, "password_change_failed", "作業者・管理者は12文字以上、ゲストは8文字以上、いずれも72バイト以内で入力してください。")
		return
	}
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: actor.OrganizationID, ActorUserID: actor.ID,
		TargetType: "user", TargetID: r.PathValue("id"), Action: "user.password_changed", Result: "success",
		RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiPartners(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	records, err := s.repository.Partners(r.Context(), user.OrganizationID, r.URL.Query().Get("includeInactive") == "true" && (user.Role == database.RoleAdmin || hasAdminPageGrant(r)))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "partners_unavailable", "取引先情報を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiPartnerCreate(w http.ResponseWriter, r *http.Request) {
	var input persistence.PartnerCreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	actor, _ := currentUser(r.Context())
	record, err := s.repository.CreatePartner(r.Context(), actor.OrganizationID, actor.ID, input)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, persistence.ErrAuctionFee) {
			writeAPIError(w, http.StatusBadRequest, "invalid_auction_fee", "手数料の料率・固定額・価格帯を確認してください。価格帯は0円から上限なしまで重複や空白なく設定してください。")
			return
		}
		if errors.Is(err, persistence.ErrPartnerInvalid) {
			status = http.StatusBadRequest
		}
		writeAPIError(w, status, "partner_create_failed", "会社情報または取引区分を確認してください。")
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: actor.OrganizationID, ActorUserID: actor.ID,
		TargetType: "business_partner", TargetID: record.ID, Action: "partner.created", AfterJSON: string(after), Result: "success",
		RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) apiPartnerUpdate(w http.ResponseWriter, r *http.Request) {
	var input persistence.PartnerUpdateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	actor, _ := currentUser(r.Context())
	before, err := s.repository.Partner(r.Context(), actor.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "partner_not_found", "取引先が見つかりません。")
		return
	}
	record, err := s.repository.UpdatePartner(r.Context(), actor.OrganizationID, r.PathValue("id"), actor.ID, input)
	if err != nil {
		if errors.Is(err, persistence.ErrAuctionFee) {
			writeAPIError(w, http.StatusBadRequest, "invalid_auction_fee", "手数料の料率・固定額・価格帯を確認してください。価格帯は0円から上限なしまで重複や空白なく設定してください。")
			return
		}
		writeAPIError(w, http.StatusBadRequest, "partner_update_failed", "会社情報または取引区分を確認してください。")
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: actor.OrganizationID, ActorUserID: actor.ID,
		TargetType: "business_partner", TargetID: record.ID, Action: "partner.updated", BeforeJSON: string(beforeJSON),
		AfterJSON: string(afterJSON), Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusOK, record)
}
