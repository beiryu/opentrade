# Vietnamese broker APIs (notes, 2026-10)

Two brokers publish APIs that an individual account can use. Both are REST + streaming over HTTPS, which makes them far easier to wrap than China's binary counter libraries. Both gate order placement behind a per-session second factor, so a bot authenticates once per morning rather than running unattended for days. Details below are from the public docs at the time of writing; verify before building, the pages move.

## SSI FastConnect

- Docs: https://guide.ssi.com.vn/ssi-products/ (Vietnamese and English), SDK samples in Python, Node.js, Java, .NET; a .NET SDK on NuGet (`SSIDeveloper.FastConnect.Sdk`, repo `SSI-Securities-Inc/ssi-sdk-dotnet`).
- Two products:
  - **FC Data**: real-time quotes and history. REST at `https://fc-data.ssi.com.vn/v2.0/Market`, streaming at `wss://fc-datahub.ssi.com.vn/v2.0` (SignalR for Python/.NET).
  - **FC Trading**: account queries, place/modify/cancel orders, order status via streaming.
- Auth: register from an SSI account to receive a `ConsumerID` and `ConsumerSecret`; trading additionally requires an OTP/PIN flow (SSI Smart OTP or PIN) when starting a trading session.
- Markets: HOSE, HNX, UPCOM equities; derivatives through the same API.
- Unknowns from the public pages: fees for API access, rate limits, token lifetime, sandbox. Ask SSI.

## DNSE LightSpeed (Entrade X)

- Docs: https://hdsd.dnse.com.vn/san-pham-dich-vu/ (two versions: pre-KRX and post-KRX; the post-KRX domains went live 2025-05-05 and the old ones were kept one month). Build against the KRX version.
- Login: `POST services.entrade.com.vn/dnse-auth-service/login` with username/password → JWT valid **8 hours**. Read-only calls (account, positions, market data) need only the JWT.
- Trading token: `POST services.entrade.com.vn/dnse-order-service/trading-token` with the JWT and either a `smart-otp` header (code from the EntradeX app) or an email OTP (requested via GET, valid 2 minutes, single use) → trading token valid **8 hours**. Place/cancel orders need both headers.
- Market data: separate user service (`dnse-user-service/api/auth`) and an MQTT/WebSocket feed documented under "Market data".
- Practical: the trading session (09:00–14:45, pre-market from 08:45) fits inside one 8-hour token. A daily routine is: log in, enter one OTP, run the day, done.

## What a provider for OpenTrade needs

`market.Provider` is three methods: `Symbols()`, `Bars(symbol, until, n)`, `Calendar()`. A DNSE/SSI data provider should:

1. Authenticate from environment variables only (`OPENTRADE_DNSE_USER`, `OPENTRADE_DNSE_PASS`), never from files in the repo.
2. Cache daily bars to CSV in `OPENTRADE_DATA_DIR` so replays are reproducible and offline.
3. Expose the real trading calendar (holidays) instead of weekdays.

A real-order broker (later) implements the same contract as `paper.Account` (`Submit`, `Roll`-equivalent fills from order status streaming) and sits behind the same `ApplyGuards`. It must be opt-in, log every request, and refuse to start without a fresh trading token.

## Legal and practical notes

- Algorithmic order placement by individuals through a broker's official API is allowed; the broker's terms govern rate limits and liability.
- Unofficial data endpoints used by community libraries (VCI, TCBS web APIs) break without notice and may violate terms; prefer the official feeds above.
- Prices from these APIs are in VND; some UIs show thousands. OpenTrade's guards fix an obvious thousands-scaled limit price, but data must be loaded in VND.
