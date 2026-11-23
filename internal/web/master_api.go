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

// normalizeMasterCode pads numeric master input without truncating invalid codes.
func normalizeMasterCode(kind, code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if kind == "brand" || kind == "brands" || kind == "material" || kind == "materials" || kind == "movement" || kind == "movements" || kind == "shape" || kind == "shapes" || kind == "product-shapes" || kind == "marking" || kind == "markings" || kind == "belt" || kind == "belts" || kind == "belt-material" || kind == "belt-materials" || kind == "accessory" || kind == "accessories" || kind == "condition" || kind == "conditions" {
		if regexp.MustCompile("^[0-9]{1,4}$").MatchString(code) {
			return strings.Repeat("0", 4-len(code)) + code
		}
	}
	return code
}

var masterCodePatterns = map[string]*regexp.Regexp{
	"brand": regexp.MustCompile(`^[0-9]{4}$`), "brands": regexp.MustCompile(`^[0-9]{4}$`),
	"material": regexp.MustCompile(`^(?:[0-9]{4}|MAT-[0-9]+|M[0-9]{2,})$`), "materials": regexp.MustCompile(`^(?:[0-9]{4}|MAT-[0-9]+|M[0-9]{2,})$`),
	"movement": regexp.MustCompile(`^(?:[0-9]{4}|D[0-9]{2,}|MOV-[0-9]+)$`), "movements": regexp.MustCompile(`^(?:[0-9]{4}|D[0-9]{2,}|MOV-[0-9]+)$`),
	"condition": regexp.MustCompile(`^(?:[0-9]{4}|C[0-9]{2,}|CON-[0-9]+)$`), "conditions": regexp.MustCompile(`^(?:[0-9]{4}|C[0-9]{2,}|CON-[0-9]+)$`),
	"accessory": regexp.MustCompile(`^(?:[0-9]{4}|ACC-[0-9]{3,})$`), "accessories": regexp.MustCompile(`^(?:[0-9]{4}|ACC-[0-9]{3,})$`),
	"auction": regexp.MustCompile(`^AUC-[0-9]{3,}$`), "auctions": regexp.MustCompile(`^AUC-[0-9]{3,}$`),
	"belt": regexp.MustCompile(`^(?:[0-9]{4}|BLT-[0-9]{3,})$`), "belts": regexp.MustCompile(`^(?:[0-9]{4}|BLT-[0-9]{3,})$`),
	"belt-material": regexp.MustCompile(`^(?:[0-9]{4}|BLT-[0-9]{3,})$`), "belt-materials": regexp.MustCompile(`^(?:[0-9]{4}|BLT-[0-9]{3,})$`),
	"dial": regexp.MustCompile(`^DIA-[0-9]{3,}$`), "dials": regexp.MustCompile(`^DIA-[0-9]{3,}$`),
	"shape": regexp.MustCompile(`^(?:[0-9]{4}|SHP-[0-9]{3,}|TYP-[0-9]+)$`), "shapes": regexp.MustCompile(`^(?:[0-9]{4}|SHP-[0-9]{3,}|TYP-[0-9]+)$`), "product-shapes": regexp.MustCompile(`^(?:[0-9]{4}|SHP-[0-9]{3,}|TYP-[0-9]+)$`),
	"marking": regexp.MustCompile(`^(?:[0-9]{4}|MRK-[0-9]{3,})$`), "markings": regexp.MustCompile(`^(?:[0-9]{4}|MRK-[0-9]{3,})$`),
	"part-name": regexp.MustCompile(`^PRT-[0-9]{3,}$`), "part-names": regexp.MustCompile(`^PRT-[0-9]{3,}$`),
}

