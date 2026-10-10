# Landscape, as of 2026-10-10

Numbers from the GitHub API on that date. Stars are not quality; recency and license matter more.

## Three bots people ask about

| | freqtrade | chrisleekr/binance-trading-bot | NoFxAiOS/nofx |
|---|---|---|---|
| Last commit | 2026-10-10 | 2026-10-03 | 2026-10-04 |
| Last release | 2026.9 (2026-09-29) | v1.6.0 (2026-10-03) | none tagged |
| Stars | 55.2k | 5.6k | 13.0k |
| Age | since 2017 | since 2020 | since 2025-10 |
| Open issues | 30 | 4 | 520 |
| Language | Python | TypeScript (Bun) | Go + React |
| License | GPL-3.0 | Apache-2.0 | AGPL-3.0 |
| Strategy | your Python code; backtest, hyperopt, FreqAI | three shipped strategies (trailing, momentum, rebalance) | an LLM prompt |
| Verifiable | yes (backtest, dry-run) | partly (backtest) | no |

freqtrade is the reference framework: nine years, monthly releases, 400+ contributors, no CVE on record (the "vulnerabilities" you find are typosquatted PyPI names). It is a tool, not a strategy; its docs warn about overfitting hyperopt, and public case studies of popular strategies over 24 months lose money. binance-trading-bot is one author's rewrite, README says "not recommended for real funds". nofx is the idea OpenTrade tests; see [nofx-architecture.md](nofx-architecture.md).

## Frameworks to build on

| Repo | Stars | Pushed | License | Language | Use |
|---|---|---|---|---|---|
| ccxt/ccxt | 44k | 2026-10-08 | MIT | Python, JS/TS, PHP, C#, Go | 100+ crypto exchanges, one API |
| nautechsystems/nautilus_trader | 30k | 2026-10-10 | LGPL-3.0 | Rust + Python | event-driven engine, backtest = live code |
| hummingbot | 20k | 2026-10-06 | Apache-2.0 | Python | market making, DEX |
| vnpy/vnpy | 46k | 2026-10-06 | MIT | Python | China: CTP futures, many stock counters |
| jesse-ai/jesse | 8.6k | 2026-10-05 | MIT | Python | compact crypto bot, fast backtest |
| Drakkar-Software/OctoBot | 6.7k | 2026-10-08 | GPL-3.0 | Python | bot with UI |

None covers Vietnamese equities.

## How China connects (why vnpy exists)

vnpy wraps brokers' binary "counter" APIs, one gateway each:

| Gateway | API | Market | Provider |
|---|---|---|---|
| vnpy_ctp (242★) | CTP | futures, options | SFIT, a Shanghai Futures Exchange subsidiary; every futures broker must support it |
| vnpy_xtp | XTP | stocks | Zhongtai Securities |
| vnpy_tora | Tora | stocks | Huaxin Securities |
| vnpy_uft / vnpy_ust | UFT / UST | stocks | Hundsun |
| vnpy_emt | EMT | stocks | East Money |
| vnpy_tts, vnpy_gm | CTP simulators | paper | community |
| vnpy_ib | IB API | overseas | Interactive Brokers |

CTP is the key: an exchange-mandated standard any retail futures account can use, so one gateway reaches hundreds of brokers. Chinese stocks have no such standard, hence half a dozen vendor gateways. Retail also uses QMT/Ptrade, broker-distributed software with a Python API and asset minimums.

## Vietnam

No equivalent of vnpy or freqtrade exists. What GitHub has (2026-10-10):

| Repo | Stars | Pushed | License | What |
|---|---|---|---|---|
| thinh-vu/vnstock | 1.4k | 2026-10-08 | custom (not OSI) | data library (VCI, TCBS, SSI sources); v3 moved to a proprietary license with paid tiers |
| phamdinhkhanh/vnquant | 480 | 2025-07 | none | data, unmaintained |
| mrgoonie/vnstock-agent | 104 | 2026-04 | none | MCP server over vnstock |
| ttqteo/vnstock-js | 14 | 2026-10 | Apache-2.0 | TypeScript data fetcher |
| archiephan78/ssi-stock-mcp-server | 17 | 2025-08 | Apache-2.0 | read-only MCP over SSI FastConnect data |
| various "TradingAgent-VN", "Azoth", "vn_stock_bot" | 0–25 | 2026 | mixed | LLM analysis agents; a few 0-star FastConnect order bots |

Why: no standard broker API (two brokers expose one: SSI, DNSE; see [vn-broker-apis.md](vn-broker-apis.md)); market structure removes most bot use cases (T+2, no short selling for retail, no day trading, ±7 % bands, lot 100, one liquid futures contract); a small quant community; and the one sizeable project chose to go closed. The open gap is the boring layer: a standard connector plus a backtester that knows the rules. OpenTrade's engine is a start on the second half.
