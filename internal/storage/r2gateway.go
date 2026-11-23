package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
)

// r2GatewayStore accesses the private R2 binding through an authenticated Worker.
// The application continues enforcing product-level authorization before reads.
type r2GatewayStore struct {
	baseURL string
	token   string
	client  *http.Client
}

func newR2GatewayStore(cfg config.Config) (*r2GatewayStore, error) {
	if err := cfg.ValidateR2Gateway(); err != nil {
		return nil, err
	}
	return &r2GatewayStore{
		baseURL: strings.TrimRight(cfg.R2GatewayURL, "/"),
		token:   cfg.R2GatewayToken,
		client: &http.Client{
			Timeout: 2 * time.Minute,
			// Never forward a storage credential across redirects.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// R2 is S3-compatible object storage; retain the persisted driver vocabulary.
func (s *r2GatewayStore) Driver() string { return "s3" }

func (s *r2GatewayStore) objectURL(key string) (string, error) {
	if key == "" || len(key) > 1024 || strings.ContainsRune(key, '\\') {
		return "", fmt.Errorf("invalid object key")
	}
	for _, r := range key {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("invalid object key")
		}
	}
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid object key")
		}
		segments[i] = url.PathEscape(segment)
	}
	return s.baseURL + "/" + strings.Join(segments, "/"), nil
}

func (s *r2GatewayStore) request(ctx context.Context, method, key, contentType string, body io.Reader) (*http.Response, error) {
	target, err := s.objectURL(key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("create R2 request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("R2 gateway request failed: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("R2 object missing: %w", os.ErrNotExist)
		}
		// Error bodies may contain upstream details; never copy them into logs.
		return nil, fmt.Errorf("R2 gateway returned HTTP %d", response.StatusCode)
	}
	return response, nil
}

func (s *r2GatewayStore) Put(ctx context.Context, key, contentType string, body io.Reader) (int64, error) {
	response, err := s.request(ctx, http.MethodPut, key, contentType, body)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	var result struct {
		Size *int64 `json:"size"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&result); err != nil {
		return 0, fmt.Errorf("invalid R2 upload response: %w", err)
	}
	if result.Size == nil || *result.Size < 0 {
		return 0, fmt.Errorf("invalid R2 upload size")
	}
	return *result.Size, nil
}

func (s *r2GatewayStore) Get(ctx context.Context, key string) (Object, error) {
	response, err := s.request(ctx, http.MethodGet, key, "", nil)
	if err != nil {
		return Object{}, err
	}
	return Object{Body: response.Body, ContentType: response.Header.Get("Content-Type")}, nil
}

func (s *r2GatewayStore) Delete(ctx context.Context, key string) error {
	response, err := s.request(ctx, http.MethodDelete, key, "", nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return nil
}
