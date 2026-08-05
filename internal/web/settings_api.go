package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiSettings(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	records, err := s.repository.Settings(r.Context(), user.OrganizationID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "settings_unavailable", "設定を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records})
}

func (s *Server) apiSettingUpdate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Value string `json:"value"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	user, _ := currentUser(r.Context())
	record, err := s.repository.UpdateSetting(r.Context(), user.OrganizationID, user.ID, r.PathValue("key"), input.Value)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_setting", "設定値を確認してください。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiAdminAccessCode(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.AdminAccessCode(r.Context(), user.OrganizationID, user.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "admin_access_code_unavailable", "管理者認証コードを取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiAdminAccessCodeRotate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.RotateAdminAccessCode(r.Context(), user.OrganizationID, user.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "admin_access_code_update_failed", "管理者認証コードを更新できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiAdminAccessCodeVerify(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "6桁の管理者認証コードを入力してください。")
		return
	}
	user, _ := currentUser(r.Context())
	valid, err := s.repository.VerifyAdminAccessCode(r.Context(), user.OrganizationID, user.ID, input.Code)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "admin_access_code_verify_failed", "管理者認証コードを確認できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"valid": valid})
}

func (s *Server) apiCompanyInfo(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	record, err := s.repository.CompanyInfo(r.Context(), user.OrganizationID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "company_unavailable", "会社情報を取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiCompanyInfoUpdate(w http.ResponseWriter, r *http.Request) {
	var input persistence.CompanyInfoRecord
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "入力内容を確認してください。")
		return
	}
	user, _ := currentUser(r.Context())
	record, err := s.repository.UpdateCompanyInfo(r.Context(), user.OrganizationID, user.ID, input)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "company_update_failed", "会社情報と振込先を確認してください。")
		return
	}
	after, _ := json.Marshal(record)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: user.OrganizationID, ActorUserID: user.ID,
		TargetType: "organization", TargetID: user.OrganizationID, Action: "company.updated", AfterJSON: string(after),
		Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) apiExchangeRates(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.repository.ExchangeRates(r.Context(), user.OrganizationID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "rates_unavailable", "為替レートを取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records, "total": len(records)})
}

func (s *Server) apiExchangeRateCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Rate       string `json:"rate"`
		Provider   string `json:"provider"`
		ObservedAt string `json:"observedAt"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Rate) == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_rate", "USD/JPYレートを指定してください。")
		return
	}
	var observedAt time.Time
	if strings.TrimSpace(input.ObservedAt) != "" {
		var err error
		observedAt, err = time.Parse(time.RFC3339, input.ObservedAt)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_observed_at", "適用日時はRFC3339形式で指定してください。")
			return
		}
	}
	user, _ := currentUser(r.Context())
	record, err := s.repository.CreateExchangeRate(r.Context(), user.OrganizationID, user.ID, input.Rate, input.Provider, observedAt)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_rate", "USD/JPYレートを確認してください。")
		return
	}
	writeJSON(w, http.StatusCreated, record)
}
