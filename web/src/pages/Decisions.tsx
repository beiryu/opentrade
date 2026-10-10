import { api, parseJSON, type DecisionItem, type GuardEntry } from "../api"
import { day, vnd } from "../format"
import { useQuery } from "../hooks"
import { DecisionRow } from "./Dashboard"

export default function Decisions({ selected, version }: { selected?: number; version: number }) {
  const list = useQuery(() => api.decisions(300), [version])
  return (
    <div className="grid gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
      <section className="panel p-0 overflow-hidden self-start">
        <div className="px-4 py-3 border-b border-line flex items-baseline justify-between">
          <h2 className="label">Decisions</h2>
          {list.data && <span className="text-xs text-ink-muted num">{list.data.length}</span>}
        </div>
        {list.error && <p className="text-down text-sm p-4">{list.error}</p>}
        {list.data && list.data.length === 0 && <p className="text-ink-muted text-sm p-4">No decisions yet. Run a replay or step a day.</p>}
        {list.data && list.data.length > 0 && (
          <div className="overflow-auto max-h-[75vh]">
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
                {list.data.map((d) => (
                  <DecisionRow key={d.id} d={d} active={d.id === selected} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
      <section className="min-w-0">
        {selected ? <Detail id={selected} /> : <p className="text-ink-muted text-sm p-4">Select a decision to see what the model was asked, what it answered and what the guards did.</p>}
      </section>
    </div>
  )
}

function Detail({ id }: { id: number }) {
  const q = useQuery(() => api.decision(id), [id])
  if (q.error) return <p className="text-down text-sm p-4">{q.error}</p>
  if (!q.data) return <p className="text-ink-muted text-sm p-4">Loading…</p>
  const { decision: d, orders } = q.data
  const items = parseJSON<DecisionItem[]>(d.decisions_json, [])
  const guards = parseJSON<GuardEntry[]>(d.guard_log, [])
  return (
    <div className="grid gap-4">
      <div className="panel p-4">
        <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 mb-3">
          <h2 className="text-lg font-semibold num">{day(d.date)}</h2>
          <span className="pill accent">{d.decider}</span>
          {d.model && <span className="text-xs text-ink-muted">{d.model}</span>}
          {d.latency_ms > 0 && <span className="text-xs text-ink-muted num">{(d.latency_ms / 1000).toFixed(1)}s</span>}
          {d.prompt_tokens > 0 && (
            <span className="text-xs text-ink-muted num">
              {d.prompt_tokens} in / {d.completion_tokens} out tokens
            </span>
          )}
        </div>
        {d.error && (
          <p className="text-sm mb-3">
            <span className="pill error mr-2">decider error</span>
            <span className="num text-xs">{d.error}</span>
          </p>
        )}

        <h3 className="label mb-2">Orders after guards</h3>
        {items.length === 0 && <p className="text-ink-muted text-sm mb-3">None (hold).</p>}
        {items.length > 0 && (
          <div className="overflow-x-auto mb-3">
            <table className="grid">
              <thead>
                <tr>
                  <th>Action</th>
                  <th className="r">Lots</th>
                  <th className="r">Limit</th>
                  <th className="r">Conf.</th>
                  <th>Reasoning</th>
                  <th>Outcome</th>
                </tr>
              </thead>
              <tbody>
                {items.map((it, i) => {
                  const o = orders.find((x) => x.symbol === it.symbol && x.side === it.action)
                  return (
                    <tr key={i}>
                      <td>
                        <span className={`pill ${it.action}`}>
                          {it.action} {it.symbol}
                        </span>
                      </td>
                      <td className="r num">{it.lots}</td>
                      <td className="r num">{it.limit_price ? vnd(it.limit_price) : "open"}</td>
                      <td className="r num">{it.confidence ?? "–"}</td>
                      <td className="text-ink-muted max-w-prose">{it.reasoning}</td>
                      <td className="whitespace-nowrap">
                        {o ? (
                          <>
                            <span className={`pill ${o.status}`}>{o.status}</span>
                            {o.status === "filled" && <span className="num text-xs ml-1">@ {vnd(o.fill_price)}</span>}
                            {o.reason && <div className="text-xs text-ink-muted mt-1 max-w-56 whitespace-normal">{o.reason}</div>}
                          </>
                        ) : (
                          "–"
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}

        <h3 className="label mb-2">Guard log</h3>
        {guards.length === 0 && <p className="text-ink-muted text-sm">Nothing changed by the guards.</p>}
        {guards.length > 0 && (
          <ul className="grid gap-1 text-sm">
            {guards.map((g, i) => (
              <li key={i}>
                <span className="pill guard mr-2">{g.rule}</span>
                <span className="num text-xs mr-2">
                  {g.action} {g.symbol}
                </span>
                <span className="text-ink-muted">{g.detail}</span>
              </li>
            ))}
          </ul>
        )}
      </div>

      {d.raw_response !== undefined && (
        <details className="panel p-4" open={!!d.error}>
          <summary className="label">Raw model reply</summary>
          <pre className="block mt-3">{d.raw_response || "(empty)"}</pre>
        </details>
      )}
      {d.user_prompt && (
        <details className="panel p-4">
          <summary className="label">What the model saw (user prompt)</summary>
          <pre className="block mt-3">{d.user_prompt}</pre>
        </details>
      )}
      {d.system_prompt && (
        <details className="panel p-4">
          <summary className="label">System prompt</summary>
          <pre className="block mt-3">{d.system_prompt}</pre>
        </details>
      )}
    </div>
  )
}
