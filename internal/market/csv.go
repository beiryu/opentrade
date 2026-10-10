package market

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CSVProvider reads one CSV per symbol from a directory: `FPT.csv`,
// `VNINDEX.csv`, … with the header `date,open,high,low,close,volume` and
// dates as YYYY-MM-DD. Everything is loaded once; the data is small.
type CSVProvider struct {
	bars map[string][]Bar
	cal  []time.Time
}

// LoadCSVDir loads every *.csv in dir. The file name (upper-cased, without
// extension) is the symbol.
func LoadCSVDir(dir string) (*CSVProvider, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.csv"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("market: no CSV files in %s", dir)
	}
	p := &CSVProvider{bars: map[string][]Bar{}}
	for _, f := range files {
		sym := strings.ToUpper(strings.TrimSuffix(filepath.Base(f), filepath.Ext(f)))
		bars, err := readCSV(f)
		if err != nil {
			return nil, fmt.Errorf("market: %s: %w", filepath.Base(f), err)
		}
		p.bars[sym] = bars
	}
	p.cal = p.buildCalendar()
	return p, nil
}

func readCSV(path string) ([]Bar, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return ParseCSV(fh)
}

// ParseCSV parses bars from a reader with the standard header. Rows are
// sorted by date; duplicate dates keep the last row.
func ParseCSV(r io.Reader) ([]Bar, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, need := range []string{"date", "open", "high", "low", "close", "volume"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("missing column %q", need)
		}
	}
	byDate := map[time.Time]Bar{}
	line := 1
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		line++
		d, err := time.Parse("2006-01-02", strings.TrimSpace(rec[col["date"]]))
		if err != nil {
			return nil, fmt.Errorf("line %d: bad date: %w", line, err)
		}
		f := func(name string) (float64, error) {
			return strconv.ParseFloat(strings.TrimSpace(rec[col[name]]), 64)
		}
		var b Bar
		b.Date = Day(d)
		if b.Open, err = f("open"); err != nil {
			return nil, fmt.Errorf("line %d: open: %w", line, err)
		}
		if b.High, err = f("high"); err != nil {
			return nil, fmt.Errorf("line %d: high: %w", line, err)
		}
		if b.Low, err = f("low"); err != nil {
			return nil, fmt.Errorf("line %d: low: %w", line, err)
		}
		if b.Close, err = f("close"); err != nil {
			return nil, fmt.Errorf("line %d: close: %w", line, err)
		}
		vol, err := f("volume")
		if err != nil {
			return nil, fmt.Errorf("line %d: volume: %w", line, err)
		}
		b.Volume = int64(vol)
		byDate[b.Date] = b
	}
	bars := make([]Bar, 0, len(byDate))
	for _, b := range byDate {
		bars = append(bars, b)
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date.Before(bars[j].Date) })
	return bars, nil
}

func (p *CSVProvider) buildCalendar() []time.Time {
	if idx, ok := p.bars[IndexSymbol]; ok && len(idx) > 0 {
		cal := make([]time.Time, len(idx))
		for i, b := range idx {
			cal[i] = b.Date
		}
		return cal
	}
	set := map[time.Time]bool{}
	for _, bars := range p.bars {
		for _, b := range bars {
			set[b.Date] = true
		}
	}
	cal := make([]time.Time, 0, len(set))
	for d := range set {
		cal = append(cal, d)
	}
	sort.Slice(cal, func(i, j int) bool { return cal[i].Before(cal[j]) })
	return cal
}

// Symbols implements Provider.
func (p *CSVProvider) Symbols() []string {
	out := make([]string, 0, len(p.bars))
	for s := range p.bars {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Bars implements Provider.
func (p *CSVProvider) Bars(symbol string, until time.Time, n int) ([]Bar, error) {
	all, ok := p.bars[strings.ToUpper(symbol)]
	if !ok {
		return nil, fmt.Errorf("%w: unknown symbol %s", ErrNoData, symbol)
	}
	until = Day(until)
	end := sort.Search(len(all), func(i int) bool { return all[i].Date.After(until) })
	if end == 0 {
		return nil, fmt.Errorf("%w: %s has nothing on or before %s", ErrNoData, symbol, until.Format("2006-01-02"))
	}
	start := end - n
	if n <= 0 || start < 0 {
		start = 0
	}
	out := make([]Bar, end-start)
	copy(out, all[start:end])
	return out, nil
}

// Calendar implements Provider.
func (p *CSVProvider) Calendar() []time.Time {
	out := make([]time.Time, len(p.cal))
	copy(out, p.cal)
	return out
}

// WriteCSV writes bars in the standard format.
func WriteCSV(w io.Writer, bars []Bar) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"date", "open", "high", "low", "close", "volume"}); err != nil {
		return err
	}
	for _, b := range bars {
		rec := []string{
			b.Date.Format("2006-01-02"),
			strconv.FormatFloat(b.Open, 'f', 0, 64),
			strconv.FormatFloat(b.High, 'f', 0, 64),
			strconv.FormatFloat(b.Low, 'f', 0, 64),
			strconv.FormatFloat(b.Close, 'f', 0, 64),
			strconv.FormatInt(b.Volume, 10),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
