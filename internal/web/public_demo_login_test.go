package web

import (
	"html"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
)

func TestPublicDemoLoginOptInAndEscaping(t *testing.T) {
	password := `test-password-with-"<&'>-characters`
	file := filepath.Join(t.TempDir(), "password.txt")
	if err := os.WriteFile(file, []byte(password+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		s := &Server{cfg: config.Config{PublicDemoLogin: enabled, PublicDemoPasswordFile: file}}
		if err := s.configurePublicDemoLogin(); err != nil {
			t.Fatal(err)
		}
		for _, origin := range []string{"https://first.example", "https://replacement.example"} {
			w := httptest.NewRecorder()
			s.reactApp(w, httptest.NewRequest("GET", origin+"/app/admin-reference/index.html", nil))
			body := w.Body.String()
			if w.Code != 200 {
				t.Fatalf("status = %d", w.Code)
			}
			if enabled {
				if !strings.Contains(body, `id="admin-pw" value="`+html.EscapeString(password)+`"`) {
					t.Fatal("escaped password missing")
				}
				if strings.Contains(body, password) {
					t.Fatal("unescaped password exposed as markup")
				}
				if !strings.Contains(body, "公開テスト環境") || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("test notice or no-store missing")
				}
			} else if !strings.Contains(body, `id="admin-pw" value=""`) || strings.Contains(body, html.EscapeString(password)) {
				t.Fatal("disabled mode published a credential")
			}
		}
		w := httptest.NewRecorder()
		s.reactApp(w, httptest.NewRequest("GET", "/app/admin-reference/app.html", nil))
		if strings.Contains(w.Body.String(), html.EscapeString(password)) {
			t.Fatal("credential injected outside login page")
		}
	}
}

func TestPublicDemoLoginMissingPasswordFailsClosed(t *testing.T) {
	s := &Server{cfg: config.Config{PublicDemoLogin: true, PublicDemoPasswordFile: filepath.Join(t.TempDir(), "missing")}}
	if s.configurePublicDemoLogin() == nil {
		t.Fatal("missing password accepted")
	}
	w := httptest.NewRecorder()
	s.reactApp(w, httptest.NewRequest("GET", "/app/admin-reference/index.html", nil))
	if w.Code != 503 {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	s.cfg.PublicDemoLogin = false
	if err := s.configurePublicDemoLogin(); err != nil {
		t.Fatal("disabled mode should not read password file")
	}
}
