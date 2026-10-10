import { api, parseJSON, type Decision, type DecisionItem, type GuardEntry, type Metrics } from "../api"
import EquityChart from "../components/EquityChart"
import { day, pct } from "../format"
import { useQuery } from "../hooks"

export default function Dashboard({ version }: { version: number }) {
  const card = useQuery(() => api.scorecard(), [version])
  const equity = useQuery(() => api.equity(), [version])
  const decisions = useQuery(() => api.decisions(8), [version])

  return (
    <div className="grid gap-5">
      <section className="panel p-4">
        <div className="flex flex-wrap items-baseline justify-between gap-2 mb-3">
          <h2 className="label">Scorecard</h2>
          {card.data && card.data.strategy.days > 0 && (
            <span className="text-xs text-ink-muted num">
              {day(card.data.from)} → {day(card.data.to)} · {card.data.strategy.days} trading days
            </span>
          )}
        </div>
        {card.error && <p className="text-down text-sm">{card.error}</p>}
        {card.data && (
          <>
            <p className="text-base mb-4 max-w-prose">{card.data.verdict}</p>
            <div className="overflow-x-auto">
              <table className="grid">
                <thead>
                  <tr>
                    <th></th>
                    <th className="r">Strategy ({card.data.strategy.name})</th>
                    <th className="r">VN-Index buy &amp; hold</th>
                    <th className="r">Random trading</th>
                  </tr>
                </thead>
                <tbody>
                  <Row label="Total return" f={(m) => pct(m.total_return)} m={card.data} sign />
                  <Row label="Annualised" f={(m) => pct(m.annual_return)} m={card.data} sign />
                  <Row label="Max drawdown" f={(m) => pct(-m.max_drawdown)} m={card.data} />
                  <Row label="Volatility (ann.)" f={(m) => pct(m.volatility).replace("+", "")} m={card.data} />
                  <Row label="Sharpe" f={(m) => m.sharpe.toFixed(2)} m={card.data} />
                  <Row label="Filled orders" f={(m) => String(m.trades)} m={card.data} />
                  <Row label="Round trips / win rate" f={(m) => (m.round_trips ? `${m.round_trips} / ${(m.win_rate * 100).toFixed(0)}%` : "–")} m={card.data} />
                </tbody>
              </table>
            </div>
            {!card.data.random && (
              <p className="text-xs text-ink-muted mt-2">
                The random baseline appears after <code className="num">opentrade replay</code>; stepping day by day does not compute it.
              </p>
            )}
          </>
        )}
      </section>

      <section className="panel p-4 text-ink">
        <h2 className="label mb-3">Equity</h2>
        {equity.error && <p className="text-down text-sm">{equity.error}</p>}
        {equity.data && <EquityChart points={equity.data} />}
      </section>

      <section className="panel p-4">
        <div className="flex items-baseline justify-between mb-3">
          <h2 className="label">Latest decisions</h2>
          <a href="#decisions" className="text-xs text-accent">
            all decisions →
          </a>
        </div>
        {decisions.data && decisions.data.length === 0 && <p className="text-ink-muted text-sm">No decisions yet.</p>}
        {decisions.data && decisions.data.length > 0 && (
          <div className="overflow-x-auto">
            <table className="grid">
              <thead>
                <tr>
                  <th>Day</th>
                  <th>Decider</th>
                  <th>Orders</th>
                  <th>Guards</th>
                  <th className="r">Latency</th>
                </tr>
              </thead>
              <tbody>
                {decisions.data.map((d) => (
                  <DecisionRow key={d.id} d={d} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}

function Row({ label, f, m, sign }: { label: string; f: (m: Metrics) => string; m: { strategy: Metrics; index: Metrics; random?: Metrics }; sign?: boolean }) {
  const cls = (s: string) => (sign ? (s.startsWith("+") ? "text-up" : s.startsWith("-") ? "text-down" : "") : "")
  const s = f(m.strategy)
  const i = f(m.index)
  const r = m.random ? f(m.random) : "–"
  return (
    <tr>
      <td className="text-ink-muted">{label}</td>
      <td className={`r num font-semibold ${cls(s)}`}>{s}</td>
      <td className={`r num ${cls(i)}`}>{i}</td>
      <td className={`r num ${cls(r)}`}>{r}</td>
    </tr>
  )
}

export function DecisionRow({ d, active }: { d: Decision; active?: boolean }) {
  const items = parseJSON<DecisionItem[]>(d.decisions_json, [])
  const guards = parseJSON<GuardEntry[]>(d.guard_log, [])
  return (
    <tr className={`click ${active ? "active" : ""}`} onClick={() => (window.location.hash = `#decisions/${d.id}`)}>
      <td className="num whitespace-nowrap">{day(d.date)}</td>
      <td>
        <span className="pill accent">{d.decider}</span>
        {d.model && d.model !== d.decider && <span className="text-xs text-ink-muted ml-2">{d.model}</span>}
      </td>
      <td>
        {d.error && <span className="pill error">error</span>}
        {!d.error && items.length === 0 && <span className="text-ink-muted">hold</span>}
        {items.map((it, i) => (
          <span key={i} className={`pill ${it.action} mr-1`}>
            {it.action} {it.symbol} ×{it.lots}
          </span>
        ))}
      </td>
      <td>{guards.length > 0 && <span className="pill guard">{guards.length}</span>}</td>
      <td className="r num text-ink-muted">{d.latency_ms ? `${(d.latency_ms / 1000).toFixed(1)}s` : "–"}</td>
    </tr>
  )
}
