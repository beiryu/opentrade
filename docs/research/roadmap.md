# Roadmap

Ordered by what makes the measurement more honest, not by what looks good in a demo. Each item should land with tests and, where it touches the engine, a before/after `replay` on the same range.

## 0.1 (this repository today)

- Decision cycle, guards, paper broker with VN rules, replay + scorecard, API and web app, synthetic data.

## 0.2 — real data

1. **DNSE LightSpeed data provider** behind `market.Provider`, caching to CSV, real holiday calendar. Env-only credentials.
2. **SSI FastConnect data provider**, same contract.
3. A `fetch` command: `opentrade fetch --provider dnse --from … --to …` to fill `OPENTRADE_DATA_DIR`.
4. Replays on real 2024–2026 history for the ten sample names; publish the scorecards (llm, momentum, random) in `docs/results/` with the exact prompt version.

## 0.3 — honest measurement

5. Multiple-period evaluation: run the same decider over rolling 3-month windows and report the distribution, not one number.
6. Random baseline averaged over N seeds instead of one.
7. Cost accounting: tokens and USD per decision (the gateway reports usage), so "edge" is net of model cost.
8. Prompt versioning: hash the system prompt into the decision record; the scorecard refuses to compare runs with different prompts unless asked.

## 0.4 — closer to the market

9. ATO/ATC sessions and a fill model that uses the day's VWAP band rather than open/limit only.
10. Real trading calendar with Tết and exchange holidays.
11. Position sizing helpers in the prompt (ATR-based) so the model reasons about risk, not just direction.

## 0.5 — real orders, opt-in

12. **DNSE order adapter** behind the same guards: morning login + OTP, orders placed before 09:00 as ATO or limit, fills read from order status, hard kill switch, dry-run flag that logs what would be sent.
13. Notifications (Telegram or email) for every real order and every guard rejection.

## Not planned

Leverage, short selling, intraday loops, multi-user hosting, token or referral mechanics of any kind.
