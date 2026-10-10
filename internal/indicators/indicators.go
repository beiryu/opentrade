// Package indicators holds the handful of technical indicators the engine
// puts in front of the model. Each function takes the series oldest-first
// and returns the latest value plus ok=false when the series is too short.
// No dependencies, no magic: the point is that a reader can verify what the
// model saw.
package indicators

import "math"

// SMA is the simple moving average of the last n values.
func SMA(values []float64, n int) (float64, bool) {
	if n <= 0 || len(values) < n {
		return 0, false
	}
	sum := 0.0
	for _, v := range values[len(values)-n:] {
		sum += v
	}
	return sum / float64(n), true
}

// EMA is the exponential moving average with period n, seeded with the SMA
// of the first n values and then smoothed over the rest.
func EMA(values []float64, n int) (float64, bool) {
	if n <= 0 || len(values) < n {
		return 0, false
	}
	seed, _ := SMA(values[:n], n)
	k := 2.0 / float64(n+1)
	ema := seed
	for _, v := range values[n:] {
		ema = v*k + ema*(1-k)
	}
	return ema, true
}

// RSI is Wilder's relative strength index over n periods (needs n+1 closes).
func RSI(closes []float64, n int) (float64, bool) {
	if n <= 0 || len(closes) < n+1 {
		return 0, false
	}
	var gain, loss float64
	for i := 1; i <= n; i++ {
		d := closes[i] - closes[i-1]
		if d > 0 {
			gain += d
		} else {
			loss -= d
		}
	}
	avgGain, avgLoss := gain/float64(n), loss/float64(n)
	for i := n + 1; i < len(closes); i++ {
		d := closes[i] - closes[i-1]
		g, l := 0.0, 0.0
		if d > 0 {
			g = d
		} else {
			l = -d
		}
		avgGain = (avgGain*float64(n-1) + g) / float64(n)
		avgLoss = (avgLoss*float64(n-1) + l) / float64(n)
	}
	if avgLoss == 0 {
		return 100, true
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs), true
}

// ATR is the average true range over n periods (needs n+1 bars).
func ATR(highs, lows, closes []float64, n int) (float64, bool) {
	m := len(closes)
	if n <= 0 || m < n+1 || len(highs) != m || len(lows) != m {
		return 0, false
	}
	trs := make([]float64, 0, m-1)
	for i := 1; i < m; i++ {
		tr := math.Max(highs[i]-lows[i], math.Max(math.Abs(highs[i]-closes[i-1]), math.Abs(lows[i]-closes[i-1])))
		trs = append(trs, tr)
	}
	return SMA(trs, n)
}

// Return is the fractional change from the close n bars ago to the latest.
func Return(closes []float64, n int) (float64, bool) {
	if n <= 0 || len(closes) < n+1 {
		return 0, false
	}
	prev := closes[len(closes)-1-n]
	if prev == 0 {
		return 0, false
	}
	return closes[len(closes)-1]/prev - 1, true
}

// VolumeRatio is the latest volume divided by the average of the n bars
// before it. Above 1 means a busier-than-usual session.
func VolumeRatio(volumes []float64, n int) (float64, bool) {
	if n <= 0 || len(volumes) < n+1 {
		return 0, false
	}
	avg, _ := SMA(volumes[len(volumes)-1-n:len(volumes)-1], n)
	if avg == 0 {
		return 0, false
	}
	return volumes[len(volumes)-1] / avg, true
}

// MaxDrawdown is the largest peak-to-trough fall of an equity curve as a
// fraction of the peak (0.2 = fell 20 % from a high). Zero for a curve that
// never falls.
func MaxDrawdown(equity []float64) float64 {
	peak, worst := math.Inf(-1), 0.0
	for _, v := range equity {
		if v > peak {
			peak = v
		}
		if peak > 0 {
			if dd := (peak - v) / peak; dd > worst {
				worst = dd
			}
		}
	}
	return worst
}
