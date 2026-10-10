// Package engine runs the decision cycle: build what the model sees, ask a
// Decider for orders, pass them through the guards, hand them to the paper
// broker and record everything. The LLM is one Decider among several; the
// momentum and random deciders exist so a model's result can be compared
// with something, which is the point of OpenTrade.
package engine

import (
	"time"

	"github.com/beiryu/opentrade/internal/paper"
	"github.com/beiryu/opentrade/internal/rules"
	"github.com/beiryu/opentrade/internal/store"
)

// Action a decider may request. "hold" is accepted and ignored so a model
// can be explicit about doing nothing.
type Action string

const (
	Buy  Action = "buy"
	Sell Action = "sell"
	Hold Action = "hold"
)

// Decision is one requested order, in lots (×100 shares).
type Decision struct {
	Symbol     string  `json:"symbol"`
	Action     Action  `json:"action"`
	Lots       int     `json:"lots"`
	LimitPrice float64 `json:"limit_price,omitempty"`
	Confidence int     `json:"confidence,omitempty"`
	Reasoning  string  `json:"reasoning"`
}

// Response is what a decider returns.
type Response struct {
	Decisions []Decision `json:"decisions"`
	Notes     string     `json:"notes,omitempty"`
}

// Candidate is one tradable symbol as presented to the decider: today's
// close, tomorrow's band and a few indicators computed from the history.
type Candidate struct {
	Symbol   string         `json:"symbol"`
	Exchange rules.Exchange `json:"exchange"`
	Close    float64        `json:"close"`
	Floor    float64        `json:"floor"`   // next session
	Ceiling  float64        `json:"ceiling"` // next session
	SMA20    float64        `json:"sma20"`
	SMA50    float64        `json:"sma50"`
	RSI14    float64        `json:"rsi14"`
	ATR14    float64        `json:"atr14"`
	Ret5     float64        `json:"ret5"`
	Ret20    float64        `json:"ret20"`
	VolRatio float64        `json:"vol_ratio"`
	History  int            `json:"history"` // bars available
}

// Limits are the code-side risk limits, repeated to the model so it does
// not waste a decision on something the guards would cut.
type Limits struct {
	MaxPositions      int     `json:"max_positions"`
	MaxPositionPct    float64 `json:"max_position_pct"`
	MaxOrdersPerCycle int     `json:"max_orders_per_cycle"`
}

// IndexView is the market context.
type IndexView struct {
	Close float64 `json:"close"`
	Ret5  float64 `json:"ret5"`
	Ret20 float64 `json:"ret20"`
	SMA50 float64 `json:"sma50"`
}

// Context is everything a decider gets for one cycle.
type Context struct {
	Date        time.Time        `json:"date"`
	NextSession time.Time        `json:"next_session"`
	Cash        float64          `json:"cash"`
	Available   float64          `json:"available"`
	Receivables float64          `json:"receivables"`
	Equity      float64          `json:"equity"`
	Positions   []paper.Position `json:"positions"`
	Index       IndexView        `json:"index"`
	Candidates  []Candidate      `json:"candidates"`
	Recent      []store.Order    `json:"recent_orders"`
	Limits      Limits           `json:"limits"`
	SafeMode    bool             `json:"safe_mode"`
	CycleNo     int              `json:"cycle_no"`
}

// Candidate looks up a symbol.
func (c *Context) Candidate(symbol string) (Candidate, bool) {
	for _, cd := range c.Candidates {
		if cd.Symbol == symbol {
			return cd, true
		}
	}
	return Candidate{}, false
}

// Position looks up a holding.
func (c *Context) Position(symbol string) (paper.Position, bool) {
	for _, p := range c.Positions {
		if p.Symbol == symbol {
			return p, true
		}
	}
	return paper.Position{}, false
}

// GuardEntry explains one change the guards made.
type GuardEntry struct {
	Symbol string `json:"symbol"`
	Action Action `json:"action"`
	Rule   string `json:"rule"`
	Detail string `json:"detail"`
}
