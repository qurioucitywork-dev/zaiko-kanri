package web

import "testing"

func TestBrandCodesNormalizeToFourDigits(t *testing.T) {
	for _, kind := range []string{"brand", "brands"} {
		for input, want := range map[string]string{"1": "0001", " 2 ": "0002", "10": "0010", "0010": "0010", "123": "0123", "9999": "9999", "0": "0000"} {
			got := normalizeMasterCode(kind, input)
			if got != want || !masterCodePatterns[kind].MatchString(got) {
				t.Errorf("%s %q => %q, want %q", kind, input, got, want)
			}
		}
		for _, input := range []string{"", "10000", "00001", "-1", "1.5", "１", "BRD-001", "1<script>"} {
			if masterCodePatterns[kind].MatchString(normalizeMasterCode(kind, input)) {
				t.Errorf("accepted invalid brand code %q", input)
			}
		}
	}
	if normalizeMasterCode("material", "1") != "0001" {
		t.Error("material codes must also be padded")
	}
}
