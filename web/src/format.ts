const vndFormat = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 })

/** Whole VND with thousands separators. */
export function vnd(v: number | undefined | null): string {
  if (v === undefined || v === null || Number.isNaN(v)) return "–"
  return vndFormat.format(Math.round(v))
}

/** Millions of VND, one decimal, for headline numbers. */
export function mvnd(v: number | undefined | null): string {
  if (v === undefined || v === null || Number.isNaN(v)) return "–"
  return `${(v / 1_000_000).toFixed(1)}M`
}

/** Fraction → signed percent. */
export function pct(v: number | undefined | null, digits = 2): string {
  if (v === undefined || v === null || Number.isNaN(v)) return "–"
  const s = (v * 100).toFixed(digits)
  return v > 0 ? `+${s}%` : `${s}%`
}

/** ISO date → YYYY-MM-DD; empty for the zero time. */
export function day(iso: string | undefined | null): string {
  if (!iso || iso.startsWith("0001-")) return "–"
  return iso.slice(0, 10)
}

export function signClass(v: number | undefined | null): string {
  if (v === undefined || v === null || v === 0) return "text-ink-muted"
  return v > 0 ? "text-up" : "text-down"
}
