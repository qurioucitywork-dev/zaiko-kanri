package persistence

import (
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestValidJSONObject(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"", "{}"}, {"not-json", "{}"}, {"[]", "{}"}, {`{"status":"ok"}`, `{"status":"ok"}`},
	} {
		if got := validJSONObject(test.input); got != test.want {
			t.Fatalf("validJSONObject(%q)=%q want %q", test.input, got, test.want)
		}
	}
}

func TestPreviewIdentitySeedsFiveWorkersWithStableStaffCodes(t *testing.T) {
	expected := map[string]string{
		"worker":  "STF-001",
		"worker2": "STF-002",
		"worker3": "STF-003",
		"worker4": "STF-004",
		"worker5": "STF-005",
	}
	workers := 0
	usernames := map[string]bool{}
	staffCodes := map[string]bool{}
	for _, seed := range previewIdentitySeeds {
		if usernames[seed.Username] {
			t.Fatalf("duplicate preview username %q", seed.Username)
		}
		if staffCodes[seed.StaffCode] {
			t.Fatalf("duplicate preview staff code %q", seed.StaffCode)
		}
		usernames[seed.Username] = true
		staffCodes[seed.StaffCode] = true
		if seed.Role != database.RoleWorker {
			continue
		}
		workers++
		if seed.StaffCode != expected[seed.Username] {
			t.Fatalf("worker %q staff code=%q want=%q", seed.Username, seed.StaffCode, expected[seed.Username])
		}
	}
	if workers != 5 {
		t.Fatalf("preview worker count=%d want=5", workers)
	}
}
