package paper

import (
	"errors"
	"testing"
	"time"

	"github.com/beiryu/opentrade/internal/market"
	"github.com/beiryu/opentrade/internal/rules"
)

func day(d int) time.Time { return time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC) }

// Mon 2 .. Fri 6, Mon 9 .. Fri 13 March 2026.
var cal = []time.Time{day(2), day(3), day(4), day(5), day(6), day(9), day(10), day(11), day(12), day(13)}

func bar(o, h, l, c float64) market.Bar {
	return market.Bar{Open: o, High: h, Low: l, Close: c, Volume: 1}
}

func TestBuyThenT2Settlement(t *testing.T) {
	a := NewAccount(20_000_000)
	// Decide after close on Mon 2 (ref 50,000), buy 200 at the open.
	o, err := a.Submit(day(2), "FPT", Buy, 200, 0, 50_000, rules.HOSE)
	if err != nil {
		t.Fatal(err)
	}
	if o.Reserved <= 0 || a.Available() >= 20_000_000 {
		t.Fatal("buy should reserve cash")
	}
	// Tue 3: fills at open 50,500.
	res := a.Roll(day(3), map[string]market.Bar{"FPT": bar(50_500, 51_000, 50_000, 50_800)}, cal)
	if len(res) != 1 || res[0].Status != Filled || res[0].FillPrice != 50_500 {
		t.Fatalf("fill = %+v", res)
	}
	wantCash := 20_000_000 - 200*50_500*(1+rules.FeeRate)
	if a.Cash != wantCash {
		t.Fatalf("cash = %v, want %v", a.Cash, wantCash)
	}
	p := a.Position("FPT")
	if p.Qty != 200 || p.Settled != 0 {
		t.Fatalf("position after buy = %+v", p)
	}
	// Selling on Tue 3 (T+0) or Wed 4 (T+1) is refused.
	if _, err := a.Submit(day(3), "FPT", Sell, 200, 0, 50_800, rules.HOSE); !errors.Is(err, ErrRejected) {
		t.Fatal("sell at T+0 must be rejected")
	}
	a.Roll(day(4), map[string]market.Bar{"FPT": bar(51_000, 51_500, 50_500, 51_200)}, cal)
	if a.Position("FPT").Settled != 0 {
		t.Fatal("shares must not settle at T+1")
	}
	// Thu 5 = T+2: settled, sell allowed.
	a.Roll(day(5), map[string]market.Bar{"FPT": bar(52_000, 52_500, 51_500, 52_000)}, cal)
	if a.Position("FPT").Settled != 200 {
		t.Fatal("shares must settle at T+2")
	}
	if _, err := a.Submit(day(5), "FPT", Sell, 200, 0, 52_000, rules.HOSE); err != nil {
		t.Fatal(err)
	}
	// Fri 6: sell fills at open 52,500; proceeds are a receivable until T+2 (Tue 10).
	res = a.Roll(day(6), map[string]market.Bar{"FPT": bar(52_500, 53_000, 52_000, 52_800)}, cal)
	if res[0].Status != Filled || res[0].Tax == 0 {
		t.Fatalf("sell fill = %+v", res[0])
	}
	if len(a.Lots) != 0 || len(a.Receivables) != 1 {
		t.Fatalf("lots=%d receivables=%d", len(a.Lots), len(a.Receivables))
	}
	cashBefore := a.Cash
	a.Roll(day(9), map[string]market.Bar{}, cal)
	if a.Cash != cashBefore {
		t.Fatal("proceeds must not be available at T+1")
	}
	a.Roll(day(10), map[string]market.Bar{}, cal)
	if a.Cash <= cashBefore || len(a.Receivables) != 0 {
		t.Fatal("proceeds must be available at T+2")
	}
	eq, _ := a.Equity(nil)
	if eq != a.Cash {
		t.Fatal("equity should be cash only after round trip")
	}
}

func TestSubmitRejections(t *testing.T) {
	a := NewAccount(1_000_000)
	if _, err := a.Submit(day(2), "FPT", Buy, 150, 0, 50_000, rules.HOSE); !errors.Is(err, ErrRejected) {
		t.Error("odd lot must be rejected")
	}
	if _, err := a.Submit(day(2), "FPT", Buy, 100, 0, 50_000, rules.HOSE); !errors.Is(err, ErrRejected) {
		t.Error("unaffordable buy must be rejected")
	}
	a = NewAccount(100_000_000)
	if _, err := a.Submit(day(2), "FPT", Buy, 100, 60_000, 50_000, rules.HOSE); !errors.Is(err, ErrRejected) {
		t.Error("limit above ceiling must be rejected")
	}
	if _, err := a.Submit(day(2), "FPT", Buy, 100, 50_060, 50_000, rules.HOSE); !errors.Is(err, ErrRejected) {
		t.Error("off-tick limit must be rejected")
	}
	if _, err := a.Submit(day(2), "FPT", Sell, 100, 0, 50_000, rules.HOSE); !errors.Is(err, ErrRejected) {
		t.Error("selling what you do not own must be rejected")
	}
	// Two buys that together exceed cash: the second is rejected because the
	// first reserved its cost.
	if _, err := a.Submit(day(2), "FPT", Buy, 1_000, 0, 50_000, rules.HOSE); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Submit(day(2), "VNM", Buy, 1_000, 0, 60_000, rules.HOSE); !errors.Is(err, ErrRejected) {
		t.Error("second buy must respect reserved cash")
	}
}

func TestLimitOrderFillModel(t *testing.T) {
	a := NewAccount(100_000_000)
	// Limit buy below the open, touched by the low → fills at the limit.
	a.Submit(day(2), "FPT", Buy, 100, 49_000, 50_000, rules.HOSE)
	res := a.Roll(day(3), map[string]market.Bar{"FPT": bar(50_000, 51_000, 48_500, 50_500)}, cal)
	if res[0].Status != Filled || res[0].FillPrice != 49_000 {
		t.Fatalf("limit buy = %+v", res[0])
	}
	// Limit buy inside the band (floor 55,800) but never reached → expired,
	// cash released.
	if _, err := a.Submit(day(3), "VNM", Buy, 100, 56_000, 60_000, rules.HOSE); err != nil {
		t.Fatal(err)
	}
	reservedBefore := a.Reserved()
	res = a.Roll(day(4), map[string]market.Bar{"VNM": bar(60_000, 61_000, 59_000, 60_500)}, cal)
	if res[0].Status != Expired || a.Reserved() != 0 || reservedBefore == 0 {
		t.Fatalf("expired order = %+v reserved=%v", res[0], a.Reserved())
	}
}
