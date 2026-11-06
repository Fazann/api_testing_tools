# Testing for race conditions

A **race test** sends the same request many times at exactly the same moment and checks what a correct server must
guarantee when requests arrive together. It finds bugs that never appear when requests are sent one by one:

| Bug | What happens | How the race test catches it |
| --- | --- | --- |
| **Double spend / double redeem** | A coupon, gift card or withdrawal is accepted twice because two requests both saw "not used yet" | More successful responses than allowed |
| **Lost update** | Two withdrawals both succeed, but the balance only drops once | The state check: the balance changed less than *successes × amount* |
| **Duplicate IDs** | Two orders get the same order or transaction number | The value at a JSON path is not unique |
| **Crash under concurrency** | Deadlock, lock timeout, unhandled unique-constraint error | Any 5xx response |
| **Dropped requests** | Timeouts or connection resets when requests pile up | A request without a response |

Find it on any saved request: **Race test** tab, next to **Baseline**.

> Run race tests only against systems and accounts you are authorized to test, with controlled test data. A race
> test that finds a bug has, by definition, changed data more often than it should have.

## Try it in five minutes

The demo API has a coupon endpoint with a race condition, a correct version of the same endpoint, and a withdrawal
that loses updates.

### 1. Start the demo API

```sh
python3 docs/demo/security_demo_api.py      # http://localhost:8090, Python 3.9+, no dependencies
```

Reset its state at any time with `curl -X POST http://localhost:8090/reset`.

### 2. Create the requests

1. **Requests → Create collection** named `Race demo`.
2. Add and **save** (`Ctrl+S`) four requests. Race tests use the saved version of a request.
   - `Redeem coupon (racy)`: `POST http://localhost:8090/coupons/racy/redeem`
   - `Redeem coupon (safe)`: `POST http://localhost:8090/coupons/safe/redeem`
   - `Withdraw 10`: `POST http://localhost:8090/wallet/withdraw`
   - `Get wallet`: `GET http://localhost:8090/wallet`

### 3. Find a double redeem

Open `Redeem coupon (racy)` → **Race test** and set:

| Setting | Value |
| --- | --- |
| Parallel requests | `10` |
| Successful (2xx) responses allowed | **At most** `1` |
| Must be unique per success | `tx` |

Press **Run race test** and confirm. The result is **✘ Race condition found**:

- the status chips show something like `200 × 8` and `409 × 2`: the coupon was redeemed eight times;
- the check **At most 1 successful (2xx) responses** fails with *8 of 10 parallel requests succeeded — the server
  accepted the same operation more than allowed (double spend / duplicate)*;
- **No server errors (5xx)** and **Unique tx across successful responses** pass.

Click any row under **Responses** to see its full body.

### 4. Confirm the fix

Reset the demo (`curl -X POST http://localhost:8090/reset`), open `Redeem coupon (safe)` and run the same settings
with **Exactly** `1`. The result is **✔ No race condition found**: one `200` and nine `409`. This endpoint checks and
marks the coupon under one lock.

### 5. Find a lost update

Reset the demo, open `Withdraw 10` → **Race test** and set:

| Setting | Value |
| --- | --- |
| Parallel requests | `5` |
| Successful (2xx) responses allowed | **No limit** |
| Check the state before and after | ticked |
| Request that reads the value | `GET Get wallet` |
| JSON path to a number | `data.balance` |
| Change per success | `-10` |

Run it. All five requests succeed, but the result shows *State: 100 → 90* and the check
**data.balance changed by -10 per success** fails:

> data.balance went from 100 to 90 (change -10); with 5 successes it should have changed by -50 — lost update: some
> successful requests were not applied

The server reported five successful withdrawals but applied one, because each request read the balance, waited, and
wrote back its own result.

### 6. Find it again later

Settings are saved per request when you run it, so after each code or database change you only open the tab and
press **Run race test**. Every run is also saved in **History** (see below).

## Settings

| Setting | Meaning |
| --- | --- |
| **Parallel requests** | How many copies are released at once, 2–50. 10 is usually enough; more rarely finds more. |
| **Successful (2xx) responses allowed** | **At most** *n* (one coupon, limited stock), **Exactly** *n* (an idempotent operation must succeed once), **At least** *n* (all independent requests must succeed), or **No limit**. |
| **Must be unique per success** | A JSON path, such as `data.transaction_id`, whose value must differ across all successful responses. A successful response without that value also fails. Large numeric IDs are compared as text, so they cannot collide by rounding. |
| **Accept 5xx responses** | Off by default: any 5xx fails the test. |
| **Check the state before and after** | Runs a second saved request before and after the burst and reads a number from it. *after − before* must equal *successes × change per success*, within the tolerance. |
| **Change per success** | Negative for a decrease (withdraw 10 → `-10`), positive for an increase (add 1 item → `1`). Use `0` to check that nothing changed, for example when every request is expected to be rejected. |
| **Tolerance** | Allowed difference for decimals, for example `0.001`. |

