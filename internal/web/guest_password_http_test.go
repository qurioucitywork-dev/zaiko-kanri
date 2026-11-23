package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
	"time"
)

// Run only against the disposable synthetic-data application, never the live port.
func TestGuestPasswordHTTPIsolated(t *testing.T) {
	base := os.Getenv("ZAIKO_GUEST_HTTP_TEST_URL")
	if base == "" {
		t.Skip("isolated HTTP application required")
	}
	if base != "http://127.0.0.1:18089" {
		t.Fatal("unexpected test endpoint")
	}
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, Timeout: 10 * time.Second}
	}
	request := func(client *http.Client, method, path, csrf string, body any, want int) []byte {
		t.Helper()
		payload, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, base+"/api/v1"+path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", csrf)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("%s status=%d want=%d response=%s", path, resp.StatusCode, want, data)
		}
		return data
	}
	login := func(client *http.Client, user, password string, want int) string {
		data := request(client, "POST", "/auth/login", "", map[string]string{"username": user, "password": password}, want)
		var result struct {
			CSRFToken string `json:"csrfToken"`
		}
		_ = json.Unmarshal(data, &result)
		return result.CSRFToken
	}
	admin := newClient()
	csrf := login(admin, "admin", "guest-test-admin-password", 200)
	input := map[string]any{"guestCode": "G920", "buyerCode": "B920", "displayName": "HTTP Guest", "company": "HTTP Test Company", "email": "http-eight@example.invalid", "password": "test1234", "isActive": true, "address": "Test Address", "invoiceNumber": ""}
	data := request(admin, "POST", "/guest-logins", csrf, input, 200)
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(data, &created)
	if created.ID == "" {
		t.Fatal("missing saved account")
	}
	freshAdmin := newClient()
	login(freshAdmin, "admin", "guest-test-admin-password", 200)
	if !bytes.Contains(request(freshAdmin, "GET", "/users", "", nil, 200), []byte("G920")) {
		t.Fatal("saved account absent in new session")
	}
	guest := newClient()
	login(guest, "G920", "test1234", 200)
	request(guest, "GET", "/guest/catalog", "", nil, 200)
	input["userId"] = created.ID
	input["password"] = "next5678"
	request(admin, "POST", "/guest-logins", csrf, input, 200)
	login(newClient(), "G920", "test1234", 401)
	login(newClient(), "G920", "next5678", 200)
	request(admin, "POST", "/users/"+created.ID+"/password", csrf, map[string]string{"password": "last9012"}, 204)
	login(newClient(), "G920", "next5678", 401)
	login(newClient(), "G920", "last9012", 200)
	// Field-specific errors must preserve the already saved name/password.
	input["displayName"] = "Must not persist"
	for _, tc := range []struct{ password, invoice, message string }{
		{"short", "", "8文字"}, {strings.Repeat("x", 73), "", "72バイト"}, {"test1234", "invalid", "インボイス"},
	} {
		input["password"] = tc.password
		input["invoiceNumber"] = tc.invoice
		if !strings.Contains(string(request(admin, "POST", "/guest-logins", csrf, input, 400)), tc.message) {
			t.Fatal("field-specific error missing")
		}
	}
	login(newClient(), "G920", "last9012", 200)
	input["userId"] = ""
	input["guestCode"] = "G921"
	input["buyerCode"] = "B921"
	input["password"] = "test1234"
	input["invoiceNumber"] = ""
	if !strings.Contains(string(request(admin, "POST", "/guest-logins", csrf, input, 400)), "メールアドレス") {
		t.Fatal("duplicate email error missing")
	}
	users := request(admin, "GET", "/users", "", nil, 200)
	if bytes.Contains(users, []byte("G921")) || bytes.Contains(users, []byte("Must not persist")) || bytes.Contains(users, []byte("last9012")) {
		t.Fatal("partial save or plaintext password exposed")
	}
}
