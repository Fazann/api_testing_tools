// Package perf computes response-time statistics and detects performance
// regressions by comparing a response with the median of earlier ones.
package perf

import (
	"fmt"
	"math"
	"slices"

	"api-test-kits/internal/domain"
)

// Stats summarises durations. Percentiles use the nearest-rank method.
func Stats(durations []int64) domain.ResponseTimeStats {
	if len(durations) == 0 {
		return domain.ResponseTimeStats{}
	}
	s := sorted(durations)
	return domain.ResponseTimeStats{
		Count:    len(s),
		MinMs:    s[0],
		MedianMs: median(s),
		P95Ms:    percentile(s, 95),
		MaxMs:    s[len(s)-1],
	}
}

// Median returns the median of durations, or 0 when there are none.
func Median(durations []int64) int64 {
	if len(durations) == 0 {
		return 0
	}
	return median(sorted(durations))
}

func sorted(d []int64) []int64 {
	s := slices.Clone(d)
	slices.Sort(s)
	return s
}

func median(s []int64) int64 {
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2] + 1) / 2
}

func percentile(s []int64, p float64) int64 {
	rank := max(1, int(math.Ceil(p/100*float64(len(s)))))
	return s[rank-1]
}

// Threshold is the duration from which a response counts as slower than a
// reference median: at least Factor × reference and MinDeltaMs more.
func Threshold(c domain.PerformanceCheck, reference int64) int64 {
	byFactor := int64(math.Ceil(float64(reference) * c.Factor))
	byDelta := reference + c.MinDeltaMs
	return max(byFactor, byDelta)
}

// Reference prepares the check for the next response from earlier
// durations (oldest first): the median of the latest c.Window of them.
func Reference(c domain.PerformanceCheck, earlier []int64) domain.PerformanceResult {
	if len(earlier) > c.Window {
		earlier = earlier[len(earlier)-c.Window:]
	}
	r := domain.PerformanceResult{Mode: c.Mode, Samples: len(earlier), Needed: c.MinSamples}
	if len(earlier) < c.MinSamples {
		return r
	}
	r.ReferenceMs = Median(earlier)
	r.ThresholdMs = Threshold(c, r.ReferenceMs)
	return r
}

// Evaluate checks duration against earlier durations (oldest first).
func Evaluate(c domain.PerformanceCheck, earlier []int64, duration int64) domain.PerformanceResult {
	r := Reference(c, earlier)
	if r.Samples < r.Needed {
		r.Message = fmt.Sprintf("Not enough history to check response time yet (%d of %d earlier responses)",
			r.Samples, r.Needed)
		return r
	}
	if r.ReferenceMs > 0 {
		r.Ratio = math.Round(float64(duration)/float64(r.ReferenceMs)*100) / 100
	}
	r.Slower = duration >= r.ThresholdMs
	if r.Slower {
		r.Message = fmt.Sprintf("%d ms is slower than usual: median of the last %d responses is %d ms (limit %d ms)",
			duration, r.Samples, r.ReferenceMs, r.ThresholdMs)
	} else {
		r.Message = fmt.Sprintf("%d ms is within the usual range (median %d ms, limit %d ms)",
			duration, r.ReferenceMs, r.ThresholdMs)
	}
	return r
}

// Change compares the median of the latest `recent` durations with the median
// of the ones before them (oldest first). It returns the change in percent and
// false when either side has fewer than 3 durations.
func Change(durations []int64, recent int) (float64, bool) {
	if recent < 3 || len(durations) < recent+3 {
		return 0, false
	}
	split := len(durations) - recent
	before, after := Median(durations[:split]), Median(durations[split:])
	if before == 0 {
		return 0, false
	}
	return math.Round(float64(after-before)/float64(before)*1000) / 10, true
}

// Flag marks every point that would have been slower than usual compared with
// the points before it, so a chart can highlight past slowdowns.
func Flag(c domain.PerformanceCheck, points []domain.ResponseTimePoint) {
	durations := make([]int64, len(points))
	for i, p := range points {
		durations[i] = p.DurationMs
	}
	for i := range points {
		points[i].Slower = c.Mode != domain.PerformanceOff && Evaluate(c, durations[:i], durations[i]).Slower
	}
}
