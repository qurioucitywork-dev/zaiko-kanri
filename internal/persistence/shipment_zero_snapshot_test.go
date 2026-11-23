package persistence

import (
	"encoding/json"
	"testing"
)

func TestShipmentZeroPriceOnlyFromTrustedSnapshot(t *testing.T) {
	zero, negative := int64(0), int64(-1)
	lines := []ShipmentPriceInput{{ProductCode: "P1", SalePriceUSDMinor: &zero}}
	if _, err := shipmentInputPrices([]string{"P1"}, lines); err == nil {
		t.Fatal("normal zero price accepted")
	}
	if prices, err := shipmentInputPricesWithZero([]string{"P1"}, lines, true); err != nil || prices["P1"] != 0 {
		t.Fatal("snapshot zero rejected", err)
	}
	lines[0].SalePriceUSDMinor = &negative
	if _, err := shipmentInputPricesWithZero([]string{"P1"}, lines, true); err == nil {
		t.Fatal("negative snapshot accepted")
	}
	var input ShipmentCreateInput
	if err := json.Unmarshal([]byte(`{"allowZeroPrice":true}`), &input); err != nil {
		t.Fatal(err)
	}
	if input.allowZeroPrice {
		t.Fatal("internal permission set by JSON")
	}
}
