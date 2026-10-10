package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/beiryu/opentrade/internal/llm"
	"github.com/beiryu/opentrade/internal/market"
	"github.com/beiryu/opentrade/internal/paper"
	"github.com/beiryu/opentrade/internal/rules"
	"github.com/beiryu/opentrade/internal/store"
)

var testLimits = Limits{MaxPositions: 3, MaxPositionPct: 0.3, MaxOrdersPerCycle: 3}

func testMarket(t *testing.T) *market.CSVProvider {
	t.Helper()
	dir := t.TempDir()
	specs := market.DefaultSyntheticUniverse()[:4] // index, FPT, VNM, VCB
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	if err := market.GenerateSynthetic(dir, specs, start, end, 7); err != nil {
		t.Fatal(err)
	}
	p, err := market.LoadCSVDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func testUniverse() Universe {
	return Universe{Symbols: []string{"FPT", "VNM", "VCB"}, ExchangeOf: func(string) rules.Exchange { return rules.HOSE }}
}

func newEngine(t *testing.T, d Decider) (*Engine, *market.CSVProvider) {
	t.Helper()
	m := testMarket(t)
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &Engine{Market: m, Universe: testUniverse(), Store: s, Decider: d, Limits: testLimits, Account: paper.NewAccount(100_000_000)}, m
}

func TestBuildContextAndPrompt(t *testing.T) {
	m := testMarket(t)
	cal := m.Calendar()
	day := cal[55]
	c, prices, err := BuildContext(context.Background(), m, testUniverse(), day, paper.NewAccount(1e8), nil, testLimits, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Candidates) != 3 || len(prices) != 3 || c.Index.Close == 0 {
		t.Fatalf("context = %+v", c)
	}
	fpt, _ := c.Candidate("FPT")
	if fpt.SMA50 == 0 || fpt.RSI14 == 0 || fpt.Floor >= fpt.Close || fpt.Ceiling <= fpt.Close {
		t.Fatalf("candidate indicators missing: %+v", fpt)
	}
	if !c.NextSession.After(day) {
		t.Fatal("next session must follow the decision day")
	}
	up := UserPrompt(c)
	for _, want := range []string{"DATE " + day.Format("2006-01-02"), "FPT", "VNINDEX", "cash_available 100,000,000"} {
		if !strings.Contains(up, want) {
			t.Errorf("user prompt lacks %q:\n%s", want, up)
		}
	}
	if sp := SystemPrompt(testLimits); !strings.Contains(sp, "At most 3 open positions") || !strings.Contains(sp, "T+2") {
		t.Error("system prompt lacks limits or rules")
	}
}

func TestGuards(t *testing.T) {
	m := testMarket(t)
	day := m.Calendar()[55]
	acct := paper.NewAccount(100_000_000) // 30 % cap = 30M, ~2 lots of FPT at 120k
	c, _, err := BuildContext(context.Background(), m, testUniverse(), day, acct, nil, testLimits, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	fpt, _ := c.Candidate("FPT")
	vnm, _ := c.Candidate("VNM")
	resp := Response{Decisions: []Decision{
		{Symbol: "fpt.hm", Action: "BUY", Lots: 50, LimitPrice: fpt.Close / 1000, Confidence: 80}, // scaled price, trimmed by the position cap
		{Symbol: "FPT", Action: Buy, Lots: 1},                                                     // duplicate
		{Symbol: "XYZ", Action: Buy, Lots: 1},                                                     // not in universe
		{Symbol: "VNM", Action: Sell, Lots: 1},                                                    // nothing to sell
		{Symbol: "VCB", Action: Hold},
		{Symbol: "VNM", Action: Buy, Lots: 1, LimitPrice: vnm.Ceiling * 2}, // band clamp; second VNM entry after a sell is a duplicate
	}}
	out, log := ApplyGuards(c, resp, acct)
	rulesHit := map[string]int{}
	for _, g := range log {
		rulesHit[g.Rule]++
	}
	for _, want := range []string{"price_scaled", "duplicate", "not_in_universe", "nothing_to_sell", "trimmed_to_position_cap"} {
		if rulesHit[want] == 0 {
			t.Errorf("expected guard %s in %+v", want, log)
		}
	}
	if len(out) != 1 || out[0].Symbol != "FPT" || out[0].Action != Buy {
		t.Fatalf("orders = %+v", out)
	}
	if float64(out[0].Lots*100)*out[0].LimitPrice > 30_000_000 {
		t.Fatalf("buy not trimmed to the 30%% position cap: %+v", out[0])
	}
	if !rules.IsOnTick(rules.HOSE, out[0].LimitPrice) {
		t.Fatalf("limit not on tick: %v", out[0].LimitPrice)
	}
}

func TestCycleWithFakeLLM(t *testing.T) {
	fake := &llm.Fake{Reply: func(_, user string) (string, error) {
		return "```json\n{\"decisions\":[{\"symbol\":\"FPT\",\"action\":\"buy\",\"lots\":1,\"limit_price\":0,\"confidence\":70,\"reasoning\":\"test\"}],\"notes\":\"n\"}\n```", nil
	}}
	e, m := newEngine(t, &LLMDecider{Client: fake})
	ctx := context.Background()
	cal := m.Calendar()
	d0 := cal[55]
	res, err := e.Cycle(ctx, d0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Orders) != 1 || res.Orders[0].Status != paper.Pending || res.Error != "" {
		t.Fatalf("cycle = %+v", res)
	}
	rec, err := e.Store.GetDecision(ctx, res.DecisionID)
	if err != nil || !strings.Contains(rec.RawResponse, "```json") || rec.SystemPrompt == "" {
		t.Fatalf("record = %+v %v", rec, err)
	}
	// Next day: the order fills and the position appears.
	filled, err := e.Roll(ctx, cal[56])
	if err != nil || len(filled) != 1 || filled[0].Status != paper.Filled {
		t.Fatalf("roll = %+v %v", filled, err)
	}
	if e.Account.Position("FPT").Qty != 100 {
		t.Fatal("position missing after fill")
	}
	orders, _ := e.Store.ListOrders(ctx, 10)
	if orders[0].Status != "filled" || orders[0].DecisionID != res.DecisionID {
		t.Fatalf("stored order = %+v", orders[0])
	}
	curve, _ := e.Store.EquityCurve(ctx)
	if len(curve) != 2 {
		t.Fatalf("equity points = %d", len(curve))
	}
	// Persisted state survives a reload.
	e2 := &Engine{Store: e.Store, Account: paper.NewAccount(1)}
	if err := e2.Load(ctx); err != nil || e2.Account.Position("FPT").Qty != 100 || e2.State.CycleNo != 1 {
		t.Fatalf("reload = %+v %+v %v", e2.Account, e2.State, err)
	}
}

func TestDeciderFailureAndSafeMode(t *testing.T) {
	calls := 0
	fake := &llm.Fake{Reply: func(_, _ string) (string, error) {
		calls++
		if calls <= 3 {
			return "sorry, I cannot help with that", nil // not JSON
		}
		return `{"decisions":[{"symbol":"FPT","action":"buy","lots":1}]}`, nil
	}}
	e, m := newEngine(t, &LLMDecider{Client: fake})
	ctx := context.Background()
	cal := m.Calendar()
	for i := 0; i < 3; i++ {
		res, err := e.Cycle(ctx, cal[55+i])
		if err != nil {
			t.Fatal(err)
		}
		if res.Error == "" || len(res.Orders) != 0 {
			t.Fatalf("cycle %d should record the parse error and place nothing: %+v", i, res)
		}
	}
	if !e.State.SafeMode {
		t.Fatal("safe mode expected after 3 failures")
	}
	res, err := e.Cycle(ctx, cal[58])
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "" || e.State.SafeMode || len(res.Orders) != 1 {
		t.Fatalf("recovery cycle = %+v safe=%v", res, e.State.SafeMode)
	}
	// A transport error is also a decider failure, not a crash.
	e.Decider = &LLMDecider{Client: &llm.Fake{Reply: func(_, _ string) (string, error) { return "", errors.New("boom") }}}
	if res, err := e.Cycle(ctx, cal[59]); err != nil || res.Error != "boom" {
		t.Fatalf("transport error = %+v %v", res, err)
	}
}

func TestReplayIsDeterministic(t *testing.T) {
	run := func() ([]store.EquityPoint, int) {
		e, m := newEngine(t, MomentumDecider{})
		cal := m.Calendar()
		if err := e.Replay(context.Background(), cal[50], cal[len(cal)-1], 100_000_000); err != nil {
			t.Fatal(err)
		}
		curve, _ := e.Store.EquityCurve(context.Background())
		orders, _ := e.Store.ListOrders(context.Background(), 1000)
		return curve, len(orders)
	}
	c1, n1 := run()
	c2, n2 := run()
	if len(c1) == 0 || len(c1) != len(c2) || n1 != n2 || n1 == 0 {
		t.Fatalf("replays differ or empty: %d/%d points, %d/%d orders", len(c1), len(c2), n1, n2)
	}
	if c1[len(c1)-1].Equity != c2[len(c2)-1].Equity {
		t.Fatal("replay not deterministic")
	}
	b, _ := json.Marshal(c1[len(c1)-1])
	t.Logf("final point %s after %d orders", b, n1)
}

func TestRandomDeciderSeeded(t *testing.T) {
	e, m := newEngine(t, NewRandomDecider(1))
	cal := m.Calendar()
	if err := e.Replay(context.Background(), cal[50], cal[90], 100_000_000); err != nil {
		t.Fatal(err)
	}
	o1, _ := e.Store.ListOrders(context.Background(), 1000)
	e.Decider = NewRandomDecider(1)
	if err := e.Replay(context.Background(), cal[50], cal[90], 100_000_000); err != nil {
		t.Fatal(err)
	}
	o2, _ := e.Store.ListOrders(context.Background(), 1000)
	if len(o1) == 0 || len(o1) != len(o2) {
		t.Fatalf("random decider not repeatable: %d vs %d orders", len(o1), len(o2))
	}
}
