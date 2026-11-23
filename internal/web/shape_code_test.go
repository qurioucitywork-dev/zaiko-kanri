package web

import "testing"

func TestShapeCodesNormalizeToFourDigits(t *testing.T) {
	for _, kind := range []string{"shape", "shapes", "product-shapes"} {
		for input, want := range map[string]string{"1": "0001", " 2 ": "0002", "3": "0003", "0003": "0003", "TYP-001": "TYP-001", "SHP-001": "SHP-001"} {
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
