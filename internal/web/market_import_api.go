package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiMarketImportPreview(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "相場表CSV取込はPostgreSQLモードで利用してください。")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_upload", "10MB以下のCSVファイルを指定してください。")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "csv_required", "CSVファイルを指定してください。")
		return
	}
	defer file.Close()
	if extension := filepath.Ext(header.Filename); extension != ".csv" && extension != ".CSV" {
		writeAPIError(w, http.StatusBadRequest, "csv_required", "拡張子.csvのファイルを指定してください。")
		return
	}
	user, _ := currentUser(r.Context())
	batch, err := s.repository.PreviewMarketCSV(r.Context(), user.OrganizationID, user.ID, filepath.Base(header.Filename), file)
	if err != nil {
		status, code := http.StatusBadRequest, "market_import_invalid"
		if !errors.Is(err, persistence.ErrMarketImportHeader) {
			code = "market_import_failed"
		}
		writeAPIError(w, status, code, err.Error())
		return
	}
	after, _ := json.Marshal(batch)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "market_import", TargetID: batch.ID,
		Action: "market_import.previewed", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusCreated, batch)
}

func (s *Server) apiMarketImportDetail(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "相場表CSV取込はPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	batch, err := s.repository.MarketImportBatch(r.Context(), user.OrganizationID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "market_import_not_found", "取込データが見つかりません。")
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

func (s *Server) apiMarketImportCommit(w http.ResponseWriter, r *http.Request) {
	if s.repository.Driver() != "postgres" {
		writeAPIError(w, http.StatusServiceUnavailable, "postgres_required", "相場表CSV取込はPostgreSQLモードで利用してください。")
		return
	}
	user, _ := currentUser(r.Context())
	requireApproval := user.Role == database.RoleWorker
	batch, err := s.repository.CommitMarketImport(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID, requireApproval)
	if err != nil {
		status, code, message := http.StatusConflict, "market_import_state", "現在の状態では取込を確定できません。"
		if errors.Is(err, persistence.ErrMarketImportRows) {
			status, code, message = http.StatusBadRequest, "market_import_has_errors", "エラー行を修正して再取込してください。"
		}
		writeAPIError(w, status, code, message)
		return
	}
	after, _ := json.Marshal(batch)
	action := "market_import.committed"
	if requireApproval {
		action = "market_import.approval_requested"
	}
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{
		OrganizationID: user.OrganizationID, ActorUserID: user.ID, TargetType: "market_import", TargetID: batch.ID,
		Action: action, AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()),
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	writeJSON(w, http.StatusOK, batch)
}
