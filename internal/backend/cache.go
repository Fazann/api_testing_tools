package backend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"api-test-kits/internal/fsutil"
)

// CacheHeader is set on responses served from the cache: "fresh" (no request
// was made) or "stale" (the backend failed, so the last good answer was used).
const CacheHeader = "X-Backend-Cache"

// Cache keeps successful GET answers so the app shows data at once and keeps
// working while the backend is slow or down. Entries are kept per user: the
// key includes a hash of the Authorization header.
type Cache struct {
	mu       sync.Mutex
	entries  map[string]cacheEntry
	path     string
	fresh    time.Duration
	maxStale time.Duration
	now      func() time.Time
}

type cacheEntry struct {
	Status      int       `json:"status"`
	ContentType string    `json:"contentType,omitempty"`
	Body        []byte    `json:"body"`
	StoredAt    time.Time `json:"storedAt"`
}

// CacheOptions configure a Cache. Zero values are fine.
type CacheOptions struct {
	// Path is a JSON file that keeps entries across restarts; empty keeps them in memory only.
	Path string
	// Fresh is how long an entry is answered without asking the backend. Default 5 minutes.
	Fresh time.Duration
	// MaxStale is how long an entry may stand in for a failing backend. Default 7 days.
	MaxStale time.Duration
}

const (
	maxCacheEntries   = 500
	maxCacheEntrySize = 1 << 20
)

// NewCache creates a cache, loading Path when it exists. A missing or
// unreadable file starts an empty cache: the cache is never required.
func NewCache(opts CacheOptions) *Cache {
	c := &Cache{entries: map[string]cacheEntry{}, path: opts.Path, fresh: opts.Fresh, maxStale: opts.MaxStale, now: time.Now}
	if c.fresh <= 0 {
		c.fresh = 5 * time.Minute
	}
	if c.maxStale <= 0 {
		c.maxStale = 7 * 24 * time.Hour
	}
	if c.path != "" {
		if data, err := os.ReadFile(c.path); err == nil {
			_ = json.Unmarshal(data, &c.entries)
		}
	}
	return c
}

// Clear drops every entry, e.g. after the user signs out.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]cacheEntry{}
	c.saveLocked()
}

type noCacheKey struct{}

// withNoCache marks a request that must reach the backend (or fail), like the health check.
func withNoCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, noCacheKey{}, true)
}

func cacheKey(req *http.Request) string {
	key := req.Method + " " + req.URL.String()
	if auth := req.Header.Get("Authorization"); auth != "" {
		sum := sha256.Sum256([]byte(auth))
		key += " " + hex.EncodeToString(sum[:8])
	}
	return key
}

// interceptor serves fresh entries without a request, stores 2xx answers,
// and answers with a stale entry when the backend fails.
func (c *Cache) interceptor() Interceptor {
	return func(req *http.Request, next Handler) (*http.Response, error) {
		if req.Method != http.MethodGet || req.Context().Value(noCacheKey{}) != nil {
			return next(req)
		}
		key := cacheKey(req)
		entry, found := c.get(key)
		age := c.now().Sub(entry.StoredAt)
		if found && age < c.fresh {
			return entry.response(req, "fresh"), nil
		}

		resp, err := next(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxCacheEntrySize+1))
			resp.Body.Close()
			if readErr != nil {
				return nil, readErr
			}
			if len(body) <= maxCacheEntrySize {
				c.put(key, cacheEntry{
					Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body, StoredAt: c.now(),
				})
			}
			resp.Body = io.NopCloser(bytes.NewReader(body))
			return resp, nil
		}
		failed := err != nil || resp.StatusCode >= 500
		if failed && found && age < c.maxStale {
			if resp != nil {
				resp.Body.Close()
			}
			return entry.response(req, "stale"), nil
		}
		return resp, err
	}
}

func (c *Cache) get(key string) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	return e, ok
}

func (c *Cache) put(key string, e cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = e
	if len(c.entries) > maxCacheEntries {
		c.evictOldestLocked()
	}
	c.saveLocked()
}

func (c *Cache) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	for k, e := range c.entries {
		if oldestKey == "" || e.StoredAt.Before(oldest) {
			oldestKey, oldest = k, e.StoredAt
		}
	}
	delete(c.entries, oldestKey)
}

// saveLocked writes the cache file. Failures are ignored: the cache only
// speeds things up and must never break a request.
func (c *Cache) saveLocked() {
	if c.path == "" {
		return
	}
	if data, err := json.Marshal(c.entries); err == nil {
		_ = fsutil.WriteFileAtomic(c.path, data)
	}
}

func (e cacheEntry) response(req *http.Request, state string) *http.Response {
	h := http.Header{CacheHeader: {state}}
	if e.ContentType != "" {
		h.Set("Content-Type", e.ContentType)
	}
	return &http.Response{
		Status: http.StatusText(e.Status), StatusCode: e.Status, Header: h,
		Body: io.NopCloser(bytes.NewReader(e.Body)), ContentLength: int64(len(e.Body)), Request: req,
	}
}
