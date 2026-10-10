# Architecture

OpenTrade is one Go binary (API + CLI, web app embedded) and one SQLite file. There are no services, queues or background workers: a decision is a function call, and `replay` is that call in a loop.

```
                 ┌──────────────────────────────────────────────────────────────┐
  data/*.csv ───▶│ market.Provider          (daily bars, trading calendar)      │
                 └───────────────┬──────────────────────────────────────────────┘
                                 │ bars until day D
                                 ▼
 ┌──────────────────────── engine.Cycle(D) ─────────────────────────────────────┐
 │ 1. BuildContext   account · VN-Index · candidates + indicators · recent orders│
 │ 2. Decider        llm (prompt → OpenAI-compatible endpoint → JSON)            │
 │                   momentum (rule)   random (seeded)                           │
 │ 3. ApplyGuards    universe · dedupe · tick/band · settled qty · cash ·        │
 │                   position cap · max positions/orders · safe mode            │
 │ 4. paper.Submit   broker-side validation, cash reservation → pending orders  │
 │ 5. store          decision record (prompts, raw reply, guard log), orders,   │
 │                   equity point, account + engine state                        │
 └──────────────────────────────────────────────────────────────────────────────┘
                                 │ next trading day
                                 ▼
 ┌──────────────────────── engine.Roll(D+1) ────────────────────────────────────┐
 │ paper.Roll   fill/expire pending orders on D+1's bar · T+2 settlement ·      │
 │              receivables → cash · equity at the close → store                │
 └──────────────────────────────────────────────────────────────────────────────┘

 replay(from, to) = for each trading day: Roll(day); Cycle(day)   → scorecard
 serve            = HTTP API over the same engine + embedded React app
```

## Packages

| Package | Responsibility | Key types |
|---|---|---|
| `internal/rules` | Market rules as pure functions: `Lot`, `TickSize`, `Band`, `IsOnTick`, `BuyCost`, `SellProceeds`, `MaxAffordableLots`, `SettlementDays` | `Exchange` |
| `internal/market` | Bars and providers. `CSVProvider` loads a directory; `GenerateSynthetic` writes sample data | `Bar`, `Provider` |
| `internal/indicators` | SMA, EMA, RSI, ATR, N-day return, volume ratio, max drawdown. Oldest-first slices, `ok=false` on short input | |
| `internal/engine` | The cycle. `BuildContext`, `SystemPrompt`/`UserPrompt`, `Decider` implementations, `ApplyGuards`, `Engine.Cycle/Roll/Step/Replay` | `Context`, `Decision`, `GuardEntry`, `State` |
| `internal/paper` | Simulated broker: day orders, fill model, FIFO lots with settlement dates, receivables, cash reservation | `Account`, `Order`, `Lot` |
| `internal/llm` | OpenAI-compatible chat client with JSON mode and retries; `Fake` for tests; `ExtractJSON` | `Client` |
| `internal/store` | SQLite via `modernc.org/sqlite`; schema in `migrations.sql`; decisions, orders, equity, JSON state | `Decision`, `Order`, `EquityPoint` |
| `internal/scorecard` | Metrics per curve, FIFO win rate from orders, buy-and-hold benchmark, verdict | `Metrics`, `Card` |
| `internal/api` | `net/http` mux (Go 1.22 patterns), JSON handlers, SPA fallback; `ui` embeds `web/dist` | `Server` |
| `internal/config` | `OPENTRADE_*` environment and a local `.env` | `Config` |
| `cmd/opentrade` | CLI: `seed`, `replay`, `cycle`, `serve`, `version` | |
| `web` | Vite + React 19 + Tailwind 4; three pages over the API | |

## The decision contract

The decider returns:

```json
{"decisions":[{"symbol":"FPT","action":"buy","lots":2,"limit_price":0,"confidence":70,"reasoning":"…"}],"notes":"…"}
```

`lots` are board lots (×100 shares). `limit_price` is VND on a valid tick inside the next session's band, or 0 for the open. Anything else is normalised or dropped by `ApplyGuards`, and each change is a `GuardEntry{symbol, action, rule, detail}` stored with the decision. Rules, in order:

`unknown_action` · `not_in_universe` · `duplicate` · `no_quantity` · `price_scaled` (a price below a tenth of the close is read as thousands) · `tick_rounded` · `band_clamped` · `nothing_to_sell` · `trimmed_to_settled` · `safe_mode` · `max_positions` · `max_position_pct` / `trimmed_to_position_cap` · `no_cash` / `trimmed_to_cash` · `max_orders` · `broker_rejected` (from `paper.Submit`, which re-checks everything).

The model never sees anything the guards do not; the limits are repeated in the system prompt so it does not waste decisions.

## Time and settlement

- A cycle for day D runs after D's close and uses D's close as the next session's reference price.
- Orders are day orders for D+1. A market order fills at D+1's open. A limit buy fills at the open if open ≤ limit, else at the limit if the low reaches it, else expires; symmetric for sells.
- Shares bought on D+1 settle on the second trading day after (T+2) and cannot be sold before; sale proceeds become spendable on the same schedule. Settlement dates come from the provider's calendar, extrapolated by weekdays past its end.
- Pending buys reserve their worst-case cost (limit or ceiling, plus fee) so two decisions cannot spend the same cash.

## Persistence

Four tables (`decisions`, `orders`, `equity`, `state`). `state` holds the paper account and engine state as JSON documents; `replay` wipes everything and `cycle`/`serve` resume from them. The scorecard computed at the end of `replay` is stored under `state.scorecard` so the API can show the random baseline without recomputing it.

## What is deliberately missing

Intraday data and ATO/ATC matching, margin, short selling, derivatives, multiple accounts, authentication, and any real-order path. See `docs/research/roadmap.md` for what comes next and in what order.
