# NOFX, read from the source

Notes from reading [NoFxAiOS/nofx](https://github.com/NoFxAiOS/nofx) at `dev` on 2026-10-04 (commit of PR #1544). NOFX is the design OpenTrade borrows from; these notes record what it does, what it does not, and why OpenTrade diverges. A diagram is in [nofx-architecture.html](nofx-architecture.html).

## Shape

Go backend (Gin, gorm, SQLite) + React/TypeScript front end. ~660 files; the largest package is `trader/` (104 files: nine exchange adapters, grid trading, risk, throttle), then `provider/` (coinank, nofxos, vergex, twelvedata, alpaca data), `mcp/` (LLM clients and the Claw402 x402 payment layer), `api/`, `store/`, `kernel/` (strategy engine, prompt builder, decision schema).

## One cycle (`trader/auto_trader_loop.go`, `runCycle`)

1. Reload the strategy config if changed; every 10 cycles check the Claw402 USDC balance.
2. Create a `DecisionRecord`; check trading authorisation; honour a risk-control pause; reset daily P&L after 24 h.
3. `buildTradingContext`: balance, equity, positions (with peak P&L cache), candidate coins from the `StrategyEngine` (static list, AI500, OI top/low, Hyper rank, Vergex signal), 10 recent closed trades, full stats, quant data (OI, net flow), market-wide OI / net-flow / price rankings.
4. Save an equity snapshot (before anything can fail).
5. `kernel.GetFullDecisionWithStrategy`: system prompt (rules, JSON schema, OI interpretation table, common mistakes) + user prompt (the context) → one LLM call through `mcp.AIClient` → chain-of-thought text + JSON `Decision[]`.
6. Record the AI charge (per-call price, or the gateway-settled amount).
7. On error: count consecutive failures; after 3, **safe mode** (no new positions, existing SL/TP kept); keep looping.
8. Sort decisions (closes before opens), filter to the strategy universe (accepting `QNTUSDC` / `QNT-USDC` / `QNT` for the canonical `xyz:QNT`), apply the Vergex signal policy, drop opens in safe mode, throttle, then execute each decision with position-size and exit safeguards; stop if the user pressed Stop mid-loop.
9. Save the decision record with system prompt, user prompt, CoT, raw response and decisions.

A separate goroutine monitors per-position drawdown from peak and emergency-closes without asking the model.

## The decision schema (`kernel/engine.go`)

```go
type Decision struct {
    Symbol string; Action string // open_long, open_short, close_long, close_short, hold, wait
                                 // grid: place_buy_limit, place_sell_limit, cancel_order, …
    Leverage int; PositionSizeUSD, StopLoss, TakeProfit float64
    Price, Quantity float64; LevelIndex int; OrderID string
    Confidence int; RiskUSD float64; Reasoning string
}
```

The model chooses leverage and position size. Code only clamps them (`enforcePositionValueRatio`, `enforceMinPositionSize`, `enforceMaxPositions`).

## What NOFX does not have

- **No backtest or replay.** A strategy is a prompt; there is no way to run it over history. The leaderboard is self-reported live P&L.
- **No memory across cycles** beyond 10 recent trades and stats re-injected into the prompt.
- **No baseline.** Nothing compares the model to holding or to random trading.

## Business model, from the README

Referral links to seven exchanges ("fund continued development"), a per-call USDC model gateway (Claw402, x402 protocol), and the Vergex data stack behind candidate selection. The org page promises contributor airdrops "when NOFX generates revenue". AGPL-3.0.

## What OpenTrade keeps and changes

| NOFX | OpenTrade |
|---|---|
| cycle every few minutes | one decision per session (VN has T+2, no intraday round trips for retail) |
| crypto perps, leverage, shorts, 9 exchanges | VN cash equities, long only, lot/tick/band/T+2 in the engine |
| model picks leverage and USD size | model picks lots; code caps by cash and % of equity |
| no backtest | `replay` + scorecard vs VN-Index and random |
| safe mode after 3 failures | same |
| decision record with prompts, CoT, raw response | same, plus a structured guard log |
| referral / x402 / data-stack dependencies | none; any OpenAI-compatible endpoint, CSV data |
| AGPL-3.0 | MIT (no NOFX code) |
