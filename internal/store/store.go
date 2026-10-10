// Package store persists what OpenTrade must be able to show later: every
// decision with the exact prompts and raw model output, every order, the
// daily equity curve and the paper account state. SQLite, pure Go driver,
// no ORM. Schema lives in migrations.sql and is applied on open.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations.sql
var migrations string

// Store wraps the database.
type Store struct {
	db *sql.DB
}

// Open creates the file and parent directory when needed and applies the
// schema. ":memory:" gives a throwaway database for tests.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection: SQLite is single-writer and an in-memory DB would
	// otherwise be a different database per connection.
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: %s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(migrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close the database.
func (s *Store) Close() error { return s.db.Close() }

// Decision is one cycle's record: what the model was asked, what it
// answered verbatim, what the parser made of it and what the guards did.
type Decision struct {
	ID           int64     `json:"id"`
	Date         time.Time `json:"date"`
	Decider      string    `json:"decider"`
	Model        string    `json:"model"`
	SystemPrompt string    `json:"system_prompt"`
	UserPrompt   string    `json:"user_prompt"`
	RawResponse  string    `json:"raw_response"`
	// DecisionsJSON is the parsed decision list, after guards, as stored.
	DecisionsJSON string `json:"decisions_json"`
	// GuardLog is a JSON array of {symbol, action, rule, detail} entries.
	GuardLog  string `json:"guard_log"`
	Error     string `json:"error"`
	LatencyMs int64  `json:"latency_ms"`
	// Tokens in / out when the server reports them.
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	CreatedAt        time.Time `json:"created_at"`
}

// InsertDecision stores a record and returns its id.
func (s *Store) InsertDecision(ctx context.Context, d *Decision) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO decisions
		(date, decider, model, system_prompt, user_prompt, raw_response, decisions_json, guard_log, error, latency_ms, prompt_tokens, completion_tokens, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.Date.Format("2006-01-02"), d.Decider, d.Model, d.SystemPrompt, d.UserPrompt, d.RawResponse,
		d.DecisionsJSON, d.GuardLog, d.Error, d.LatencyMs, d.PromptTokens, d.CompletionTokens,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	d.ID = id
	return id, nil
}

const decisionCols = `id, date, decider, model, system_prompt, user_prompt, raw_response, decisions_json, guard_log, error, latency_ms, prompt_tokens, completion_tokens, created_at`

func scanDecision(row interface{ Scan(...any) error }) (Decision, error) {
	var d Decision
	var date, created string
	err := row.Scan(&d.ID, &date, &d.Decider, &d.Model, &d.SystemPrompt, &d.UserPrompt, &d.RawResponse,
		&d.DecisionsJSON, &d.GuardLog, &d.Error, &d.LatencyMs, &d.PromptTokens, &d.CompletionTokens, &created)
	if err != nil {
		return d, err
	}
	d.Date, _ = time.Parse("2006-01-02", date)
	d.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return d, nil
}

// GetDecision returns one record.
func (s *Store) GetDecision(ctx context.Context, id int64) (Decision, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+decisionCols+` FROM decisions WHERE id = ?`, id)
	d, err := scanDecision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return d, fmt.Errorf("store: decision %d not found", id)
	}
	return d, err
}

