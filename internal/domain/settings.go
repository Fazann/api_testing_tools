package domain

import (
	"fmt"
	"net/url"
)

// Settings are application-wide preferences stored locally.
type Settings struct {
	// MaxResponseBytes caps how much of a response body is read.
	MaxResponseBytes int64 `json:"maxResponseBytes"`
	// HistoryBodyLimitBytes caps the response body stored per history result.
	HistoryBodyLimitBytes int `json:"historyBodyLimitBytes"`
	// HistoryRetentionRuns is how many runs are kept; older runs are pruned.
	HistoryRetentionRuns int `json:"historyRetentionRuns"`
	// Concurrency is the number of requests a collection run sends in parallel.
	Concurrency int `json:"concurrency"`

	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	CACertFile         string `json:"caCertFile"`
	// ProxyMode is "none", "system" (environment variables) or "custom".
	ProxyMode string `json:"proxyMode"`
	ProxyURL  string `json:"proxyUrl"`
	// AllowSensitiveDestinations permits link-local and cloud metadata addresses.
	AllowSensitiveDestinations bool `json:"allowSensitiveDestinations"`
}

// DefaultSettings are safe defaults: TLS verified, metadata endpoints blocked,
// sequential runs and bounded history.
func DefaultSettings() Settings {
	return Settings{
		MaxResponseBytes:      10 << 20,
		HistoryBodyLimitBytes: 256 << 10,
		HistoryRetentionRuns:  500,
		Concurrency:           1,
		ProxyMode:             "system",
	}
}

func (s *Settings) Validate() error {
	if s.MaxResponseBytes < 1<<10 || s.MaxResponseBytes > 512<<20 {
		return fmt.Errorf("response size limit must be between 1 KB and 512 MB")
	}
	if s.HistoryBodyLimitBytes < 0 || s.HistoryBodyLimitBytes > 10<<20 {
		return fmt.Errorf("stored body limit must be between 0 and 10 MB")
	}
	if s.HistoryRetentionRuns < 1 || s.HistoryRetentionRuns > 100_000 {
		return fmt.Errorf("history retention must be between 1 and 100000 runs")
	}
	if s.Concurrency < 1 || s.Concurrency > 8 {
		return fmt.Errorf("concurrency must be between 1 and 8")
	}
	switch s.ProxyMode {
	case "none", "system":
	case "custom":
		u, err := url.Parse(s.ProxyURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("custom proxy URL must look like http://host:port")
		}
	default:
		return fmt.Errorf("proxy mode must be none, system or custom")
	}
	return nil
}
