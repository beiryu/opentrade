import { useState } from "react"
import { api, type CycleResult } from "./api"
import { day, vnd } from "./format"
import { useHashRoute, useQuery } from "./hooks"
import Dashboard from "./pages/Dashboard"
import Decisions from "./pages/Decisions"
import Portfolio from "./pages/Portfolio"

const TABS = [
  ["dashboard", "Dashboard"],
  ["decisions", "Decisions"],
  ["portfolio", "Portfolio"],
] as const

export default function App() {
  const route = useHashRoute()
  const page = route[0] || "dashboard"
  // Bumped after a cycle so every page refetches.
  const [version, setVersion] = useState(0)
  const state = useQuery(() => api.state(), [version])
  const [running, setRunning] = useState(false)
  const [last, setLast] = useState<{ ok: boolean; text: string } | null>(null)

  async function runNextDay() {
    setRunning(true)
    setLast(null)
    try {
      const r: CycleResult = await api.cycle()
      const n = r.orders?.length ?? 0
      setLast({
        ok: !r.error,
        text: r.error
          ? `${day(r.date)}: decider error — ${r.error}`
          : `${day(r.date)}: ${n} order${n === 1 ? "" : "s"}, equity ${vnd(r.equity)}${r.safe_mode ? " (safe mode)" : ""}`,
      })
      setVersion((v) => v + 1)
      if (r.decision_id) window.location.hash = `#decisions/${r.decision_id}`
    } catch (e) {
      setLast({ ok: false, text: e instanceof Error ? e.message : String(e) })
    } finally {
      setRunning(false)
    }
  }

  const s = state.data
  return (
    <div className="min-h-full">
      <header className="border-b border-line bg-panel">
        <div className="max-w-[1200px] mx-auto px-4 py-3 flex flex-wrap items-center gap-x-6 gap-y-2">
          <a href="#dashboard" className="font-semibold text-base tracking-tight">
            OpenTrade <span className="text-ink-muted font-normal text-xs ml-1">paper</span>
          </a>
          <nav className="flex gap-1">
            {TABS.map(([key, label]) => (
              <a
                key={key}
                href={`#${key}`}
                className={`px-3 py-1 rounded text-sm ${page === key ? "bg-bg text-ink font-semibold" : "text-ink-muted hover:text-ink"}`}
              >
                {label}
              </a>
            ))}
          </nav>
          <div className="ml-auto flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-muted">
            {s && (
              <>
                <span>
                  decider <span className="pill accent">{s.decider}</span>
                  {s.model && <span className="ml-1">{s.model}</span>}
                </span>
                <span className="num">
                  last day {day(s.state.last_date)} · cycle {s.state.cycle_no}
                </span>
                {s.state.safe_mode && <span className="pill error">safe mode</span>}
                <span className="num">
                  data {day(s.first_day)} → {day(s.last_day)}
                </span>
              </>
            )}
            <button className="btn primary" onClick={runNextDay} disabled={running}>
              {running ? "Deciding…" : "Run next day"}
            </button>
          </div>
        </div>
        {last && (
          <div className={`max-w-[1200px] mx-auto px-4 pb-2 text-xs num ${last.ok ? "text-ink-muted" : "text-down"}`}>{last.text}</div>
        )}
        {state.error && <div className="max-w-[1200px] mx-auto px-4 pb-2 text-xs text-down">API unreachable: {state.error}</div>}
      </header>
      <main className="max-w-[1200px] mx-auto px-4 py-5">
        {page === "dashboard" && <Dashboard version={version} />}
        {page === "decisions" && <Decisions selected={route[1] ? Number(route[1]) : undefined} version={version} />}
        {page === "portfolio" && <Portfolio version={version} />}
      </main>
      <footer className="max-w-[1200px] mx-auto px-4 pb-6 text-xs text-ink-muted">
        Paper trading on the data in OPENTRADE_DATA_DIR. Nothing here is investment advice; the sample data is synthetic.
      </footer>
    </div>
  )
}
