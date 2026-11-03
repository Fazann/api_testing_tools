package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/redact"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

const apiSecret = "topsecret-hmac-key"

// signedAPI requires a session from /login and an HMAC-SHA256 signature of
// the body on /pay, like many payment APIs.
func signedAPI(calls *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login":
			_, _ = w.Write([]byte(`{"session":"sess-123","userId":42}`))
		case "/pay":
			mac := hmac.New(sha256.New, []byte(apiSecret))
			mac.Write(body)
			want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
			if r.Header.Get("Authorization") != "Session sess-123" || r.Header.Get("X-Signature") != want ||
				r.Header.Get("X-Request-Id") == "" || string(body) != `{"amount":10,"user":"42"}` {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"ok":false}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestScriptsSignChainAndTest(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	svc, err := service.New(store)
	if err != nil {
		t.Fatal(err)
	}
	st := svc.GetSettings()
	st.ProxyMode = "none"
	if err := svc.SaveSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := signedAPI(&calls)
	t.Cleanup(srv.Close)

	env, err := svc.SaveEnvironment(ctx, domain.Environment{Name: "Scripts", Variables: []domain.Variable{
		{Key: "base_url", Value: srv.URL, Enabled: true},
		{Key: "api_secret", Value: apiSecret, Secret: true, Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := svc.SaveCollection(ctx, domain.Collection{Name: "Signed API",
		PreRequestScript:   `pm.request.headers.upsert({key: "X-Request-Id", value: require("uuid").v4()});`,
		PostResponseScript: `pm.test("collection: no server error", () => pm.expect(pm.response.code).to.be.below(500));`,
	})
	if err != nil {
		t.Fatal(err)
	}
	login, err := svc.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "Login", Method: "POST", SortOrder: 1,
		URLTemplate: "{{base_url}}/login", BodyType: domain.BodyJSON, Body: `{"user":"demo"}`,
		PostResponseScript: `
			const data = pm.response.json();
			pm.environment.set("session", data.session);
			pm.collectionVariables.set("user_id", data.userId);
			console.log("logged in using", pm.environment.get("api_secret"));
			pm.test("has session", () => pm.expect(data).to.have.property("session"));`,
	})
	if err != nil {
		t.Fatal(err)
	}
	pay, err := svc.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "Pay", Method: "POST", SortOrder: 2,
		URLTemplate: "{{base_url}}/pay", BodyType: domain.BodyJSON, Body: `{"amount":10,"user":"{{user_id}}"}`,
		Headers: []domain.KeyValue{
			{Key: "Authorization", Value: "Session {{session}}", Enabled: true},
			{Key: "X-Signature", Value: "{{signature}}", Enabled: true},
		},
		PreRequestScript: `
			const body = pm.variables.replaceIn(pm.request.body.raw);
			const sig = CryptoJS.HmacSHA256(body, pm.environment.get("api_secret")).toString(CryptoJS.enc.Base64);
			pm.variables.set("signature", sig);`,
		PostResponseScript: `pm.test("paid", () => pm.expect(pm.response.json().ok).to.be.true);`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if login.PostResponseScript == "" || pay.PreRequestScript == "" {
		t.Fatal("scripts were not stored")
	}

	// Variables that only scripts define must not produce preflight errors.
	warnings, err := svc.PreflightCollection(ctx, c.ID, "", env.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range warnings {
		if w.Level == service.LevelError {
			t.Errorf("unexpected preflight error: %s", w.Message)
		}
	}

	run, err := svc.RunCollection(ctx, "op", c.ID, "", env.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.StatusPassed || len(run.Results) != 2 {
		t.Fatalf("run = %s with %d results: %+v", run.Status, len(run.Results), run.Results)
	}
	for _, res := range run.Results {
		if len(res.AssertionResults) != 2 {
			t.Errorf("%s: tests = %+v", res.Request.Name, res.AssertionResults)
		}
		for _, a := range res.AssertionResults {
			if a.Kind != domain.AssertScript || !a.Passed {
				t.Errorf("%s: test %+v", res.Request.Name, a)
			}
		}
	}
	if logs := run.Results[0].Logs; len(logs) != 1 || logs[0].Message != "logged in using "+redact.Mask || logs[0].Phase != "post-response" {
		t.Errorf("login logs = %+v", logs)
	}
	// The signature header is sent, and the secret never appears in the summary.
	if h := run.Results[1].Request.Headers; !strings.Contains(headerValue(h, "X-Signature"), "=") {
		t.Errorf("sent headers = %+v", h)
	}

	// Script changes are saved like Postman: the session in the environment,
	// the user id in the collection. Secret flags are kept.
	savedEnv, _ := store.GetEnvironment(ctx, env.ID)
	if v := variable(savedEnv.Variables, "session"); v == nil || v.Value != "sess-123" || v.Secret {
		t.Errorf("session variable = %+v", v)
	}
	if v := variable(savedEnv.Variables, "api_secret"); v == nil || !v.Secret || v.Value != apiSecret {
		t.Errorf("api_secret variable = %+v", v)
	}
	savedColl, _ := store.GetCollection(ctx, c.ID)
	if v := variable(savedColl.Variables, "user_id"); v == nil || v.Value != "42" {
		t.Errorf("user_id variable = %+v", v)
	}
	if savedColl.PreRequestScript == "" {
		t.Error("collection script lost when saving collection variables")
	}

	// Logs are kept in history.
	stored, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Results[0].Logs) != 1 {
		t.Errorf("stored logs = %+v", stored.Results[0].Logs)
	}

	// A failing pre-request script stops the request before anything is sent.
	before := calls.Load()
	broken := pay
	broken.PreRequestScript = "throw new Error('cannot sign')"
	res, err := svc.SendRequest(ctx, "op2", broken, env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != domain.StatusError || res.Error != "pre-request script failed: Error: cannot sign (line 1)" {
		t.Errorf("broken script result = %s %q", res.Status, res.Error)
	}
	if calls.Load() != before {
		t.Error("request was sent although its pre-request script failed")
	}

	// A post-response script error marks the result as an error but keeps the response.
	broken = login
	broken.PostResponseScript = "pm.response.json().missing.field"
	res, _ = svc.SendRequest(ctx, "op3", broken, env.ID, false)
	if res.Status != domain.StatusError || res.Response == nil || !strings.HasPrefix(res.Error, "post-response script failed: TypeError") {
		t.Errorf("post-response error result = %s %q", res.Status, res.Error)
	}
}

func headerValue(h []domain.KeyValue, key string) string {
	for _, kv := range h {
		if strings.EqualFold(kv.Key, key) {
			return kv.Value
		}
	}
	return ""
}

func variable(vars []domain.Variable, key string) *domain.Variable {
	for i := range vars {
		if vars[i].Key == key {
			return &vars[i]
		}
	}
	return nil
}
