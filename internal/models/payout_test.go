package models

import (
	"testing"
	"time"
)

// ===== CalculatePayoutSplit TESTS =====

func TestCalculatePayoutSplit_StandardPrice(t *testing.T) {
	// $4.99 note (499 cents)
	flat, mkt, payout := CalculatePayoutSplit(499)
	if flat != 25 {
		t.Fatalf("flat fee = %d, want 25", flat)
	}
	// 15% of 499 = 74 (integer division)
	if mkt != 74 {
		t.Fatalf("marketplace fee = %d, want 74", mkt)
	}
	// 499 - 25 - 74 = 400
	if payout != 400 {
		t.Fatalf("creator payout = %d, want 400", payout)
	}
	if flat+mkt+payout != 499 {
		t.Fatalf("fees don't add up: %d + %d + %d != 499", flat, mkt, payout)
	}
}

func TestCalculatePayoutSplit_OneHundredDollars(t *testing.T) {
	// $100 note (10000 cents)
	flat, mkt, payout := CalculatePayoutSplit(10000)
	if flat != 25 {
		t.Fatalf("flat fee = %d, want 25", flat)
	}
	if mkt != 1500 {
		t.Fatalf("marketplace fee = %d, want 1500", mkt)
	}
	if payout != 8475 {
		t.Fatalf("creator payout = %d, want 8475", payout)
	}
	if flat+mkt+payout != 10000 {
		t.Fatalf("fees don't add up: %d + %d + %d != 10000", flat, mkt, payout)
	}
}

func TestCalculatePayoutSplit_FreeNote(t *testing.T) {
	flat, mkt, payout := CalculatePayoutSplit(0)
	if flat != 0 || mkt != 0 || payout != 0 {
		t.Fatalf("free note: expected (0,0,0), got (%d,%d,%d)", flat, mkt, payout)
	}
}

func TestCalculatePayoutSplit_NegativePrice(t *testing.T) {
	flat, mkt, payout := CalculatePayoutSplit(-100)
	if flat != 0 || mkt != 0 || payout != 0 {
		t.Fatalf("negative price: expected (0,0,0), got (%d,%d,%d)", flat, mkt, payout)
	}
}

func TestCalculatePayoutSplit_MinimumPrice(t *testing.T) {
	// 1 cent note — flat fee alone exceeds price
	flat, mkt, payout := CalculatePayoutSplit(1)
	if payout != 0 {
		t.Fatalf("creator payout = %d, want 0 (price too low)", payout)
	}
	if flat != 1 {
		t.Fatalf("flat fee = %d, want 1 (capped to gross)", flat)
	}
	if mkt != 0 {
		t.Fatalf("marketplace fee = %d, want 0", mkt)
	}
}

func TestCalculatePayoutSplit_PriceEqualsFlatFee(t *testing.T) {
	// 25 cents — exactly flat fee
	flat, mkt, payout := CalculatePayoutSplit(25)
	if flat != 25 {
		t.Fatalf("flat fee = %d, want 25", flat)
	}
	// marketplace fee = 15% of 25 = 3, but payout would be 25-25-3 = -3 → capped
	// After adjustment: payout=0, marketplace = 25-25 = 0
	if payout != 0 {
		t.Fatalf("creator payout = %d, want 0", payout)
	}
	if flat+mkt+payout != 25 {
		t.Fatalf("fees don't add up: %d + %d + %d != 25", flat, mkt, payout)
	}
}

func TestCalculatePayoutSplit_LowPrice(t *testing.T) {
	// 50 cents — fees would be 25 + 7 = 32, payout = 18
	flat, mkt, payout := CalculatePayoutSplit(50)
	if flat != 25 {
		t.Fatalf("flat fee = %d, want 25", flat)
	}
	if mkt != 7 {
		t.Fatalf("marketplace fee = %d, want 7", mkt)
	}
	if payout != 18 {
		t.Fatalf("creator payout = %d, want 18", payout)
	}
	if flat+mkt+payout != 50 {
		t.Fatalf("fees don't add up: %d + %d + %d != 50", flat, mkt, payout)
	}
}

