package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"

	"github.com/beiryu/opentrade/internal/llm"
	"github.com/beiryu/opentrade/internal/rules"
)

// Outcome is a decider's answer plus everything needed to audit it.
type Outcome struct {
	Response         Response
	SystemPrompt     string
	UserPrompt       string
	Raw              string
	Model            string
	LatencyMs        int64
	PromptTokens     int
	CompletionTokens int
}

// Decider turns a Context into orders.
type Decider interface {
	Name() string
	Decide(ctx context.Context, c *Context) (Outcome, error)
}

// LLMDecider asks a language model.
type LLMDecider struct {
	Client llm.Client
}

// Name implements Decider.
func (d *LLMDecider) Name() string { return "llm" }

// Decide implements Decider. A reply that is not valid JSON of the expected
// shape is returned as an error together with the raw text, so the record
// shows exactly what the model said.
func (d *LLMDecider) Decide(ctx context.Context, c *Context) (Outcome, error) {
	out := Outcome{SystemPrompt: SystemPrompt(c.Limits), UserPrompt: UserPrompt(c), Model: d.Client.Name()}
	res, err := d.Client.Complete(ctx, out.SystemPrompt, out.UserPrompt)
	out.LatencyMs, out.PromptTokens, out.CompletionTokens = res.LatencyMs, res.PromptTokens, res.CompletionTokens
	if res.Model != "" {
		out.Model = res.Model
	}
	if err != nil {
		return out, err
	}
	out.Raw = res.Text
	resp, err := ParseResponse(res.Text)
	if err != nil {
		return out, err
	}
	out.Response = resp
	return out, nil
}

// ParseResponse decodes a model reply into a Response, tolerating fences
// and prose around the JSON.
func ParseResponse(text string) (Response, error) {
	var r Response
	raw := llm.ExtractJSON(text)
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return r, fmt.Errorf("engine: reply is not the expected JSON: %w", err)
	}
	return r, nil
}

// MomentumDecider is a transparent rule used as a baseline and for offline
// runs: buy the strongest 20-day movers that trade above their 50-day
// average and are not overbought; sell what falls below the average or gets
// overbought. It is not meant to be good, it is meant to be explainable.
type MomentumDecider struct{}

// Name implements Decider.
func (MomentumDecider) Name() string { return "momentum" }

// Decide implements Decider.
func (MomentumDecider) Decide(_ context.Context, c *Context) (Outcome, error) {
	var resp Response
	held := map[string]bool{}
	for _, p := range c.Positions {
		held[p.Symbol] = true
		cd, ok := c.Candidate(p.Symbol)
		if !ok || p.Settled == 0 {
			continue
		}
		if (cd.SMA50 > 0 && cd.Close < cd.SMA50) || cd.RSI14 > 75 {
			resp.Decisions = append(resp.Decisions, Decision{Symbol: p.Symbol, Action: Sell, Lots: p.Settled / rules.Lot, Confidence: 60,
				Reasoning: fmt.Sprintf("close %.0f vs sma50 %.0f, rsi %.0f", cd.Close, cd.SMA50, cd.RSI14)})
		}
	}
	slots := c.Limits.MaxPositions - len(c.Positions)
	if slots <= 0 || c.SafeMode {
		resp.Notes = "no free slots"
		return Outcome{Response: resp, Model: "momentum"}, nil
	}
	cands := make([]Candidate, 0, len(c.Candidates))
	for _, cd := range c.Candidates {
		if held[cd.Symbol] || cd.History < 50 || cd.SMA50 == 0 || cd.Close <= cd.SMA50 || cd.RSI14 >= 70 || cd.Ret20 <= 0 {
			continue
		}
		cands = append(cands, cd)
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].Ret20 > cands[j].Ret20 })
	budget := c.Equity * c.Limits.MaxPositionPct
	if budget > c.Available {
		budget = c.Available
	}
	for i, cd := range cands {
		if i >= 2 || i >= slots {
			break
		}
		lots := rules.MaxAffordableLots(budget/2, cd.Ceiling)
		if lots == 0 {
			continue
		}
		resp.Decisions = append(resp.Decisions, Decision{Symbol: cd.Symbol, Action: Buy, Lots: lots, Confidence: 55,
			Reasoning: fmt.Sprintf("ret20 %+.1f%%, above sma50, rsi %.0f", cd.Ret20*100, cd.RSI14)})
	}
	return Outcome{Response: resp, Model: "momentum"}, nil
}

// RandomDecider trades at random with the same constraints. Its equity
// curve is the floor any strategy must beat; a seed makes it repeatable.
type RandomDecider struct {
	rng *rand.Rand
}

// NewRandomDecider seeds the generator.
func NewRandomDecider(seed int64) *RandomDecider {
	return &RandomDecider{rng: rand.New(rand.NewSource(seed))}
}

// Name implements Decider.
func (*RandomDecider) Name() string { return "random" }

// Decide implements Decider: on a third of the days, one random buy (if a
// slot is free) and, independently, one random sell of a settled holding.
func (d *RandomDecider) Decide(_ context.Context, c *Context) (Outcome, error) {
	var resp Response
	if d.rng.Float64() < 0.33 && len(c.Positions) < c.Limits.MaxPositions && !c.SafeMode && len(c.Candidates) > 0 {
		cd := c.Candidates[d.rng.Intn(len(c.Candidates))]
		if _, held := c.Position(cd.Symbol); !held {
			budget := c.Equity * c.Limits.MaxPositionPct * (0.3 + 0.7*d.rng.Float64())
			if lots := rules.MaxAffordableLots(budget, cd.Ceiling); lots > 0 {
				resp.Decisions = append(resp.Decisions, Decision{Symbol: cd.Symbol, Action: Buy, Lots: lots, Confidence: 50, Reasoning: "random"})
			}
		}
	}
	if d.rng.Float64() < 0.33 && len(c.Positions) > 0 {
		p := c.Positions[d.rng.Intn(len(c.Positions))]
		if p.Settled >= rules.Lot {
			resp.Decisions = append(resp.Decisions, Decision{Symbol: p.Symbol, Action: Sell, Lots: p.Settled / rules.Lot, Confidence: 50, Reasoning: "random"})
		}
	}
	return Outcome{Response: resp, Model: "random"}, nil
}
