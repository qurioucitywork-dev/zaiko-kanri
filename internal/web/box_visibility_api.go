package web

import (
	"encoding/json"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"net/http"
)

func (s *Server) apiBoxProductVisibility(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Visible *bool `json:"visible"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.Visible == nil {
		writeAPIError(w, 400, "invalid_request", "表示・非表示を指定してください。")
		return
	}
	actor, _ := currentUser(r.Context())
	err := s.repository.SetBoxProductVisibility(r.Context(), actor.OrganizationID, r.PathValue("code"), r.PathValue("productCode"), *input.Visible)
	if err != nil {
		status, message := http.StatusConflict, "商品の表示設定を変更できませんでした。画面を更新して確認してください。"
		if errors.Is(err, persistence.ErrBoxVisibilityStatus) {
			message = err.Error()
		}
		if errors.Is(err, persistence.ErrBoxNotFound) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, "box_visibility_failed", message)
		return
	}
	after, _ := json.Marshal(input)
	_ = s.apiWriteAudit(r.Context(), database.AuditEntry{OrganizationID: actor.OrganizationID, ActorUserID: actor.ID, TargetType: "publication_box_product", TargetID: r.PathValue("code") + "/" + r.PathValue("productCode"), Action: "publication_box_product.visibility_changed", AfterJSON: string(after), Result: "success", RequestID: requestID(r.Context()), IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	w.WriteHeader(http.StatusNoContent)
}
