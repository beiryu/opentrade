package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beiryu/opentrade/internal/indicators"
	"github.com/beiryu/opentrade/internal/market"
	"github.com/beiryu/opentrade/internal/paper"
	"github.com/beiryu/opentrade/internal/rules"
	"github.com/beiryu/opentrade/internal/store"
)

// Universe is the set of symbols a decider may trade and their boards.
type Universe struct {
	Symbols    []string
	ExchangeOf func(symbol string) rules.Exchange
}

// Contains reports whether a symbol is tradable.
func (u Universe) Contains(symbol string) bool {
	for _, s := range u.Symbols {
		if s == symbol {
			return true
		}
	}
	return false
}

func (u Universe) exchange(symbol string) rules.Exchange {
	if u.ExchangeOf == nil {
		return rules.HOSE
	}
	return u.ExchangeOf(symbol)
}

// HistoryBars is how many daily bars feed the indicators. 60 covers SMA50
// and RSI14 with margin.
const HistoryBars = 60

// BuildContext assembles what the decider sees for a decision taken after
// the close of `date`. It returns the context and the closes used to value
// the account. A symbol with no bar on `date` is left out of the candidates
// (it did not trade), one with too little history gets zeroed indicators
// and a History count the model can see.
func BuildContext(ctx context.Context, m market.Provider, u Universe, date time.Time, acct *paper.Account, recent []store.Order, limits Limits, safeMode bool, cycleNo int) (*Context, map[string]float64, error) {
	date = market.Day(date)
	cal := m.Calendar()
	next, ok := market.NextTradingDay(cal, date)
	if !ok {
		// Last day of data: assume the next weekday.
		next = date.AddDate(0, 0, 1)
		for wd := next.Weekday(); wd == time.Saturday || wd == time.Sunday; wd = next.Weekday() {
			next = next.AddDate(0, 0, 1)
		}
	}
	c := &Context{Date: date, NextSession: next, Limits: limits, SafeMode: safeMode, CycleNo: cycleNo, Recent: recent}
	prices := map[string]float64{}

	for _, sym := range u.Symbols {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		bars, err := m.Bars(sym, date, HistoryBars)
		if err != nil {
			if errors.Is(err, market.ErrNoData) {
				continue
			}
			return nil, nil, fmt.Errorf("engine: bars for %s: %w", sym, err)
		}
		last := bars[len(bars)-1]
		if !last.Date.Equal(date) {
			// Did not trade today (suspended, listed later…): price it for
			// equity but do not offer it.
			prices[sym] = last.Close
			continue
		}
		ex := u.exchange(sym)
		floor, ceiling := rules.Band(ex, last.Close)
		closes := market.Closes(bars)
		highs := make([]float64, len(bars))
		lows := make([]float64, len(bars))
		for i, b := range bars {
			highs[i], lows[i] = b.High, b.Low
		}
		cd := Candidate{Symbol: sym, Exchange: ex, Close: last.Close, Floor: floor, Ceiling: ceiling, History: len(bars)}
		cd.SMA20, _ = indicators.SMA(closes, 20)
		cd.SMA50, _ = indicators.SMA(closes, 50)
		cd.RSI14, _ = indicators.RSI(closes, 14)
		cd.ATR14, _ = indicators.ATR(highs, lows, closes, 14)
		cd.Ret5, _ = indicators.Return(closes, 5)
		cd.Ret20, _ = indicators.Return(closes, 20)
		cd.VolRatio, _ = indicators.VolumeRatio(market.Volumes(bars), 20)
		c.Candidates = append(c.Candidates, cd)
		prices[sym] = last.Close
	}
	if len(c.Candidates) == 0 {
		return nil, nil, fmt.Errorf("engine: no candidate traded on %s", date.Format("2006-01-02"))
	}

	if idx, err := m.Bars(market.IndexSymbol, date, HistoryBars); err == nil {
		closes := market.Closes(idx)
		c.Index.Close = closes[len(closes)-1]
		c.Index.Ret5, _ = indicators.Return(closes, 5)
		c.Index.Ret20, _ = indicators.Return(closes, 20)
		c.Index.SMA50, _ = indicators.SMA(closes, 50)
	}

	// Account view, valued at today's closes.
	c.Cash = acct.Cash
	c.Available = acct.Available()
	for _, r := range acct.Receivables {
		c.Receivables += r.Amount
	}
	c.Equity, _ = acct.Equity(prices)
	for _, p := range acct.Holdings() {
		if px, ok := prices[p.Symbol]; ok {
			p.LastPrice = px
			p.Value = float64(p.Qty) * px
			p.PnL = p.Value - p.Cost
			if p.Cost > 0 {
				p.PnLPct = p.PnL / p.Cost
			}
		}
		c.Positions = append(c.Positions, p)
	}
	return c, prices, nil
}

// normalizeSymbol upper-cases and strips exchange suffixes a model may add
// ("fpt", "FPT.HM", "HOSE:FPT").
func normalizeSymbol(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, "."); i >= 0 {
		s = s[:i]
	}
	return s
}