// ListDecisions returns the newest records first, prompts included (the
// UI shows them on demand; the list is small).
func (s *Store) ListDecisions(ctx context.Context, limit int) ([]Decision, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+decisionCols+` FROM decisions ORDER BY date DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Decision
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Order is a persisted order with its outcome.
type Order struct {
	ID           string    `json:"id"`
	DecisionID   int64     `json:"decision_id"`
	DecisionDate time.Time `json:"decision_date"`
	Symbol       string    `json:"symbol"`
	Side         string    `json:"side"`
	Qty          int       `json:"qty"`
	LimitPrice   float64   `json:"limit_price"`
	Status       string    `json:"status"`
	FillPrice    float64   `json:"fill_price"`
	FillDate     string    `json:"fill_date"`
	Fee          float64   `json:"fee"`
	Tax          float64   `json:"tax"`
	Reason       string    `json:"reason"`
}

// UpsertOrder inserts or updates an order by id.
func (s *Store) UpsertOrder(ctx context.Context, o Order) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO orders
		(id, decision_id, decision_date, symbol, side, qty, limit_price, status, fill_price, fill_date, fee, tax, reason)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status, fill_price=excluded.fill_price,
		fill_date=excluded.fill_date, fee=excluded.fee, tax=excluded.tax, reason=excluded.reason`,
		o.ID, o.DecisionID, o.DecisionDate.Format("2006-01-02"), o.Symbol, o.Side, o.Qty, o.LimitPrice,
		o.Status, o.FillPrice, o.FillDate, o.Fee, o.Tax, o.Reason)
	return err
}

// ListOrders returns orders newest first.
func (s *Store) ListOrders(ctx context.Context, limit int) ([]Order, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, decision_id, decision_date, symbol, side, qty, limit_price, status, fill_price, fill_date, fee, tax, reason
		FROM orders ORDER BY decision_date DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		var o Order
		var date string
		if err := rows.Scan(&o.ID, &o.DecisionID, &date, &o.Symbol, &o.Side, &o.Qty, &o.LimitPrice, &o.Status, &o.FillPrice, &o.FillDate, &o.Fee, &o.Tax, &o.Reason); err != nil {
			return nil, err
		}
		o.DecisionDate, _ = time.Parse("2006-01-02", date)
		out = append(out, o)
	}
	return out, rows.Err()
}

// EquityPoint is one day of the curve.
type EquityPoint struct {
	Date       time.Time `json:"date"`
	Cash       float64   `json:"cash"`
	Holdings   float64   `json:"holdings"`
	Equity     float64   `json:"equity"`
	IndexClose float64   `json:"index_close"`
}

// UpsertEquity records the account value for a day.
func (s *Store) UpsertEquity(ctx context.Context, p EquityPoint) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO equity (date, cash, holdings, equity, index_close) VALUES (?,?,?,?,?)
		ON CONFLICT(date) DO UPDATE SET cash=excluded.cash, holdings=excluded.holdings, equity=excluded.equity, index_close=excluded.index_close`,
		p.Date.Format("2006-01-02"), p.Cash, p.Holdings, p.Equity, p.IndexClose)
	return err
}

// EquityCurve returns every point, oldest first.
func (s *Store) EquityCurve(ctx context.Context) ([]EquityPoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT date, cash, holdings, equity, index_close FROM equity ORDER BY date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EquityPoint
	for rows.Next() {
		var p EquityPoint
		var date string
		if err := rows.Scan(&date, &p.Cash, &p.Holdings, &p.Equity, &p.IndexClose); err != nil {
			return nil, err
		}
		p.Date, _ = time.Parse("2006-01-02", date)
		out = append(out, p)
	}
	return out, rows.Err()
}

// SaveState stores a JSON document under a key (the paper account, the
// engine's safe-mode counters). LoadState returns ok=false when absent.
func (s *Store) SaveState(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO state (key, json, updated_at) VALUES (?,?,?)
		ON CONFLICT(key) DO UPDATE SET json=excluded.json, updated_at=excluded.updated_at`,
		key, string(b), time.Now().UTC().Format(time.RFC3339))
	return err
}

// LoadState reads a JSON document into v.
func (s *Store) LoadState(ctx context.Context, key string, v any) (bool, error) {
	var b string
	err := s.db.QueryRowContext(ctx, `SELECT json FROM state WHERE key = ?`, key).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(b), v)
}

// Reset wipes every table. Used by `replay`, which always starts clean.
func (s *Store) Reset(ctx context.Context) error {
	for _, t := range []string{"decisions", "orders", "equity", "state"} {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM `+t); err != nil {
			return err
		}
	}
	return nil
}