A request that gets no response (timeout, connection reset, script error) always fails the test, unless the run was
cancelled.

The state-check request must be a different request than the one under test, and should be a read-only `GET`. It
fails the check if it returns a non-2xx status, non-JSON, or no number at the path. Numeric strings such as
`"150.25"` are accepted.

## How requests are sent together

1. Each copy runs the collection and request **pre-request scripts** on its own, so each one can compute its own
   signature, nonce or idempotency key.
2. The copies wait at a start gate until every copy is ready, or has failed before sending (for example because its
   script threw an error), so one failing copy never holds up the rest.
3. All copies are released together.

The gate removes the delay between copies on this computer. Network distance and TLS handshakes still add a little
spread, so a race window of a few milliseconds may need a few runs to hit, and some races only show over a fast
local or LAN connection.

## How to read the result

The verdict only uses the settings above. The request's own **Tests** and **Baseline** are not part of it, because
the copies that lose the race are expected to fail with 409, 422 or similar. Their post-response scripts still run,
and their logs are kept.

Variables that scripts change during a race test (`pm.environment.set`, …) are **not saved**, so a burst cannot
overwrite a token or ID you rely on.

In **History** the run is listed under the collection and contains, in order:

1. `State before: Get wallet` (if there is a state check)
2. `#1 Withdraw 10` … `#5 Withdraw 10`: every response. These are marked passed whenever a response arrived,
   whatever its status; they are evidence, not verdicts.
3. `State after: Get wallet`
4. `Race test verdict: 5 × Withdraw 10`: one check per invariant, with expected and actual values. This result
   decides whether the run passed.

HTML and JSON reports exported from History include all of them, with secrets removed.

## Safety

- Before every run a confirmation dialog shows the number of requests, the environment, and any warnings, such as
  unsaved changes (the saved version is sent) or a protected environment. On a protected environment the run is
  refused unless you confirm.
- 50 copies is the maximum, so a race test cannot turn into a load test.
- **Cancel** stops the copies that are still waiting or in flight.
- The usual network safety rules apply: blocked destinations, TLS verification and proxy settings from **Settings**.

## Tips

- **Make the race possible.** A coupon that is already used, or a balance that covers every request, cannot show a
  double spend. Reset the test data before each run, for example with a `POST /test/reset` request, or a coupon
  created in a pre-request script.
- **Use realistic limits.** For a balance of 25 and withdrawals of 10, **At most** `2` is correct; three or more
  successes is an overdraft.
- **Idempotency keys.** To test that a repeated request with the *same* key is applied once, put a fixed key in the
  header and add a state check. If the API answers duplicates with 409, use **Exactly** `1` and the per-request
  change. If it replays the first `200` to every duplicate, every copy counts as a success, so the state check
  would expect *n ×* the change. The race test cannot express "applied once" for that style yet; compare the
  **State before** and **State after** results in History instead. To test that different keys are all
  applied, use `{{$uuid}}` in the header (resolved separately for every copy) and **At least** *n*.
- **Run more than once.** Races depend on timing. A test that passes five times in a row is good evidence. One that
  fails once is a real bug.
- **Read the 4xx mix.** Losing copies should get a clear 409 or 422. A mix of 409 and 500 means the server relied
  on a database error to stop the duplicate.

## Stored format

The settings are stored per request in the local database (table `race_tests`, migration `0006`) as JSON, and
deleted with the request:

```json
{
  "requestId": "…",
  "count": 5,
  "successOp": "",
  "successCount": 0,
  "uniquePath": "",
  "allowServerErrors": false,
  "check": { "requestId": "…", "path": "data.balance", "deltaPerSuccess": -10, "tolerance": 0 }
}
```

`successOp` is `"lte"`, `"eq"`, `"gte"` or `""` (no limit). Race test settings are not yet part of collection
export or duplication.

## Automated tests (for contributors)

```sh
go test ./internal/security -run Race -v                  # invariants: success rules, uniqueness, state check
go test ./internal/runner -run 'Burst|Gate' -race -v      # start gate: all copies in flight together, no deadlock
go test ./tests/integration -run TestRaceTest -v          # double redeem, locked endpoint, lost update, protected env
```

The code is in `internal/runner/burst.go` (start gate), `internal/security/race.go` (evaluation),
`internal/service/security.go` (orchestration) and `frontend/src/components/security/RaceTestPanel.vue`. The integration
tests use the mock API in `tests/integration/security_test.go`.
