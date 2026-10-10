import type { EquityPoint } from "../api"
import { day, mvnd } from "../format"

/**
 * Strategy equity against VN-Index buy-and-hold on the same cash, drawn to
 * one scale. Hand-made SVG: a chart library would be the largest thing in
 * the bundle for two lines.
 */
export default function EquityChart({ points }: { points: EquityPoint[] }) {
  if (points.length < 2) {
    return <div className="text-ink-muted text-sm">Not enough equity points yet. Run a replay or step a few days.</div>
  }
  const W = 960
  const H = 280
  const padL = 64
  const padR = 16
  const padT = 12
  const padB = 28
  const first = points[0]
  const idx0 = first.index_close || 1
  const strat = points.map((p) => p.equity)
  const index = points.map((p) => (p.index_close ? (first.equity * p.index_close) / idx0 : first.equity))
  const all = strat.concat(index)
  let lo = Math.min(...all)
  let hi = Math.max(...all)
  const pad = (hi - lo) * 0.08 || hi * 0.02
  lo -= pad
  hi += pad
  const x = (i: number) => padL + (i / (points.length - 1)) * (W - padL - padR)
  const y = (v: number) => padT + (1 - (v - lo) / (hi - lo)) * (H - padT - padB)
  const path = (vals: number[]) => vals.map((v, i) => `${i === 0 ? "M" : "L"}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(" ")

  // Five horizontal gridlines at round numbers.
  const ticks: number[] = []
  const step = niceStep((hi - lo) / 4)
  for (let v = Math.ceil(lo / step) * step; v <= hi; v += step) ticks.push(v)
  // Date labels: first, middle, last.
  const dateIdx = [0, Math.floor((points.length - 1) / 2), points.length - 1]

  return (
    <figure className="m-0">
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-auto block" role="img" aria-label="Equity of the strategy versus VN-Index buy and hold">
        {ticks.map((t) => (
          <g key={t}>
            <line x1={padL} x2={W - padR} y1={y(t)} y2={y(t)} stroke="currentColor" strokeOpacity="0.12" />
            <text x={padL - 8} y={y(t) + 4} textAnchor="end" fontSize="11" fill="currentColor" fillOpacity="0.6" className="num">
              {mvnd(t)}
            </text>
          </g>
        ))}
        {dateIdx.map((i) => (
          <text key={i} x={x(i)} y={H - 8} textAnchor={i === 0 ? "start" : i === points.length - 1 ? "end" : "middle"} fontSize="11" fill="currentColor" fillOpacity="0.6" className="num">
            {day(points[i].date)}
          </text>
        ))}
        <path d={path(index)} fill="none" stroke="currentColor" strokeOpacity="0.45" strokeWidth="1.5" strokeDasharray="4 3" />
        <path d={path(strat)} fill="none" stroke="var(--color-accent)" strokeWidth="2" />
        <circle cx={x(points.length - 1)} cy={y(strat[strat.length - 1])} r="3.5" fill="var(--color-accent)" />
      </svg>
      <figcaption className="flex gap-5 text-xs text-ink-muted mt-1">
        <span>
          <span className="inline-block w-5 border-t-2 border-accent align-middle mr-1.5" />
          strategy equity
        </span>
        <span>
          <span className="inline-block w-5 border-t-2 border-dashed border-ink-muted align-middle mr-1.5" />
          VN-Index buy &amp; hold, same cash
        </span>
      </figcaption>
    </figure>
  )
}

function niceStep(raw: number): number {
  const p = Math.pow(10, Math.floor(Math.log10(raw)))
  const m = raw / p
  const n = m < 1.5 ? 1 : m < 3.5 ? 2 : m < 7.5 ? 5 : 10
  return n * p
}
