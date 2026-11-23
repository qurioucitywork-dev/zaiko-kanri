package web

import (
	"context"
	"net/http"
	"regexp"
	"strings"
)

// No role change: the grant is valid only for this specific API request.
const adminPageGrantKey contextKey = "admin-page-grant"

func hasAdminPageGrant(r *http.Request) bool {
	ok, _ := r.Context().Value(adminPageGrantKey).(bool)
	return ok
}
func adminPageAllows(page, path, method string) bool {
	p := strings.TrimPrefix(path, "/api/v1/")
	switch page {
	case "master", "client":
		return regexp.MustCompile(`^(boxes(?:/[^/]+(?:/products/[^/]+/visibility)?)?|users(?:/[^/]+(?:/password)?)?|company|settings(?:/dashboard\.(?:sales_target_jpy|purchase_budget_jpy))?|masters/[a-z-]+(?:/[^/]+)?|partners(?:/[^/]+)?|purchase-staff(?:/[^/]+)?|guest-logins|exchange-rates)$`).MatchString(p)
	case "box":
		if p == "users" && method == "GET" {
			return true
		}
		return regexp.MustCompile(`^(boxes(?:/[^/]+(?:/products/[^/]+/visibility)?)?|guest-logins|partners(?:/[^/]+)?)$`).MatchString(p)
	case "cost-adjustment", "cost-adjustment-slips":
		return regexp.MustCompile(`^(cost-merges(?:/[^/]+/confirm)?|cost-adjustments/(direct|reset|[^/]+/scan)|products/[^/]+/cost-adjustments/(start|confirm)|products(?:/[^/]+)?|parts(?:/[^/]+)?)$`).MatchString(p)
	case "stocktake":
		return regexp.MustCompile(`^stocktakes/(current|start|[^/]+(?:/(sync|scan|complete|documents/[^/]+/[^/]+/confirm))?)$`).MatchString(p)
	case "purchase-list":
		return regexp.MustCompile(`^purchase-requests(?:/[^/]+(?:/(approve|reject|decide))?)?$`).MatchString(p)
	case "performance":
		return p == "dashboard" && method == "GET"
	}
	return false
}
func (s *Server) withVerifiedPageAccess(r *http.Request, org, actor string) *http.Request {
	if !adminPageAllows(r.Header.Get("X-Admin-Page"), r.URL.Path, r.Method) {
		return r
	}
	code := r.Header.Get("X-Admin-Page-Code")
	if code == "" {
		return r
	}
	ok, err := s.repository.VerifyAdminAccessCode(r.Context(), org, actor, code)
	if err != nil || !ok {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), adminPageGrantKey, true))
}
