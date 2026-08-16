package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
)

func (s *Server) apiCurrentStocktake(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	session, err := s.repository.CurrentStocktake(r.Context(), user.OrganizationID)
	if errors.Is(err, persistence.ErrStocktakeNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"session": nil})
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "stocktake_unavailable", "棚卸途中データを取得できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": session})
}

func (s *Server) apiStartStocktake(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	session, err := s.repository.StartStocktake(r.Context(), user.OrganizationID, user.ID)
	if err != nil {
		s.log.Error("start stocktake", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "stocktake_start_failed", "棚卸を開始できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": session})
}

func (s *Server) apiSyncStocktake(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	session, added, err := s.repository.SyncStocktake(r.Context(), user.OrganizationID, r.PathValue("id"))
	if errors.Is(err, persistence.ErrStocktakeNotFound) {
		writeAPIError(w, http.StatusNotFound, "stocktake_not_found", "進行中の棚卸がありません。")
		return
	}
	if err != nil {
		s.log.Error("sync stocktake", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "stocktake_sync_failed", "最新の在庫を棚卸へ反映できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": session, "added": added})
}

func (s *Server) apiScanStocktake(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	var input struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&input); err != nil || strings.TrimSpace(input.Code) == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_stocktake_code", "管理番号を入力してください。")
		return
	}
	session, result, err := s.repository.ScanStocktake(r.Context(), user.OrganizationID, r.PathValue("id"), input.Code)
	if errors.Is(err, persistence.ErrStocktakeNotFound) {
		writeAPIError(w, http.StatusNotFound, "stocktake_not_found", "進行中の棚卸がありません。")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "stocktake_scan_failed", "棚卸結果を保存できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": session, "result": result})
}

func (s *Server) apiSaveStocktake(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	var input struct {
		Lines []struct{ ID, Reason, Note string } `json:"lines"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_stocktake", "入力内容を確認してください。")
		return
	}
	updates := make(map[string]struct{ Reason, Note string }, len(input.Lines))
	for _, line := range input.Lines {
		if strings.TrimSpace(line.ID) != "" {
			updates[line.ID] = struct{ Reason, Note string }{line.Reason, line.Note}
		}
	}
	session, err := s.repository.SaveStocktake(r.Context(), user.OrganizationID, r.PathValue("id"), updates)
	if errors.Is(err, persistence.ErrStocktakeNotFound) {
		writeAPIError(w, http.StatusNotFound, "stocktake_not_found", "進行中の棚卸がありません。")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "stocktake_save_failed", "棚卸途中データを保存できませんでした。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": session})
}

func (s *Server) apiCompleteStocktake(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	session, err := s.repository.CompleteStocktake(r.Context(), user.OrganizationID, r.PathValue("id"), user.ID)
	if errors.Is(err, persistence.ErrStocktakeNotFound) {
		writeAPIError(w, http.StatusNotFound, "stocktake_not_found", "進行中の棚卸がありません。")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusConflict, "stocktake_unresolved", "理由未入力の不一致があります。不一致リストを確認してください。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": session})
}
