import type { ReactNode } from "react"
import { api } from "../api"
import { day, pct, signClass, vnd } from "../format"
import { useQuery } from "../hooks"

export default function Portfolio({ version }: { version: number }) {
  const pf = useQuery(() => api.portfolio(), [version])
  const orders = useQuery(() => api.orders(300), [version])
  const p = pf.data
  const ret = p && p.initial_cash > 0 ? p.equity / p.initial_cash - 1 : 0
  return (
    <div className="grid gap-5">
      {pf.error && <p className="text-down text-sm">{pf.error}</p>}
      {p && (
        <section className="grid grid-cols-2 md:grid-cols-4 gap-3">
          <Tile label="Equity" value={vnd(p.equity)} sub={<span className={signClass(ret)}>{pct(ret)} since start</span>} />
          <Tile label="Cash available" value={vnd(p.available)} sub={p.reserved > 0 ? `${vnd(p.reserved)} reserved for pending buys` : "nothing reserved"} />
          <Tile label="Receivables (T+2)" value={vnd(p.receivables)} sub="sale proceeds not yet spendable" />
          <Tile label="Holdings" value={vnd(p.holdings)} sub={`${p.positions.length} position${p.positions.length === 1 ? "" : "s"} · as of ${day(p.as_of)}`} />
        </section>
      )}

      <section className="panel p-4">
        <h2 className="label mb-3">Positions</h2>
        {p && p.positions.length === 0 && <p className="text-ink-muted text-sm">No open positions.</p>}
        {p && p.positions.length > 0 && (
          <div className="overflow-x-auto">
            <table className="grid">
              <thead>
                <tr>
                  <th>Symbol</th>
                  <th className="r">Qty</th>
                  <th className="r">Settled</th>
                  <th className="r">Avg cost</th>
                  <th className="r">Last</th>
                  <th className="r">Value</th>
                  <th className="r">P&amp;L</th>
                </tr>
              </thead>
              <tbody>
                {p.positions.map((pos) => (
                  <tr key={pos.symbol}>
                    <td className="font-semibold">{pos.symbol}</td>
                    <td className="r num">{pos.qty}</td>
                    <td className="r num">
                      {pos.settled}
                      {pos.settled < pos.qty && <span className="text-xs text-ink-muted ml-1">({pos.qty - pos.settled} T+2)</span>}
                    </td>
                    <td className="r num">{vnd(pos.avg_price)}</td>
                    <td className="r num">{vnd(pos.last_price)}</td>
                    <td className="r num">{vnd(pos.value)}</td>
                    <td className={`r num ${signClass(pos.pnl)}`}>
                      {vnd(pos.pnl)} <span className="text-xs">({pct(pos.pnl_pct, 1)})</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {p && p.pending && p.pending.length > 0 && (
        <section className="panel p-4">
          <h2 className="label mb-3">Pending for the next session</h2>
          <ul className="grid gap-1 text-sm">
            {p.pending.map((o) => (
              <li key={o.id}>
                <span className={`pill ${o.side} mr-2`}>
                  {o.side} {o.symbol}
                </span>
                <span className="num">
                  {o.qty} sh {o.limit_price ? `@ ${vnd(o.limit_price)}` : "at the open"}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="panel p-4">
        <h2 className="label mb-3">Order history</h2>
        {orders.error && <p className="text-down text-sm">{orders.error}</p>}
        {orders.data && orders.data.length === 0 && <p className="text-ink-muted text-sm">No orders yet.</p>}
        {orders.data && orders.data.length > 0 && (
          <div className="overflow-x-auto">
            <table className="grid">
              <thead>
                <tr>
                  <th>Decided</th>
                  <th>Order</th>
                  <th className="r">Qty</th>
                  <th className="r">Limit</th>
                  <th>Status</th>
                  <th className="r">Fill</th>
                  <th className="r">Fee + tax</th>
                  <th>Note</th>
                </tr>
              </thead>
              <tbody>
                {orders.data.map((o) => (
                  <tr key={o.id} className="click" onClick={() => (window.location.hash = `#decisions/${o.decision_id}`)}>
                    <td className="num whitespace-nowrap">{day(o.decision_date)}</td>
                    <td>
                      <span className={`pill ${o.side}`}>
                        {o.side} {o.symbol}
                      </span>
                    </td>
                    <td className="r num">{o.qty}</td>
                    <td className="r num">{o.limit_price ? vnd(o.limit_price) : "open"}</td>
                    <td>
                      <span className={`pill ${o.status}`}>{o.status}</span>
                    </td>
                    <td className="r num">{o.status === "filled" ? `${vnd(o.fill_price)} · ${o.fill_date}` : "–"}</td>
                    <td className="r num text-ink-muted">{o.status === "filled" ? vnd(o.fee + o.tax) : "–"}</td>
                    <td className="text-xs text-ink-muted max-w-64">{o.reason}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}

function Tile({ label, value, sub }: { label: string; value: string; sub?: ReactNode }) {
  return (
    <div className="panel p-3 min-w-0">
      <div className="label">{label}</div>
      <div className="num text-xl font-semibold mt-1 truncate">{value}</div>
      {sub && <div className="text-xs text-ink-muted mt-0.5 truncate">{sub}</div>}
    </div>
  )
}
