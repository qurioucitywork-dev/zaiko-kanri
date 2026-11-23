package web

import "testing"

func TestMaterialCodesSupportUserNumbersAndExistingCodes(t *testing.T) {
	for _, kind := range []string{"material", "materials"} {
		for _, code := range []string{"1", "2", "6", "7", "01", "MAT-001", "M01"} {
			if !masterCodePatterns[kind].MatchString(normalizeMasterCode(kind, code)) {
				t.Errorf("%s rejected %q", kind, code)
			}
		}
		for _, code := range []string{"", "10000", "00001", "-1", "1.5", "１", "SS/Calf", "MAT-", "1<script>"} {
			if masterCodePatterns[kind].MatchString(normalizeMasterCode(kind, code)) {
				t.Errorf("%s accepted invalid %q", kind, code)
			}
		}
	}
}

func TestMaterialCodesNormalizeToFourDigits(t *testing.T) {
	for _, kind := range []string{"material", "materials"} {
		for input, want := range map[string]string{"1": "0001", " 2 ": "0002", "01": "0001", "0001": "0001", "9999": "9999", "MAT-001": "MAT-001"} {
			if got := normalizeMasterCode(kind, input); got != want {
				t.Errorf("%s %q=%q, want %q", kind, input, got, want)
			}
		}
	}
}
