package perf

import (
	"testing"

	"api-test-kits/internal/domain"
)

func TestStats(t *testing.T) {
	s := Stats([]int64{50, 10, 40, 20, 30})
	if s.Count != 5 || s.MinMs != 10 || s.MedianMs != 30 || s.MaxMs != 50 || s.P95Ms != 50 {
		t.Fatalf("got %+v", s)
	}
	if Median([]int64{10, 20}) != 15 || Median(nil) != 0 {
		t.Fatal("median of an even count is the mean of the middle two")
	}
	if (Stats(nil) != domain.ResponseTimeStats{}) {
		t.Fatal("empty stats must be zero")
	}
	// Nearest rank: p95 of 1..20 is the 19th value.
	d := make([]int64, 20)
	for i := range d {
		d[i] = int64(i + 1)
	}
	if got := Stats(d).P95Ms; got != 19 {
		t.Fatalf("p95 of 1..20 = %d, want 19", got)
	}
}

func TestEvaluateNeedsHistory(t *testing.T) {
	c := domain.DefaultPerformanceCheck("r")
	r := Evaluate(c, []int64{100, 100}, 5000)
	if r.Slower || r.Checked() || r.Samples != 2 || r.Needed != 5 {
		t.Fatalf("too little history must not flag: %+v", r)
	}
}

func TestEvaluateFactorAndMinimumDelta(t *testing.T) {
	c := domain.DefaultPerformanceCheck("r") // 1.5×, +100 ms, window 20, 5 samples
	slowish := []int64{200, 210, 190, 205, 195}

	if r := Evaluate(c, slowish, 260); r.Slower {
		t.Fatalf("1.3× must not flag: %+v", r)
	}
	r := Evaluate(c, slowish, 320)
	if !r.Slower || r.ReferenceMs != 200 || r.ThresholdMs != 300 || r.Ratio != 1.6 || !r.Checked() {
		t.Fatalf("1.6× and +120 ms must flag: %+v", r)
	}

	// A fast endpoint tripling from 5 to 15 ms is under the 100 ms minimum.
	fast := []int64{5, 5, 5, 5, 5}
	if r := Evaluate(c, fast, 15); r.Slower || r.ThresholdMs != 105 {
		t.Fatalf("jitter on a fast endpoint must not flag: %+v", r)
	}
	if r := Evaluate(c, fast, 105); !r.Slower {
		t.Fatalf("reaching the threshold flags: %+v", r)
	}
}

func TestEvaluateUsesOnlyTheWindow(t *testing.T) {
	c := domain.DefaultPerformanceCheck("r")
	c.Window, c.MinSamples = 5, 5
	// Old fast responses fall out of the window; the API got slower and that
	// is the new normal, so 500 ms is not flagged against the last five.
	earlier := []int64{100, 100, 100, 100, 100, 500, 500, 500, 500, 500}
	if r := Evaluate(c, earlier, 520); r.Slower || r.ReferenceMs != 500 || r.Samples != 5 {
		t.Fatalf("got %+v", r)
	}
}

func TestChange(t *testing.T) {
	if _, ok := Change([]int64{1, 2, 3, 4, 5}, 5); ok {
		t.Fatal("needs at least 3 earlier durations")
	}
	pct, ok := Change([]int64{100, 100, 100, 150, 150, 150}, 3)
	if !ok || pct != 50 {
		t.Fatalf("got %v %v, want +50%%", pct, ok)
	}
	pct, _ = Change([]int64{200, 200, 200, 100, 100, 100}, 3)
	if pct != -50 {
		t.Fatalf("got %v, want -50%%", pct)
	}
}

func TestFlag(t *testing.T) {
	c := domain.DefaultPerformanceCheck("r")
	pts := make([]domain.ResponseTimePoint, 0, 7)
	for _, d := range []int64{100, 100, 100, 100, 100, 400, 100} {
		pts = append(pts, domain.ResponseTimePoint{DurationMs: d})
	}
	Flag(c, pts)
	for i, p := range pts {
		if p.Slower != (i == 5) {
			t.Fatalf("point %d (%d ms) slower=%v", i, p.DurationMs, p.Slower)
		}
	}
	c.Mode = domain.PerformanceOff
	Flag(c, pts)
	if pts[5].Slower {
		t.Fatal("mode off flags nothing")
	}
}
