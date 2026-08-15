package persistence

import "testing"

func TestConvertShipmentUSDToJPYRoundUpToThousand(t *testing.T) {
	tests := []struct {
		amountUSD int64
		rate      int64
		scale     int64
		want      int64
	}{
		{amountUSD: 0, rate: 15525, scale: 100, want: 0},
		{amountUSD: 1, rate: 1, scale: 1000, want: 1000},
		{amountUSD: 1000, rate: 1, scale: 1, want: 1000},
		{amountUSD: 1001, rate: 15500, scale: 100, want: 156000},
		{amountUSD: 10000, rate: 15525, scale: 100, want: 1553000},
	}
	for _, test := range tests {
		if got := convertShipmentUSDToJPYRoundUpToThousand(test.amountUSD, test.rate, test.scale); got != test.want {
			t.Fatalf("convertShipmentUSDToJPYRoundUpToThousand(%d,%d,%d)=%d, want %d",
				test.amountUSD, test.rate, test.scale, got, test.want)
		}
	}
}
