package web

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

//go:embed react-dist
var reactAssets embed.FS

var legacyReactPages = map[string]string{
	"products":          "inventory",
	"products/new":      "purchase",
	"purchases":         "purchase-entry",
	"purchases/new":     "purchase-entry",
	"market-prices":     "market",
	"sales":             "sales-list",
	"sales/new":         "sales",
	"shipments":         "shipping",
	"shipments/new":     "shipping",
	"returns":           "returns",
	"returns/new":       "returns",
	"documents":         "sales-list",
	"boxes":             "box",
	"purchase-requests": "purchase-list",
	"approvals":         "approval",
	"stocktake":         "stocktake",
	"masters":           "master",
	"partners":          "client",
	"users":             "password",
	"settings":          "company",
	"audit":             "performance",
}

func referenceAsset(requested string) (string, bool) {
	requested = strings.Trim(strings.ReplaceAll(requested, "\\", "/"), "/")
	if strings.HasPrefix(requested, "admin-reference/") {
		return path.Join("react-dist", requested), true
	}
	return "", false
}

func referencePage(requested string) (string, bool) {
	requested = strings.Trim(strings.ReplaceAll(requested, "\\", "/"), "/")
	if pageName, ok := legacyReactPages[requested]; ok {
		return pageName, true
	}
	if strings.HasPrefix(requested, "products/") {
		return "inventory", true
	}
	return "", false
}

func serveEmbeddedAsset(w http.ResponseWriter, r *http.Request, assetPath string) bool {
	content, err := reactAssets.ReadFile(assetPath)
	if err != nil {
		return false
	}
	if contentType := mime.TypeByExtension(path.Ext(assetPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if path.Ext(assetPath) == ".html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, path.Base(assetPath), time.Time{}, bytes.NewReader(content))
	return true
}

func (s *Server) reactApp(w http.ResponseWriter, r *http.Request) {
	requested := strings.TrimPrefix(r.URL.Path, "/app/")
	if pageName, ok := referencePage(requested); ok {
		query := r.URL.Query()
		query.Set("page", pageName)
		target := &url.URL{Path: "/app/", RawQuery: query.Encode()}
		http.Redirect(w, r, target.String(), http.StatusTemporaryRedirect)
		return
	}
	if assetPath, ok := referenceAsset(requested); ok && serveEmbeddedAsset(w, r, assetPath) {
		return
	}
	if requested != "" && requested != "." && serveEmbeddedAsset(w, r, path.Join("react-dist", requested)) {
		return
	}
	index, err := fs.ReadFile(reactAssets, "react-dist/index.html")
	if err != nil {
		http.Error(w, "application is not built", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

func redirectToReact(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/app/", http.StatusTemporaryRedirect)
}
