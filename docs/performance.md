# Response time trends and performance regression

Every time you send a request or run a collection, the app records how long each request took. It uses that
history for two things:

- **Trends.** A chart of a request's recent response times, with its median and p95, and an overview of every request
  in a collection.
- **Performance regression.** Each new response is compared with that request's usual response time. A response that
  is much slower than usual is flagged, or fails the request if you choose.

There's nothing to set up. Trends start with the first response, and checks start once a request has
5 earlier responses.

## Where to find it

| Where | What you see |
| --- | --- |
| Request → **Performance** tab | Chart of the last 100 responses, median, p95, fastest/slowest, the time from which the next response counts as slower, and the check settings |
| Response summary (after **Send**, in **Run** and **History**) | **▲ 1.6× slower than usual** when a response was flagged. Hover over it for the numbers |
| Run summary | A **Slower than usual** count, and **▲ 1.6× slower** on each slow result |
| Collection **⋯ → Response times…** | Every request's median, p95, latest time, change and a sparkline. Sort by slowest median, slowest p95 or biggest slowdown |
| HTML report | A **Response time** line for every checked result |
| CLI JUnit report | The reason, when a slowdown fails a request |

All of these use the environment selected in the top bar (see [Environments](#environments)).

## How "slower than usual" is decided

For each saved request, the app takes the **median of its last 20 responses** in the same environment. A new response
is **slower than usual** when **both** of these are true:

1. it took at least **1.5 ×** that median, **and**
2. it took at least **100 ms** longer than that median.

So the limit is `max(median × 1.5, median + 100 ms)`:

| Usual median | Slower than usual from | Why |
| --- | --- | --- |
| 5 ms | 105 ms | The 100 ms minimum: a jump from 5 to 15 ms is jitter, not a regression |
| 150 ms | 250 ms | The 100 ms minimum still decides (150 × 1.5 = 225) |
| 400 ms | 600 ms | The factor decides (400 + 100 = 500) |
| 2 s | 3 s | The factor decides |

The **median** is used instead of the average so a single slow outlier in the history doesn't raise the reference.
Only the latest 20 responses count, so after an intended change (a new index, a slower but correct query) the reference
follows the new normal within about 10 runs.

The check starts after **5 earlier responses**. Until then the response shows *Not enough history (2 of 5 earlier
responses)*, and the Performance tab shows how many are still needed.

### What counts

| Counted | Not counted |
| --- | --- |
| **Send** and collection runs, including runs of a folder | Race tests and permission matrices. They send requests at the same time, so they are slower by design |
| Responses that **passed or failed** their tests | Network errors, timeouts, cancelled requests: they have no response time |
| Saved requests | Unsaved drafts (no history to compare with) |

A response that fails its tests still counts. If a request's tests fail often, its trend still shows how fast it
answers.

## Choosing what happens: Warn, Fail or Off

Set this per request in **Performance → When a response is slower than usual**:

| Mode | Effect |
| --- | --- |
| **Warn** (default) | The response is flagged (**▲ slower than usual**), but the request keeps its result. The run doesn't fail |
| **Fail** | The request **fails**, so the run fails too. Use this for endpoints with a performance requirement |
| **Off** | No check. The trend is still recorded |

Warn is the default because response times vary on shared test servers and laptops. Turn on **Fail** for the
endpoints you care about once their trend looks stable.

### Tuning

| Setting | Default | Range | Use it to |
| --- | --- | --- | --- |
| At least × median | 1.5 | 1.1 – 10 | Raise for noisy endpoints, lower for stable ones |
| And at least (ms) more | 100 | 0 – 600 000 | Ignore small absolute jumps on fast endpoints |
| Median of last | 20 | 5 – 200 | Use more responses for a steadier reference, or fewer to follow changes faster |
| Start after | 5 | 3 – window | Wait for more history before the first check |

**Defaults** restores these values. The settings are saved per request and deleted with it.

For a **fixed budget** ("must answer within 300 ms", whatever the history), add a **Response time** test in the
**Tests** tab instead. The two complement each other: the test catches a response over the budget, and the
regression check catches a response much slower than usual while still within it.

## Environments

Response times depend heavily on where the API runs, so each environment has its own history. For example, *Local*
answering in 20 ms doesn't make *Staging* at 300 ms look slow. Switch the environment in the top bar to see its trend.
Requests sent with **No environment** have their own history too.

History is linked to the environment's ID, so renaming an environment keeps its trend. Runs saved before this feature
are matched by environment name.

## Reading the chart

- The **blue line** shows response times, oldest on the left and latest on the right.
- The **grey line** is the current median and the **amber line** is the limit for the next response. There's no
  limit line while the check is off or there isn't enough history yet.
- **Amber dots** are responses that were slower than usual compared with the responses before them.
- Hover over the chart, or focus it and use **← / →**, to read each response's time, date, HTTP status and result.
  **Show table** lists the latest 50 responses as a table.

On the collection overview, **Change** compares the median of the latest 5 responses with the median of the up to
25 before them. Changes of 20 % or more are highlighted: slower in red, faster in green. **Latest** has an amber dot
when the latest response was slower than usual.

## Try it

```sh
python3 docs/demo/security_demo_api.py      # http://localhost:8090
```

The demo API has an endpoint whose speed you choose: `GET /slow?ms=300` answers after 300 ms.

1. Create a collection with the variable `delay` = `0`, and a request `GET http://localhost:8090/slow?ms={{delay}}`.
   Save it and press **Send** 6 times.
2. Open the **Performance** tab. The median is a few milliseconds, and the next response counts as slower from about
   100 ms.
3. Simulate a regression: set `delay` to `300` and send again. The response shows **▲ … × slower than usual**, and
   an amber dot appears on the chart. The request still passes, because the mode is **Warn**.
4. Choose **Fail**, press **Save check**, and send again. The request now fails with *300 ms is slower than usual:
   median of the last … responses is … ms*.
5. Set `delay` back to `0`. The next responses are within the usual range again.

## History retention

Trends are built from run history, so they are bounded by **Settings → History retention** (500 runs by default).
**Delete all history** also clears every trend. Stored response bodies are not needed for trends: durations are
kept even when the stored body limit is 0.

## Limits

- **The CLI has no history.** `test-tool-kit-cli` runs in an in-memory database, so responses there show *Not enough
  history* and never fail for response time. To gate CI on speed, use **Response time** tests. Reports from the
  desktop app include the check.
- **Not exported.** Performance settings are not part of collection exports, and **Duplicate** doesn't copy them
  (race tests don't either). A duplicated request starts with an empty trend and the default check.
- **Parallel runs.** With **Settings → Concurrency** above 1, requests share the connection and the server, so
  durations in parallel runs are noisier. Prefer concurrency 1 for collections whose speed you track.
- The duration is measured by the app from sending the request to reading the last byte of the response, including
  DNS, TLS and redirects. It is the time a client sees, not server processing time.

## For developers

| Piece | Where |
| --- | --- |
| Median, p95, limit, "slower than usual", change | `internal/perf` (pure functions, `perf_test.go`) |
| Check settings, history queries | `internal/storage/performance.go`; migration `0007_response_times.sql` adds `runs.environment_id`, `runs.kind` (`run` / `burst`), `run_results.performance_json` and `performance_checks` |
| Applying the check during a run | `Service.execute` loads each request's history **before** the run, then `applyPerformance` sets `RunResult.performance` and, in Fail mode, the status |
| Trend and collection overview | `Service.GetResponseTimeTrend`, `Service.GetCollectionResponseTimes` (`internal/service/performance.go`) |
| UI | `PerformancePanel.vue`, `ResponseTimeChart.vue`, `CollectionResponseTimes.vue`, `services/responseTimes.ts` |
| Tests | `tests/integration/performance_test.go` (warn → fail → off against a slow mock API, per-environment history, race tests excluded), `internal/storage/sqlite_test.go` (filters, legacy runs, migration backfill), `internal/reports/reports_test.go`, `frontend/src/components/performance/ResponseTimes.test.ts` |

Race tests and permission matrices are saved with `kind = 'burst'`. For history saved before migration 0007, runs
that sent one request more than once are marked as `burst` by the migration.
