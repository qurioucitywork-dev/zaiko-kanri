package web

import (
	"bytes"
	"errors"
	"html"
	"net/http"
	"os"
	"strings"
)

// This opt-in test mode intentionally publishes the administrator credential.
// The password stays out of source control and is loaded from a local file.
func (s *Server) configurePublicDemoLogin() error {
	if !s.cfg.PublicDemoLogin {
		return nil
	}
	raw, err := os.ReadFile(s.cfg.PublicDemoPasswordFile)
	if err != nil {
		return errors.New("cannot read public demo login password file")
	}
	password := strings.TrimSpace(string(raw))
	if len(password) < 20 || len(password) > 72 {
		return errors.New("public demo login password must be 20 to 72 bytes")
	}
	s.publicDemoPassword = password
	return nil
}

func (s *Server) servePublicDemoLogin(w http.ResponseWriter, r *http.Request) bool {
	if !s.cfg.PublicDemoLogin {
		return false
	}
	content, err := reactAssets.ReadFile("react-dist/admin-reference/index.html")
	marker := []byte(`id="admin-pw" value=""`)
	if err != nil || s.publicDemoPassword == "" || bytes.Count(content, marker) != 1 {
		http.Error(w, "public demo login is unavailable", http.StatusServiceUnavailable)
		return true
	}
	content = bytes.Replace(content, marker, []byte(`id="admin-pw" value="`+html.EscapeString(s.publicDemoPassword)+`"`), 1)
	content = bytes.Replace(content, []byte("登録済みアカウントでログインしてください"), []byte("公開テスト環境：誰でも管理者としてログインできます"), 1)
	content = bytes.Replace(content, []byte("この環境で発行されたログイン情報を使用してください。"), []byte("入力済みです。「管理者としてログイン」を押してください。"), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	_, _ = w.Write(content)
	return true
}
