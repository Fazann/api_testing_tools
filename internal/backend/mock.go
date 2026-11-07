package backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// MockFunc answers one mocked request. Read path values with r.PathValue("id").
// Return the body to encode as JSON, or an error: an *APIError keeps its
// status, code and message, and any other error becomes a 500.
type MockFunc func(r *http.Request) (any, error)

// Mock answers requests in-process while the real backend does not exist.
// Requests still go through the client's interceptors and Call[T], so the
// code using them does not change when a real BaseURL is set.
type Mock struct {
	mux   *http.ServeMux
	delay time.Duration
}

// NewMock returns a mock without routes. Use Handle to add them, or
// DefaultMock for the routes the app ships with.
func NewMock() *Mock { return &Mock{mux: http.NewServeMux()} }

// Handle mocks one endpoint. Path placeholders like {id} match one segment.
func (m *Mock) Handle(ep Endpoint, fn MockFunc) *Mock {
	m.mux.HandleFunc(ep.Method+" "+ep.Path, func(w http.ResponseWriter, r *http.Request) {
		body, err := fn(r)
		if err != nil {
			apiErr, ok := errors.AsType[*APIError](err)
			if !ok {
				apiErr = &APIError{Status: http.StatusInternalServerError, Message: err.Error()}
			}
			writeJSON(w, apiErr.Status, map[string]string{"code": apiErr.Code, "message": apiErr.Message})
			return
		}
		if body == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, body)
	})
	return m
}

// WithDelay makes every mocked answer take d, to try loading states in the UI.
func (m *Mock) WithDelay(d time.Duration) *Mock {
	m.delay = d
	return m
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// handler is the final step of the chain in mock mode, in place of the network.
func (m *Mock) handler() Handler {
	return func(req *http.Request) (*http.Response, error) {
		if m.delay > 0 {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(m.delay):
			}
		}
		rec := &recorder{header: http.Header{}, status: http.StatusOK}
		if _, pattern := m.mux.Handler(req); pattern == "" {
			writeJSON(rec, http.StatusNotFound, map[string]string{
				"code": "no_mock", "message": fmt.Sprintf("no mock for %s %s", req.Method, req.URL.Path),
			})
		} else {
			m.mux.ServeHTTP(rec, req)
		}
		return &http.Response{
			Status:        strconv.Itoa(rec.status) + " " + http.StatusText(rec.status),
			StatusCode:    rec.status,
			Header:        rec.header,
			Body:          io.NopCloser(bytes.NewReader(rec.body.Bytes())),
			ContentLength: int64(rec.body.Len()),
			Request:       req,
		}, nil
	}
}

// recorder is a minimal http.ResponseWriter that keeps what a mock wrote.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
	wrote  bool
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(status int) {
	if !r.wrote {
		r.status, r.wrote = status, true
	}
}

func (r *recorder) Write(b []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	return r.body.Write(b)
}

// DefaultMock holds the mocked answers the app uses until the backend exists.
// Add a mock here for every endpoint added to endpoints.go.
func DefaultMock() *Mock {
	return NewMock().
		Handle(EndpointHealth, func(*http.Request) (any, error) {
			return map[string]string{"status": "ok"}, nil
		})
}
