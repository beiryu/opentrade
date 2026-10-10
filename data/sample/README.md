# Sample data: synthetic, not real

Every CSV in this directory is **made up**. `opentrade seed` generates them
with a seeded geometric random walk (`internal/market/synthetic.go`): an
index series first, then ten HOSE-like symbols that mix the index's daily
move in by a beta plus their own noise, clamped to real tick sizes and the
±7 % daily band. Start prices and volumes are in the right ballpark for the
names used, nothing else about them is.

They exist so the whole pipeline (context → decider → guards → paper broker
→ scorecard) runs out of the box and in CI without any key or network. A
result on this data says nothing about the Vietnamese market.

Format, one file per symbol, `VNINDEX.csv` for the index:

```
date,open,high,low,close,volume
2024-10-01,120000,122200,119200,121800,2538919
```

Prices in VND, volume in shares, dates `YYYY-MM-DD`, oldest first. Point
`OPENTRADE_DATA_DIR` at any directory with files in this format to run on
your own data; a provider for a broker's data API (DNSE LightSpeed, SSI
FastConnect) is on the roadmap in `docs/research/roadmap.md`.

Regenerate with a different seed or range:

```
go run ./cmd/opentrade seed --seed 7 --from 2023-01-02 --to 2026-10-09
```
