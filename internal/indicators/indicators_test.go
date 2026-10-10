package indicators

import (
	"math"
	"testing"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestSMAAndEMA(t *testing.T) {
	v := []float64{1, 2, 3, 4, 5}
	if got, ok := SMA(v, 3); !ok || got != 4 {
		t.Errorf("SMA = %v %v, want 4", got, ok)
	}
	if _, ok := SMA(v, 6); ok {
		t.Error("SMA should fail on short series")
	}
	// EMA(3) seeded with SMA(1,2,3)=2, k=0.5: 4→3, 5→4.
	if got, ok := EMA(v, 3); !ok || got != 4 {
		t.Errorf("EMA = %v %v, want 4", got, ok)
	}
}

func TestRSI(t *testing.T) {
	up := []float64{10, 11, 12, 13, 14, 15}
	if got, ok := RSI(up, 5); !ok || got != 100 {
		t.Errorf("RSI all-up = %v, want 100", got)
	}
	// Alternating equal gains and losses → RS 1 → RSI 50.
	alt := []float64{10, 11, 10, 11, 10, 11, 10}
	if got, ok := RSI(alt, 6); !ok || !near(got, 50, 1e-9) {
		t.Errorf("RSI alternating = %v, want 50", got)
	}
	if _, ok := RSI(up, 6); ok {
		t.Error("RSI needs n+1 closes")
	}
}

func TestATR(t *testing.T) {
	h := []float64{10, 12, 13}
	l := []float64{8, 9, 11}
	c := []float64{9, 11, 12}
	// TR1 = max(12-9, |12-9|, |9-9|) = 3; TR2 = max(13-11, |13-11|, |11-11|) = 2.
	if got, ok := ATR(h, l, c, 2); !ok || got != 2.5 {
		t.Errorf("ATR = %v, want 2.5", got)
	}
}

func TestReturnAndVolumeRatio(t *testing.T) {
	c := []float64{100, 110, 121}
	if got, ok := Return(c, 2); !ok || !near(got, 0.21, 1e-9) {
		t.Errorf("Return = %v, want 0.21", got)
	}
	vol := []float64{100, 100, 100, 200}
	if got, ok := VolumeRatio(vol, 3); !ok || got != 2 {
		t.Errorf("VolumeRatio = %v, want 2", got)
	}
}

func TestMaxDrawdown(t *testing.T) {
	if got := MaxDrawdown([]float64{100, 120, 90, 130}); !near(got, 0.25, 1e-9) {
		t.Errorf("MaxDrawdown = %v, want 0.25", got)
	}
	if got := MaxDrawdown([]float64{1, 2, 3}); got != 0 {
		t.Errorf("MaxDrawdown rising = %v, want 0", got)
	}
}
