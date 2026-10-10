// Command opentrade is the CLI: seed sample data, replay a period, step one
// day, or serve the API and web app.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/beiryu/opentrade/internal/api"
	"github.com/beiryu/opentrade/internal/config"
	"github.com/beiryu/opentrade/internal/engine"
	"github.com/beiryu/opentrade/internal/llm"
	"github.com/beiryu/opentrade/internal/market"
	"github.com/beiryu/opentrade/internal/paper"
	"github.com/beiryu/opentrade/internal/scorecard"
	"github.com/beiryu/opentrade/internal/store"
)

// Version is set by the Makefile from git.
var Version = "dev"

const usage = `OpenTrade — LLM trading terminal for the Vietnamese stock market (paper-first)

Usage:
  opentrade seed    [--dir data/sample] [--from 2024-10-01] [--to 2026-10-09] [--seed 42]
  opentrade replay  --from YYYY-MM-DD --to YYYY-MM-DD [--decider llm|momentum|random] [--cash 100000000]
  opentrade cycle   [--decider ...]      run the next trading day (roll + decide) on the saved state
  opentrade serve   [--decider ...]      serve the API and web app (default 127.0.0.1:8080)
  opentrade version

Settings come from OPENTRADE_* environment variables or a local .env (see .env.example).
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch os.Args[1] {
	case "seed":
		err = runSeed(os.Args[2:])
	case "replay":
		err = runReplay(ctx, os.Args[2:])
	case "cycle":
		err = runCycle(ctx, os.Args[2:])
	case "serve":
		err = runServe(ctx, os.Args[2:])
	case "version":
		fmt.Println(Version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func runSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	dir := fs.String("dir", "data/sample", "output directory")
	from := fs.String("from", "2024-10-01", "first day")
	to := fs.String("to", "2026-10-09", "last day")
	seed := fs.Int64("seed", 42, "random seed")
	fs.Parse(args)
	f, err := parseDate(*from)
	if err != nil {
		return err
	}
	t, err := parseDate(*to)
	if err != nil {
		return err
	}
	if err := market.GenerateSynthetic(*dir, market.DefaultSyntheticUniverse(), f, t, *seed); err != nil {
		return err
	}
	fmt.Printf("wrote synthetic sample data for %d symbols to %s (%s → %s, seed %d)\n",
		len(market.DefaultSyntheticUniverse()), *dir, *from, *to, *seed)
	fmt.Println("this data is made up; see data/sample/README.md")
	return nil
}

// deps opens everything a run needs.
type deps struct {
	cfg   config.Config
	m     market.Provider
	st    *store.Store
	limit engine.Limits
}

func open(dbPath string) (*deps, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	m, err := market.LoadCSVDir(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("%w (run `opentrade seed` for sample data or set OPENTRADE_DATA_DIR)", err)
	}
	if dbPath == "" {
		dbPath = cfg.DBPath
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &deps{cfg: cfg, m: m, st: st, limit: engine.Limits{
		MaxPositions: cfg.MaxPositions, MaxPositionPct: cfg.MaxPositionPct, MaxOrdersPerCycle: cfg.MaxOrdersPerCycle,
	}}, nil
}

func (d *deps) universe() engine.Universe {
	return engine.Universe{Symbols: d.cfg.Universe, ExchangeOf: d.cfg.ExchangeOf}
}

func (d *deps) decider(name string) (engine.Decider, error) {
	switch strings.ToLower(name) {
	case "llm", "":
		if d.cfg.LLMAPIKey == "" && !strings.Contains(d.cfg.LLMBaseURL, "localhost") && !strings.Contains(d.cfg.LLMBaseURL, "127.0.0.1") {
			return nil, errors.New("OPENTRADE_LLM_API_KEY is not set; use --decider momentum or random to run without a model")
		}
		c, err := llm.New(llm.Options{BaseURL: d.cfg.LLMBaseURL, APIKey: d.cfg.LLMAPIKey, Model: d.cfg.LLMModel,
			JSONMode: true, Temperature: 0.2, MaxTokens: 1500, Timeout: 90 * time.Second, Retries: 1})
		if err != nil {
			return nil, err
		}
		return &engine.LLMDecider{Client: c}, nil
	case "momentum":
		return engine.MomentumDecider{}, nil
	case "random":
		return engine.NewRandomDecider(1), nil
	}
	return nil, fmt.Errorf("unknown decider %q (llm, momentum, random)", name)
}

func (d *deps) engine(dec engine.Decider) *engine.Engine {
	return &engine.Engine{Market: d.m, Universe: d.universe(), Store: d.st, Decider: dec, Limits: d.limit,
		Account: paper.NewAccount(d.cfg.InitialCash), Logf: func(f string, a ...any) { fmt.Printf(f+"\n", a...) }}
}

func runReplay(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	from := fs.String("from", "", "first trading day (YYYY-MM-DD)")
	to := fs.String("to", "", "last trading day (YYYY-MM-DD)")
	deciderName := fs.String("decider", "llm", "llm | momentum | random")
	cash := fs.Float64("cash", 0, "initial cash (default OPENTRADE_INITIAL_CASH)")
	noRandom := fs.Bool("no-random", false, "skip the random baseline run")
	fs.Parse(args)
	if *from == "" || *to == "" {
		return errors.New("--from and --to are required")
	}
	f, err := parseDate(*from)
	if err != nil {
		return err
	}
	t, err := parseDate(*to)
	if err != nil {
		return err
	}
	d, err := open("")
	if err != nil {
		return err
	}
	defer d.st.Close()
	dec, err := d.decider(*deciderName)
	if err != nil {
		return err
	}
	if *cash == 0 {
		*cash = d.cfg.InitialCash
	}
	e := d.engine(dec)
	fmt.Printf("replay %s → %s with decider %s, cash %.0f, universe %s\n", *from, *to, dec.Name(), *cash, strings.Join(d.cfg.Universe, ","))
	start := time.Now()
	if err := e.Replay(ctx, f, t, *cash); err != nil {
		return err
	}
	curve, _ := d.st.EquityCurve(ctx)
	orders, _ := d.st.ListOrders(ctx, 100000)

	var rcurve []store.EquityPoint
	var rorders []store.Order
	if !*noRandom {
		mem, err := store.Open(":memory:")
		if err != nil {
			return err
		}
		re := &engine.Engine{Market: d.m, Universe: d.universe(), Store: mem, Decider: engine.NewRandomDecider(1), Limits: d.limit}
		if err := re.Replay(ctx, f, t, *cash); err != nil {
			return fmt.Errorf("random baseline: %w", err)
		}
		rcurve, _ = mem.EquityCurve(ctx)
		rorders, _ = mem.ListOrders(ctx, 100000)
		mem.Close()
	}
	card := scorecard.Build(dec.Name(), curve, orders, rcurve, rorders)
	if err := d.st.SaveState(ctx, api.ScorecardKey, card); err != nil {
		return err
	}
	fmt.Printf("\ndone in %s\n\n", time.Since(start).Round(time.Second))
	printCard(card)
	return nil
}

func printCard(c scorecard.Card) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "SCORECARD %s → %s\n", c.From.Format("2006-01-02"), c.To.Format("2006-01-02"))
	fmt.Fprintln(w, "\tstrategy\tVN-Index\trandom")
	row := func(label string, f func(m scorecard.Metrics) string) {
		r := "-"
		if c.Random != nil {
			r = f(*c.Random)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", label, f(c.Strategy), f(c.Index), r)
	}
	pct := func(v float64) string { return fmt.Sprintf("%+.2f%%", v*100) }
	row("total return", func(m scorecard.Metrics) string { return pct(m.TotalReturn) })
	row("annualised", func(m scorecard.Metrics) string { return pct(m.AnnualReturn) })
	row("max drawdown", func(m scorecard.Metrics) string { return fmt.Sprintf("%.2f%%", m.MaxDrawdown*100) })
	row("volatility", func(m scorecard.Metrics) string { return fmt.Sprintf("%.2f%%", m.Volatility*100) })
	row("sharpe", func(m scorecard.Metrics) string { return fmt.Sprintf("%.2f", m.Sharpe) })
	row("filled orders", func(m scorecard.Metrics) string { return fmt.Sprint(m.Trades) })
	row("round trips", func(m scorecard.Metrics) string { return fmt.Sprint(m.RoundTrips) })
	row("win rate", func(m scorecard.Metrics) string {
		if m.RoundTrips == 0 {
			return "-"
		}
		return fmt.Sprintf("%.0f%%", m.WinRate*100)
	})
	w.Flush()
	fmt.Printf("\n%s\n", c.Verdict)
}

func runCycle(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("cycle", flag.ExitOnError)
	deciderName := fs.String("decider", "llm", "llm | momentum | random")
	fs.Parse(args)
	d, err := open("")
	if err != nil {
		return err
	}
	defer d.st.Close()
	dec, err := d.decider(*deciderName)
	if err != nil {
		return err
	}
	e := d.engine(dec)
	if err := e.Load(ctx); err != nil {
		return err
	}
	res, ok, err := e.Step(ctx)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Printf("no trading day after %s in the data\n", e.State.LastDate.Format("2006-01-02"))
		return nil
	}
	for _, g := range res.GuardLog {
		fmt.Printf("  guard %s %s %s: %s\n", g.Symbol, g.Action, g.Rule, g.Detail)
	}
	for _, o := range res.Orders {
		fmt.Printf("  order %s %s %d @ %.0f → %s %s\n", o.Side, o.Symbol, o.Qty, o.LimitPrice, o.Status, o.Reason)
	}
	return nil
}

func runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	deciderName := fs.String("decider", "llm", "llm | momentum | random")
	listen := fs.String("listen", "", "address (default OPENTRADE_LISTEN)")
	fs.Parse(args)
	d, err := open("")
	if err != nil {
		return err
	}
	defer d.st.Close()
	dec, err := d.decider(*deciderName)
	if err != nil {
		// Serving read-only views without a model is still useful.
		fmt.Println("note:", err, "— serving with the momentum decider")
		dec = engine.MomentumDecider{}
	}
	e := d.engine(dec)
	e.Logf = engine.StdLogf
	if err := e.Load(ctx); err != nil {
		return err
	}
	if *listen == "" {
		*listen = d.cfg.Listen
	}
	srv := &http.Server{Addr: *listen, Handler: (&api.Server{Engine: e, Version: Version}).Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	fmt.Printf("OpenTrade %s listening on http://%s (decider %s, data %s, db %s)\n", Version, *listen, dec.Name(), d.cfg.DataDir, d.cfg.DBPath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
