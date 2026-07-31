package web

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestBaseLayoutUsesMockTopbarForEveryApplicationSection(t *testing.T) {
	app, _ := testServer(t)
	activeSections := []string{
		"dashboard",
		"products",
		"slips",
		"market",
		"purchases",
		"product-new",
		"shipments",
		"sales",
		"returns",
		"requests",
		"masters",
		"guest-management",
		"performance",
		"stocktakes",
		"approvals",
		"users",
		"settings",
		"audit",
	}

	for _, active := range activeSections {
		t.Run(active, func(t *testing.T) {
			var rendered bytes.Buffer
			data := pageData{
				Title:       "レイアウト確認",
				Active:      active,
				User:        database.User{ID: "usr_admin", DisplayName: "管理者", Role: database.RoleAdmin},
				AlertCount:  5,
				PreviewMode: true,
			}
			if err := app.templates["dashboard"].ExecuteTemplate(&rendered, "base", data); err != nil {
				t.Fatal(err)
			}
			body := rendered.String()
			for _, expected := range []string{
				`class="date"`,
				`class="notification-button"`,
				`class="button primary topbar-create"`,
			} {
				if !strings.Contains(body, expected) {
					t.Errorf("active=%q missing common topbar element %q", active, expected)
				}
			}
			for _, forbidden := range []string{
				`<span class="badge phase">`,
				"preview-banner",
				"制作プレビュー",
				`href="/users"`,
				`href="/settings"`,
				`href="/audit"`,
				"利用者・権限",
				"組織設定",
				"監査ログ",
			} {
				if strings.Contains(body, forbidden) {
					t.Errorf("active=%q rendered mock-external layout element %q", active, forbidden)
				}
			}
		})
	}
}
