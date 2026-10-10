// Package paper is the simulated broker. It holds cash and share lots,
// accepts orders after the close of day D, fills them against day D+1's
// bar, and applies T+2 settlement so a position cannot be sold, and sale
// proceeds cannot be spent, before they would be in a real account.
//
// It is deliberately conservative: fills are at the open or at the limit,
// never better; unsettled cash cannot be reused; there is no margin.
package paper

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/beiryu/opentrade/internal/market"
	"github.com/beiryu/opentrade/internal/rules"
)

// Side of an order.
type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

// Status of an order.
type Status string

const (
	Pending  Status = "pending"
	Filled   Status = "filled"
	Expired  Status = "expired" // day order not filled during D+1
	Rejected Status = "rejected"
)

// Order is a day order placed after the close of DecisionDate, live during
// the next session only.
type Order struct {
	ID           string    `json:"id"`
	DecisionDate time.Time `json:"decision_date"`
	Symbol       string    `json:"symbol"`
	Side         Side      `json:"side"`
	Qty          int       `json:"qty"`
	// LimitPrice 0 means "at the open" (an ATO-like market order).
	LimitPrice float64   `json:"limit_price"`
	Status     Status    `json:"status"`
	FillPrice  float64   `json:"fill_price,omitempty"`
	FillDate   time.Time `json:"fill_date,omitempty"`
	Fee        float64   `json:"fee,omitempty"`
	Tax        float64   `json:"tax,omitempty"`
	// Reason explains a rejection or expiry.
	Reason string `json:"reason,omitempty"`
	// Reserved is the cash held for a pending buy.
	Reserved float64 `json:"reserved,omitempty"`
}

// Lot is a parcel of shares bought together. Sellable from SettleDate.
type Lot struct {
	Symbol     string    `json:"symbol"`
	Qty        int       `json:"qty"`
	Price      float64   `json:"price"`
	BoughtOn   time.Time `json:"bought_on"`
	SettleDate time.Time `json:"settle_date"`
}

// Receivable is sale cash not yet available.
type Receivable struct {
	Amount      float64   `json:"amount"`
	AvailableOn time.Time `json:"available_on"`
}

// Position is the aggregate view of one symbol.
type Position struct {
	Symbol    string  `json:"symbol"`
	Qty       int     `json:"qty"`
	Settled   int     `json:"settled"` // sellable today
	AvgPrice  float64 `json:"avg_price"`
	Cost      float64 `json:"cost"`
	LastPrice float64 `json:"last_price,omitempty"`
	Value     float64 `json:"value,omitempty"`
	PnL       float64 `json:"pnl,omitempty"`
	PnLPct    float64 `json:"pnl_pct,omitempty"`
}

// Account is the whole state of the paper account. It is a plain value so
// the store can persist it as JSON.
type Account struct {
	InitialCash float64      `json:"initial_cash"`
	Cash        float64      `json:"cash"` // settled, spendable
	Receivables []Receivable `json:"receivables"`
	Lots        []Lot        `json:"lots"`
	Pending     []Order      `json:"pending"`
	// Today is the last trading day the account was rolled to.
	Today time.Time `json:"today"`
	seq   int
}

// NewAccount starts with cash only.
func NewAccount(cash float64) *Account {
	return &Account{InitialCash: cash, Cash: cash}
}

// Exchanger tells the account which board a symbol trades on.
type Exchanger interface {
	ExchangeOf(symbol string) rules.Exchange
}

// Reserved is the cash held for pending buys.
func (a *Account) Reserved() float64 {
	sum := 0.0
	for _, o := range a.Pending {
		sum += o.Reserved
	}
	return sum
}

// Available is the cash that can back a new buy right now.
func (a *Account) Available() float64 {
	return a.Cash - a.Reserved()
}

