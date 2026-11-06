# Comparing two environments

An **environment comparison** runs the same collection against two environments at the same time (for example
**Production** and **Staging**, or the old build and the new build) and lists only what differs between their
responses. It answers *"is the new deploy behaving like the old one?"* without approving a single baseline first.

| Difference                                | Example                                                  | Shown as                    |
| ----------------------------------------- | -------------------------------------------------------- | --------------------------- |
| **Status code**                           | `200` in Production, `500` in Staging                    | Status `200 → 500`          |
| **Body field changed, added or removed**  | `data.total` is `120.5` vs `120`; `data.currency` is gone | One row per JSON path       |
| **JSON type changed**                     | `amount` is a number in one, a string in the other       | Type changed                |
| **Header added, removed or changed**      | `Cache-Control` differs, `Strict-Transport-Security` missing | One row per header      |
| **Much slower**                           | 120 ms vs 900 ms                                         | Slower in Staging (7.5×)    |
| **Request failed in one environment**     | Timeout, connection refused, script error                | Error, with the message     |

Fields whose value changes on every call (IDs, timestamps, tokens) would make every request look different, so the
comparison first [detects changing fields](changing-fields.md) and leaves them out.

Find it in a collection's **⋯** menu → **Compare environments…**.

## How it works

1. Choose the two environments: **A** (the reference, for example Production or the old build) and **B** (the one you
   are checking). They must be different.
2. By default only requests that cannot modify data (`GET`, `HEAD` and `OPTIONS`) are
   compared. Tick **Include requests that may modify data** to compare the others too; they are sent once to each
   environment.
3. **Detect changing fields** (on by default) sends every read-only request a second time to each environment. A
   field whose value differs between the two calls *within one environment* is changing by nature and is not
   compared. The fields left out are listed with each request.
4. Optionally list more **ignored paths** (one per line, same patterns as baselines: `meta.request_id`,
   `items[*].updated_at`, `**.timestamp`). A line `header:X-Build-Id` ignores a header. Ignored paths of the request's
   baselines in either environment are applied as well.
5. Press **Compare**. Both environments run at the same time; within each environment requests run in collection
   order with the **Parallel requests** setting, sharing one script scope, as in a normal collection run.

The result shows a summary (**same**, **different**, **errors**, **slower**) and a row per request. Only requests with
differences are shown until you tick **Show identical requests**. Open a row for its status, header and body
differences, the durations, and the fields that were left out as changing. **Open in history** shows each side's full
response.

### What counts as a difference

- **Status:** the status codes differ.
- **Body:** both bodies are JSON: compared field by field, independent of key order; arrays by index; numbers equal
  when their values are equal (`1` and `1.0`). One or both are not JSON: the bodies differ as text. A truncated body
  (larger than the response size limit) is not compared and says so.
- **Headers:** names are compared case-insensitively and repeated headers are joined with `, `. Headers that differ on
  every response are ignored: `Date`, `Age`, `Expires`, `Last-Modified`, `ETag`, `Set-Cookie`, `Content-Length`,
  `Transfer-Encoding`, `Connection`, `Keep-Alive`, `Server-Timing`, `Report-To`, `NEL`, `Alt-Svc`, `Via`, and
  tracing headers (`X-Request-Id`, `X-Correlation-Id`, `X-Trace-Id`, `Traceparent`, `Tracestate`, `X-Amzn-*`,
  `X-Amz-Cf-*`, `CF-Ray`, `X-Cache*`, `X-Served-By`, `X-Timer`, `X-Runtime`, `X-Response-Time`). Secret header values
  are redacted on both sides before comparing.
- **Errors:** a request that fails (no response) in either environment is reported as an error with its message.
- **Timing** is shown for every request and flagged as **slower** when one side takes at least 1.5 times as long as the
  other *and* at least 100 ms longer. Slower is reported next to the verdict; on its own it does not make a request
  "different", because timing between environments is noisy.
- A request is **same** when it has no status, header or body difference and no error.

Socket requests are skipped: they have no single response to compare.

### Safety rules

- Before the comparison, the same preflight as a collection run is done for **each** environment: missing required
  variables, blocked destinations and requests that may modify data. A protected environment combined with requests
  that may modify data needs explicit confirmation.
- Variables changed by scripts are **not** saved, so a comparison never changes your environments.
- **Cancel** stops both sides; requests not started yet are reported as cancelled.
- Each environment's first pass is saved in **History** as an ordinary run (one run per environment, named after the
  collection). The extra calls made to detect changing fields are not saved.
- Secrets are redacted from everything shown and stored.

The settings (environments, ignored paths, the two checkboxes) are saved with the collection. The comparison result
itself is not stored; run it again at any time.

## Try it in five minutes

The demo serves an "old" build on port 8091 and a "new" build on port 8092 with a few deliberate regressions:

```sh
python3 docs/demo/compare_demo_api.py      # Python 3.9+, no dependencies
```

1. **Environments:** create `Old build` with `base_url = http://localhost:8091` and `New build` with
   `base_url = http://localhost:8092`.
2. **Requests → Create collection** named `Compare demo` with these saved requests:
   - `Get account`: `GET {{base_url}}/account`
   - `List orders`: `GET {{base_url}}/orders`
   - `Health`: `GET {{base_url}}/health`
   - `Get prices`: `GET {{base_url}}/prices`
3. Collection **⋯ → Compare environments…**, A = `Old build`, B = `New build`, **Compare**.

You should see:

| Request       | Verdict   | Why                                                                                 |
| ------------- | --------- | ----------------------------------------------------------------------------------- |
| `Get account` | different | `balance` changed type (number → string); `request_id` and `server_time` were left out as changing |
| `List orders` | different | `orders[1].status` changed; header `Cache-Control` changed; `generated_at` was left out as changing |
| `Health`      | same      | `uptime_s` and `checked_at` change on every call, so they were left out             |
| `Get prices`  | same, slower | the new build answers in about 400 ms instead of 10 ms                           |

Untick **Detect changing fields** and compare again: `Health` becomes *different* and the other requests also report
`uptime_s`, `checked_at`, `request_id`, `server_time` and `generated_at`, which is exactly the noise detection
removes.

## Limits

- Requests are matched by the request itself, so both environments must be reachable with the same collection
  (usually through `{{base_url}}` and similar variables).
- Arrays are compared by index. A list that comes back in a different order shows as changed items; ignore the list
  (`orders`) or its fields (`orders[*].position`) if order does not matter.
- At most 500 differences are listed per request.
- One comparison runs at a time.
