// Package api serves the JSON endpoints the web app reads and the embedded
// web app itself. Local-only by design: no auth, bind to 127.0.0.1.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/beiryu/opentrade/internal/api/ui"
	"github.com/beiryu/opentrade/internal/engine"
	"github.com/beiryu/opentrade/internal/market"
	"github.com/beiryu/opentrade/internal/scorecard"
	"github.com/beiryu/opentrade/internal/store"
)

// Server owns one engine; Step is serialised.
type Server struct {
	Engine *engine.Engine
	// Version string shown in /api/health.
	Version string
	mu      sync.Mutex
}

// Handler builds the mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/decisions", s.decisions)
	mux.HandleFunc("GET /api/decisions/{id}", s.decision)
	mux.HandleFunc("GET /api/orders", s.orders)
	mux.HandleFunc("GET /api/portfolio", s.portfolio)
	mux.HandleFunc("GET /api/equity", s.equity)
	mux.HandleFunc("GET /api/scorecard", s.scorecard)
	mux.HandleFunc("POST /api/cycle", s.cycle)
	mux.Handle("/", s.static())
	return logRequests(mux)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "version": s.Version, "ui_built": ui.Built()})
}

// stateView is what the dashboard header shows.
type stateView struct {
	Decider     string        `json:"decider"`
	Model       string        `json:"model"`
	State       engine.State  `json:"state"`
	Universe    []string      `json:"universe"`
	Limits      engine.Limits `json:"limits"`
	FirstDay    time.Time     `json:"first_day"`
	LastDay     time.Time     `json:"last_day"`
	InitialCash float64       `json:"initial_cash"`
}

func (s *Server) state(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cal := s.Engine.Market.Calendar()
	v := stateView{Decider: s.Engine.Decider.Name(), State: s.Engine.State, Universe: s.Engine.Universe.Symbols,
		Limits: s.Engine.Limits, InitialCash: s.Engine.Account.InitialCash}
	if n, ok := s.Engine.Decider.(interface{ ModelName() string }); ok {
		v.Model = n.ModelName()
	}
	if len(cal) > 0 {
		v.FirstDay, v.LastDay = cal[0], cal[len(cal)-1]
	}
	writeJSON(w, 200, v)
}

func (s *Server) decisions(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Engine.Store.ListDecisions(r.Context(), limit)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if list == nil {
		list = []store.Decision{}
	}
	// The list omits prompts and raw output; the detail endpoint has them.
	type row struct {
		store.Decision
		SystemPrompt string `json:"system_prompt,omitempty"`
		UserPrompt   string `json:"user_prompt,omitempty"`
		RawResponse  string `json:"raw_response,omitempty"`
	}
	out := make([]row, len(list))
	for i, d := range list {
		d.SystemPrompt, d.UserPrompt, d.RawResponse = "", "", ""
		out[i] = row{Decision: d}
	}
	writeJSON(w, 200, out)
}

func (s *Server) decision(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, errors.New("bad id"))
		return
	}
	d, err := s.Engine.Store.GetDecision(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	orders, _ := s.Engine.Store.ListOrders(r.Context(), 1000)
	var mine []store.Order
	for _, o := range orders {
		if o.DecisionID == id {
			mine = append(mine, o)
		}
	}
	if mine == nil {
		mine = []store.Order{}
	}
	writeJSON(w, 200, map[string]any{"decision": d, "orders": mine})
}

func (s *Server) orders(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Engine.Store.ListOrders(r.Context(), limit)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if list == nil {
		list = []store.Order{}
	}
	writeJSON(w, 200, list)
}

func (s *Server) portfolio(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	acct := s.Engine.Account
	day := s.Engine.State.LastDate
	prices := map[string]float64{}
	if !day.IsZero() {
		for _, sym := range s.Engine.Universe.Symbols {
			if b, err := s.Engine.Market.Bars(sym, day, 1); err == nil {
				prices[sym] = b[0].Close
			}
		}
	}
	equity, holdings := acct.Equity(prices)
	positions := acct.Holdings()
	for i := range positions {
		p := &positions[i]
		if px, ok := prices[p.Symbol]; ok {
			p.LastPrice = px
			p.Value = float64(p.Qty) * px
			p.PnL = p.Value - p.Cost
			if p.Cost > 0 {
				p.PnLPct = p.PnL / p.Cost
			}
		}
	}
	recv := 0.0
	for _, r := range acct.Receivables {
		recv += r.Amount
	}
	writeJSON(w, 200, map[string]any{
		"as_of":        day,
		"cash":         acct.Cash,
		"available":    acct.Available(),
		"reserved":     acct.Reserved(),
		"receivables":  recv,
		"holdings":     holdings,
		"equity":       equity,
		"initial_cash": acct.InitialCash,
		"positions":    positions,
		"pending":      acct.Pending,
	})
}

func (s *Server) equity(w http.ResponseWriter, r *http.Request) {
	curve, err := s.Engine.Store.EquityCurve(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if curve == nil {
		curve = []store.EquityPoint{}
	}
	writeJSON(w, 200, curve)
}

// ScorecardKey is where `replay` stores the card with the random baseline.
const ScorecardKey = "scorecard"

func (s *Server) scorecard(w http.ResponseWriter, r *http.Request) {
	var card scorecard.Card
	if ok, err := s.Engine.Store.LoadState(r.Context(), ScorecardKey, &card); err == nil && ok {
		writeJSON(w, 200, card)
		return
	}
	// No stored card: build one without the random baseline.
	curve, err := s.Engine.Store.EquityCurve(r.Context())
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	orders, _ := s.Engine.Store.ListOrders(r.Context(), 100000)
	writeJSON(w, 200, scorecard.Build(s.Engine.Decider.Name(), curve, orders, nil, nil))
}

func (s *Server) cycle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	res, ok, err := s.Engine.Step(ctx)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if !ok {
		writeJSON(w, 409, map[string]any{"error": "no more trading days in the data", "last": s.Engine.State.LastDate})
		return
	}
	writeJSON(w, 200, res)
}

// static serves the embedded app with SPA fallback to index.html.
func (s *Server) static() http.Handler {
	fsys := ui.FS()
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ui.Built() {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(notBuilt))
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(fsys, p); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

const notBuilt = `<!doctype html><meta charset="utf-8"><title>OpenTrade</title>
<body style="font-family:system-ui;max-width:40rem;margin:4rem auto;line-height:1.5">
<h1>OpenTrade API is running</h1>
<p>The web app is not built into this binary. Run <code>make web</code> (needs Node 20 and pnpm) and restart, or use the API directly: <a href="/api/state">/api/state</a>, <a href="/api/decisions">/api/decisions</a>, <a href="/api/portfolio">/api/portfolio</a>, <a href="/api/equity">/api/equity</a>, <a href="/api/scorecard">/api/scorecard</a>.</p>
</body>`

// Unused import guard for market (kept for the portfolio handler's types).
var _ = market.IndexSymbol
