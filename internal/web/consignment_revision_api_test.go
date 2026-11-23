package web

import (
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConsignmentRevisionRejectsWorkerAndUnknownProductEdits(t *testing.T) {
	for _, tc := range []struct {
		name, role, body string
		want             int
	}{
		{"worker", database.RoleWorker, `{"reason":"change"}`, http.StatusForbidden},
		{"product identity is immutable", database.RoleAdmin, `{"reason":"change","lines":[{"id":"line","unitPriceMinor":100,"productCode":"other"}]}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPatch, "/api/v1/consignments/id/revision", strings.NewReader(tc.body))
			r = r.WithContext(withUser(r.Context(), database.User{ID: "actor", OrganizationID: "org", Role: tc.role}))
			w := httptest.NewRecorder()
			(&Server{}).apiConsignmentRevision(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
