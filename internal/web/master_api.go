package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

var masterCodePatterns = map[string]*regexp.Regexp{
	"brand": regexp.MustCompile(`^BRD-[0-9]{3,}$`), "brands": regexp.MustCompile(`^BRD-[0-9]{3,}$`),
	"material": regexp.MustCompile(`^M[0-9]{2,}$`), "materials": regexp.MustCompile(`^M[0-9]{2,}$`),
	"movement": regexp.MustCompile(`^D[0-9]{2,}$`), "movements": regexp.MustCompile(`^D[0-9]{2,}$`),
	"condition": regexp.MustCompile(`^C[0-9]{2,}$`), "conditions": regexp.MustCompile(`^C[0-9]{2,}$`),
	"accessory": regexp.MustCompile(`^ACC-[0-9]{3,}$`), "accessories": regexp.MustCompile(`^ACC-[0-9]{3,}$`),
}

func (s *Server) apiMasterItems(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "マスタAPIはPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	kind := strings.ToLower(r.PathValue("kind"))
	includeInactive := r.URL.Query().Get("includeInactive") == "true" && user.Role == database.RoleAdmin
	items, err := s.repository.MasterItems(r.Context(), user.OrganizationID, kind, includeInactive)
	if errors.Is(err, persistence.ErrUnsupportedMaster) {
		writeAPIError(w, http.StatusNotFound, "master_not_found", "指定されたマスタ種別はありません。")
		return
	}
	if err != nil {
		s.log.Error("load masters", "error", err, "kind", kind, "request_id", requestID(r.Context()))
		writeAPIError(w, http.StatusInternalServerError, "masters_unavailable", "マスタを取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "items": items})
}

func (s *Server) apiMasterCreate(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "マスタAPIはPostgreSQLモードで利用してください。")
		return
	}
	kind := strings.ToLower(r.PathValue("kind"))
	pattern, ok := masterCodePatterns[kind]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "master_not_found", "指定されたマスタ種別はありません。")
		return
	}
	var input struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		SortOrder int    `json:"sortOrder"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	input.Code = strings.ToUpper(strings.TrimSpace(input.Code))
	input.Name = strings.TrimSpace(input.Name)
	if !pattern.MatchString(input.Code) || input.Name == "" || len([]rune(input.Name)) > 200 {
		writeAPIError(w, http.StatusBadRequest, "invalid_master", "コードまたは名称の形式が正しくありません。")
		return
	}
	user, _ := currentUser(r.Context())
	item, err := s.repository.CreateMasterItem(r.Context(), user.OrganizationID, user.ID, kind, input.Code, input.Name, input.SortOrder)
	if err != nil {
		s.log.Error("create master", "error", err, "kind", kind, "request_id", requestID(r.Context()))
		writeAPIError(w, http.StatusConflict, "master_conflict", "同じコードが既に登録されています。")
		return
	}
	after, _ := json.Marshal(item)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "master." + kind,
		TargetID: item.ID, Action: "master.created", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) apiMasterUpdate(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "マスタAPIはPostgreSQLモードで利用してください。")
		return
	}
	kind := strings.ToLower(r.PathValue("kind"))
	if _, ok := masterCodePatterns[kind]; !ok {
		writeAPIError(w, http.StatusNotFound, "master_not_found", "指定されたマスタ種別はありません。")
		return
	}
	var input struct {
		Name      *string `json:"name"`
		IsActive  *bool   `json:"isActive"`
		SortOrder *int    `json:"sortOrder"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" || len([]rune(trimmed)) > 200 {
			writeAPIError(w, http.StatusBadRequest, "invalid_master", "名称の形式が正しくありません。")
			return
		}
		input.Name = &trimmed
	}
	if input.Name == nil && input.IsActive == nil && input.SortOrder == nil {
		writeAPIError(w, http.StatusBadRequest, "empty_update", "変更項目を指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	before, err := s.repository.MasterItem(r.Context(), user.OrganizationID, kind, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "master_item_not_found", "マスタ項目が見つかりません。")
		return
	}
	item, err := s.repository.UpdateMasterItem(r.Context(), user.OrganizationID, user.ID, kind, r.PathValue("id"), input.Name, input.IsActive, input.SortOrder)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "master_update_failed", "マスタを更新できませんでした。")
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(item)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "master." + kind,
		TargetID: item.ID, Action: "master.updated", BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON),
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, item)
}
