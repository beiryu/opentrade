package market

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/beiryu/opentrade/internal/rules"
)

const sampleCSV = `date,open,high,low,close,volume
2026-01-05,100,110,95,105,1000
2026-01-06,105,112,104,110,1200
2026-01-07,110,115,108,108,900
`

func TestParseCSV(t *testing.T) {
	bars, err := ParseCSV(strings.NewReader(sampleCSV))
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 3 || bars[0].Close != 105 || bars[2].Volume != 900 {
		t.Fatalf("unexpected bars: %+v", bars)
	}
	if _, err := ParseCSV(strings.NewReader("date,open\n2026-01-01,1\n")); err == nil {
		t.Error("expected missing-column error")
	}
}

func TestCSVProviderBarsAndCalendar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fpt.csv"), []byte(sampleCSV), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadCSVDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Symbols(); len(got) != 1 || got[0] != "FPT" {
		t.Fatalf("Symbols = %v", got)
	}
	d6 := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)
	bars, err := p.Bars("FPT", d6, 10)
	if err != nil || len(bars) != 2 || bars[1].Close != 110 {
		t.Fatalf("Bars until 01-06 = %v, %v", bars, err)
	}
	bars, _ = p.Bars("FPT", d6.AddDate(0, 0, 5), 1)
	if len(bars) != 1 || bars[0].Close != 108 {
		t.Fatalf("last bar = %v", bars)
	}
	if _, err := p.Bars("FPT", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), 5); err == nil {
		t.Error("expected ErrNoData before history")
	}
	cal := p.Calendar()
	if len(cal) != 3 {
		t.Fatalf("Calendar = %v", cal)
	}
	next, ok := NextTradingDay(cal, d6)
	if !ok || next.Day() != 7 {
		t.Errorf("NextTradingDay = %v %v", next, ok)
	}
	if _, ok := NextTradingDay(cal, cal[2]); ok {
		t.Error("no day after the last one")
	}
}

func TestGenerateSyntheticRespectsRules(t *testing.T) {
	dir := t.TempDir()
	specs := DefaultSyntheticUniverse()[:3] // index + FPT + VNM
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	if err := GenerateSynthetic(dir, specs, start, end, 42); err != nil {
		t.Fatal(err)
	}
	p, err := LoadCSVDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Symbols()) != 3 {
		t.Fatalf("Symbols = %v", p.Symbols())
	}
	bars, err := p.Bars("FPT", end, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(bars); i++ {
		prev, b := bars[i-1], bars[i]
		if !rules.IsOnTick(rules.HOSE, b.Close) {
			t.Fatalf("close %v not on tick", b.Close)
		}
		if !rules.InBand(rules.HOSE, prev.Close, b.Close) {
			t.Fatalf("close %v outside band of %v", b.Close, prev.Close)
		}
		if b.High < b.Low || b.High < b.Close || b.Low > b.Open {
			t.Fatalf("bad OHLC %+v", b)
		}
	}
	// Deterministic: same seed, same first close.
	dir2 := t.TempDir()
	if err := GenerateSynthetic(dir2, specs, start, end, 42); err != nil {
		t.Fatal(err)
	}
	p2, _ := LoadCSVDir(dir2)
	b1, _ := p.Bars("FPT", end, 1)
	b2, _ := p2.Bars("FPT", end, 1)
	if b1[0].Close != b2[0].Close {
		t.Error("synthetic data not deterministic")
	}
}
