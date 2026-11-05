package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

type perfFixture struct {
	svc   *service.Service
	env   domain.Environment
	other domain.Environment
	coll  domain.Collection
	req   domain.Request
	delay *atomic.Int64 // milliseconds the API waits before answering
}

func setupPerf(t *testing.T) *perfFixture {
	t.Helper()
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
	delay := &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Duration(delay.Load()) * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	f := &perfFixture{svc: svc, delay: delay}
	for i, name := range []string{"Local", "Staging"} {
		e, err := svc.SaveEnvironment(ctx, domain.Environment{Name: name, Variables: []domain.Variable{
			{Key: "base_url", Value: srv.URL, Enabled: true}}})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			f.env = e
		} else {
			f.other = e
		}
	}
	if f.coll, err = svc.SaveCollection(ctx, domain.Collection{Name: "Perf"}); err != nil {
		t.Fatal(err)
	}
	if f.req, err = svc.SaveRequest(ctx, domain.Request{CollectionID: f.coll.ID, Name: "Orders", Method: "GET",
		URLTemplate: "{{base_url}}/orders"}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *perfFixture) send(t *testing.T, envID string, delayMs int64) domain.RunResult {
	t.Helper()
	f.delay.Store(delayMs)
	res, err := f.svc.SendRequest(context.Background(), "", f.req, envID, true)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestPerformanceRegressionWarnsThenFails(t *testing.T) {
	ctx := context.Background()
	f := setupPerf(t)

	// The first responses build the history; nothing is checked yet.
	for i := 0; i < 5; i++ {
		res := f.send(t, f.env.ID, 0)
		if res.Performance == nil || res.Performance.Checked() || res.Performance.Samples != i {
			t.Fatalf("send %d: want 'not enough history' with %d samples, got %+v", i, i, res.Performance)
		}
	}
	if res := f.send(t, f.env.ID, 0); !res.Performance.Checked() || res.Performance.Slower {
		t.Fatalf("a usual response must pass the check: %+v", res.Performance)
	}

	// Default mode warns: the response is flagged but the request passes.
	slow := f.send(t, f.env.ID, 250)
	if !slow.Performance.Slower || slow.Status != domain.StatusPassed || slow.Performance.ReferenceMs > 100 {
		t.Fatalf("warn mode: want slower but passed, got %s %+v", slow.Status, slow.Performance)
	}

	c, err := f.svc.GetPerformanceCheck(ctx, f.req.ID)
	if err != nil || c.Mode != domain.PerformanceWarn {
		t.Fatalf("default check: %+v %v", c, err)
	}
	c.Mode = domain.PerformanceFail
	if _, err := f.svc.SavePerformanceCheck(ctx, c); err != nil {
		t.Fatal(err)
	}
	if res := f.send(t, f.env.ID, 250); res.Status != domain.StatusFailed || !res.Performance.Slower {
		t.Fatalf("fail mode: want failed, got %s %+v", res.Status, res.Performance)
	}

	// The check result is stored with the run.
	run, err := f.svc.GetRun(ctx, slow.RunID)
	if err != nil || run.Results[0].Performance == nil || !run.Results[0].Performance.Slower {
		t.Fatalf("performance result not saved in history: %+v %v", run.Results, err)
	}

	trend, err := f.svc.GetResponseTimeTrend(ctx, f.req.ID, f.env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trend.Points) != 8 || !trend.Points[6].Slower || !trend.Points[7].Slower || trend.Points[5].Slower {
		t.Fatalf("trend points: %+v", trend.Points)
	}
	if trend.Stats.Count != 8 || trend.Stats.MaxMs < 250 || trend.Next.ThresholdMs < trend.Next.ReferenceMs+100 {
		t.Fatalf("trend stats: %+v next %+v", trend.Stats, trend.Next)
	}

	c.Mode = domain.PerformanceOff
	if _, err := f.svc.SavePerformanceCheck(ctx, c); err != nil {
		t.Fatal(err)
	}
	if res := f.send(t, f.env.ID, 250); res.Performance != nil || res.Status != domain.StatusPassed {
		t.Fatalf("mode off: want no check, got %s %+v", res.Status, res.Performance)
	}
}

func TestResponseTimesAreKeptPerEnvironmentAndSkipRaceTests(t *testing.T) {
	ctx := context.Background()
	f := setupPerf(t)
	for i := 0; i < 3; i++ {
		f.send(t, f.env.ID, 0)
	}
	if res := f.send(t, f.other.ID, 0); res.Performance.Samples != 0 {
		t.Fatalf("another environment starts without history: %+v", res.Performance)
	}

	// A race test sends the request concurrently; it must not count.
	f.delay.Store(0)
	if _, err := f.svc.RunRaceTest(ctx, "", domain.RaceTest{RequestID: f.req.ID, Count: 4}, f.env.ID, true); err != nil {
		t.Fatal(err)
	}
	trend, err := f.svc.GetResponseTimeTrend(ctx, f.req.ID, f.env.ID)
	if err != nil || len(trend.Points) != 3 {
		t.Fatalf("want 3 points in Local (race test excluded), got %d %v", len(trend.Points), err)
	}

	rows, err := f.svc.GetCollectionResponseTimes(ctx, f.coll.ID, f.other.ID)
	if err != nil || len(rows) != 1 || rows[0].Stats.Count != 1 || rows[0].Recent == nil {
		t.Fatalf("collection overview for Staging: %+v %v", rows, err)
	}
	if rows[0].ChangePct != nil {
		t.Fatal("one response is not enough for a change")
	}
}
