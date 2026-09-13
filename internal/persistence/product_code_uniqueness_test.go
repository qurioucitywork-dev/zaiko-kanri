package persistence

import (
	"errors"
	"testing"
)

func TestIsProductCodeUniqueViolation(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "normalized index", err: errors.New("duplicate key violates unique constraint ux_products_organization_product_code_normalized"), want: true},
		{name: "legacy index", err: errors.New("duplicate key violates unique constraint products_org_product_code_key"), want: true},
		{name: "unrelated", err: errors.New("connection closed"), want: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := isProductCodeUniqueViolation(testCase.err); got != testCase.want {
				t.Fatalf("isProductCodeUniqueViolation(%v) = %v, want %v", testCase.err, got, testCase.want)
			}
		})
	}
}
