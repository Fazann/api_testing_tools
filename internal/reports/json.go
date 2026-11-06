// Package reports exports runs as JSON or self-contained HTML. Reports are
// built from already-redacted history and redacted again before writing.
package reports

import (
	"encoding/json"
	"io"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/redact"
)

const FormatVersion = 1

// Report is the exported document.
type Report struct {
	Format      string        `json:"format"`
	Version     int           `json:"version"`
	AppName     string        `json:"appName"`
	AppVersion  string        `json:"appVersion"`
	GeneratedAt time.Time     `json:"generatedAt"`
	Environment string        `json:"environment"`
	Collection  string        `json:"collection"`
	Summary     Summary       `json:"summary"`
	Network     NetworkConfig `json:"network"`
	Run         domain.Run    `json:"run"`
}

type Summary struct {
	Status           domain.ResultStatus `json:"status"`
	Total            int                 `json:"total"`
	Passed           int                 `json:"passed"`
	Failed           int                 `json:"failed"`
	Errors           int                 `json:"errors"`
	Assertions       int                 `json:"assertions"`
	AssertionsFailed int                 `json:"assertionsFailed"`
	DurationMs       int64               `json:"durationMs"`
	StartedAt        time.Time           `json:"startedAt"`
}

// NetworkConfig records the TLS and proxy behavior in effect, so readers know
// how results were obtained.
type NetworkConfig struct {
	TLSVerification bool   `json:"tlsVerification"`
	ProxyMode       string `json:"proxyMode"`
}

// Meta is the information about the app and environment included in reports.
type Meta struct {
	AppName    string
	AppVersion string
	Settings   domain.Settings
	// Secrets are current secret values, redacted once more as a safety net.
	Secrets []string
}

// Build assembles a report from a stored run.
func Build(run domain.Run, meta Meta, now time.Time) Report {
	red := redact.New(meta.Secrets...)
	run = sanitizeRun(run, red)
	sum := Summary{
		Status: run.Status, Total: len(run.Results), Passed: run.PassedCount, Failed: run.FailedCount,
		Errors: run.ErrorCount, DurationMs: run.DurationMs, StartedAt: run.StartedAt,
	}
	for _, r := range run.Results {
		sum.Assertions += len(r.AssertionResults)
		for _, a := range r.AssertionResults {
			if !a.Passed {
				sum.AssertionsFailed++
			}
		}
	}
	return Report{
		Format:      "api-regression-guard.report",
		Version:     FormatVersion,
		AppName:     meta.AppName,
		AppVersion:  meta.AppVersion,
		GeneratedAt: now.UTC(),
		Environment: run.EnvironmentName,
		Collection:  run.CollectionName,
		Summary:     sum,
		Network: NetworkConfig{
			TLSVerification: !meta.Settings.InsecureSkipVerify,
			ProxyMode:       meta.Settings.ProxyMode,
		},
		Run: run,
	}
}

// WriteJSON writes the report as indented JSON.
func WriteJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// sanitizeRun drops sensitive headers entirely and redacts every string.
func sanitizeRun(run domain.Run, red *redact.Redactor) domain.Run {
	run.CollectionName = red.String(run.CollectionName)
	run.EnvironmentName = red.String(run.EnvironmentName)
	results := make([]domain.RunResult, len(run.Results))
	for i, r := range run.Results {
		r.Error = red.String(r.Error)
		r.Request.Name = red.String(r.Request.Name)
		r.Request.URL = red.URL(r.Request.URL)
		r.Request.Headers = dropSensitive(r.Request.Headers, red)
		r.Request.Body = red.String(r.Request.Body)
		if r.Response != nil {
			resp := *r.Response
			resp.Headers = dropSensitive(resp.Headers, red)
			resp.Body = red.String(resp.Body)
			redirects := make([]string, len(resp.Redirects))
			for j, u := range resp.Redirects {
				redirects[j] = red.URL(u)
			}
			resp.Redirects = redirects
			r.Response = &resp
		}
		ars := make([]domain.AssertionResult, len(r.AssertionResults))
		for j, a := range r.AssertionResults {
			a.Expected, a.Actual, a.Message = red.String(a.Expected), red.String(a.Actual), red.String(a.Message)
			a.Description = red.String(a.Description)
			ars[j] = a
		}
		r.AssertionResults = ars
		if r.Socket != nil {
			tr := *r.Socket
			frames := make([]domain.SocketFrame, len(tr.Frames))
			for j, f := range tr.Frames {
				f.Event, f.Data = red.String(f.Event), red.String(f.Data)
				frames[j] = f
			}
			tr.Frames, tr.CloseReason = frames, red.String(tr.CloseReason)
			r.Socket = &tr
		}
		if r.Regression != nil {
			reg := *r.Regression
			diffs := make([]domain.DiffEntry, len(reg.Diffs))
			for j, d := range reg.Diffs {
				d.Before, d.After = red.String(d.Before), red.String(d.After)
				diffs[j] = d
			}
			reg.Diffs = diffs
			r.Regression = &reg
		}
		results[i] = r
	}
	run.Results = results
	return run
}

func dropSensitive(headers []domain.KeyValue, red *redact.Redactor) []domain.KeyValue {
	out := []domain.KeyValue{}
	for _, h := range headers {
		if red.IsSensitiveHeader(h.Key) || h.Value == redact.Mask {
			continue
		}
		h.Value = red.String(h.Value)
		out = append(out, h)
	}
	return out
}
