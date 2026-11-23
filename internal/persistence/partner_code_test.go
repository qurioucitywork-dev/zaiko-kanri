package persistence

import "testing"

func TestPartnerCodeNormalization(t *testing.T) {
	for input, want := range map[string]string{"1": "0001", " 2 ": "0002", "10": "0010", "123": "0123", "0010": "0010", "9999": "9999", "0": "0000"} {
		got := normalizePartnerCode(input)
		if got != want || !partnerCodePattern.MatchString(got) {
			t.Errorf("%q => %q, want %q", input, got, want)
		}
	}
	for _, code := range []string{"", "10000", "00001", "-1", "1.5", "１", "CLI-001", "A001"} {
		if partnerCodePattern.MatchString(normalizePartnerCode(code)) {
			t.Errorf("accepted invalid code %q", code)
		}
	}
}
