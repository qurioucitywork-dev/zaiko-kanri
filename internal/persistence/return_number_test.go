package persistence

import "testing"

func TestReturnSlipNumberSeparatesReturnTypes(t *testing.T) {
	for _, tc := range []struct{ operation, want string }{
		{"return", "UH"}, {"purchase_return", "SH"}, {"takeout", "RT"},
	} {
		if got := returnSlipPrefix(tc.operation); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.operation, got, tc.want)
		}
	}
}
