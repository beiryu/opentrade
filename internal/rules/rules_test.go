package rules

import "testing"

func TestTickSize(t *testing.T) {
	cases := []struct {
		ex    Exchange
		price float64
		want  float64
	}{
		{HOSE, 9_990, 10},
		{HOSE, 10_000, 50},
		{HOSE, 49_950, 50},
		{HOSE, 50_000, 100},
		{HOSE, 120_000, 100},
		{HNX, 5_000, 100},
		{UPCOM, 150_000, 100},
	}
	for _, c := range cases {
		if got := TickSize(c.ex, c.price); got != c.want {
			t.Errorf("TickSize(%s, %v) = %v, want %v", c.ex, c.price, got, c.want)
		}
	}
}

func TestBand(t *testing.T) {
	// FPT-like reference 120,000 on HOSE: ±7 % = 111,600 / 128,400, both on
	// the 100 tick already.
	floor, ceiling := Band(HOSE, 120_000)
	if floor != 111_600 || ceiling != 128_400 {
		t.Fatalf("HOSE band = %v/%v, want 111600/128400", floor, ceiling)
	}
	// Reference 23,450: 7 % = 1,641.5 → raw 25,091.5 / 21,808.5; ceiling
	// rounds down to 25,050, floor rounds up to 21,850 (50 tick).
	floor, ceiling = Band(HOSE, 23_450)
	if floor != 21_850 || ceiling != 25_050 {
		t.Fatalf("HOSE band = %v/%v, want 21850/25050", floor, ceiling)
	}
	// HNX ±10 % on 100 tick.
	floor, ceiling = Band(HNX, 15_300)
	if floor != 13_800 || ceiling != 16_800 {
		t.Fatalf("HNX band = %v/%v, want 13800/16800", floor, ceiling)
	}
	if !InBand(HOSE, 120_000, 128_400) || InBand(HOSE, 120_000, 128_500) {
		t.Fatal("InBand edge handling wrong")
	}
}

func TestRoundToTick(t *testing.T) {
	if got := RoundToTick(HOSE, 23_462); got != 23_450 {
		t.Errorf("RoundToTick = %v, want 23450", got)
	}
	if got := RoundToTick(HOSE, 23_476); got != 23_500 {
		t.Errorf("RoundToTick = %v, want 23500", got)
	}
	if !IsOnTick(HOSE, 23_450) || IsOnTick(HOSE, 23_460) {
		t.Error("IsOnTick wrong")
	}
}

func TestLotsAndCosts(t *testing.T) {
	if IsLotMultiple(150) || !IsLotMultiple(300) || IsLotMultiple(0) {
		t.Error("IsLotMultiple wrong")
	}
	// Buying 100 shares at 50,000: notional 5,000,000; fee 7,500.
	if got := BuyCost(5_000_000); got != 5_007_500 {
		t.Errorf("BuyCost = %v, want 5007500", got)
	}
	// Selling the same: fee 7,500 + tax 5,000 → 4,987,500.
	if got := SellProceeds(5_000_000); got != 4_987_500 {
		t.Errorf("SellProceeds = %v, want 4987500", got)
	}
	// 10,020,000 VND buys exactly two lots at 50,000 (2 × 5,007,500 =
	// 10,015,000), not three.
	if got := MaxAffordableLots(10_020_000, 50_000); got != 2 {
		t.Errorf("MaxAffordableLots = %d, want 2", got)
	}
	if MaxAffordableLots(1_000, 50_000) != 0 {
		t.Error("MaxAffordableLots should be 0 when one lot is unaffordable")
	}
}

func TestParseExchange(t *testing.T) {
	if ex, err := ParseExchange("hose"); err != nil || ex != HOSE {
		t.Errorf("ParseExchange(hose) = %v, %v", ex, err)
	}
	if _, err := ParseExchange("NYSE"); err == nil {
		t.Error("expected error for unknown exchange")
	}
}
