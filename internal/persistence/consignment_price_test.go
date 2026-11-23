package persistence

import (
	"errors"
	"testing"
)

func TestConsignmentInputPrices(t *testing.T) {
	value := func(n int64) *int64 { return &n }
	for _, tc := range []struct {
		name    string
		items   []ConsignmentPriceInput
		invalid bool
	}{
		{"legacy", nil, false},
		{"override", []ConsignmentPriceInput{{" a ", value(12345)}}, false},
		{"zero", []ConsignmentPriceInput{{"A", value(0)}}, false},
		{"missing", []ConsignmentPriceInput{}, true},
		{"missing amount", []ConsignmentPriceInput{{ProductCode: "A"}}, true},
		{"negative", []ConsignmentPriceInput{{"A", value(-1)}}, true},
		{"overflow", []ConsignmentPriceInput{{"A", value(1000000000001)}}, true},
		{"wrong product", []ConsignmentPriceInput{{"B", value(1)}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := consignmentInputPrices([]string{"A"}, tc.items)
			if tc.invalid {
				if !errors.Is(err, ErrConsignmentPrice) {
					t.Fatalf("expected price error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.items != nil && got["A"] != *tc.items[0].SalePriceUSDMinor {
				t.Fatalf("price changed: %v", got)
			}
		})
	}
	if _, err := consignmentInputPrices([]string{"A", "B"}, []ConsignmentPriceInput{{"A", value(1)}, {"A", value(2)}}); !errors.Is(err, ErrConsignmentPrice) {
		t.Fatal("duplicate price accepted")
	}
}