// Holdings aggregates lots per symbol, settled vs total as of a.Today.
func (a *Account) Holdings() []Position {
	m := map[string]*Position{}
	for _, l := range a.Lots {
		p := m[l.Symbol]
		if p == nil {
			p = &Position{Symbol: l.Symbol}
			m[l.Symbol] = p
		}
		p.Qty += l.Qty
		p.Cost += float64(l.Qty) * l.Price
		if !l.SettleDate.After(a.Today) {
			p.Settled += l.Qty
		}
	}
	out := make([]Position, 0, len(m))
	for _, p := range m {
		if p.Qty > 0 {
			p.AvgPrice = p.Cost / float64(p.Qty)
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}

// Position returns one symbol's aggregate (zero value when not held).
func (a *Account) Position(symbol string) Position {
	for _, p := range a.Holdings() {
		if p.Symbol == symbol {
			return p
		}
	}
	return Position{Symbol: symbol}
}

// SellablePending counts shares of a symbol already committed to pending
// sell orders, so a second sell cannot double-spend them.
func (a *Account) sellPending(symbol string) int {
	n := 0
	for _, o := range a.Pending {
		if o.Symbol == symbol && o.Side == Sell {
			n += o.Qty
		}
	}
	return n
}

// Equity values holdings at the given closes; symbols without a price are
// valued at cost. Receivables count as equity (they are cash in transit).
func (a *Account) Equity(prices map[string]float64) (equity, holdingsValue float64) {
	for _, l := range a.Lots {
		px, ok := prices[l.Symbol]
		if !ok {
			px = l.Price
		}
		holdingsValue += float64(l.Qty) * px
	}
	recv := 0.0
	for _, r := range a.Receivables {
		recv += r.Amount
	}
	return a.Cash + recv + holdingsValue, holdingsValue
}

// ErrRejected wraps validation failures so callers can tell them from bugs.
var ErrRejected = errors.New("paper: order rejected")

// Submit validates and queues a day order after the close of decisionDate.
// ref is that day's close (the next session's reference price). The order
// is checked against the board lot, the price band and tick, settled
// shares for sells and available cash for buys. On rejection the returned
// order carries Status Rejected and a Reason, and the error wraps
// ErrRejected.
func (a *Account) Submit(decisionDate time.Time, symbol string, side Side, qty int, limit float64, ref float64, ex rules.Exchange) (Order, error) {
	a.seq++
	o := Order{
		ID:           fmt.Sprintf("%s-%s-%d", decisionDate.Format("20060102"), strings.ToUpper(symbol), a.seq),
		DecisionDate: market.Day(decisionDate),
		Symbol:       strings.ToUpper(symbol),
		Side:         side,
		Qty:          qty,
		LimitPrice:   limit,
		Status:       Pending,
	}
	reject := func(reason string) (Order, error) {
		o.Status = Rejected
		o.Reason = reason
		return o, fmt.Errorf("%w: %s", ErrRejected, reason)
	}
	if side != Buy && side != Sell {
		return reject(fmt.Sprintf("unknown side %q", side))
	}
	if !rules.IsLotMultiple(qty) {
		return reject(fmt.Sprintf("quantity %d is not a multiple of %d", qty, rules.Lot))
	}
	if ref <= 0 {
		return reject("no reference price")
	}
	floor, ceiling := rules.Band(ex, ref)
	if limit != 0 {
		if !rules.IsOnTick(ex, limit) {
			return reject(fmt.Sprintf("limit %.0f is not on a %.0f tick", limit, rules.TickSize(ex, limit)))
		}
		if limit < floor || limit > ceiling {
			return reject(fmt.Sprintf("limit %.0f outside band %.0f–%.0f", limit, floor, ceiling))
		}
	}
	switch side {
	case Sell:
		p := a.Position(o.Symbol)
		free := p.Settled - a.sellPending(o.Symbol)
		if qty > free {
			return reject(fmt.Sprintf("only %d settled shares free to sell (T+2), asked %d", free, qty))
		}
	case Buy:
		// Reserve at the worst case: the limit, or the ceiling for a market
		// order, plus fee.
		worst := limit
		if worst == 0 {
			worst = ceiling
		}
		need := rules.BuyCost(worst * float64(qty))
		if need > a.Available()+1e-6 {
			return reject(fmt.Sprintf("needs %.0f VND, only %.0f available", need, a.Available()))
		}
		o.Reserved = need
	}
	a.Pending = append(a.Pending, o)
	return o, nil
}

// Roll advances the account to trading day `day`: pending orders are filled
// or expired against that day's bars, lots and receivables whose settlement
// date has come are released. cal is the trading calendar used to compute
// T+2 dates. Returns the orders that were resolved.
func (a *Account) Roll(day time.Time, bars map[string]market.Bar, cal []time.Time) []Order {
	day = market.Day(day)
	var resolved []Order
	settle := settleDate(cal, day, rules.SettlementDays)
	for _, o := range a.Pending {
		b, ok := bars[o.Symbol]
		if !ok {
			o.Status = Expired
			o.Reason = "no bar for " + day.Format("2006-01-02")
			resolved = append(resolved, o)
			continue
		}
		price, filled := fillPrice(o, b)
		if !filled {
			o.Status = Expired
			o.Reason = fmt.Sprintf("limit %.0f not reached (open %.0f, low %.0f, high %.0f)", o.LimitPrice, b.Open, b.Low, b.High)
			resolved = append(resolved, o)
			continue
		}
		notional := price * float64(o.Qty)
		o.Status = Filled
		o.FillPrice = price
		o.FillDate = day
		switch o.Side {
		case Buy:
			o.Fee = notional * rules.FeeRate
			a.Cash -= notional + o.Fee
			a.Lots = append(a.Lots, Lot{Symbol: o.Symbol, Qty: o.Qty, Price: price, BoughtOn: day, SettleDate: settle})
		case Sell:
			o.Fee = notional * rules.FeeRate
			o.Tax = notional * rules.SellTaxRate
			a.removeShares(o.Symbol, o.Qty)
			a.Receivables = append(a.Receivables, Receivable{Amount: notional - o.Fee - o.Tax, AvailableOn: settle})
		}
		o.Reserved = 0
		resolved = append(resolved, o)
	}
	a.Pending = nil
	a.Today = day
	// Release receivables that have settled.
	var keep []Receivable
	for _, r := range a.Receivables {
		if !r.AvailableOn.After(day) {
			a.Cash += r.Amount
		} else {
			keep = append(keep, r)
		}
	}
	a.Receivables = keep
	return resolved
}

// fillPrice applies the day-order fill model.
func fillPrice(o Order, b market.Bar) (float64, bool) {
	if o.LimitPrice == 0 {
		return b.Open, true
	}
	switch o.Side {
	case Buy:
		if b.Open <= o.LimitPrice {
			return b.Open, true
		}
		if b.Low <= o.LimitPrice {
			return o.LimitPrice, true
		}
	case Sell:
		if b.Open >= o.LimitPrice {
			return b.Open, true
		}
		if b.High >= o.LimitPrice {
			return o.LimitPrice, true
		}
	}
	return 0, false
}

// removeShares takes qty shares of symbol out of the oldest settled lots
// first (FIFO). Submit already checked they exist.
func (a *Account) removeShares(symbol string, qty int) {
	sort.SliceStable(a.Lots, func(i, j int) bool { return a.Lots[i].BoughtOn.Before(a.Lots[j].BoughtOn) })
	var keep []Lot
	for _, l := range a.Lots {
		if l.Symbol != symbol || qty == 0 || l.SettleDate.After(a.Today) {
			keep = append(keep, l)
			continue
		}
		take := int(math.Min(float64(l.Qty), float64(qty)))
		l.Qty -= take
		qty -= take
		if l.Qty > 0 {
			keep = append(keep, l)
		}
	}
	a.Lots = keep
}

// settleDate is the trading day n sessions after day on the calendar; when
// the calendar runs out it extrapolates by weekdays.
func settleDate(cal []time.Time, day time.Time, n int) time.Time {
	i := sort.Search(len(cal), func(i int) bool { return !cal[i].Before(day) })
	if i < len(cal) && cal[i].Equal(day) && i+n < len(cal) {
		return cal[i+n]
	}
	d := day
	for k := 0; k < n; {
		d = d.AddDate(0, 0, 1)
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			k++
		}
	}
	return d
}
