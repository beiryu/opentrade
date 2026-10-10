// Typed client for the OpenTrade API. Every call goes through request() so
// errors carry the server's message.

export interface EngineState {
  cycle_no: number
  last_date: string
  consecutive_failures: number
  safe_mode: boolean
  decider: string
}

export interface StateView {
  decider: string
  model: string
  state: EngineState
  universe: string[]
  limits: { max_positions: number; max_position_pct: number; max_orders_per_cycle: number }
  first_day: string
  last_day: string
  initial_cash: number
}

export interface Decision {
  id: number
  date: string
  decider: string
  model: string
  system_prompt?: string
  user_prompt?: string
  raw_response?: string
  decisions_json: string
  guard_log: string
  error: string
  latency_ms: number
  prompt_tokens: number
  completion_tokens: number
  created_at: string
}

export interface DecisionItem {
  symbol: string
  action: "buy" | "sell" | "hold"
  lots: number
  limit_price?: number
  confidence?: number
  reasoning: string
}

export interface GuardEntry {
  symbol: string
  action: string
  rule: string
  detail: string
}

export interface Order {
  id: string
  decision_id: number
  decision_date: string
  symbol: string
  side: "buy" | "sell"
  qty: number
  limit_price: number
  status: "pending" | "filled" | "expired" | "rejected"
  fill_price: number
  fill_date: string
  fee: number
  tax: number
  reason: string
}

export interface Position {
  symbol: string
  qty: number
  settled: number
  avg_price: number
  cost: number
  last_price?: number
  value?: number
  pnl?: number
  pnl_pct?: number
}

export interface Portfolio {
  as_of: string
  cash: number
  available: number
  reserved: number
  receivables: number
  holdings: number
  equity: number
  initial_cash: number
  positions: Position[]
  pending: Order[] | null
}

export interface EquityPoint {
  date: string
  cash: number
  holdings: number
  equity: number
  index_close: number
}

export interface Metrics {
  name: string
  days: number
  start_equity: number
  end_equity: number
  total_return: number
  annual_return: number
  volatility: number
  sharpe: number
  max_drawdown: number
  trades: number
  round_trips: number
  win_rate: number
}

export interface Scorecard {
  from: string
  to: string
  strategy: Metrics
  index: Metrics
  random?: Metrics
  verdict: string
}

export interface CycleResult {
  date: string
  decision_id: number
  decisions: DecisionItem[] | null
  orders: Order[] | null
  guard_log: GuardEntry[] | null
  error?: string
  safe_mode: boolean
  equity: number
}

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { ...init, headers: { Accept: "application/json", ...(init?.headers ?? {}) } })
  const text = await res.text()
  let body: unknown = null
  try {
    body = text ? JSON.parse(text) : null
  } catch {
    body = null
  }
  if (!res.ok) {
    const msg = (body as { error?: string } | null)?.error ?? `${res.status} ${res.statusText}`
    throw new ApiError(msg, res.status)
  }
  return body as T
}

export const api = {
  state: () => request<StateView>("/api/state"),
  decisions: (limit = 200) => request<Decision[]>(`/api/decisions?limit=${limit}`),
  decision: (id: number) => request<{ decision: Decision; orders: Order[] }>(`/api/decisions/${id}`),
  orders: (limit = 500) => request<Order[]>(`/api/orders?limit=${limit}`),
  portfolio: () => request<Portfolio>("/api/portfolio"),
  equity: () => request<EquityPoint[]>("/api/equity"),
  scorecard: () => request<Scorecard>("/api/scorecard"),
  cycle: () => request<CycleResult>("/api/cycle", { method: "POST" }),
}

export function parseJSON<T>(s: string, fallback: T): T {
  try {
    return JSON.parse(s) as T
  } catch {
    return fallback
  }
}
