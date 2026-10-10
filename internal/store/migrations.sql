-- OpenTrade schema. Applied in full on every open; every statement is
-- idempotent. Add new tables/columns below with IF NOT EXISTS; never edit
-- an existing statement in a way that breaks an old database.

CREATE TABLE IF NOT EXISTS decisions (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  date              TEXT    NOT NULL,            -- trading day decided after its close, YYYY-MM-DD
  decider           TEXT    NOT NULL,            -- llm | momentum | random
  model             TEXT    NOT NULL DEFAULT '',
  system_prompt     TEXT    NOT NULL DEFAULT '',
  user_prompt       TEXT    NOT NULL DEFAULT '',
  raw_response      TEXT    NOT NULL DEFAULT '', -- verbatim model output
  decisions_json    TEXT    NOT NULL DEFAULT '[]',
  guard_log         TEXT    NOT NULL DEFAULT '[]',
  error             TEXT    NOT NULL DEFAULT '',
  latency_ms        INTEGER NOT NULL DEFAULT 0,
  prompt_tokens     INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  created_at        TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS decisions_date ON decisions(date);

CREATE TABLE IF NOT EXISTS orders (
  id            TEXT PRIMARY KEY,
  decision_id   INTEGER NOT NULL REFERENCES decisions(id) ON DELETE CASCADE,
  decision_date TEXT    NOT NULL,
  symbol        TEXT    NOT NULL,
  side          TEXT    NOT NULL,                -- buy | sell
  qty           INTEGER NOT NULL,
  limit_price   REAL    NOT NULL DEFAULT 0,      -- 0 = at the open
  status        TEXT    NOT NULL,                -- pending | filled | expired | rejected
  fill_price    REAL    NOT NULL DEFAULT 0,
  fill_date     TEXT    NOT NULL DEFAULT '',
  fee           REAL    NOT NULL DEFAULT 0,
  tax           REAL    NOT NULL DEFAULT 0,
  reason        TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS orders_decision ON orders(decision_id);

CREATE TABLE IF NOT EXISTS equity (
  date        TEXT PRIMARY KEY,
  cash        REAL NOT NULL,
  holdings    REAL NOT NULL,
  equity      REAL NOT NULL,
  index_close REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS state (
  key        TEXT PRIMARY KEY,
  json       TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
