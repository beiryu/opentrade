package market

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/beiryu/opentrade/internal/rules"
)

// SyntheticSpec describes one synthetic series. Nothing here is real market
// data: the sample set exists so the pipeline runs end to end before a real
// data provider is wired in.
type SyntheticSpec struct {
	Symbol     string
	Exchange   rules.Exchange
	StartPrice float64
	// AnnualDrift and AnnualVol parameterise a geometric random walk.
	AnnualDrift float64
	AnnualVol   float64
	// AvgVolume is the mean daily volume in shares.
	AvgVolume int64
	// Beta scales the index's daily move mixed into this symbol.
	Beta float64
}

// DefaultSyntheticUniverse is the sample set: ten large HOSE names plus the
// index, with made-up parameters in the right ballpark.
func DefaultSyntheticUniverse() []SyntheticSpec {
	return []SyntheticSpec{
		{Symbol: IndexSymbol, Exchange: rules.HOSE, StartPrice: 1250, AnnualDrift: 0.08, AnnualVol: 0.16, AvgVolume: 700_000_000, Beta: 0},
		{Symbol: "FPT", Exchange: rules.HOSE, StartPrice: 120_000, AnnualDrift: 0.15, AnnualVol: 0.28, AvgVolume: 3_000_000, Beta: 0.9},
		{Symbol: "VNM", Exchange: rules.HOSE, StartPrice: 68_000, AnnualDrift: 0.02, AnnualVol: 0.22, AvgVolume: 2_500_000, Beta: 0.7},
		{Symbol: "VCB", Exchange: rules.HOSE, StartPrice: 90_000, AnnualDrift: 0.08, AnnualVol: 0.24, AvgVolume: 1_500_000, Beta: 1.0},
		{Symbol: "HPG", Exchange: rules.HOSE, StartPrice: 27_000, AnnualDrift: 0.05, AnnualVol: 0.35, AvgVolume: 25_000_000, Beta: 1.3},
		{Symbol: "MWG", Exchange: rules.HOSE, StartPrice: 55_000, AnnualDrift: 0.10, AnnualVol: 0.33, AvgVolume: 6_000_000, Beta: 1.1},
		{Symbol: "VIC", Exchange: rules.HOSE, StartPrice: 42_000, AnnualDrift: 0.00, AnnualVol: 0.38, AvgVolume: 4_000_000, Beta: 1.2},
		{Symbol: "VHM", Exchange: rules.HOSE, StartPrice: 40_000, AnnualDrift: -0.02, AnnualVol: 0.36, AvgVolume: 8_000_000, Beta: 1.2},
		{Symbol: "GAS", Exchange: rules.HOSE, StartPrice: 75_000, AnnualDrift: 0.03, AnnualVol: 0.25, AvgVolume: 1_200_000, Beta: 0.8},
		{Symbol: "MSN", Exchange: rules.HOSE, StartPrice: 70_000, AnnualDrift: 0.04, AnnualVol: 0.34, AvgVolume: 3_500_000, Beta: 1.1},
		{Symbol: "TCB", Exchange: rules.HOSE, StartPrice: 24_000, AnnualDrift: 0.09, AnnualVol: 0.30, AvgVolume: 15_000_000, Beta: 1.1},
	}
}

// GenerateSynthetic writes one CSV per spec into dir covering the weekdays
// from start to end (no holiday calendar), deterministic for a seed. The
// index is generated first and its daily returns are mixed into each stock
// by Beta so the set moves like a market. Prices respect tick sizes and
// stay inside the daily band of the previous close.
func GenerateSynthetic(dir string, specs []SyntheticSpec, start, end time.Time, seed int64) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	days := weekdays(Day(start), Day(end))
	if len(days) < 2 {
		return fmt.Errorf("market: need at least two days, got %d", len(days))
	}
	rng := rand.New(rand.NewSource(seed))

	// Index first.
	var indexRet []float64
	for _, s := range specs {
		if s.Symbol == IndexSymbol {
			bars, rets := walk(rng, s, days, nil, false)
			indexRet = rets
			if err := writeSymbol(dir, s.Symbol, bars); err != nil {
				return err
			}
		}
	}
	for _, s := range specs {
		if s.Symbol == IndexSymbol {
			continue
		}
		bars, _ := walk(rng, s, days, indexRet, true)
		if err := writeSymbol(dir, s.Symbol, bars); err != nil {
			return err
		}
	}
	return nil
}

func writeSymbol(dir, symbol string, bars []Bar) error {
	fh, err := os.Create(filepath.Join(dir, symbol+".csv"))
	if err != nil {
		return err
	}
	defer fh.Close()
	return WriteCSV(fh, bars)
}

func weekdays(start, end time.Time) []time.Time {
	var out []time.Time
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			out = append(out, d)
		}
	}
	return out
}

// walk produces bars for one spec. Returns the bars and the daily returns
// (so the index's can be reused). clamp applies tick and band rules, which
// the index itself does not have.
func walk(rng *rand.Rand, s SyntheticSpec, days []time.Time, indexRet []float64, clamp bool) ([]Bar, []float64) {
	const tradingDays = 250.0
	mu := s.AnnualDrift / tradingDays
	sigma := s.AnnualVol / math.Sqrt(tradingDays)
	bars := make([]Bar, 0, len(days))
	rets := make([]float64, 0, len(days))
	prevClose := s.StartPrice
	for i, d := range days {
		idio := rng.NormFloat64() * sigma
		ret := mu + idio
		if indexRet != nil && i < len(indexRet) {
			// Mix the market move in; keep idiosyncratic part at ~60 %.
			ret = mu + s.Beta*indexRet[i] + idio*0.6
		}
		closeP := prevClose * math.Exp(ret)
		openP := prevClose * math.Exp(rng.NormFloat64()*sigma*0.3)
		hi := math.Max(openP, closeP) * (1 + math.Abs(rng.NormFloat64())*sigma*0.4)
		lo := math.Min(openP, closeP) * (1 - math.Abs(rng.NormFloat64())*sigma*0.4)
		if clamp {
			floor, ceiling := rules.Band(s.Exchange, prevClose)
			closeP = clampTick(s.Exchange, closeP, floor, ceiling)
			openP = clampTick(s.Exchange, openP, floor, ceiling)
			hi = clampTick(s.Exchange, math.Max(hi, math.Max(openP, closeP)), floor, ceiling)
			lo = clampTick(s.Exchange, math.Min(lo, math.Min(openP, closeP)), floor, ceiling)
		} else {
			closeP = math.Round(closeP*100) / 100
			openP = math.Round(openP*100) / 100
			hi = math.Round(math.Max(hi, math.Max(openP, closeP))*100) / 100
			lo = math.Round(math.Min(lo, math.Min(openP, closeP))*100) / 100
		}
		volMult := math.Exp(rng.NormFloat64()*0.35) * (1 + 3*math.Abs(ret))
		vol := int64(float64(s.AvgVolume) * volMult)
		bars = append(bars, Bar{Date: d, Open: openP, High: hi, Low: lo, Close: closeP, Volume: vol})
		if prevClose > 0 {
			rets = append(rets, math.Log(closeP/prevClose))
		} else {
			rets = append(rets, 0)
		}
		prevClose = closeP
	}
	return bars, rets
}

func clampTick(ex rules.Exchange, p, floor, ceiling float64) float64 {
	p = rules.RoundToTick(ex, p)
	if p < floor {
		return floor
	}
	if p > ceiling {
		return ceiling
	}
	return p
}
