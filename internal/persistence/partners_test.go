package persistence

import (
	"errors"
	"testing"
)

func TestValidatePartnerFieldsAcceptsUnifiedTradeTypes(t *testing.T) {
	closingDay := 20
	roles := []PartnerRoleInput{{RoleType: "buyer"}, {RoleType: "supplier"}}
	if err := validatePartnerFields("テスト取引先", "香港テスト地区", "partner@example.com", "T1234567890123", "overseas", &closingDay, false, roles); err != nil {
		t.Fatalf("both buyer and supplier roles should be accepted: %v", err)
	}
	if err := validatePartnerFields("その他取引先", "東京都千代田区", "", "", "domestic", nil, true, nil); err != nil {
		t.Fatalf("other-only partner should be accepted: %v", err)
	}
}

func TestValidatePartnerFieldsRejectsInvalidClassification(t *testing.T) {
	if err := validatePartnerFields("区分なし", "東京都千代田区", "", "", "domestic", nil, false, nil); !errors.Is(err, ErrPartnerInvalid) {
		t.Fatalf("partner without any classification must be rejected: %v", err)
	}
	invalidClosingDay := 32
	if err := validatePartnerFields("締め日不正", "東京都千代田区", "", "", "domestic", &invalidClosingDay, true, nil); !errors.Is(err, ErrPartnerInvalid) {
		t.Fatalf("invalid closing day must be rejected: %v", err)
	}
	if err := validatePartnerFields("地域不正", "東京都千代田区", "", "", "unknown", nil, true, nil); !errors.Is(err, ErrPartnerInvalid) {
		t.Fatalf("invalid region must be rejected: %v", err)
	}
	if err := validatePartnerFields("住所なし", "", "", "", "domestic", nil, true, nil); !errors.Is(err, ErrPartnerInvalid) {
		t.Fatalf("partner address is required: %v", err)
	}
	if err := validatePartnerFields("番号不正", "東京都千代田区", "", "T123", "domestic", nil, true, nil); !errors.Is(err, ErrPartnerInvalid) {
		t.Fatalf("invoice number must be T plus 13 digits: %v", err)
	}
}

func TestNormalizePartnerRoleCodes(t *testing.T) {
	buyer, err := normalizePartnerRole(PartnerRoleInput{RoleType: " BUYER ", RoleCode: " b004 "})
	if err != nil || buyer.RoleType != "buyer" || buyer.RoleCode != "B004" {
		t.Fatalf("buyer role normalization failed: %#v, %v", buyer, err)
	}
	supplier, err := normalizePartnerRole(PartnerRoleInput{RoleType: "supplier", RoleCode: "s005"})
	if err != nil || supplier.RoleCode != "S005" {
		t.Fatalf("supplier role normalization failed: %#v, %v", supplier, err)
	}
	if _, err := normalizePartnerRole(PartnerRoleInput{RoleType: "other"}); !errors.Is(err, ErrPartnerInvalid) {
		t.Fatalf("other must be represented by isOther, not a partner role: %v", err)
	}
}
