// Package scorecard turns an equity curve into the numbers that answer the
// only question that matters: did the strategy beat doing nothing (holding
// the index) and doing anything (trading at random)?
package scorecard

import (
	"math"
	"sort"
	"time"

	"github.com/beiryu/opentrade/internal/indicators"
	"github.com/beiryu/opentrade/internal/store"
)

// TradingDaysPerYear annualises returns and volatility.
const TradingDaysPerYear = 250.0

// Metrics for one curve.
type Metrics struct {
	Name         string  `json:"name"`
	Days         int     `json:"days"`
	StartEquity  float64 `json:"start_equity"`
	EndEquity    float64 `json:"end_equity"`
	TotalReturn  float64 `json:"total_return"`  // fraction
	AnnualReturn float64 `json:"annual_return"` // fraction, compounded
	Volatility   float64 `json:"volatility"`    // annualised stdev of daily returns
	Sharpe       float64 `json:"sharpe"`        // annual return / volatility, rf = 0
	MaxDrawdown  float64 `json:"max_drawdown"`  // fraction
	Trades       int     `json:"trades"`        // filled orders
	RoundTrips   int     `json:"round_trips"`   // sells matched to earlier buys
	WinRate      float64 `json:"win_rate"`      // fraction of round trips that gained after costs
}

// FromCurve computes metrics for an equity series (oldest first).
func FromCurve(name string, equity []float64) Metrics {
	m := Metrics{Name: name, Days: len(equity)}
	if len(equity) < 2 {
		if len(equity) == 1 {
			m.StartEquity, m.EndEquity = equity[0], equity[0]
		}
		return m
	}
	m.StartEquity, m.EndEquity = equity[0], equity[len(equity)-1]
	if m.StartEquity > 0 {
		m.TotalReturn = m.EndEquity/m.StartEquity - 1
		years := float64(len(equity)-1) / TradingDaysPerYear
		if years > 0 && m.EndEquity > 0 {
			m.AnnualReturn = math.Pow(m.EndEquity/m.StartEquity, 1/years) - 1
		}
	}
	rets := make([]float64, 0, len(equity)-1)
	for i := 1; i < len(equity); i++ {
		if equity[i-1] > 0 {
			rets = append(rets, equity[i]/equity[i-1]-1)
		}
	}
	m.Volatility = stdev(rets) * math.Sqrt(TradingDaysPerYear)
	if m.Volatility > 0 {
		m.Sharpe = m.AnnualReturn / m.Volatility
	}
	m.MaxDrawdown = indicators.MaxDrawdown(equity)
	return m
}

func stdev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	mean := 0.0
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	ss := 0.0
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return math.Sqrt(ss / float64(len(xs)-1))
}

// BuyAndHold is the equity of putting initial cash into a price series at
// its first value (no costs; the benchmark should be hard to beat).
func BuyAndHold(initial float64, closes []float64) []float64 {
	out := make([]float64, len(closes))
	if len(closes) == 0 || closes[0] == 0 {
		return out
	}
	for i, c := range closes {
		out[i] = initial * c / closes[0]
	}
	return out
}

// WithTrades fills Trades, RoundTrips and WinRate from the order log. A
// round trip matches each filled sell against the earliest unmatched
// filled buys of the same symbol (FIFO) and wins when the sale proceeds
// after fee and tax exceed the cost including fee.
func WithTrades(m Metrics, orders []store.Order) Metrics {
	type lot struct {
		qty  int
		cost float64 // per share, fee included
	}
	// Oldest first by decision date then id.
	sorted := make([]store.Order, 0, len(orders))
	for _, o := range orders {
		if o.Status == "filled" {
			sorted = append(sorted, o)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].DecisionDate.Equal(sorted[j].DecisionDate) {
			return sorted[i].DecisionDate.Before(sorted[j].DecisionDate)
		}
		return sorted[i].ID < sorted[j].ID
	})
	open := map[string][]lot{}
	wins, trips := 0, 0
	for _, o := range sorted {
		m.Trades++
		switch o.Side {
		case "buy":
			per := o.FillPrice + o.Fee/float64(o.Qty)
			open[o.Symbol] = append(open[o.Symbol], lot{qty: o.Qty, cost: per})
		case "sell":
			remaining := o.Qty
			cost := 0.0
			matched := 0
			lots := open[o.Symbol]
			for len(lots) > 0 && remaining > 0 {
				take := lots[0].qty
				if take > remaining {
					take = remaining
				}
				cost += float64(take) * lots[0].cost
				matched += take
				remaining -= take
				lots[0].qty -= take
				if lots[0].qty == 0 {
					lots = lots[1:]
				}
			}
			open[o.Symbol] = lots
			if matched == 0 {
				continue
			}
			proceeds := (o.FillPrice*float64(o.Qty) - o.Fee - o.Tax) * float64(matched) / float64(o.Qty)
			trips++
			if proceeds > cost {
				wins++
			}
		}
	}
	m.RoundTrips = trips
	if trips > 0 {
		m.WinRate = float64(wins) / float64(trips)
	}
	return m
}

// Card is the full comparison for a period.
type Card struct {
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Strategy Metrics   `json:"strategy"`
	Index    Metrics   `json:"index"`
	Random   *Metrics  `json:"random,omitempty"`
	// Verdict is one sentence a reader can act on.
	Verdict string `json:"verdict"`
}

// Build assembles the card from the strategy's curve, its orders, the
// index closes over the same days and, when available, a random run.
func Build(name string, curve []store.EquityPoint, orders []store.Order, random []store.EquityPoint, randomOrders []store.Order) Card {
	var card Card
	if len(curve) == 0 {
		card.Verdict = "no data"
		return card
	}
	card.From, card.To = curve[0].Date, curve[len(curve)-1].Date
	eq := make([]float64, len(curve))
	idx := make([]float64, len(curve))
	for i, p := range curve {
		eq[i] = p.Equity
		idx[i] = p.IndexClose
	}
	card.Strategy = WithTrades(FromCurve(name, eq), orders)
	card.Index = FromCurve("VN-Index buy & hold", BuyAndHold(eq[0], idx))
	if len(random) > 0 {
		req := make([]float64, len(random))
		for i, p := range random {
			req[i] = p.Equity
		}
		r := WithTrades(FromCurve("random", req), randomOrders)
		card.Random = &r
	}
	card.Verdict = verdict(card)
	return card
}

func verdict(c Card) string {
	beatIndex := c.Strategy.TotalReturn > c.Index.TotalReturn
	beatRandom := c.Random == nil || c.Strategy.TotalReturn > c.Random.TotalReturn
	switch {
	case c.Strategy.Days < 40:
		return "Too short to say anything: fewer than 40 trading days."
	case beatIndex && beatRandom:
		return "Beat both the index and random trading over this period. Keep testing; one period proves little."
	case beatIndex:
		return "Beat the index but not random trading: the edge is luck-sized."
	case beatRandom:
		return "Beat random trading but not the index: holding VN-Index would have done better."
	default:
		return "Lost to both the index and random trading. Do not fund this."
	}
}
