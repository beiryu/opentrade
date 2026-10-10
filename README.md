# OpenTrade

**An open-source trading terminal for the Vietnamese stock market where the strategy is a language model, and where that claim is measured instead of trusted.**

[Tiếng Việt](README.vi.md) · [Architecture](docs/architecture.md) · [Research notes](docs/research/) · [Roadmap](docs/research/roadmap.md)

[![CI](https://github.com/beiryu/opentrade/actions/workflows/ci.yml/badge.svg)](https://github.com/beiryu/opentrade/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](go.mod)

OpenTrade runs one decision per trading day. After the close it assembles the account, the market and a fixed list of candidate stocks into a prompt, asks a model for orders as JSON, passes those orders through guard rails written in code, and hands them to a **paper broker** that applies the real rules of HOSE/HNX: board lots, tick sizes, daily price bands, T+2 settlement, fees and tax. Every call is stored with the exact prompt, the model's raw reply and what the guards changed.

Then it does what the LLM-trading projects it is modelled on do not: `opentrade replay` runs that loop day by day over history and scores the result against **VN-Index buy-and-hold** and **a random trader** under the same constraints.

```
SCORECARD 2026-01-02 → 2026-10-09
               strategy  VN-Index  random
total return   +12.53%   +17.13%   -1.18%
annualised     +15.90%   +21.85%   -1.47%
max drawdown   3.54%     6.87%     6.84%
sharpe         1.61      1.51      -0.19
filled orders  73        0         87
win rate       49%       -         35%

Beat random trading but not the index: holding VN-Index would have done better.
```

That is the momentum baseline on the **synthetic** sample data. It is here to show the output, not to claim anything.

## What it is, and is not

- **Is**: a measuring instrument for "can a model trade VN stocks?" with the market's rules built in, a transparent log of every decision, and two baselines that any strategy has to beat.
- **Is**: paper-first. Nothing talks to a broker today. A real-order adapter is on the roadmap and will stay opt-in behind the same guard rails.
- **Is not**: a bot that makes money. Nobody has shown that an LLM reading indicators has an edge, and the sample data is made up. Treat any result as an experiment.
- **Is not**: a fork. The design follows [NOFX](https://github.com/NoFxAiOS/nofx) (cycle → prompt → JSON → guards → execute, everything logged); the code is new and MIT, because 90 % of NOFX is crypto-perpetuals machinery the Vietnamese market has no use for.

## Quick start

Needs Go 1.26+ (older Go downloads it automatically) and, for the web app, Node 20 with pnpm.

```bash
git clone https://github.com/beiryu/opentrade && cd opentrade
go run ./cmd/opentrade seed                 # synthetic sample data in data/sample
make replay                                 # momentum baseline over 2026, prints the scorecard
make build && ./opentrade serve             # http://127.0.0.1:8080 with the web app
```

To decide with a model, set an OpenAI-compatible endpoint and run a replay or step a day at a time:

```bash
cp .env.example .env                        # then put your key in OPENTRADE_LLM_API_KEY
./opentrade replay --from 2026-07-01 --to 2026-09-30 --decider llm
./opentrade cycle --decider llm             # one trading day on the saved state
```

The default endpoint is the [Vercel AI Gateway](https://vercel.com/docs/ai-gateway) with `deepseek/deepseek-v4.1-flash`. Any OpenAI-compatible server works (`OPENTRADE_LLM_BASE_URL`): DeepSeek directly, OpenAI, Ollama, LM Studio.

## How a day works

```
after the close of day D
  1. context      account (cash, receivables, positions with settled qty), VN-Index,
                  candidates with close, next-day floor/ceiling, SMA20/50, RSI14, ATR14,
                  5/20-day returns, volume ratio, last 10 orders
  2. decider      llm | momentum | random  →  {"decisions":[{symbol, action, lots, limit_price, confidence, reasoning}]}
  3. guards       symbol in universe · one order per symbol · lots > 0 · price on tick and inside the band ·
                  sells ≤ settled shares (T+2) · buys ≤ cash and ≤ 30 % of equity per symbol ·
                  max positions · max orders · safe mode after 3 decider failures (sells only)
  4. paper broker day orders for D+1: fill at the open or the limit, fee 0.15 %, sell tax 0.1 %,
                  shares and proceeds settle T+2
  5. record       prompt, raw reply, parsed orders, guard log, order outcomes, equity at the close
```

`replay` = roll to D (fill yesterday's orders, settle), then decide for D, for every trading day in the range; it finishes with the scorecard and a random-trader run over the same days. Details and the package map are in [docs/architecture.md](docs/architecture.md).

## Commands

| Command | What it does |
|---|---|
| `opentrade seed` | Write synthetic sample data (`--from`, `--to`, `--seed`, `--dir`) |
| `opentrade replay --from --to [--decider] [--cash]` | Wipe the database, run every trading day, print and store the scorecard |
| `opentrade cycle [--decider]` | Roll + decide the next trading day on the saved state |
| `opentrade serve [--decider] [--listen]` | API and web app, `127.0.0.1:8080` by default |

Settings are environment variables (or a local `.env`): see [`.env.example`](.env.example). `OPENTRADE_UNIVERSE` picks the symbols, `OPENTRADE_EXCHANGES` their boards (`SHS:HNX,BSR:UPCOM`; HOSE by default), `OPENTRADE_MAX_POSITIONS` / `OPENTRADE_MAX_POSITION_PCT` the limits.

## Web app

Three pages on top of the API: a **dashboard** with the scorecard and the equity curve against VN-Index, **decisions** with every prompt, raw reply and guard entry, and the **portfolio** with settled vs unsettled shares, receivables and the order log. "Run next day" steps one trading day from the browser.

`make web` builds it into `internal/api/ui/dist`, which the binary embeds; `cd web && pnpm dev` runs it against a local `serve` with hot reload.

## Data

`data/sample` is synthetic (see [its README](data/sample/README.md)). Any directory of `SYMBOL.csv` files with `date,open,high,low,close,volume` works through `OPENTRADE_DATA_DIR`. Providers for DNSE LightSpeed and SSI FastConnect are the first items on the [roadmap](docs/research/roadmap.md); until then, export your own history into CSV.

## Development

```bash
make vet test      # gofmt, go vet, go test ./...
make lint          # + web lint
cd web && pnpm dev # UI with hot reload, proxies /api to :8080
```

Dependencies are deliberately few: the standard library's HTTP mux, [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (pure Go, no cgo), React, Vite, Tailwind. No ORM, no web framework, no provider SDK. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

OpenTrade is meant to run on your own machine: it binds to `127.0.0.1`, has no authentication, and the only secret is your model API key, which lives in your environment and never in the repository. Report vulnerabilities privately through [GitHub security advisories](https://github.com/beiryu/opentrade/security/advisories/new); see [SECURITY.md](SECURITY.md).

## Disclaimer

This software is for research and education. Automated trading can lose money; a model can be confidently wrong. Nothing here is investment advice, and the authors accept no liability for how you use it. The sample data is fictional.

## License

[MIT](LICENSE) © 2026 beiryu.
