package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/beiryu/opentrade/internal/paper"
	"github.com/beiryu/opentrade/internal/rules"
)

// ApplyGuards turns a decider's response into orders the broker will accept,
// or explains why not. It never trusts the model: symbols are normalised
// and checked against the candidates, quantities are trimmed to what
// settled shares and available cash allow, prices are put on a tick inside
// the band, the position and order limits are applied, and safe mode drops
// every buy. The log is stored with the decision so the UI can show what
// the model asked for and what actually went out.
func ApplyGuards(c *Context, resp Response, acct *paper.Account) ([]Decision, []GuardEntry) {
	var log []GuardEntry
	note := func(d Decision, rule, detail string) {
		log = append(log, GuardEntry{Symbol: d.Symbol, Action: d.Action, Rule: rule, Detail: detail})
	}

	var sells, buys []Decision
	seen := map[string]bool{}
	available := acct.Available()
	nPositions := len(c.Positions)

	for _, d := range resp.Decisions {
		d.Symbol = normalizeSymbol(d.Symbol)
		d.Action = Action(strings.ToLower(strings.TrimSpace(string(d.Action))))
		switch d.Action {
		case Hold:
			continue
		case Buy, Sell:
		default:
			note(d, "unknown_action", fmt.Sprintf("action %q is not buy/sell/hold", d.Action))
			continue
		}
		cd, ok := c.Candidate(d.Symbol)
		if !ok {
			note(d, "not_in_universe", "symbol is not in today's candidate list")
			continue
		}
		if seen[d.Symbol] {
			note(d, "duplicate", "second order for the same symbol dropped")
			continue
		}
		if d.Lots <= 0 {
			note(d, "no_quantity", fmt.Sprintf("lots must be positive, got %d", d.Lots))
			continue
		}
		// Price: models sometimes quote thousands. A limit below a tenth of
		// the close is almost certainly that; fix it and say so.
		if d.LimitPrice > 0 && d.LimitPrice < cd.Close/10 {
			fixed := d.LimitPrice * 1000
			note(d, "price_scaled", fmt.Sprintf("limit %.3f read as thousands → %.0f", d.LimitPrice, fixed))
			d.LimitPrice = fixed
		}
		if d.LimitPrice > 0 {
			onTick := rules.RoundToTick(cd.Exchange, d.LimitPrice)
			if onTick != d.LimitPrice {
				note(d, "tick_rounded", fmt.Sprintf("limit %.0f → %.0f", d.LimitPrice, onTick))
				d.LimitPrice = onTick
			}
			if d.LimitPrice < cd.Floor || d.LimitPrice > cd.Ceiling {
				clamped := d.LimitPrice
				if clamped < cd.Floor {
					clamped = cd.Floor
				} else {
					clamped = cd.Ceiling
				}
				note(d, "band_clamped", fmt.Sprintf("limit %.0f outside %.0f–%.0f → %.0f", d.LimitPrice, cd.Floor, cd.Ceiling, clamped))
				d.LimitPrice = clamped
			}
		}
		seen[d.Symbol] = true

		switch d.Action {
		case Sell:
			p, held := c.Position(d.Symbol)
			free := p.Settled / rules.Lot
			if !held || free == 0 {
				note(d, "nothing_to_sell", fmt.Sprintf("no settled shares of %s (held %d, settled %d)", d.Symbol, p.Qty, p.Settled))
				continue
			}
			if d.Lots > free {
				note(d, "trimmed_to_settled", fmt.Sprintf("%d lots → %d settled lots (T+2)", d.Lots, free))
				d.Lots = free
			}
			sells = append(sells, d)
		case Buy:
			if c.SafeMode {
				note(d, "safe_mode", "buys are suspended while the decider keeps failing")
				continue
			}
			p, held := c.Position(d.Symbol)
			if !held && nPositions+len(buys) >= c.Limits.MaxPositions {
				note(d, "max_positions", fmt.Sprintf("already %d of %d positions", nPositions+len(buys), c.Limits.MaxPositions))
				continue
			}
			worst := d.LimitPrice
			if worst == 0 {
				worst = cd.Ceiling
			}
			// Position size cap: existing value + new ≤ pct × equity.
			capValue := c.Equity*c.Limits.MaxPositionPct - p.Value
			maxByCap := int(capValue / (worst * rules.Lot))
			if maxByCap < d.Lots {
				if maxByCap <= 0 {
					note(d, "max_position_pct", fmt.Sprintf("%s already at %.0f%% of equity", d.Symbol, p.Value/c.Equity*100))
					continue
				}
				note(d, "trimmed_to_position_cap", fmt.Sprintf("%d lots → %d (max %.0f%% of equity per symbol)", d.Lots, maxByCap, c.Limits.MaxPositionPct*100))
				d.Lots = maxByCap
			}
			maxByCash := rules.MaxAffordableLots(available, worst)
			if maxByCash < d.Lots {
				if maxByCash == 0 {
					note(d, "no_cash", fmt.Sprintf("needs %.0f per lot, %.0f available", rules.BuyCost(worst*rules.Lot), available))
					continue
				}
				note(d, "trimmed_to_cash", fmt.Sprintf("%d lots → %d affordable", d.Lots, maxByCash))
				d.Lots = maxByCash
			}
			available -= rules.BuyCost(worst * float64(d.Lots*rules.Lot))
			buys = append(buys, d)
		}
	}

	// Sells first (they free nothing today because of T+2, but they are the
	// risk-reducing side), then buys by confidence.
	sort.SliceStable(buys, func(i, j int) bool { return buys[i].Confidence > buys[j].Confidence })
	out := append(sells, buys...)
	if max := c.Limits.MaxOrdersPerCycle; max > 0 && len(out) > max {
		for _, d := range out[max:] {
			note(d, "max_orders", fmt.Sprintf("more than %d orders in one day", max))
		}
		out = out[:max]
	}
	return out, log
}