func TestCalculatePayoutSplit_FeesSumToGross(t *testing.T) {
	// Property: for any positive price, flat + marketplace + payout == gross
	prices := []int64{1, 10, 25, 26, 50, 99, 100, 199, 499, 999, 1000, 5000, 10000, 50000}
	for _, price := range prices {
		flat, mkt, payout := CalculatePayoutSplit(price)
		if flat+mkt+payout != price {
			t.Errorf("price=%d: %d + %d + %d = %d, want %d", price, flat, mkt, payout, flat+mkt+payout, price)
		}
		if flat < 0 || mkt < 0 || payout < 0 {
			t.Errorf("price=%d: negative component (%d, %d, %d)", price, flat, mkt, payout)
		}
	}
}

func TestCalculatePayoutSplit_OneDollar(t *testing.T) {
	// $1.00 — flat=25, mkt=15, payout=60
	flat, mkt, payout := CalculatePayoutSplit(100)
	if flat != 25 {
		t.Fatalf("flat fee = %d, want 25", flat)
	}
	if mkt != 15 {
		t.Fatalf("marketplace fee = %d, want 15", mkt)
	}
	if payout != 60 {
		t.Fatalf("creator payout = %d, want 60", payout)
	}
}

// ===== PayoutStatus TESTS =====

func TestPayoutStatus_Constants(t *testing.T) {
	if PayoutPending != "pending" {
		t.Fatalf("PayoutPending = %q", PayoutPending)
	}
	if PayoutCompleted != "completed" {
		t.Fatalf("PayoutCompleted = %q", PayoutCompleted)
	}
	if PayoutFailed != "failed" {
		t.Fatalf("PayoutFailed = %q", PayoutFailed)
	}
	if PayoutRetained != "retained" {
		t.Fatalf("PayoutRetained = %q", PayoutRetained)
	}
}

// ===== FeeConstants TESTS =====

func TestFeeConstants(t *testing.T) {
	if FlatFeeCents != 25 {
		t.Fatalf("FlatFeeCents = %d, want 25", FlatFeeCents)
	}
	if MarketplaceFeePercent != 15 {
		t.Fatalf("MarketplaceFeePercent = %d, want 15", MarketplaceFeePercent)
	}
}

// ===== PayoutRecord struct TESTS =====

func TestPayoutRecord_DefaultValues(t *testing.T) {
	record := PayoutRecord{}
	if record.GrossCents != 0 {
		t.Fatalf("expected 0 GrossCents, got %d", record.GrossCents)
	}
	if record.Status != "" {
		t.Fatalf("expected empty Status, got %q", record.Status)
	}
}

// ===== CalculatePayoutSplit edge: price that barely covers flat fee =====

func TestCalculatePayoutSplit_BarelyOverFlatFee(t *testing.T) {
	// 30 cents — flat=25, mkt=15%*30=4, payout=30-25-4=1
	flat, mkt, payout := CalculatePayoutSplit(30)
	if flat != 25 {
		t.Fatalf("flat fee = %d, want 25", flat)
	}
	if mkt != 4 {
		t.Fatalf("marketplace fee = %d, want 4", mkt)
	}
	if payout != 1 {
		t.Fatalf("creator payout = %d, want 1", payout)
	}
}

// ===== CalculatePayoutSplit: ensure no negative payout boundary =====

func TestCalculatePayoutSplit_NegativePayoutBoundary(t *testing.T) {
	// 26 cents: flat=25, mkt=15%*26=3, payout=26-25-3=-2 → capped to 0
	flat, mkt, payout := CalculatePayoutSplit(26)
	if payout != 0 {
		t.Fatalf("creator payout = %d, want 0 (should be capped)", payout)
	}
	if flat+mkt+payout != 26 {
		t.Fatalf("fees don't add up: %d + %d + %d != 26", flat, mkt, payout)
	}
}

// ===== CalculatePayoutSplit: large amounts (prevent overflow) =====

func TestCalculatePayoutSplit_LargeAmount(t *testing.T) {
	// $999.99 = 99999 cents
	flat, mkt, payout := CalculatePayoutSplit(99999)
	if flat != 25 {
		t.Fatalf("flat fee = %d", flat)
	}
	expectedMkt := int64(99999 * 15 / 100) // 14999
	if mkt != expectedMkt {
		t.Fatalf("marketplace fee = %d, want %d", mkt, expectedMkt)
	}
	expectedPayout := int64(99999 - 25 - 14999) // 84975
	if payout != expectedPayout {
		t.Fatalf("creator payout = %d, want %d", payout, expectedPayout)
	}
	if flat+mkt+payout != 99999 {
		t.Fatalf("fees don't add up")
	}
}

