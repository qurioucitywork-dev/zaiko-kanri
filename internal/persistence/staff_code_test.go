package persistence

import "testing"

func TestStaffCodesNormalizeToFourDigits(t *testing.T) {
	for input, want := range map[string]string{"1": "0001", " 2 ": "0002", "3": "0003", "0003": "0003", "9999": "9999", "BUY-001": "BUY-001"} {
		got := normalizeStaffCode(input)
		if got != want || !staffCodePattern.MatchString(got) {
			t.Errorf("%q => %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"10000", "00001", "-1", "1.5", "１", ""} {
		if staffCodePattern.MatchString(normalizeStaffCode(input)) {
			t.Errorf("accepted %q", input)
		}
	}
}
