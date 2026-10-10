package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "sub", "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	d := &Decision{Date: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), Decider: "llm", Model: "m",
		SystemPrompt: "sys", UserPrompt: "usr", RawResponse: `{"decisions":[]}`, DecisionsJSON: "[]", GuardLog: "[]", LatencyMs: 12}
	id, err := s.InsertDecision(ctx, d)
	if err != nil || id == 0 {
		t.Fatal(err)
	}
	got, err := s.GetDecision(ctx, id)
	if err != nil || got.RawResponse != d.RawResponse || !got.Date.Equal(d.Date) {
		t.Fatalf("GetDecision = %+v, %v", got, err)
	}
	list, err := s.ListDecisions(ctx, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListDecisions = %v, %v", list, err)
	}

	o := Order{ID: "o1", DecisionID: id, DecisionDate: d.Date, Symbol: "FPT", Side: "buy", Qty: 100, Status: "pending"}
	if err := s.UpsertOrder(ctx, o); err != nil {
		t.Fatal(err)
	}
	o.Status, o.FillPrice = "filled", 50_000
	if err := s.UpsertOrder(ctx, o); err != nil {
		t.Fatal(err)
	}
	orders, _ := s.ListOrders(ctx, 10)
	if len(orders) != 1 || orders[0].Status != "filled" || orders[0].FillPrice != 50_000 {
		t.Fatalf("orders = %+v", orders)
	}

	if err := s.UpsertEquity(ctx, EquityPoint{Date: d.Date, Cash: 1, Holdings: 2, Equity: 3, IndexClose: 1250}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertEquity(ctx, EquityPoint{Date: d.Date, Cash: 1, Holdings: 3, Equity: 4}); err != nil {
		t.Fatal(err)
	}
	curve, _ := s.EquityCurve(ctx)
	if len(curve) != 1 || curve[0].Equity != 4 {
		t.Fatalf("curve = %+v", curve)
	}

	type st struct{ N int }
	if err := s.SaveState(ctx, "k", st{N: 7}); err != nil {
		t.Fatal(err)
	}
	var back st
	ok, err := s.LoadState(ctx, "k", &back)
	if err != nil || !ok || back.N != 7 {
		t.Fatalf("LoadState = %v %v %+v", ok, err, back)
	}
	if ok, _ := s.LoadState(ctx, "missing", &back); ok {
		t.Fatal("missing key should be ok=false")
	}

	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListDecisions(ctx, 10); len(list) != 0 {
		t.Fatal("Reset should clear decisions")
	}
}

func TestOpenTwiceIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.db")
	for i := 0; i < 2; i++ {
		s, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
}
