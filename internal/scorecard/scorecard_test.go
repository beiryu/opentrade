package scorecard

import (
	"math"
	"testing"
	"time"

	"github.com/beiryu/opentrade/internal/store"
)

func TestFromCurve(t *testing.T) {
	eq := make([]float64, 251) // one trading year, +10 % straight line in log terms
	for i := range eq {
		eq[i] = 100 * math.Pow(1.10, float64(i)/250)
	}
	m := FromCurve("s", eq)
	if math.Abs(m.TotalReturn-0.10) > 1e-9 || math.Abs(m.AnnualReturn-0.10) > 1e-9 {
		t.Fatalf("returns = %+v", m)
	}
	if m.MaxDrawdown != 0 || m.Volatility > 1e-9 {
		t.Fatalf("straight line should have no drawdown or vol: %+v", m)
	}
	if FromCurve("x", nil).Days != 0 {
		t.Fatal("empty curve")
	}
}

func TestBuyAndHold(t *testing.T) {
	bh := BuyAndHold(1000, []float64{1250, 1300, 1200})
	if bh[0] != 1000 || bh[1] != 1040 || bh[2] != 960 {
		t.Fatalf("BuyAndHold = %v", bh)
	}
}

func TestWithTradesFIFO(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC) }
	orders := []store.Order{
		{ID: "1", DecisionDate: d(2), Symbol: "FPT", Side: "buy", Qty: 100, Status: "filled", FillPrice: 100_000, Fee: 15_000},
		{ID: "2", DecisionDate: d(3), Symbol: "FPT", Side: "buy", Qty: 100, Status: "filled", FillPrice: 120_000, Fee: 18_000},
		// Sell 100 at 110,000: matches the first lot (cost 100,150/sh) → win.
		{ID: "3", DecisionDate: d(6), Symbol: "FPT", Side: "sell", Qty: 100, Status: "filled", FillPrice: 110_000, Fee: 16_500, Tax: 11_000},
		// Sell 100 at 110,000: matches the second lot (cost 120,180/sh) → loss.
		{ID: "4", DecisionDate: d(9), Symbol: "FPT", Side: "sell", Qty: 100, Status: "filled", FillPrice: 110_000, Fee: 16_500, Tax: 11_000},
		{ID: "5", DecisionDate: d(9), Symbol: "VNM", Side: "buy", Qty: 100, Status: "expired"},
	}
	m := WithTrades(Metrics{}, orders)
	if m.Trades != 4 || m.RoundTrips != 2 || m.WinRate != 0.5 {
		t.Fatalf("trades = %+v", m)
	}
}

func TestBuildVerdict(t *testing.T) {
	var curve, rnd []store.EquityPoint
	for i := 0; i < 60; i++ {
		day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		curve = append(curve, store.EquityPoint{Date: day, Equity: 100 + float64(i), IndexClose: 1000 + float64(i)*0.5})
		rnd = append(rnd, store.EquityPoint{Date: day, Equity: 100 + float64(i)*0.2})
	}
	card := Build("llm", curve, nil, rnd, nil)
	if card.Random == nil || card.Strategy.TotalReturn <= card.Index.TotalReturn || card.Verdict == "" {
		t.Fatalf("card = %+v", card)
	}
	if card.Verdict[:9] != "Beat both" {
		t.Fatalf("verdict = %q", card.Verdict)
	}
}
