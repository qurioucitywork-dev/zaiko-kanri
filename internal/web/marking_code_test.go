package web

import "testing"

func TestMarkingCodesNormalizeToFourDigits(t *testing.T) {
	for _, kind := range []string{"marking", "markings"} {
		for input, want := range map[string]string{"1": "0001", " 2 ": "0002", "3": "0003", "0003": "0003", "MRK-001": "MRK-001"} {
			got := normalizeMasterCode(kind, input)
			if got != want || !masterCodePatterns[kind].MatchString(got) {
				t.Errorf("%s %q=%q, want %q", kind, input, got, want)
			}
		}
		for _, input := range []string{"10000", "00001", "-1", "1.5", "１", ""} {
			if masterCodePatterns[kind].MatchString(normalizeMasterCode(kind, input)) {
				t.Errorf("accepted %q", input)
			}
		}
	}
}
