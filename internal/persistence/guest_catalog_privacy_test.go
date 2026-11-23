package persistence

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGuestCatalogJSONHidesBoxInformation(t *testing.T) {
	encoded, err := json.Marshal(GuestCatalogItem{ProductCode: "1709260001", BoxCodes: "BOX-PRIVATE-01"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "boxCodes") || strings.Contains(string(encoded), "BOX-PRIVATE") {
		t.Fatalf("guest catalog leaks BOX information: %s", encoded)
	}
	if !strings.Contains(string(encoded), "1709260001") {
		t.Fatal("catalog product missing")
	}
}