// ===== CalculatePayoutSplit: monotonicity =====

func TestCalculatePayoutSplit_PayoutIncreases(t *testing.T) {
	// Payout should non-decrease as price increases
	var lastPayout int64 = -1
	for price := int64(0); price <= 1000; price++ {
		_, _, payout := CalculatePayoutSplit(price)
		if payout < lastPayout {
			t.Fatalf("payout decreased from %d to %d at price=%d", lastPayout, payout, price)
		}
		lastPayout = payout
	}
}

// ===== Ban duration / expiry helpers =====

func TestBanDuration_Constants(t *testing.T) {
	if BanDuration1Day != "1d" {
		t.Fatalf("BanDuration1Day = %q", BanDuration1Day)
	}
	if BanDuration7Days != "7d" {
		t.Fatalf("BanDuration7Days = %q", BanDuration7Days)
	}
	if BanDuration30Days != "30d" {
		t.Fatalf("BanDuration30Days = %q", BanDuration30Days)
	}
	if BanDuration1Year != "1y" {
		t.Fatalf("BanDuration1Year = %q", BanDuration1Year)
	}
	if BanDurationForever != "permanent" {
		t.Fatalf("BanDurationForever = %q", BanDurationForever)
	}
}

func TestParseBanDuration_KnownValues(t *testing.T) {
	cases := []struct {
		d       BanDuration
		wantDur time.Duration
		wantOK  bool
	}{
		{BanDuration1Day, 24 * time.Hour, true},
		{BanDuration7Days, 7 * 24 * time.Hour, true},
		{BanDuration30Days, 30 * 24 * time.Hour, true},
		{BanDuration1Year, 365 * 24 * time.Hour, true},
		{BanDurationForever, 0, false},
	}
	for _, tc := range cases {
		dur, ok := ParseBanDuration(tc.d)
		if ok != tc.wantOK {
			t.Errorf("ParseBanDuration(%q) ok=%v, want %v", tc.d, ok, tc.wantOK)
		}
		if ok && dur != tc.wantDur {
			t.Errorf("ParseBanDuration(%q) dur=%v, want %v", tc.d, dur, tc.wantDur)
		}
	}
}

func TestParseBanDuration_Invalid(t *testing.T) {
	_, ok := ParseBanDuration("invalid")
	if ok {
		t.Fatal("expected invalid duration to return false")
	}
}

func TestValidBanDuration(t *testing.T) {
	valid := []BanDuration{"1d", "7d", "30d", "1y", "permanent"}
	for _, d := range valid {
		if !ValidBanDuration(d) {
			t.Errorf("ValidBanDuration(%q) = false, want true", d)
		}
	}
	invalid := []BanDuration{"", "2d", "forever", "1m"}
	for _, d := range invalid {
		if ValidBanDuration(d) {
			t.Errorf("ValidBanDuration(%q) = true, want false", d)
		}
	}
}

func TestBan_IsExpired(t *testing.T) {
	// Not expired: future time
	future := time.Now().Add(24 * time.Hour)
	ban := Ban{ExpiresAt: &future}
	if ban.IsExpired() {
		t.Fatal("future ban should not be expired")
	}

	// Expired: past time
	past := time.Now().Add(-24 * time.Hour)
	ban.ExpiresAt = &past
	if !ban.IsExpired() {
		t.Fatal("past ban should be expired")
	}

	// Permanent: nil expiry
	ban.ExpiresAt = nil
	if ban.IsExpired() {
		t.Fatal("permanent ban (nil expiry) should not be expired")
	}
}

func TestBan_IsActive(t *testing.T) {
	// Active: future expiry
	future := time.Now().Add(24 * time.Hour)
	ban := Ban{ExpiresAt: &future}
	if !ban.IsActive() {
		t.Fatal("future ban should be active")
	}

	// Inactive: past expiry
	past := time.Now().Add(-24 * time.Hour)
	ban.ExpiresAt = &past
	if ban.IsActive() {
		t.Fatal("expired ban should not be active")
	}

	// Permanent: always active
	ban.ExpiresAt = nil
	if !ban.IsActive() {
		t.Fatal("permanent ban should be active")
	}
}
