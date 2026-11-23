package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

const readBatchMaxItems = 32
const readBatchMaxBytes = 8 << 20
const readBatchTimeout = 30 * time.Second

// The unexported context key is deliberately unrelated to any HTTP header.
// It only exists while dispatching validated GETs within one authenticated request.
type readBatchContextKey struct{}
type readBatchAuth struct {
	session     database.Session
	permissions map[string]bool
}

func readBatchAuthentication(ctx context.Context) (*readBatchAuth, bool) {
	auth, ok := ctx.Value(readBatchContextKey{}).(*readBatchAuth)
	user, hasUser := currentUser(ctx)
	return auth, ok && auth != nil && hasUser && user.ID == auth.session.User.ID && user.OrganizationID == auth.session.User.OrganizationID && user.Role == auth.session.User.Role
}

type readBatchItem struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}
type readBatchResult struct {
	ID     string          `json:"id"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

var readBatchSafeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var readBatchQueries = map[string]string{
	"/dashboard": "months", "/parts": "", "/products": "q,status,sort,page,pageSize,includeCancelled",
	"/partners": "includeInactive", "/purchase-staff": "", "/exchange-rates": "limit", "/users": "includeInactive",
	"/purchases": "page,pageSize,limit", "/purchases-deleted": "page,pageSize", "/market-prices": "limit",
	"/boxes": "", "/purchase-requests": "status", "/notifications": "limit,unread", "/approvals": "status",
	"/sales": "limit", "/shipments": "limit", "/consignments": "limit", "/returns": "limit",
	"/cost-adjustments": "limit", "/settings": "", "/company": "",
}
var readBatchMasterKinds = map[string]bool{
	"brands": true, "materials": true, "movements": true, "conditions": true, "accessories": true,
	"auctions": true, "belt-materials": true, "dials": true, "product-shapes": true, "markings": true, "part-names": true,
}

// Keep this list limited to the existing JSON reads needed by hydration. Never
// admit mutations, arbitrary URLs, file contents, exports, or authentication APIs.
func readBatchURL(path string) (*url.URL, bool) {
	if len(path) == 0 || len(path) > 4096 || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, false
	}
	parsed, err := url.ParseRequestURI(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" || parsed.RawPath != "" || strings.ContainsAny(parsed.Path, "\\%#") {
		return nil, false
	}
	keys, allowed := readBatchQueries[parsed.Path]
	parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if !allowed && len(parts) == 2 && parts[0] == "masters" && readBatchMasterKinds[parts[1]] {
		allowed = true
	}
	if !allowed && len(parts) == 2 && readBatchSafeID.MatchString(parts[1]) {
		switch parts[0] {
		case "products", "purchases", "purchases-deleted", "sales", "shipments", "consignments", "returns":
			allowed = true
		}
	}
	// This is image metadata only; product-files/{id} binary reads are excluded.
	if !allowed && len(parts) == 3 && parts[0] == "products" && readBatchSafeID.MatchString(parts[1]) && parts[2] == "files" {
		allowed = true
	}
	if !allowed {
		return nil, false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return nil, false
	}
	permitted := map[string]bool{}
	for _, key := range strings.Split(keys, ",") {
		if key != "" {
			permitted[key] = true
		}
	}
	for key, values := range query {
		if !permitted[key] || len(values) != 1 || len(values[0]) > 2048 {
			return nil, false
		}
	}
	parsed.Path = "/api/v1" + parsed.Path
	return parsed, true
}

// Each result has its own buffer; the total response budget is shared atomically
// across at most eight concurrent handlers. A full buffer cannot grow further.
type readBatchWriter struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	budget   *atomic.Int64
	exceeded bool
}

func (w *readBatchWriter) Header() http.Header { return w.header }
func (w *readBatchWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *readBatchWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	for {
		available := w.budget.Load()
		if int64(len(p)) > available {
			w.exceeded = true
			return 0, errors.New("read batch response limit exceeded")
		}
		if w.budget.CompareAndSwap(available, available-int64(len(p))) {
			return w.body.Write(p)
		}
	}
}
func readBatchErrorBody(code, message string) json.RawMessage {
	var payload apiError
	payload.Error.Code, payload.Error.Message = code, message
	encoded, _ := json.Marshal(payload)
	return encoded
}

func (s *Server) apiReadBatch(routes http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeAPIError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "JSON形式で送信してください。")
			return
		}
		// Elevated master/box workflows use their original individual GETs.
		if r.Header.Get("X-Admin-Page") != "" || r.Header.Get("X-Admin-Page-Code") != "" {
			writeAPIError(w, http.StatusBadRequest, "read_batch_not_supported", "この取得は個別に実行してください。")
			return
		}
		var input struct {
			Requests []readBatchItem `json:"requests"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || len(input.Requests) == 0 || len(input.Requests) > readBatchMaxItems {
			writeAPIError(w, http.StatusBadRequest, "invalid_read_batch", "一括取得は1件から32件まで指定してください。")
			return
		}
		if decoder.Decode(new(any)) != io.EOF {
			writeAPIError(w, http.StatusBadRequest, "invalid_read_batch", "入力内容を確認してください。")
			return
		}
		targets := make([]*url.URL, len(input.Requests))
		seen := make(map[string]bool)
		for i, item := range input.Requests {
			target, allowed := readBatchURL(item.Path)
			if !allowed || len(item.ID) > 64 || !readBatchSafeID.MatchString(item.ID) || seen[item.ID] {
				writeAPIError(w, http.StatusBadRequest, "read_batch_not_supported", "一括取得できない項目が含まれています。")
				return
			}
			seen[item.ID], targets[i] = true, target
		}
		session, authenticated := currentSession(r.Context())
		if !authenticated {
			writeAPIError(w, http.StatusUnauthorized, "authentication_required", "ログインが必要です。")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), readBatchTimeout)
		defer cancel()
		permissions, err := s.repository.PermissionDecisions(ctx, session.User)
		if err != nil {
			writeAPIError(w, http.StatusServiceUnavailable, "permissions_unavailable", "権限を確認できませんでした。再試行してください。")
			return
		}
		ctx = context.WithValue(ctx, readBatchContextKey{}, &readBatchAuth{session: session, permissions: permissions})
		results := make([]readBatchResult, len(input.Requests))
		var remaining atomic.Int64
		remaining.Store(readBatchMaxBytes)
		var exceeded atomic.Bool
		slots := make(chan struct{}, 8)
		var tasks sync.WaitGroup
		safeRoutes := s.recoverer(routes)
		for i, item := range input.Requests {
			tasks.Add(1)
			go func(i int, item readBatchItem) {
				defer tasks.Done()
				result := readBatchResult{ID: item.ID, Status: http.StatusGatewayTimeout, Body: readBatchErrorBody("read_batch_timeout", "取得がタイムアウトしました。再試行してください。")}
				defer func() { results[i] = result }()
				select {
				case slots <- struct{}{}:
					defer func() { <-slots }()
				case <-ctx.Done():
					return
				}
				if ctx.Err() != nil {
					return
				}
				inner := r.Clone(ctx)
				inner.Method, inner.URL, inner.RequestURI = http.MethodGet, targets[i], targets[i].RequestURI()
				inner.Body, inner.GetBody, inner.ContentLength = nil, nil, 0
				inner.Header.Del("Content-Type")
				inner.Header.Del("X-CSRF-Token")
				response := &readBatchWriter{header: make(http.Header), budget: &remaining}
				safeRoutes.ServeHTTP(response, inner)
				if response.exceeded {
					exceeded.Store(true)
					return
				}
				if ctx.Err() != nil {
					return
				}
				if response.status == 0 {
					response.status = http.StatusOK
				}
				if !json.Valid(response.body.Bytes()) {
					result.Status, result.Body = http.StatusInternalServerError, readBatchErrorBody("invalid_read_response", "取得できませんでした。再試行してください。")
					return
				}
				result.Status, result.Body = response.status, json.RawMessage(response.body.Bytes())
			}(i, item)
		}
		tasks.Wait()
		if exceeded.Load() {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "batch_response_too_large", "取得量が多いため個別に取得してください。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"responses": results})
	})
}
