package domain

import (
	"encoding/json"
	"time"
)

type ResultStatus string

const (
	StatusPassed    ResultStatus = "passed"
	StatusFailed    ResultStatus = "failed"
	StatusError     ResultStatus = "error"
	StatusCancelled ResultStatus = "cancelled"
	StatusRunning   ResultStatus = "running"
)

// RequestSummary describes what was sent, with secrets already removed.
type RequestSummary struct {
	Name    string     `json:"name"`
	Method  string     `json:"method"`
	URL     string     `json:"url"`
	Headers []KeyValue `json:"headers"`
	Body    string     `json:"body,omitempty"`
}

// ResponseSummary describes what was received, with secrets already removed.
type ResponseSummary struct {
	StatusCode  int        `json:"statusCode"`
	Status      string     `json:"status"`
	Headers     []KeyValue `json:"headers"`
	Body        string     `json:"body"`
	BodyBytes   int64      `json:"bodyBytes"`
	Truncated   bool       `json:"truncated"`
	ContentType string     `json:"contentType"`
	// Redirects lists the URLs that were followed, in order.
	Redirects []string `json:"redirects,omitempty"`
}

// DiffKind classifies a single difference between a baseline and a response.
type DiffKind string

const (
	DiffAdded       DiffKind = "added"
	DiffRemoved     DiffKind = "removed"
	DiffChanged     DiffKind = "changed"
	DiffTypeChanged DiffKind = "type_changed"
)

type DiffEntry struct {
	Path   string   `json:"path"`
	Kind   DiffKind `json:"kind"`
	Before string   `json:"before,omitempty"`
	After  string   `json:"after,omitempty"`
}

// RegressionResult is the outcome of comparing a response against its baseline.
type RegressionResult struct {
	BaselineID string      `json:"baselineId"`
	Passed     bool        `json:"passed"`
	Diffs      []DiffEntry `json:"diffs"`
	Message    string      `json:"message,omitempty"`
}

// ScriptLog is one console message written by a script, already redacted.
type ScriptLog struct {
	// Phase is "pre-request" or "post-response".
	Phase   string `json:"phase"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// RunResult is the outcome of running one request.
type RunResult struct {
	ID               string            `json:"id"`
	RunID            string            `json:"runId"`
	RequestID        string            `json:"requestId"`
	Request          RequestSummary    `json:"request"`
	Response         *ResponseSummary  `json:"response,omitempty"`
	AssertionResults []AssertionResult `json:"assertionResults"`
	Regression       *RegressionResult `json:"regression,omitempty"`
	Logs             []ScriptLog       `json:"logs,omitempty"`
	DurationMs       int64             `json:"durationMs"`
	Status           ResultStatus      `json:"status"`
	Error            string            `json:"error,omitempty"`
	CreatedAt        time.Time         `json:"createdAt"`
}

// Run is a single execution of one request or a whole collection.
type Run struct {
	ID              string       `json:"id"`
	CollectionID    string       `json:"collectionId"`
	CollectionName  string       `json:"collectionName"`
	EnvironmentName string       `json:"environmentName"`
	Status          ResultStatus `json:"status"`
	StartedAt       time.Time    `json:"startedAt"`
	FinishedAt      *time.Time   `json:"finishedAt,omitempty"`
	PassedCount     int          `json:"passedCount"`
	FailedCount     int          `json:"failedCount"`
	ErrorCount      int          `json:"errorCount"`
	DurationMs      int64        `json:"durationMs"`
	Results         []RunResult  `json:"results,omitempty"`
}

// Tally recomputes counts and overall status from Results.
func (r *Run) Tally() {
	r.PassedCount, r.FailedCount, r.ErrorCount = 0, 0, 0
	cancelled := false
	for _, res := range r.Results {
		switch res.Status {
		case StatusPassed:
			r.PassedCount++
		case StatusFailed:
			r.FailedCount++
		case StatusCancelled:
			cancelled = true
		default:
			r.ErrorCount++
		}
	}
	switch {
	case cancelled:
		r.Status = StatusCancelled
	case r.ErrorCount > 0:
		r.Status = StatusError
	case r.FailedCount > 0:
		r.Status = StatusFailed
	default:
		r.Status = StatusPassed
	}
}

// RunFilter narrows run history queries.
type RunFilter struct {
	CollectionID string     `json:"collectionId"`
	RequestID    string     `json:"requestId"`
	From         *time.Time `json:"from"`
	To           *time.Time `json:"to"`
	Status       string     `json:"status"`
	Limit        int        `json:"limit"`
	Offset       int        `json:"offset"`
}

// Baseline is an approved snapshot of a response used for regression checks.
// A baseline is never updated automatically: users approve every change.
type Baseline struct {
	ID            string `json:"id"`
	RequestID     string `json:"requestId"`
	EnvironmentID string `json:"environmentId"`
	// Snapshot is the approved JSON. When SelectedPaths is set it is an
	// object mapping each path to the value it selected.
	Snapshot      json.RawMessage `json:"snapshot"`
	SelectedPaths []string        `json:"selectedPaths"`
	IgnoredPaths  []string        `json:"ignoredPaths"`
	// Tolerance is an absolute tolerance used when comparing numbers.
	Tolerance float64   `json:"tolerance"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
