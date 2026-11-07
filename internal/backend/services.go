package backend

import (
	"context"
	"errors"
	"time"
)

// Status is what the app knows about its backend connection.
type Status struct {
	// Mode is "live" (real server), "mock" (in-process mock data) or "off".
	Mode       Mode   `json:"mode"`
	Configured bool   `json:"configured"`
	BaseURL    string `json:"baseUrl"`
	Reachable  bool   `json:"reachable"`
	// Offline is true while requests fail fast (or use cached data) because the server failed.
	Offline bool `json:"offline"`
	// LatencyMs is the health check round trip; 0 when not reachable.
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

// Health calls the health endpoint, never from the cache. Any 2xx answer means the backend is up.
func (c *Client) Health(ctx context.Context) error {
	_, err := Call[struct{}](ctx, c, EndpointHealth, Request{NoCache: true})
	return err
}

// CheckStatus reports the mode and whether the backend answers its health
// check. While the server is known to be down it answers at once, without
// waiting on the network.
func (c *Client) CheckStatus(ctx context.Context) Status {
	st := Status{Mode: c.Mode(), Configured: c.Configured(), BaseURL: c.BaseURL()}
	if c != nil && c.disabled != nil {
		st.Error = c.disabled.Error()
	}
	if st.Mode == ModeOff {
		return st
	}
	start := time.Now()
	err := c.Health(ctx)
	if c.breaker != nil {
		st.Offline, _ = c.breaker.offline()
	}
	if err == nil {
		st.Reachable = true
		st.LatencyMs = max(time.Since(start).Milliseconds(), 1)
		return st
	}
	if _, ok := errors.AsType[*APIError](err); ok {
		// The server answered, so it is reachable even though the check failed.
		st.Reachable = true
	}
	st.Error = err.Error()
	return st
}
