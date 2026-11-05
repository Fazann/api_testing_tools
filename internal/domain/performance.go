package domain

import (
	"fmt"
	"time"
)

// PerformanceMode says what happens when a request is slower than usual.
type PerformanceMode string

const (
	// PerformanceOff disables the check for the request.
	PerformanceOff PerformanceMode = "off"
	// PerformanceWarn reports a slowdown without failing the request.
	PerformanceWarn PerformanceMode = "warn"
	// PerformanceFail fails the request when it is slower than usual.
	PerformanceFail PerformanceMode = "fail"
)

// PerformanceCheck configures response-time regression detection for one
// request. Requests without a stored check use DefaultPerformanceCheck.
type PerformanceCheck struct {
	RequestID string          `json:"requestId"`
	Mode      PerformanceMode `json:"mode"`
	// Factor: a response is slower than usual when it takes at least Factor
	// times the median of recent responses...
	Factor float64 `json:"factor"`
	// MinDeltaMs: ...and at least this many milliseconds more, so fast
	// endpoints don't flag a 4 ms → 9 ms jitter.
	MinDeltaMs int64 `json:"minDeltaMs"`
	// Window is how many recent responses the median is taken from.
	Window int `json:"window"`
	// MinSamples is how many earlier responses are needed before checking.
	MinSamples int       `json:"minSamples"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Limits for PerformanceCheck fields.
const (
	MinPerformanceFactor = 1.1
	MaxPerformanceFactor = 10
	MaxPerformanceDelta  = 10 * 60 * 1000
	MinPerformanceWindow = 5
	MaxPerformanceWindow = 200
	MinPerformanceSample = 3
)

// DefaultPerformanceCheck warns when a response takes 1.5× the median of the
// last 20 and at least 100 ms longer, once there are 5 earlier responses.
func DefaultPerformanceCheck(requestID string) PerformanceCheck {
	return PerformanceCheck{RequestID: requestID, Mode: PerformanceWarn, Factor: 1.5, MinDeltaMs: 100,
		Window: 20, MinSamples: 5}
}

// Normalize fills in defaults for zero values.
func (c *PerformanceCheck) Normalize() {
	d := DefaultPerformanceCheck(c.RequestID)
	if c.Mode == "" {
		c.Mode = d.Mode
	}
	if c.Factor == 0 {
		c.Factor = d.Factor
	}
	if c.Window == 0 {
		c.Window = d.Window
	}
	if c.MinSamples == 0 {
		c.MinSamples = d.MinSamples
	}
}

func (c *PerformanceCheck) Validate() error {
	if c.RequestID == "" {
		return fmt.Errorf("save the request before configuring its performance check")
	}
	switch c.Mode {
	case PerformanceOff, PerformanceWarn, PerformanceFail:
	default:
		return fmt.Errorf("unknown performance mode %q", c.Mode)
	}
	if c.Factor < MinPerformanceFactor || c.Factor > MaxPerformanceFactor {
		return fmt.Errorf("slowdown factor must be between %.1f× and %.0f×", MinPerformanceFactor, float64(MaxPerformanceFactor))
	}
	if c.MinDeltaMs < 0 || c.MinDeltaMs > MaxPerformanceDelta {
		return fmt.Errorf("minimum slowdown must be between 0 and %d ms", MaxPerformanceDelta)
	}
	if c.Window < MinPerformanceWindow || c.Window > MaxPerformanceWindow {
		return fmt.Errorf("history window must be between %d and %d responses", MinPerformanceWindow, MaxPerformanceWindow)
	}
	if c.MinSamples < MinPerformanceSample || c.MinSamples > c.Window {
		return fmt.Errorf("minimum history must be between %d and the window size (%d)", MinPerformanceSample, c.Window)
	}
	return nil
}

// PerformanceResult is the response-time check for one result.
type PerformanceResult struct {
	Mode PerformanceMode `json:"mode"`
	// ReferenceMs is the median of the earlier responses (0 without enough).
	ReferenceMs int64 `json:"referenceMs"`
	// ThresholdMs is the duration from which a response counts as slower.
	ThresholdMs int64 `json:"thresholdMs"`
	// Samples is how many earlier responses the reference is based on.
	Samples int `json:"samples"`
	// Needed is MinSamples; Samples < Needed means not checked yet.
	Needed int `json:"needed"`
	// Ratio is duration / reference.
	Ratio   float64 `json:"ratio"`
	Slower  bool    `json:"slower"`
	Message string  `json:"message"`
}

// Checked reports whether there was enough history to compare.
func (p *PerformanceResult) Checked() bool { return p != nil && p.Samples >= p.Needed && p.Needed > 0 }

// ResponseTimePoint is one past response of a request.
type ResponseTimePoint struct {
	RunID      string       `json:"runId"`
	ResultID   string       `json:"resultId"`
	At         time.Time    `json:"at"`
	DurationMs int64        `json:"durationMs"`
	Status     ResultStatus `json:"status"`
	StatusCode int          `json:"statusCode"`
	Slower     bool         `json:"slower"`
}

// ResponseTimeStats summarises a set of durations.
type ResponseTimeStats struct {
	Count    int   `json:"count"`
	MinMs    int64 `json:"minMs"`
	MedianMs int64 `json:"medianMs"`
	P95Ms    int64 `json:"p95Ms"`
	MaxMs    int64 `json:"maxMs"`
}

// ResponseTimeTrend is a request's response-time history in one environment.
type ResponseTimeTrend struct {
	RequestID string `json:"requestId"`
	// Points are oldest first.
	Points []ResponseTimePoint `json:"points"`
	Stats  ResponseTimeStats   `json:"stats"`
	Check  PerformanceCheck    `json:"check"`
	// Next is the check the next response would get: the reference and
	// threshold from the latest Window responses.
	Next PerformanceResult `json:"next"`
}

// ResponseTimeRow is one request in a collection's response-time overview.
type ResponseTimeRow struct {
	RequestID string            `json:"requestId"`
	Name      string            `json:"name"`
	Method    string            `json:"method"`
	ParentID  string            `json:"parentId"`
	Stats     ResponseTimeStats `json:"stats"`
	LastMs    int64             `json:"lastMs"`
	// ChangePct compares the median of the recent responses with the median
	// of the earlier ones; nil without enough history.
	ChangePct *float64 `json:"changePct"`
	// Recent durations, oldest first, for a sparkline.
	Recent []int64 `json:"recent"`
	// LastSlower is true when the latest response was flagged.
	LastSlower bool            `json:"lastSlower"`
	Mode       PerformanceMode `json:"mode"`
}
