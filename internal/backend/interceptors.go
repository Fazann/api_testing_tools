package backend

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Handler sends one request and returns its response.
type Handler func(*http.Request) (*http.Response, error)

// Interceptor wraps a Handler to change the request before it is sent or the
// response after it arrives. It calls next to continue the chain, or returns
// without calling it to stop the request. Interceptors run in the order they
// are given to New: the first one is the outermost.
//
// Interceptors must not log request or response bodies, headers or URLs with
// query strings: they can carry tokens and user data.
type Interceptor func(req *http.Request, next Handler) (*http.Response, error)

func chain(final Handler, interceptors []Interceptor) Handler {
	h := final
	for i := len(interceptors) - 1; i >= 0; i-- {
		ic, next := interceptors[i], h
		h = func(r *http.Request) (*http.Response, error) { return ic(r, next) }
	}
	return h
}

// UserAgent sets the User-Agent header, e.g. "api-test-kits/0.1.2".
func UserAgent(value string) Interceptor {
	return func(req *http.Request, next Handler) (*http.Response, error) {
		req.Header.Set("User-Agent", value)
		return next(req)
	}
}

// BearerAuth adds "Authorization: Bearer <token>" while token returns a value.
// token is called for every request, so a refreshed token is used immediately.
func BearerAuth(token func() string) Interceptor {
	return func(req *http.Request, next Handler) (*http.Response, error) {
		if t := token(); t != "" {
			req.Header.Set("Authorization", "Bearer "+t)
		}
		return next(req)
	}
}

// retry resends idempotent requests (GET, HEAD, PUT, DELETE, OPTIONS) after a
// network error or a 502/503/504, up to attempts times in total, waiting
// backoff, then twice as long, between tries. Requests with a body are only
// retried when the body can be replayed (GetBody is set, as for NewRequest
// with a bytes reader). The client adds it innermost from Options.Retries,
// so the cache and the offline breaker see the outcome of all tries.
func retry(attempts int, backoff time.Duration) Interceptor {
	return func(req *http.Request, next Handler) (*http.Response, error) {
		if attempts < 2 || !idempotent(req.Method) || (req.Body != nil && req.GetBody == nil) {
			return next(req)
		}
		wait := backoff
		for try := 1; ; try++ {
			resp, err := next(req)
			if try >= attempts || !retryable(resp, err) || req.Context().Err() != nil {
				return resp, err
			}
			if resp != nil {
				resp.Body.Close()
			}
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(wait):
			}
			wait *= 2
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				req.Body = body
			}
		}
	}
}

func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	}
	return false
}

func retryable(resp *http.Response, err error) bool {
	if err != nil {
		return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}
