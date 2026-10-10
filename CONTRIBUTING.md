# Contributing to OpenTrade

Thanks for considering it. OpenTrade is small on purpose; the most useful contributions keep it that way.

## What helps most

1. **Real-data providers.** A `market.Provider` for DNSE LightSpeed or SSI FastConnect, with the auth flow documented and no credentials in code. See `docs/research/vn-broker-apis.md`.
2. **Market-rule fixes.** If a tick size, band, holiday or settlement detail in `internal/rules` or `internal/paper` is wrong, open an issue with the source (exchange circular, broker page) and a failing test.
3. **Measurement.** Better baselines, more honest metrics in `internal/scorecard`, evaluation across several periods and seeds.
4. **Prompts, with evidence.** A prompt change is welcome when it comes with `replay` results before and after on the same range and decider.

What does not fit: anything that lets an order bypass `engine.ApplyGuards` or `paper.Account.Submit`, leverage or short selling, intraday loops, referral links, and payment or token features.

## Workflow

```bash
git clone https://github.com/<you>/opentrade && cd opentrade
go run ./cmd/opentrade seed
make vet test            # gofmt, go vet, go test ./...
cd web && pnpm install && pnpm lint && pnpm build
```

- Branch from `main`, keep PRs focused, one topic each.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/): `feat(engine): …`, `fix(paper): …`, `docs: …`, `ci: …`.
- Add or update tests next to the code (`*_test.go`). Engine changes should keep `TestReplayIsDeterministic` passing.
- Run `gofmt`; CI rejects unformatted files.
- Never commit `.env`, databases, decision logs, or real market data you are not allowed to redistribute.

## Code layout

| Package | Owns |
|---|---|
| `internal/rules` | lot, tick, band, T+2, fee/tax |
| `internal/market` | `Bar`, `Provider`, CSV provider, synthetic generator |
| `internal/indicators` | SMA, EMA, RSI, ATR, returns, drawdown |
| `internal/engine` | context, prompts, deciders, guards, cycle/replay |
| `internal/paper` | the simulated broker |
| `internal/store` | SQLite persistence |
| `internal/scorecard` | metrics and verdict |
| `internal/api` | HTTP handlers and the embedded web app |
| `web` | React UI |

More in [docs/architecture.md](docs/architecture.md).

## Reporting

Bugs and ideas go to [issues](https://github.com/beiryu/opentrade/issues). Security problems go to a [private advisory](https://github.com/beiryu/opentrade/security/advisories/new), not an issue.

By contributing you agree that your contributions are licensed under the MIT license of this repository.
