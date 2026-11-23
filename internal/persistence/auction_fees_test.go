package persistence

import (
	"encoding/json"
	"errors"
	"testing"
)

func auctionPtr(v int64) *int64 { return &v }
func TestAuctionFeeCalculation(t *testing.T) {
	tier := AuctionFeeRule{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, To: auctionPtr(100000), RateBasisPoints: 1000}, {From: 100000, To: nil, RateBasisPoints: 500}}}
	mixed := AuctionFeeRule{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, To: auctionPtr(100000), RateBasisPoints: 333}, {From: 100000, Mode: "fixed", FixedAmount: 550}}}
	for _, tc := range []struct {
		name      string
		base      int64
		rule      AuctionFeeRule
		direction string
		want, fee int64
	}{
		{"purchase percentage truncation", 100019, AuctionFeeRule{Mode: "rate", RateBasisPoints: 333}, "purchase", 103349, 3330},
		{"sales percentage truncation", 100019, AuctionFeeRule{Mode: "rate", RateBasisPoints: 333}, "sales", 96689, 3330},
		{"lower tier", 99999, tier, "purchase", 109998, 9999},
		{"boundary belongs to next tier", 100000, tier, "sales", 95000, 5000},
		{"fixed per item", 100000, AuctionFeeRule{Mode: "fixed", FixedAmount: 550}, "purchase", 100550, 550},
		{"mixed percentage lower band", 99999, mixed, "purchase", 103328, 3329},
		{"mixed fixed boundary purchase", 100000, mixed, "purchase", 100550, 550},
		{"mixed fixed boundary sales", 100000, mixed, "sales", 99450, 550},
		{"fixed band upper unlimited", 200000, mixed, "sales", 199450, 550},
		{"fixed zero band", 0, AuctionFeeRule{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, Mode: "fixed"}}}, "purchase", 0, 0},
		{"zero result", 550, AuctionFeeRule{Mode: "fixed", FixedAmount: 550}, "sales", 0, 550},
		{"large safe integer does not overflow", 9007199254740000, AuctionFeeRule{Mode: "rate", RateBasisPoints: 9999}, "sales", 900719925474, 9006298534814526},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, snapshot, err := calculateAuctionFee(tc.base, "JPY", tc.direction, &tc.rule)
			if err != nil || actual != tc.want || snapshot.FeeMinor != tc.fee || snapshot.BaseAmountMinor != tc.base || snapshot.ResultAmountMinor != actual {
				t.Fatalf("result=%v snapshot=%+v err=%v", actual, snapshot, err)
			}
		})
	}
	for _, currency := range []string{"USD", "HKD", "EUR"} {
		if _, _, err := calculateAuctionFee(100, currency, "purchase", &tier); !errors.Is(err, ErrAuctionFee) {
			t.Fatal(currency, err)
		}
	}
	if _, _, err := calculateAuctionFee(100, "JPY", "sales", &AuctionFeeRule{Mode: "fixed", FixedAmount: 101}); !errors.Is(err, ErrAuctionFee) {
		t.Fatal("negative net allowed")
	}
	if amount, snap, err := calculateAuctionFee(123, "USD", "purchase", nil); err != nil || amount != 123 || snap.Rule != nil {
		t.Fatal("OFF altered amount")
	}
}
func TestAuctionFeeRejectInvalidConfiguration(t *testing.T) {
	for _, rule := range []AuctionFeeRule{
		{Mode: "unknown"}, {Mode: "none", FixedAmount: 1}, {Mode: "rate", RateBasisPoints: -1}, {Mode: "rate", RateBasisPoints: 10001}, {Mode: "fixed", FixedAmount: -1},
		{Mode: "tiers"}, {Mode: "tiers", Tiers: []AuctionFeeTier{{From: 1}}},
		{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, To: auctionPtr(100)}, {From: 99}}},
		{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, To: auctionPtr(100)}, {From: 101}}},
		{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, To: auctionPtr(100)}}},
		{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, Mode: "fixed", FixedAmount: -1}}},
		{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, Mode: "fixed", FixedAmount: maxAuctionAmount + 1}}},
		{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, Mode: "fixed", FixedAmount: 100, RateBasisPoints: 1}}},
		{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, Mode: "rate", FixedAmount: 100}}},
		{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, Mode: "unknown"}}},
	} {
		if _, err := normalizeAuctionFeeRule(rule); !errors.Is(err, ErrAuctionFee) {
			t.Fatalf("accepted %#v", rule)
		}
	}
	if _, err := normalizeAuctionFees(AuctionFeeSettings{Purchase: AuctionFeeRule{Mode: "rate"}}); !errors.Is(err, ErrAuctionFee) {
		t.Fatal("non-auction fee accepted")
	}
}
func TestAuctionFeeMixedTierJSONCompatibility(t *testing.T) {
	var legacy AuctionFeeRule
	if err := json.Unmarshal([]byte(`{"mode":"tiers","tiers":[{"from":0,"to":null,"rateBasisPoints":500}]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	value, snapshot, err := calculateAuctionFee(10000, "JPY", "purchase", &legacy)
	if err != nil || value != 10500 {
		t.Fatal(value, err)
	}
	if snapshot.Rule.Tiers[0].Mode != "" {
		t.Fatal("legacy rule shape changed")
	}
	rule := AuctionFeeRule{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, To: auctionPtr(100000), Mode: "rate", RateBasisPoints: 100}, {From: 100000, Mode: "fixed", FixedAmount: 550}}}
	normalized, err := normalizeAuctionFeeRule(rule)
	if err != nil || rule.Tiers[0].Mode != "rate" || normalized.Tiers[0].Mode != "" {
		t.Fatal("normalization mutated original", err)
	}
	settings := AuctionFeeSettings{IsAuction: true, Purchase: normalized, Sales: normalized}
	raw, err := settings.Value()
	if err != nil {
		t.Fatal(err)
	}
	var saved AuctionFeeSettings
	if err = saved.Scan(raw); err != nil {
		t.Fatal(err)
	}
	value, snapshot, err = calculateAuctionFee(100000, "JPY", "sales", &saved.Sales)
	if err != nil || value != 99450 || snapshot.Rule.Tiers[1].FixedAmount != 550 {
		t.Fatal(value, snapshot, err)
	}
	tooLarge := AuctionFeeRule{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, Mode: "fixed", FixedAmount: 101}}}
	if _, _, err = calculateAuctionFee(100, "JPY", "sales", &tooLarge); !errors.Is(err, ErrAuctionFee) {
		t.Fatal("negative net accepted", err)
	}
}
func TestAuctionFeeJSONAndResubmission(t *testing.T) {
	settings := AuctionFeeSettings{IsAuction: true, Purchase: AuctionFeeRule{Mode: "rate", RateBasisPoints: 500}, Sales: AuctionFeeRule{Mode: "fixed", FixedAmount: 300}}
	raw, err := settings.Value()
	if err != nil {
		t.Fatal(err)
	}
	var scanned AuctionFeeSettings
	if err = scanned.Scan(raw); err != nil || scanned.Sales.FixedAmount != 300 {
		t.Fatal(err, scanned)
	}
	_, snapshot, err := calculateAuctionFee(10000, "JPY", "purchase", &settings.Purchase)
	if err != nil {
		t.Fatal(err)
	}
	identity := &returnedDraftIdentity{lineIDs: []string{"old", ""}, auctionFees: map[string]AuctionFeeSnapshot{"old": snapshot}}
	kept := returnedAuctionFee(identity, 0, AuctionFeeSnapshot{})
	b, _ := json.Marshal(kept)
	a, _ := json.Marshal(snapshot)
	if string(a) != string(b) {
		t.Fatal("snapshot lost")
	}
	if returnedAuctionFee(identity, 1, AuctionFeeSnapshot{}).Rule != nil {
		t.Fatal("new line inherited unrelated fee")
	}
}
