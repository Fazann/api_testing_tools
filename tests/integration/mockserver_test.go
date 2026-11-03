package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/google/uuid"
)

const testToken = "integration-secret-token"

// mockAPI is a small stateful wallet/report API used by the integration tests.
type mockAPI struct {
	mu       sync.Mutex
	balance  float64
	currency string
	// leakPassword simulates a regression that exposes a sensitive field.
	leakPassword bool
	// balanceAsString simulates a contract break: the balance is sent as a string.
	balanceAsString bool
	slow            time.Duration
}

func newMockAPI() (*mockAPI, *httptest.Server) {
	m := &mockAPI{balance: 150.25, currency: "USD"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /wallet/balance", m.auth(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var balance any = m.balance
		if m.balanceAsString {
			balance = fmt.Sprintf("%.2f", m.balance)
		}
		writeJSON(w, 200, map[string]any{
			"data": map[string]any{"balance": balance, "currency": m.currency},
			"meta": map[string]any{"request_id": uuid.NewString(), "generated_at": time.Now().Format(time.RFC3339Nano)},
		})
	}))
	mux.HandleFunc("POST /wallet/transactions", m.auth(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Amount float64 }
		_ = json.NewDecoder(r.Body).Decode(&in)
		m.mu.Lock()
		defer m.mu.Unlock()
		before := m.balance
		if in.Amount > m.balance {
			writeJSON(w, 422, map[string]any{"error": map[string]any{"code": "insufficient_funds"}, "balance_before": before, "balance_after": m.balance})
			return
		}
		m.balance -= in.Amount
		writeJSON(w, 201, map[string]any{"balance_before": before, "balance_after": m.balance})
	}))
	mux.HandleFunc("GET /reports/daily", m.auth(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		writeJSON(w, 200, map[string]any{
			"range":       map[string]any{"from": q.Get("from"), "to": q.Get("to")},
			"items":       []map[string]any{{"amount": 10.10}, {"amount": 20.20}, {"amount": 30.30}},
			"total_count": 3,
			"total":       60.60,
		})
	}))
	mux.HandleFunc("GET /me", m.auth(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		out := map[string]any{"id": 7, "name": "Test user"}
		if m.leakPassword {
			out["password_hash"] = "$2a$10$abc"
		}
		writeJSON(w, 200, out)
	}))
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(m.slow):
		case <-r.Context().Done():
			return
		}
		writeJSON(w, 200, map[string]any{})
	})
	return m, httptest.NewServer(mux)
}

func (m *mockAPI) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			writeJSON(w, 401, map[string]any{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (m *mockAPI) set(fn func(*mockAPI)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(m)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
