package persistence

import (
	"testing"
	"time"
)

func TestFormatProductCodeUsesDDMMYYAndFourDigitSequence(t *testing.T) {
	date := time.Date(2026, time.August, 29, 0, 0, 0, 0, time.UTC)
	for sequence, want := range map[int]string{
		1:  "2908260001",
		42: "2908260042",
	} {
		if got := formatProductCode(date, sequence); got != want {
			t.Fatalf("formatProductCode(%d) = %q, want %q", sequence, got, want)
		}
	}
}

func TestIsProductCodeForDate(t *testing.T) {
	date := time.Date(2026, time.August, 29, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		code string
		want bool
	}{
		{"2908260001", true},
		{"2908269999", true},
		{"2808260001", false},
		{"2908260000", false},
		{"290826001", false},
		{"29082600A1", false},
	} {
		if got := isProductCodeForDate(test.code, date); got != test.want {
			t.Errorf("isProductCodeForDate(%q) = %v, want %v", test.code, got, test.want)
		}
	}
}
