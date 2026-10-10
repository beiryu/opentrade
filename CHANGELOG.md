# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Decision cycle: context builder, system/user prompts, `llm` / `momentum` / `random` deciders, guard rails, safe mode after three decider failures.
- Paper broker with Vietnamese market rules: board lot 100, tick sizes, daily bands (±7/10/15 %), day orders filled at the open or limit, 0.15 % fee, 0.1 % sale tax, T+2 settlement of shares and cash.
- `replay` over a date range with a scorecard against VN-Index buy-and-hold and a seeded random trader; `cycle` to step one day; `serve` for the API and web app; `seed` for synthetic sample data.
- SQLite store (pure Go) keeping every prompt, raw reply, guard log, order and daily equity.
- React web app: dashboard (scorecard, equity vs index), decisions (prompts, replies, guards), portfolio (settled vs unsettled, receivables, orders).
- Research notes on NOFX's architecture, the trading-bot landscape, Vietnamese broker APIs, and the roadmap.

[Unreleased]: https://github.com/beiryu/opentrade/commits/main