func (s *Server) apiMasterItems(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "マスタAPIはPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	kind := strings.ToLower(r.PathValue("kind"))
	includeInactive := r.URL.Query().Get("includeInactive") == "true" && (user.Role == database.RoleAdmin || hasAdminPageGrant(r))
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
	response := map[string]any{"kind": kind, "items": items}
	if kind == "accessory" || kind == "accessories" {
		all, loadErr := s.repository.MasterItems(r.Context(), user.OrganizationID, kind, true)
		if loadErr != nil {
			writeAPIError(w, http.StatusInternalServerError, "masters_unavailable", "マスタを取得できませんでした。")
			return
		}
		references := make([]persistence.MasterItem, 0)
		for _, item := range all {
			if item.IsLegacy {
				references = append(references, item)
			}
		}
		response["referenceItems"] = references
	}
	writeJSON(w, http.StatusOK, response)
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
		persistence.MasterDisplayDetailsInput
		Code       string `json:"code"`
		Name       string `json:"name"`
		Meaning    string `json:"meaning"`
		Reading    string `json:"reading"`
		Symbol     string `json:"symbol"`
		Supplement string `json:"supplement"`
		SortOrder  int    `json:"sortOrder"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	if !input.MasterDisplayDetailsInput.Valid() {
		writeAPIError(w, http.StatusBadRequest, "invalid_master", "詳細は200文字以内で指定してください。")
		return
	}
	input.Code = normalizeMasterCode(kind, input.Code)
	input.Reading = strings.TrimSpace(input.Reading)
	input.Symbol = strings.TrimSpace(input.Symbol)
	input.Supplement = strings.TrimSpace(input.Supplement)
	input.Name = strings.TrimSpace(input.Name)
	if !pattern.MatchString(input.Code) || input.Name == "" || len([]rune(input.Name)) > 200 || len([]rune(input.Reading)) > 200 || len([]rune(input.Symbol)) > 200 || len([]rune(input.Supplement)) > 200 {
		writeAPIError(w, http.StatusBadRequest, "invalid_master", "コードまたは名称の形式が正しくありません。")
		return
	}
	user, _ := currentUser(r.Context())
	item, err := s.repository.CreateMasterItem(r.Context(), user.OrganizationID, user.ID, kind, input.Code, input.Name, input.Meaning, input.Reading, input.SortOrder, input.Symbol, input.Supplement, input.MasterDisplayDetailsInput)
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
		persistence.MasterDisplayDetailsInput
		Name       *string `json:"name"`
		Meaning    *string `json:"meaning"`
		Reading    *string `json:"reading"`
		Symbol     *string `json:"symbol"`
		Supplement *string `json:"supplement"`
		IsActive   *bool   `json:"isActive"`
		SortOrder  *int    `json:"sortOrder"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	if !input.MasterDisplayDetailsInput.Valid() {
		writeAPIError(w, http.StatusBadRequest, "invalid_master", "詳細は200文字以内で指定してください。")
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
	if input.Meaning != nil {
		trimmed := strings.TrimSpace(*input.Meaning)
		if len([]rune(trimmed)) > 200 {
			writeAPIError(w, http.StatusBadRequest, "invalid_master", "意味は200文字以内で指定してください。")
			return
		}
		input.Meaning = &trimmed
	}
	if input.Reading != nil {
		trimmed := strings.TrimSpace(*input.Reading)
		if len([]rune(trimmed)) > 200 {
			writeAPIError(w, http.StatusBadRequest, "invalid_master", "読みは200文字以内で指定してください。")
			return
		}
		input.Reading = &trimmed
	}
	if input.Symbol != nil {
		trimmed := strings.TrimSpace(*input.Symbol)
		if len([]rune(trimmed)) > 200 {
			writeAPIError(w, http.StatusBadRequest, "invalid_master", "記号は200文字以内で指定してください。")
			return
		}
		input.Symbol = &trimmed
	}
	if input.Supplement != nil {
		trimmed := strings.TrimSpace(*input.Supplement)
		if len([]rune(trimmed)) > 200 {
			writeAPIError(w, http.StatusBadRequest, "invalid_master", "補足は200文字以内で指定してください。")
			return
		}
		input.Supplement = &trimmed
	}
	if input.MasterDisplayDetailsInput.Empty() && input.Supplement == nil && input.Symbol == nil && input.Name == nil && input.Meaning == nil && input.Reading == nil && input.IsActive == nil && input.SortOrder == nil {
		writeAPIError(w, http.StatusBadRequest, "empty_update", "変更項目を指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	before, err := s.repository.MasterItem(r.Context(), user.OrganizationID, kind, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "master_item_not_found", "マスタ項目が見つかりません。")
		return
	}
	item, err := s.repository.UpdateMasterItem(r.Context(), user.OrganizationID, user.ID, kind, r.PathValue("id"), input.Name, input.Meaning, input.Reading, input.IsActive, input.SortOrder, input.Symbol, input.Supplement, input.MasterDisplayDetailsInput)
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
