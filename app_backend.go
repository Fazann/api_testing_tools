package main

import (
	"context"
	"path/filepath"
	"time"

	"api-test-kits/internal/appdata"
	"api-test-kits/internal/backend"
	"api-test-kits/internal/service"
)

// newBackendClient builds the client for the remote backend server. The
// frontend never calls that server directly: it calls App methods through
// Wails, and they call the server through this client.
//
// Without a backend URL (backend.BaseURL or API_TEST_KITS_BACKEND_URL) the
// client answers from backend.DefaultMock, so features built on it work
// before the server exists. With a URL, GET answers are cached in the data
// directory and served first and while the server is down. A malformed URL
// falls back to the mock and returns an error to log; it never stops the app.
func newBackendClient() (*backend.Client, error) {
	return backend.NewOrDisabled(backend.Options{
		BaseURL: backend.ConfiguredURL(),
		Timeout: 15 * time.Second,
		Mock:    backend.DefaultMock(),
		Cache:   backend.NewCache(backend.CacheOptions{Path: backendCachePath()}),
		Retries: 3,
		Interceptors: []backend.Interceptor{
			backend.UserAgent("api-test-kits/" + service.Version),
			// Add backend.BearerAuth(tokenFunc) here once the backend has sign-in.
		},
	})
}

// backendCachePath is the cache file next to the database; "" (memory only) when there is no data directory.
func backendCachePath() string {
	dir, err := appdata.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "backend-cache.json")
}

// BackendStatus reports the backend mode (live, mock or off) and whether it answers its health check.
func (a *App) BackendStatus() backend.Status {
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	return a.backend.CheckStatus(ctx)
}
