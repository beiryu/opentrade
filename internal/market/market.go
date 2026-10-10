// Package market defines the price data the engine reads and the providers
// that supply it. Daily bars only: OpenTrade decides once per session, so
// intraday ticks are out of scope for now.
package market

import (
	"errors"
	"sort"
	"time"
)

// IndexSymbol is the pseudo-symbol for the VN-Index series, used as the
// market context in prompts and as the buy-and-hold benchmark.
const IndexSymbol = "VNINDEX"

// Bar is one trading day of a symbol. Prices in VND, volume in shares.
type Bar struct {
	Date   time.Time `json:"date"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume int64     `json:"volume"`
}

// ErrNoData is returned when a symbol has no bars on or before a date.
var ErrNoData = errors.New("market: no data")

// Provider is a source of daily bars. Implementations: the CSV directory
// provider (sample and offline data); real brokers' data APIs later.
type Provider interface {
	// Symbols lists every symbol the provider has, index included.
	Symbols() []string
	// Bars returns up to n bars for the symbol dated on or before `until`,
	// oldest first. Fewer than n when history is short; ErrNoData when none.
	Bars(symbol string, until time.Time, n int) ([]Bar, error)
	// Calendar returns the trading days the provider knows, ascending. It is
	// the index's dates when the index is present, otherwise the union of
	// all symbols' dates.
	Calendar() []time.Time
}

// Day truncates a time to the date in UTC. All bar dates are days.
func Day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// NextTradingDay returns the first calendar day strictly after d, ok=false
// when d is the last day known.
func NextTradingDay(cal []time.Time, d time.Time) (time.Time, bool) {
	i := sort.Search(len(cal), func(i int) bool { return cal[i].After(d) })
	if i >= len(cal) {
		return time.Time{}, false
	}
	return cal[i], true
}

// TradingDaysBetween returns the calendar days in [from, to] inclusive.
func TradingDaysBetween(cal []time.Time, from, to time.Time) []time.Time {
	var out []time.Time
	for _, d := range cal {
		if d.Before(from) || d.After(to) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// Closes extracts the close series of bars, oldest first.
func Closes(bars []Bar) []float64 {
	out := make([]float64, len(bars))
	for i, b := range bars {
		out[i] = b.Close
	}
	return out
}

// Volumes extracts the volume series as floats.
func Volumes(bars []Bar) []float64 {
	out := make([]float64, len(bars))
	for i, b := range bars {
		out[i] = float64(b.Volume)
	}
	return out
}
