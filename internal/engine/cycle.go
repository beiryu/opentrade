package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/beiryu/opentrade/internal/market"
	"github.com/beiryu/opentrade/internal/paper"
	"github.com/beiryu/opentrade/internal/store"
)

// SafeModeAfter is how many consecutive decider failures suspend buys.
const SafeModeAfter = 3

// State is what the engine remembers between cycles, persisted in the store.
type State struct {
	CycleNo             int       `json:"cycle_no"`
	LastDate            time.Time `json:"last_date"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	SafeMode            bool      `json:"safe_mode"`
	Decider             string    `json:"decider"`
}

const (
	stateKey   = "engine"
	accountKey = "account"
)

// Engine wires market data, a decider, the paper account and the store.
type Engine struct {
	Market   market.Provider
	Universe Universe
	Store    *store.Store
	Decider  Decider
	Limits   Limits
	Account  *paper.Account
	State    State
	// Logf receives progress lines; nil silences them.
	Logf func(format string, args ...any)
}

func (e *Engine) logf(format string, args ...any) {
	if e.Logf != nil {
		e.Logf(format, args...)
	}
}

// Load restores the account and state from the store; a fresh store
// leaves the account as given and the state zeroed.
func (e *Engine) Load(ctx context.Context) error {
	var acct paper.Account
	if ok, err := e.Store.LoadState(ctx, accountKey, &acct); err != nil {
		return err
	} else if ok {
		e.Account = &acct
	}
	if _, err := e.Store.LoadState(ctx, stateKey, &e.State); err != nil {
		return err
	}
	return nil
}

// Save persists the account and state.
func (e *Engine) Save(ctx context.Context) error {
	if err := e.Store.SaveState(ctx, accountKey, e.Account); err != nil {
		return err
	}
	return e.Store.SaveState(ctx, stateKey, e.State)
}

// CycleResult summarises one decision.
type CycleResult struct {
	Date       time.Time     `json:"date"`
	DecisionID int64         `json:"decision_id"`
	Decisions  []Decision    `json:"decisions"`
	Orders     []paper.Order `json:"orders"`
	GuardLog   []GuardEntry  `json:"guard_log"`
	Error      string        `json:"error,omitempty"`
	SafeMode   bool          `json:"safe_mode"`
	Equity     float64       `json:"equity"`
}

// Roll moves the account to trading day `day`: fills or expires the
// pending orders against that day's bars, settles T+2 and records the
// equity at the close. Call it for every trading day, before Cycle.
func (e *Engine) Roll(ctx context.Context, day time.Time) ([]paper.Order, error) {
	day = market.Day(day)
	bars := map[string]market.Bar{}
	prices := map[string]float64{}
	for _, sym := range e.Universe.Symbols {
		b, err := e.Market.Bars(sym, day, 1)
		if err != nil {
			continue
		}
		if b[0].Date.Equal(day) {
			bars[sym] = b[0]
		}
		prices[sym] = b[0].Close
	}
	resolved := e.Account.Roll(day, bars, e.Market.Calendar())
	for _, o := range resolved {
		if err := e.Store.UpsertOrder(ctx, toStoreOrder(o, 0)); err != nil {
			return nil, err
		}
		e.logf("  %s %s %s %d → %s %s", day.Format("01-02"), o.Side, o.Symbol, o.Qty, o.Status, fillInfo(o))
	}
	if err := e.recordEquity(ctx, day, prices); err != nil {
		return nil, err
	}
	return resolved, e.Save(ctx)
}

func fillInfo(o paper.Order) string {
	if o.Status == paper.Filled {
		return fmt.Sprintf("@ %.0f", o.FillPrice)
	}
	return o.Reason
}

func (e *Engine) recordEquity(ctx context.Context, day time.Time, prices map[string]float64) error {
	equity, holdings := e.Account.Equity(prices)
	var idx float64
	if b, err := e.Market.Bars(market.IndexSymbol, day, 1); err == nil {
		idx = b[0].Close
	}
	return e.Store.UpsertEquity(ctx, store.EquityPoint{Date: day, Cash: e.Account.Cash, Holdings: holdings, Equity: equity, IndexClose: idx})
}

// Cycle takes the decision for trading day `day` (after its close) and
// queues the resulting orders for the next session. The decider's failure
// is not the cycle's failure: it is recorded, counted towards safe mode,
// and the cycle returns normally with no orders.
func (e *Engine) Cycle(ctx context.Context, day time.Time) (*CycleResult, error) {
	day = market.Day(day)
	e.State.CycleNo++
	recent, err := e.Store.ListOrders(ctx, 10)
	if err != nil {
		return nil, err
	}
	c, prices, err := BuildContext(ctx, e.Market, e.Universe, day, e.Account, recent, e.Limits, e.State.SafeMode, e.State.CycleNo)
	if err != nil {
		return nil, err
	}
	res := &CycleResult{Date: day, SafeMode: e.State.SafeMode, Equity: c.Equity}
	rec := &store.Decision{Date: day, Decider: e.Decider.Name()}

	out, derr := e.Decider.Decide(ctx, c)
	rec.Model, rec.SystemPrompt, rec.UserPrompt, rec.RawResponse = out.Model, out.SystemPrompt, out.UserPrompt, out.Raw
	rec.LatencyMs, rec.PromptTokens, rec.CompletionTokens = out.LatencyMs, out.PromptTokens, out.CompletionTokens

	var decisions []Decision
	var guardLog []GuardEntry
	if derr != nil {
		if errors.Is(derr, context.Canceled) || errors.Is(derr, context.DeadlineExceeded) && ctx.Err() != nil {
			return nil, derr
		}
		rec.Error = derr.Error()
		res.Error = derr.Error()
		e.State.ConsecutiveFailures++
		if e.State.ConsecutiveFailures >= SafeModeAfter && !e.State.SafeMode {
			e.State.SafeMode = true
			e.logf("  SAFE MODE on after %d failures: %v", e.State.ConsecutiveFailures, derr)
		}
	} else {
		if e.State.ConsecutiveFailures > 0 || e.State.SafeMode {
			e.logf("  decider recovered; safe mode off")
		}
		e.State.ConsecutiveFailures = 0
		e.State.SafeMode = false
		c.SafeMode = false // recovery lifts safe mode for this very cycle
		res.SafeMode = false
		decisions, guardLog = ApplyGuards(c, out.Response, e.Account)
	}

	// Submit through the broker; its own rejections are logged too.
	var orders []paper.Order
	for _, d := range decisions {
		cd, _ := c.Candidate(d.Symbol)
		o, err := e.Account.Submit(day, d.Symbol, paper.Side(d.Action), d.Lots*100, d.LimitPrice, cd.Close, cd.Exchange)
		if err != nil {
			guardLog = append(guardLog, GuardEntry{Symbol: d.Symbol, Action: d.Action, Rule: "broker_rejected", Detail: o.Reason})
		}
		orders = append(orders, o)
	}
	dj, _ := json.Marshal(decisions)
	gj, _ := json.Marshal(guardLog)
	if decisions == nil {
		dj = []byte("[]")
	}
	if guardLog == nil {
		gj = []byte("[]")
	}
	rec.DecisionsJSON, rec.GuardLog = string(dj), string(gj)
	id, err := e.Store.InsertDecision(ctx, rec)
	if err != nil {
		return nil, err
	}
	for _, o := range orders {
		if err := e.Store.UpsertOrder(ctx, toStoreOrder(o, id)); err != nil {
			return nil, err
		}
	}
	res.DecisionID, res.Decisions, res.Orders, res.GuardLog = id, decisions, orders, guardLog
	e.State.LastDate = day
	if err := e.recordEquity(ctx, day, prices); err != nil {
		return nil, err
	}
	e.logf("%s cycle %d: %d decision(s), %d order(s), equity %.0f%s", day.Format("2006-01-02"), e.State.CycleNo, len(decisions), len(orders), c.Equity, errSuffix(res.Error))
	return res, e.Save(ctx)
}

func errSuffix(s string) string {
	if s == "" {
		return ""
	}
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return " | ERROR " + s
}

// Step runs the next trading day after the last one processed: roll then
// cycle. ok=false when there is no further day in the data.
func (e *Engine) Step(ctx context.Context) (*CycleResult, bool, error) {
	cal := e.Market.Calendar()
	var day time.Time
	if e.State.LastDate.IsZero() {
		if len(cal) == 0 {
			return nil, false, errors.New("engine: no trading days")
		}
		day = cal[0]
	} else {
		next, ok := market.NextTradingDay(cal, e.State.LastDate)
		if !ok {
			return nil, false, nil
		}
		day = next
	}
	if _, err := e.Roll(ctx, day); err != nil {
		return nil, false, err
	}
	res, err := e.Cycle(ctx, day)
	return res, true, err
}

// Replay wipes the store and runs roll+cycle for every trading day in
// [from, to]. The account starts from initialCash.
func (e *Engine) Replay(ctx context.Context, from, to time.Time, initialCash float64) error {
	if err := e.Store.Reset(ctx); err != nil {
		return err
	}
	e.Account = paper.NewAccount(initialCash)
	e.State = State{Decider: e.Decider.Name()}
	days := market.TradingDaysBetween(e.Market.Calendar(), market.Day(from), market.Day(to))
	if len(days) == 0 {
		return fmt.Errorf("engine: no trading days between %s and %s", from.Format("2006-01-02"), to.Format("2006-01-02"))
	}
	for _, day := range days {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := e.Roll(ctx, day); err != nil {
			return err
		}
		if _, err := e.Cycle(ctx, day); err != nil {
			return err
		}
	}
	return nil
}

func toStoreOrder(o paper.Order, decisionID int64) store.Order {
	so := store.Order{
		ID: o.ID, DecisionID: decisionID, DecisionDate: o.DecisionDate, Symbol: o.Symbol, Side: string(o.Side),
		Qty: o.Qty, LimitPrice: o.LimitPrice, Status: string(o.Status), FillPrice: o.FillPrice, Fee: o.Fee, Tax: o.Tax, Reason: o.Reason,
	}
	if !o.FillDate.IsZero() {
		so.FillDate = o.FillDate.Format("2006-01-02")
	}
	return so
}

// StdLogf is a Logf that writes to the standard logger.
func StdLogf(format string, args ...any) { log.Printf(format, args...) }
