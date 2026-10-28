package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/httpclient"
)

func client(t *testing.T) *httpclient.Client {
	c, err := httpclient.New(httpclient.Options{ProxyMode: httpclient.ProxyNone})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func testEnv(base string) *domain.Environment {
	return &domain.Environment{Name: "Local", Variables: []domain.Variable{
		{Key: "base_url", Value: base, Enabled: true},
		{Key: "token", Value: "tok-secret-1", Secret: true, Enabled: true},
		{Key: "apikey", Value: "key-secret-2", Secret: true, Enabled: true},
	}}
}

func TestPrepareResolvesEverything(t *testing.T) {
	req := domain.Request{
		Name: "x", Method: "post", URLTemplate: "{{base_url}}/items?existing=1",
		Query:    []domain.KeyValue{{Key: "q", Value: "{{token}}", Enabled: true}, {Key: "skip", Value: "1"}},
		Headers:  []domain.KeyValue{{Key: "X-Trace", Value: "t", Enabled: true}},
		Body:     `{"token":"{{token}}"}`,
		BodyType: domain.BodyJSON,
		Auth:     domain.Auth{Type: domain.AuthAPIKey, KeyName: "X-Api-Key", KeyValue: "{{apikey}}", KeyIn: domain.APIKeyHeader},
	}
	p, err := Prepare(req, testEnv("http://h"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.HTTP.URL, "http://h/items?") || !strings.Contains(p.HTTP.URL, "q=tok-secret-1") || strings.Contains(p.HTTP.URL, "skip") {
		t.Fatalf("url: %s", p.HTTP.URL)
	}
	if p.HTTP.Header.Get("X-Api-Key") != "key-secret-2" || p.HTTP.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers: %v", p.HTTP.Header)
	}
	sum, _ := json.Marshal(p.Summary)
	if strings.Contains(string(sum), "tok-secret-1") || strings.Contains(string(sum), "key-secret-2") {
		t.Fatalf("summary leaked secrets: %s", sum)
	}
}

func TestPrepareMissingVariables(t *testing.T) {
	_, err := Prepare(domain.Request{Name: "x", Method: "GET", URLTemplate: "{{nope}}/a"}, nil)
	if _, ok := err.(*MissingVariablesError); !ok {
		t.Fatalf("expected MissingVariablesError, got %v", err)
	}
}

func TestRunEvaluatesAndRedacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-secret-1" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Echo the token back to prove responses are redacted too.
		_, _ = io.WriteString(w, `{"balance":10,"currency":"USD","echo":"tok-secret-1"}`)
	}))
	defer srv.Close()

	req := domain.Request{ID: "r1", Name: "balance", Method: "GET", URLTemplate: "{{base_url}}/b",
		Auth: domain.Auth{Type: domain.AuthBearer, Token: "{{token}}"},
		Assertions: []domain.Assertion{
			{Kind: domain.AssertStatus, Operator: domain.OpEq, Expected: json.RawMessage("200"), Enabled: true},
			{Kind: domain.AssertJSONPath, Path: "echo", Operator: domain.OpEq, Expected: json.RawMessage(`"nope"`), Enabled: true},
		}}
	res := New(client(t)).Run(context.Background(), req, testEnv(srv.URL), nil)
	if res.Status != domain.StatusFailed || len(res.AssertionResults) != 2 || !res.AssertionResults[0].Passed {
		t.Fatalf("unexpected result: %+v", res)
	}
	blob, _ := json.Marshal(res)
	if strings.Contains(string(blob), "tok-secret-1") {
		t.Fatalf("result leaked a secret: %s", blob)
	}
}

func TestRunWithBaseline(t *testing.T) {
	balance := atomic.Int64{}
	balance.Store(10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"balance":`+itoa(balance.Load())+`,"ts":"`+time.Now().String()+`"}`)
	}))
	defer srv.Close()
	base := &domain.Baseline{ID: "b", Snapshot: json.RawMessage(`{"balance":10,"ts":"old"}`), IgnoredPaths: []string{"ts"}}
	req := domain.Request{ID: "r", Name: "b", Method: "GET", URLTemplate: srv.URL}
	r := New(client(t))
	if res := r.Run(context.Background(), req, nil, base); res.Status != domain.StatusPassed {
		t.Fatalf("expected pass, got %+v", res.Regression)
	}
	balance.Store(11)
	res := r.Run(context.Background(), req, nil, base)
	if res.Status != domain.StatusFailed || len(res.Regression.Diffs) != 1 || res.Regression.Diffs[0].Path != "balance" {
		t.Fatalf("expected one balance diff, got %+v", res.Regression)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestRunAllOrderConcurrencyAndCancel(t *testing.T) {
	var inFlight, maxInFlight atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	defer srv.Close()
	var items []Item
	for i := 0; i < 8; i++ {
		items = append(items, Item{Request: domain.Request{ID: itoa(int64(i)), Name: "r", Method: "GET", URLTemplate: srv.URL + "/" + itoa(int64(i))}})
	}
	r := New(client(t))
	results := r.RunAll(context.Background(), items, nil, 3, nil)
	for i, res := range results {
		if res.RequestID != itoa(int64(i)) || res.Response.Body != "/"+itoa(int64(i)) {
			t.Fatalf("results out of order at %d: %+v", i, res)
		}
	}
	if maxInFlight.Load() > 3 {
		t.Fatalf("concurrency limit exceeded: %d", maxInFlight.Load())
	}

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	results = r.RunAll(ctx, items, nil, 1, func(i int, _ domain.RunResult) {
		calls++
		if i == 1 {
			cancel()
		}
	})
	if calls != len(items) {
		t.Fatalf("progress should be reported for every item, got %d", calls)
	}
	if results[0].Status != domain.StatusPassed || results[7].Status != domain.StatusCancelled {
		t.Fatalf("expected later items cancelled: first=%s last=%s", results[0].Status, results[7].Status)
	}
}
