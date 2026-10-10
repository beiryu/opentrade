package engine

import (
	"fmt"
	"strings"
)

// SystemPrompt is the stable part of every LLM call: role, the rules of
// the Vietnamese market the model must respect, the exact output schema and
// the mistakes we have seen models make. Keep it stable so providers can
// cache it; everything that changes per day goes in the user prompt.
func SystemPrompt(l Limits) string {
	var b strings.Builder
	b.WriteString(`You are the decision engine of OpenTrade, a paper-trading terminal for the Vietnamese stock market (HOSE, HNX, UPCOM). Once per trading day, after the close, you receive the account, the market and a fixed list of candidate stocks, and you return the orders to place in the NEXT session.

Market rules you must respect:
- Long only. No short selling, no margin, no derivatives. You can only sell shares you hold and that have settled.
- Settlement is T+2: shares bought today can be sold from the second trading day after; cash from a sale is spendable from the second trading day after.
- Board lot is 100 shares. Quantities are expressed in LOTS (1 lot = 100 shares).
- Each session has a price band around the reference price (previous close): ±7 % on HOSE, ±10 % on HNX, ±15 % on UPCOM. The floor and ceiling for the next session are given per candidate. A limit price outside the band is invalid.
- Tick size on HOSE: 10 VND below 10,000; 50 VND from 10,000 to below 50,000; 100 VND from 50,000. HNX and UPCOM: 100 VND. Prices are in VND.
- Costs: 0.15 % brokerage fee on every trade and 0.1 % tax on sale proceeds. A round trip costs about 0.4 %; do not churn.
- Orders are day orders for the next session. limit_price 0 means "at the opening price".

`)
	fmt.Fprintf(&b, `Risk limits enforced by code after you answer (orders beyond them are trimmed or dropped):
- At most %d open positions.
- At most %.0f %% of equity in one symbol.
- At most %d orders per day.
- When safe_mode is true, only sells are executed.

`, l.MaxPositions, l.MaxPositionPct*100, l.MaxOrdersPerCycle)
	b.WriteString(`Trade only symbols in the candidate list, spelled exactly as given. Doing nothing is a valid answer; most days should have zero or one order. Size positions by conviction and by the limits above.

Respond with ONE JSON object and nothing else, in this exact shape:
{
  "decisions": [
    {"symbol": "FPT", "action": "buy", "lots": 2, "limit_price": 0, "confidence": 70, "reasoning": "one or two sentences"}
  ],
  "notes": "optional one-line view of the market"
}
Fields: action is "buy" or "sell" (list only the orders you want; do not list holds); lots is a positive integer; limit_price is a VND price on a valid tick inside the band, or 0 for the open; confidence is 0–100; reasoning is at most 30 words. Keep the whole reply under 1500 characters so it is never cut off.

Common mistakes to avoid: inventing a symbol or a suffix ("FPT.HM"); quoting prices in thousands (write 120000, not 120.0); selling shares that have not settled (check the "settled" count); buying with cash that is still receivable; more than one order for the same symbol; returning prose or markdown around the JSON.`)
	return b.String()
}

// UserPrompt renders the day's context as structured text. Numbers are
// rounded so the prompt stays compact and stable.
func UserPrompt(c *Context) string {
	var b strings.Builder
	fmt.Fprintf(&b, "DATE %s (decided after the close). Orders execute in the session of %s.\n", c.Date.Format("2006-01-02"), c.NextSession.Format("2006-01-02"))
	fmt.Fprintf(&b, "CYCLE %d. SAFE_MODE %v.\n\n", c.CycleNo, c.SafeMode)

	fmt.Fprintf(&b, "ACCOUNT (VND)\ncash_available %s\nreceivables_T+2 %s\nequity %s\npositions %d of max %d\n\n",
		vnd(c.Available), vnd(c.Receivables), vnd(c.Equity), len(c.Positions), c.Limits.MaxPositions)

	b.WriteString("POSITIONS\n")
	if len(c.Positions) == 0 {
		b.WriteString("(none)\n")
	}
	for _, p := range c.Positions {
		fmt.Fprintf(&b, "%s qty %d (settled %d) avg %s last %s pnl %+.1f%% value %s\n",
			p.Symbol, p.Qty, p.Settled, vnd(p.AvgPrice), vnd(p.LastPrice), p.PnLPct*100, vnd(p.Value))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "MARKET\nVNINDEX %.1f ret5 %+.1f%% ret20 %+.1f%% sma50 %.1f\n\n", c.Index.Close, c.Index.Ret5*100, c.Index.Ret20*100, c.Index.SMA50)

	b.WriteString("CANDIDATES (prices VND; floor/ceiling are for the next session)\n")
	b.WriteString("symbol close floor ceiling sma20 sma50 rsi14 ret5 ret20 vol_ratio atr14 history\n")
	for _, cd := range c.Candidates {
		fmt.Fprintf(&b, "%s %s %s %s %s %s %.0f %+.1f%% %+.1f%% %.2f %s %d\n",
			cd.Symbol, vnd(cd.Close), vnd(cd.Floor), vnd(cd.Ceiling), vnd(cd.SMA20), vnd(cd.SMA50),
			cd.RSI14, cd.Ret5*100, cd.Ret20*100, cd.VolRatio, vnd(cd.ATR14), cd.History)
	}
	b.WriteString("\n")

	b.WriteString("RECENT ORDERS (newest first)\n")
	if len(c.Recent) == 0 {
		b.WriteString("(none)\n")
	}
	for i, o := range c.Recent {
		if i >= 10 {
			break
		}
		fmt.Fprintf(&b, "%s %s %s %d sh %s", o.DecisionDate.Format("2006-01-02"), o.Side, o.Symbol, o.Qty, o.Status)
		if o.Status == "filled" {
			fmt.Fprintf(&b, " @ %s", vnd(o.FillPrice))
		} else if o.Reason != "" {
			fmt.Fprintf(&b, " (%s)", o.Reason)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nReturn the JSON object now.")
	return b.String()
}

// vnd formats a price or amount with thousands separators and no decimals.
func vnd(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%.0f", v)
	var out []byte
	for i, ch := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, ch)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
