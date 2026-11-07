# API Testing & Regression Guard

**Target platforms:** Windows and Linux first; macOS may be added later.  
**Suggested stack:** Go, Wails, Vue 3, TypeScript, Pinia, SQLite.  
**Product approach:** Keep core desktop testing free. Consider optional paid cloud/team features only after validating recurring user demand.

---

## 1. Product Vision

Help developers and QA testers catch API regressions before deployment, especially failures caused by code changes, environment variables, configuration changes, database settings, and business-logic errors.

The app should be easy to start using, work offline for local collections and test history, and not require a cloud account for core functionality.

### Problem to solve

API regressions are not limited to HTTP errors. An endpoint can return `200 OK` while returning incorrect balances, totals, permissions, or report data. The tool should validate both transport-level behavior and business-level expectations.

### Target users

- Backend developers
- QA engineers and manual testers
- Small software teams without a dedicated test platform
- Developers maintaining APIs with scheduled jobs or Telegram/email reports
- Independent developers who need repeatable checks before releasing changes

### Product positioning

A lightweight, local-first alternative to heavyweight API workflows, focused on:

1. Repeatable HTTP API tests
2. Business-rule assertions
3. Regression comparisons
4. Environment and configuration checks
5. Business-security checks that request-by-request clients miss: race conditions and access control
6. Regression checks without setup: comparing two environments and detecting changing fields
7. Clear failure reports

---

## 2. Goals and Non-Goals

### MVP goals

- Create and organize HTTP requests.
- Run requests individually or as a collection.
- Support environment variables without committing secrets to source control.
- Validate status codes, headers, response time, JSON fields, and custom assertions.
- Compare selected JSON response fields against a saved baseline.
- Store test collections and run history locally.
- Export a readable HTML or JSON test report.
- Build distributable Windows and Linux desktop packages.
- Work without requiring a user account or hosted backend.

### Non-goals for the first release

- Full browser automation
- Load testing at scale (a race test sends at most 50 simultaneous copies of one request; it checks correctness under
  concurrency, not throughput)
- Automatic vulnerability exploitation (the permission matrix only sends the user's own saved requests, as roles the
  user defines, and compares the results with the user's expectations)
- Replacing a full API client or API gateway
- Hosted collaboration and organization management
- Running arbitrary user scripts without strict controls
- Automatic production testing that may modify real data
- macOS support before the Windows/Linux release is validated

---

## 3. Core User Workflows

### Workflow A: Create and run an API test

1. Create a workspace or collection.
2. Add a request with method, URL, headers, query parameters, and body.
3. Select an environment, such as Local, Staging, or Production.
4. Add assertions, such as expected status code and JSON field values.
5. Run the request.
6. Review the request, response, assertion results, and duration.
7. Save the test and rerun it later.

### Workflow B: Detect a regression

1. Run a known-good request against a controlled environment.
2. Save a baseline for selected stable response fields.
3. Change application code or configuration.
4. Run the same collection again.
5. Compare actual results with the baseline and assertions.
6. Review added, removed, changed, or type-changed JSON fields.
7. Export the regression report.

A baseline is not automatically the truth. The user must review and approve a new baseline after verifying the behavior is correct.

### Workflow C: Check environment or configuration changes

1. Select a named environment.
2. Run a test collection against it.
3. Verify required environment variables are present.
4. Execute read-only health and business-rule tests.
5. Compare the result with the previous run.
6. Review failures before promoting the configuration.

### Workflow D: Verify reports and scheduled jobs

For the MVP, test report APIs or endpoints that expose job status and generated data. The CLI (`api-test-kits-cli`, see [cli.md](cli.md)) runs collections from cron, systemd timers, or CI pipelines; sending results to a notification endpoint is still open.

Do not promise that a desktop GUI process can run jobs while the computer is powered off.

### Workflow E: Find a race condition

1. Save a request that changes state, such as redeeming a coupon or withdrawing money.
2. Open its **Race test** tab and set how many copies to send at once and how many may succeed.
3. Optionally choose a JSON field that must be unique per success, and a second request that reads a number (such as
   a balance) before and after the burst.
4. Confirm and run. All copies are released at the same moment.
5. Review the verdict: too many successes (double spend), duplicate IDs, 5xx responses, or a state change that does
   not match the number of successes (lost update).
6. After a fix, rerun with the saved settings. The run and every response are stored in history.

### Workflow F: Verify access control

1. Define roles for a collection; each role overrides variables such as `token`, and one role may send no
   credentials.
2. Tick the requests to check, read-only ones by default.
3. Run the matrix: every ticked request is sent once as every role.
4. Mark each cell as expected **allow** or **deny**, or approve the observed results after reviewing them.
5. Rerun after authentication, role or middleware changes. A request allowed where deny is expected is reported as
   broken access control (including IDOR); a request denied where allow is expected is reported as lost access.

### Workflow G: Test a WebSocket or Socket.IO API

1. Create a request and set its type to **WebSocket** or **Socket.IO**; enter the URL (for Socket.IO the URL path is
   the namespace) and the auth, headers or Socket.IO auth payload the server expects.
2. Open the **Console**, connect, send messages or events and watch what arrives; save useful messages.
3. Turn the exchange into a **test**: send steps, expect steps (event, acknowledgement and checks on the message body,
   within a time limit) and waits.
4. **Run test**. The steps run in order on one connection; the result lists each step and every message.
5. Collection runs and the CLI run the test with the HTTP requests, so CI catches real-time regressions too.

### Workflow H: Check a new deploy against the old one

1. Have one environment per deployment, such as Production and Staging, or the old and the new build.
2. Open a collection's **Compare environments**, choose A (reference) and B (to check).
3. Compare: both environments run the collection's read-only requests at the same time, after leaving out fields that
   change on every call.
4. Review only what differs: status codes, body fields, headers, errors and much slower responses.
5. Fix the regression or accept the difference; no baseline is needed.

### Workflow I: Set up a baseline without hunting for dynamic fields

1. Send a request and choose **Approve as baseline…**.
2. Press **Detect changing fields**: the request is sent a few more times and every field whose value changed is
   suggested as an ignored path.
3. Review the suggestions, add them, and approve the baseline. The next run does not fail on timestamps or IDs.

### Workflow J: Check business logic across requests

1. Put the requests of a flow in a collection in order, for example *balance before*, *transfer*, *balance after*.
2. Open the collection's **Rules** and add a rule that combines values from several requests, such as
   *balance after = balance before − transfer amount*, or *the created order's id appears in the order list*.
3. Check the rule against the last run while writing it, then save.
4. Every collection run (and the CLI) evaluates the rules after the requests and fails when one does not hold.

### Workflow L: Keep the API and its documentation in step

1. Import the API's OpenAPI file, or attach it to an existing collection (**⋯ → API contract…**).
2. Run the collection. Responses that drift from the spec fail with the field and the problem: wrong types, missing
   required or undocumented fields, enum values, undocumented status codes or content types.
3. Fix the API or update the spec and **Replace spec…**; the CLI runs the same check in CI.

### Workflow K: Sign off a release

1. Run the collection (and its race tests and permission matrix) in the release environment.
2. Open the collection's **Sign-off report**, enter the release name and your name, and preview.
3. Review the verdict and its reasons: failures, regressions, security checks, slower requests, coverage gaps and who
   approved each baseline.
4. Save the HTML or print it to PDF, sign it and hand it over.

---

## 4. MVP Feature Requirements

### P0 — Required for first usable release

#### Request builder

- HTTP methods: GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS
- URL, query parameters, headers, and request body
- JSON and plain-text request bodies
- Authentication: Bearer token, Basic Auth, and API key
- Request timeout
- Redirect handling
- Cancel an in-progress request
- Display status code, response headers, response body, and duration
- Response size limit configurable with a safe default

#### Collections and environments

- Create, rename, duplicate, and delete collections and requests
- Organize requests in folders
- Store environment variables locally
- Variable interpolation in URL, headers, query parameters, and body
- Mark sensitive variables as secret
- Mask secrets in the UI and reports
- Import/export collections using a documented JSON format
- Warn before exporting secrets
- Never include secrets in logs by default

#### Assertions

- Expected HTTP status
- Response header exists or equals a value
- JSON path exists
- JSON field equals, not-equals, or matches a type
- Numeric comparisons: greater-than, less-than, greater-than-or-equal, less-than-or-equal
- String contains or matches a safe pattern
- Response time below a threshold
- Response body is valid JSON
- Required fields are present
- Assertion results include expected value, actual value, and a useful failure message

#### Regression comparison

- Save a baseline for a response or selected JSON paths
- Compare JSON objects independent of key order
- Report added, removed, and changed fields
- Detect JSON type changes
- Allow users to ignore dynamic fields such as timestamps, request IDs, and random tokens
- Support numeric tolerance for floating-point fields
- Show a readable diff
- Require explicit user approval to update a baseline

#### Run history and reports

- Persist run summaries and assertion results in SQLite
- Show pass/fail counts and duration
- Filter history by collection, request, and date
- Export JSON and self-contained HTML reports
- Redact configured secret values from reports
- Include app version, run time, target environment name, and test counts
- Do not include authorization headers or secret environment values

#### Desktop quality

- Keyboard-friendly request editing
- Clear empty states and error messages
- Confirm destructive operations
- Recover safely from interrupted writes
- Provide Windows and Linux build instructions
- Keep core features usable without login or network access except when making API requests

### P1 — After MVP feedback

- ~~CLI mode for running exported collections~~ — done: `api-test-kits-cli` ([cli.md](cli.md)). Pure Go, runs an export
  (optionally with baselines) or a collection from the app database, writes JUnit/HTML/JSON reports, exit codes
  0/1/2/3/130, secrets from `APIGUARD_VAR_*`
- ~~CI integration examples for GitHub Actions and other CI systems~~ — done: GitHub Actions and GitLab CI in
  [cli.md](cli.md)
- Scheduled local test runs
- Notifications through a configurable webhook
- CSV export for selected tabular results
- Encrypted secret storage using platform facilities where practical
- Request chaining and extracting values from prior responses
- OpenAPI import
- Test templates for common API patterns
- ~~WebSocket and Socket.IO requests~~ — done, see the section below and [sockets.md](sockets.md)

### Business-security tests — implemented after the MVP

These set the product apart from general-purpose API clients. Guides: [race-test.md](race-test.md) and
[permission-matrix.md](permission-matrix.md).

#### Race-condition test (per request)

- Send 2–50 copies of a saved request at the same moment. Each copy runs its pre-request scripts first, then waits at
  a start gate; a copy that fails before sending must not block the others.
- Invariants: at most, exactly or at least *n* successful (2xx) responses; a JSON path that must be unique across
  successful responses; no 5xx unless accepted; every request gets a response.
- Optional state check: a second saved request reads a number before and after the burst; the change must equal
  successes × the configured change per success, within a tolerance.
- The request's own assertions and baseline are not part of the verdict; losing copies are expected to fail.
- Store the settings per request. Record the run in history with every response and a verdict result listing the
  invariants.

#### Permission matrix (per collection)

- Roles override environment and collection variables; references such as `{{bob_token}}` are resolved, and values
  derived from secret variables are redacted. A role with no credentials removes auth and `Authorization`/`Cookie`
  header rows.
- Each cell (request × role) has an expectation: allow, deny or none.
- Judge 2xx/3xx as allowed; 401, 403 and 404 as denied; other 4xx against a deny expectation as suspicious (not a
  failure); 5xx and missing responses as failures.
- Allow approving observed results as expectations, after an explicit warning.
- Store roles, ticked requests and expectations per collection. Record each run in history with one result per cell.

#### Shared rules for both

- Confirm before every run, listing requests that may modify data and protected environments; refuse runs on
  protected environments without confirmation.
- Never save variable changes made by scripts during these runs.
- Support cancellation; redact secrets from results, history and reports.

**Not yet done:** including race tests and matrices in collection export and duplication; dedicated sections in HTML
reports; running them from the CLI.

### WebSocket and Socket.IO requests — implemented after the MVP

Real-time APIs regress like HTTP ones: a renamed event, a changed payload or a broken auth check. Guide:
[sockets.md](sockets.md).

#### Requests

- A request has a **kind**: `http` (the default, also for older data), `websocket` or `socketio`. Socket requests share
  URL, query, headers, auth (including inherited collection auth), timeout, variables and pre-request scripts with
  HTTP requests, and always use `GET` for the handshake. Body, HTTP assertions and post-response scripts do not apply;
  switching the kind keeps them so switching back restores them.
- **WebSocket:** `ws://` and `wss://` URLs (`http(s)://` are converted); optional subprotocols.
- **Socket.IO v3/v4:** Engine.IO protocol 4 over the WebSocket transport only. As in the official client, the URL path
  is the namespace and the server path is a separate setting (default `/socket.io/`). An optional JSON-object auth
  payload is sent with CONNECT; `connect_error` messages are shown. Ping/pong is handled; a server that stops pinging
  ends the connection. Binary attachments, long-polling and Engine.IO 3 (Socket.IO v2) are not supported.
- **Messages** have a body sent as JSON or text; Socket.IO messages also have an event name (reserved names such as
  `connect` are rejected) and may ask for an acknowledgement. Saved messages are stored with the request.
- Connections follow the HTTP client's rules: TLS verification and extra CA certificates, proxy setting, blocked
  link-local and metadata destinations, no redirects. Incoming messages are bounded by the response size limit.

#### Console

- Connect, send (saved or ad-hoc) messages, see sent, received and connection messages live with timestamps; filter,
  search and expand them. At most 1,000 messages per console and 20 open connections.
- Connecting runs the same preflight checks as Send and the pre-request scripts (keeping the variables they set). In a
  protected environment the user confirms first, because console messages may change data.
- Console traffic is not stored in history.
- **Heartbeat** (presence testing): send the composer's message now and then every 0.1–600 s from the backend, so
  the rhythm holds while the window is in the background; heartbeat messages are marked ♥ and can be hidden in the
  log; stopped by the user, by disconnecting, or when a send fails. Built-in variables are filled in per heartbeat.

#### Tests

- A socket request's test is an ordered list of steps on one connection: **send** a message, **expect** a received
  message (optional Socket.IO event, acknowledgement or event, and checks on the body using the existing JSON path,
  body, valid-JSON, required-fields and aggregate checks) within a timeout, or **wait**.
- **Start heartbeat** (message + interval 100 ms–600 s) repeats a message in the background while later steps run;
  **stop heartbeat** stops it and reports the count; **expect disconnect** passes when the server closes the
  connection within a timeout (with the close code) and stops the remaining steps. A heartbeat still running at the
  end of the test is stopped and noted in the transcript. Guide: [sockets.md](sockets.md#heartbeats-testing-online-and-offline-presence).
- Expect steps match any not-yet-matched message received since connecting, so replies may arrive in any order; each
  message satisfies at most one expect step. A failed expect reports how many candidates there were and why the
  closest one failed.
- The request timeout bounds connecting and the whole test. When it expires, the run is cancelled or the server closes
  the connection, the remaining steps are reported as not run.
- Results are ordinary run results: one result per step, the handshake as the response, and a transcript of every
  message. They appear in history, collection runs, JSON/HTML/JUnit reports and the CLI; secrets are redacted.
- Tests that send messages count as possibly modifying data in preflight warnings and protected environments.
- Race tests, permission matrices, baselines and response-time checks stay HTTP-only.

### Regression checks without setup — implemented after the MVP

Baselines need a known-good response and a list of dynamic fields before they are useful. These two features remove
that setup. Guides: [environment-comparison.md](environment-comparison.md) and [changing-fields.md](changing-fields.md).

#### Changing-field detection

- Send a request (saved or draft) 3 times in a row with the selected environment, its scripts, auth and variables.
- Compare the JSON responses with each other: leaf values per path, array indices generalized to `[*]`; report paths
  whose value or type changed or that are missing from some responses; report arrays whose length changed separately
  (not selected by default). Report a status code that changed between calls as a warning.
- Fail with a clear message for a non-JSON or truncated response or a failed call.
- Leave out paths already covered by the current ignored patterns. List at most 200 suggestions, each with up to three
  redacted example values.
- Available in the approve-baseline dialog and the Baseline tab; the user adds the selected suggestions to the ignored
  paths, nothing is saved automatically.
- Requests that may modify data need explicit confirmation; protected environments follow the Send rules. Script
  variable changes are not saved; the calls are not recorded in history. Cancellable.

#### Environment comparison (per collection)

- Choose two different environments A and B. Run the collection's HTTP requests (read-only ones unless the user
  includes requests that may modify data) against both at the same time; within each environment in collection order
  with the configured concurrency and one script scope.
- Optionally detect changing fields: send each read-only request a second time to each environment; paths whose value
  differs between the two calls in either environment are not compared, and are listed per request.
- Per request compare: status code; JSON bodies with the baseline diff engine (ignored paths: the comparison's own, the
  request's baselines' in A and B, and changing fields), or bodies as text when either is not JSON; headers
  case-insensitively, except a fixed list of volatile headers and those ignored with `header:Name`.
- Verdict per request: same, different, error (no response from A or B) or skipped (socket requests, requests that
  may modify data when not included). Flag *slower* when one side takes ≥ 1.5× and ≥ 100 ms longer; it does not make a
  request different.
- Preflight both environments; confirmation for protected environments with requests that may modify data. Script
  variable changes are not saved. Cancellable; progress events. Save each environment's first pass as a normal run in
  history; do not save the detection calls. Store the settings per collection; do not store the comparison result.
- Socket requests are skipped. At most 500 differences per request.

### Collection rules — implemented after the MVP

Checks that span several requests of one run, without scripts. Guide: [collection-rules.md](collection-rules.md).

- Stored with the collection (`rules`), so they are exported, imported, duplicated and run by the CLI; request
  references are remapped when IDs are regenerated. At most 50 rules, 10 right-side terms each.
- A value is read from one request's result in the run: response body or request body (JSON path, optionally
  count/sum/min/max), status code, or response header; or it is a constant.
- **Compare** (`=`, `≠`, `>`, `≥`, `<`, `≤`, numeric tolerance): left value against one value, or a sum of values with
  `+`/`−` signs (numbers only). One value with `=`/`≠` compares any JSON values.
- **Contains** (*appears in*, *does not appear in*): a value against the list selected by a path; numbers and strings
  compare by value.
- Evaluated after a collection run, in any request order or concurrency. A rule runs only when all its requests were
  in the run and it is enabled. A value that cannot be read fails the rule with a clear message.
- The results form one more run result, *Collection rules*, with one check per rule showing the values used; it counts
  in the run's tally, history, reports, JUnit and the CLI exit code. Values come from redacted results.
- **Check against last run** evaluates unsaved rules against the newest history run of the collection without sending
  requests.

### Release sign-off report — implemented after the MVP

A summary of one collection in one environment for QA hand-off and audits. Guide: [signoff-report.md](signoff-report.md).

- Inputs: release name (required), environment, prepared by (default: the reviewer name in Settings), notes.
- Built only from stored data (history, baselines, response times); no request is sent.
- Sections: verdict with reasons; the newest collection run in the environment (counts, coverage, failures with
  messages, collection rules); regressions with their first differences; the newest race test per request and the
  newest permission matrix run; requests flagged slower than usual or whose median changed by ≥ 20 %; baselines with
  mode, approval time and approver; coverage (requests without tests, without a baseline, not in the run); sign-off
  block with signature lines.
- *Ready for release* requires a run that passed with no regressions and passing newest security checks; everything
  else is listed, not blocking.
- Baselines record **approved by** (the name entered when approving). Race tests and permission matrices are recorded
  with their own run kinds (`race`, `matrix`); older `burst` runs are still recognised.
- Output: one self-contained HTML file without scripts, with print styles; PDF through the system print dialog from an
  in-app preview. Values are redacted again with the current secrets; response bodies are not included.

### OpenAPI contract check — implemented after the MVP

Checks real responses against the OpenAPI spec attached to the collection. Guide: [openapi-contract.md](openapi-contract.md).

- One spec per collection (OpenAPI 3.x or Swagger 2.0, JSON or YAML, ≤ 10 MB), stored as JSON; attached automatically
  when an OpenAPI file is imported, or with **API contract… → Attach spec…**. Exported, imported and duplicated with
  the collection, so the CLI checks it too. Options: on/off, allow undocumented fields.
- Runs on Send and collection runs (not race tests, matrices or comparisons) for HTTP requests whose method and final
  URL path match an operation: server path prefix, then suffix match; most fixed segments wins; host ignored.
- Checks the status code (exact, `NXX`, `default`), required response headers, Content-Type, and the JSON body against
  the schema: types, nullable, required, undocumented fields (unless allowed or `additionalProperties`), enum, const,
  numeric and length limits, pattern, common formats, items, allOf/anyOf/oneOf/not and local `$ref`s. External `$ref`s
  are not followed; truncated bodies are not checked.
- Results are assertion results of kind `contract` (one passing result, or one per problem, ≤ 20) that fail the
  request, count in history, reports, JUnit and the CLI exit code. Messages are redacted and quote values ≤ 40
  characters only.
- The **API contract** tab shows the spec, the options and coverage: which operation each request matches in the
  selected environment, and operations without a request.

### P2 — Only after validating demand

- Optional hosted run history
- Team workspaces and shared collections
- Centralized scheduled runs
- Team dashboards and report retention
- Role-based access and audit logs
- Organization-level secret management

---

## 5. Example Business-Rule Tests

The tool should make business logic easy to verify, not just status codes.

### Wallet balance

Example assertions:

- Response status is `200`.
- `data.balance` exists and is numeric.
- `data.currency` equals the expected currency.
- Balance after a test transaction equals the expected value.
- A rejected transaction does not change the balance.
- Under concurrent requests, a withdrawal is applied once per successful response and never overdraws the account
  (race test with a state check on the balance).

Use a dedicated test account and controlled data. Never run destructive financial tests against real customer accounts.

### Report totals

Example assertions:

- Report date range matches the requested range.
- `total_count` equals the number of returned records, where the API contract defines that relationship.
- Sum of item amounts equals the reported total within an explicitly configured tolerance.
- Empty periods return a valid empty result instead of stale data.
- Pagination does not double-count records.

### Authorization

Example assertions:

- Unauthenticated requests return the documented status.
- A user cannot access another user's protected resource.
- A read-only role cannot perform a write operation.
- Sensitive fields are not present in responses.

The permission matrix covers the first three for a whole collection at once: one row per endpoint, one column per
role.

Only test systems and accounts for which the user has authorization.

---

## 6. Technical Architecture

```text
┌─────────────────────────────────────────┐
│             Vue 3 + TypeScript           │
│  Request Editor · Collections · Results │
│  Environments · Assertions · Diff View  │
└────────────────────┬────────────────────┘
                     │ Wails bindings
┌────────────────────▼────────────────────┐
│                  Go App                  │
│ Request Runner · Assertion Engine       │
│ Regression Comparator · Report Exporter │
│ Collection/Environment Services         │
└──────────┬─────────────────────┬────────┘
           │                     │
┌──────────▼─────────┐  ┌────────▼────────┐
│ SQLite Repository  │  │ HTTP Client     │
│ Collections        │  │ TLS · Timeout   │
│ Environments       │  │ Redirect Policy │
│ Baselines/History  │  │ Response Limits │
└────────────────────┘  └─────────────────┘
```

### Recommended stack

- **Backend:** Go
- **Desktop framework:** Wails
- **Frontend:** Vue 3, TypeScript, Vite
- **State management:** Pinia
- **UI:** accessible component library or a small consistent design system
- **Local database:** SQLite
- **Validation:** explicit Go validation functions and typed domain models
- **Testing:** Go unit tests, integration tests with local mock servers, frontend unit tests, packaged-app smoke tests
- **CI:** GitHub Actions with Windows and Linux build/test jobs

Keep request execution and assertion logic independent of Wails so the same engine can later power a CLI.

### Code standards

Prefer reusing existing code over writing new code. Before adding a component, helper or package, check whether
one already does the job; when the same markup or logic appears a second time, move it into a shared place instead of
copying it. Use standard, framework-recommended patterns rather than custom ones. The full frontend conventions,
including the table of base components, are in [architecture.md](architecture.md#frontend).

#### Frontend: reusable building blocks

- **Base components** (`components/base/`) are the only place that writes raw form, table and button markup. Features
  use `BaseButton`, `BaseInput`, `TextField`, `SelectField`, `CheckboxField`, `RadioGroup`, `FormField`, `DataTable`, `TabBar`,
  `SelectableList`, `StatTile`, `StatusBadge`, `MethodBadge`, `FilterChip`, `CalloutBox`, `EmptyState`, `ModalFrame`,
  `MenuButton` and `CodeEditor`, never a hand-written `<button>`, `<table>`, `<input>`, `<textarea>` or radio group. The one exception is
  `VariableInput`, which is itself a control (an input with `{{variable}}` highlighting) built on the stores. Base components take
  props and emit events only; they do not use stores or call the backend.
- **Feature components** live in a folder per area (`layout`, `collection`, `request`, `response`, `run`,
  `performance`, `security`, `variables`); **views** compose them, one per top-bar tab.
- **Backend calls** go through the `useAsyncAction()` composable, which provides the busy flag and shows errors as a
  toast. Do not repeat try / catch / finally around API calls; App startup is the one exception, because it must tell
  a missing backend apart from other errors.
- **Shared logic:** display formatting (bytes, durations, times, changes) belongs in `utils/format.ts`, generic object
  and HTTP helpers in `utils/object.ts` and `utils/http.ts`, and domain logic without Vue in `services/`. Components
  call these helpers instead of formatting or parsing inline.
- **State** shared between screens lives in Pinia stores (`workspace`, `environments`, `runner`, `ui`); local UI state
  stays in the component.
- No new runtime dependencies without discussing them first.

#### Frontend: Vue standards

Follow the [Vue style guide](https://vuejs.org/style-guide/) (essential and strongly recommended rules) and the
Composition API:

- Single-file components in the order `<script setup lang="ts">`, `<template>`, `<style scoped>`.
- Component files and names in PascalCase with more than one word (`RequestEditor.vue`, `BaseButton.vue`); base
  components start with `Base` or name a generic control (`TextField`, `DataTable`).
- Typed `defineProps<…>()` and `defineEmits<…>()`; props flow down, events flow up. Never mutate a prop; use
  `defineModel()` / `v-model` for two-way bindings.
- `computed` for derived values, `watch` only for side effects; no direct DOM access except through template refs.
- Every `v-for` has a stable `:key`; do not put `v-if` and `v-for` on the same element.
- Composables are named `useX` and return refs; Pinia stores use `defineStore` and are named `useX` too.
- Styles are scoped; reach into a child component only with `:deep()`. Colours, shadows and radii come from the CSS
  variables in `style.css`.

#### Wails standards

The app follows the standard [Wails v2](https://wails.io/docs/introduction) project layout (`wails.json`,
`main.go`, `app.go`, `frontend/`, `build/`):

- **Thin bindings.** `app.go` holds the one bound struct (`App`). Each exported method only forwards to
  `internal/service`, passing the context saved in `OnStartup`; no business logic lives in the binding layer, so the
  CLI can reuse the same service.
- **Bindable types.** Bound methods take and return plain structs from `internal/domain` with JSON tags, and return
  `error` as the last result when they can fail (simple getters such as `AppInfo` or `Cancel` do not). Wails turns a returned error into a rejected promise on the frontend.
- **One frontend entry point.** Components never call `window.go` or `window.runtime` directly. Every backend call
  is a typed function in `services/api.ts`, which throws `BackendUnavailableError` when the page runs without the
  desktop backend (for example in plain `vite` dev or tests).
- **Events for progress.** Long runs report progress with `runtime.EventsEmit` under a namespaced name
  (`run:progress`, `matrix:progress`) and carry an operation ID; the frontend subscribes through helpers in
  `services/api.ts` and unsubscribes when the component unmounts. Long operations can be cancelled by operation ID.
- **Native features from Go.** File open/save dialogs and other OS features use the Wails `runtime` package in
  `app.go`, not browser APIs, so they look native on Windows and Linux.
- **Builds** use `wails build` for Windows and Linux; `wails dev` for development with hot reload.

#### Frontend: style

All frontend code (`.ts`, `.vue`, `.css`) is formatted by Prettier (`frontend/.prettierrc.json`) and checked with
`npm run lint`:

- **Imports:** `@/…` for anything outside the component's own folder, `./…` for files in the same folder. They are
  sorted automatically into three groups separated by blank lines: packages, `@/` imports, relative imports.
- **Semicolons**, single quotes and trailing commas.
- **Maximum line length: 120 characters.** Prettier wraps code at 120 columns; `scripts/check-line-length.mjs` fails
  the lint step for anything it cannot wrap, such as long strings or template text.

Run `npm run format` to apply the style and `npm run lint` to check it.

#### Go

- Format with `gofmt` and keep `go vet` clean.
- One package per responsibility under `internal/` (runner, assertions, regression, redact, storage, …). The desktop
  app (`internal/service`) and the CLI (`internal/cli`) call the same packages; neither duplicates engine logic.
- Shared concerns live in one place and are reused everywhere: secret redaction in `internal/redact`, the HTTP
  client and its safety rules in `internal/httpclient`, domain types in `internal/domain`, atomic file writes in
  `internal/fsutil`.
- Use the standard library first; return wrapped errors (`fmt.Errorf("…: %w", err)`) instead of panicking.

#### Tests and checks

- Tests sit next to the code they cover (`*_test.go`, `*.test.ts`); base components are covered in
  `components/base/BaseComponents.test.ts`.
- Every bug fix comes with a test that fails without the fix.
- `make check` runs frontend lint, typecheck and tests, `gofmt`, `go vet` and the Go tests; `make hooks` installs it
  as a pre-commit hook.

---

## 7. Suggested Repository Structure

```text
api-test-kits/
├── app.go
├── main.go
├── go.mod
├── internal/
│   ├── domain/
│   │   ├── collection.go
│   │   ├── request.go
│   │   ├── environment.go
│   │   ├── assertion.go
│   │   └── run.go
│   ├── httpclient/
│   │   ├── client.go
│   │   ├── safety.go
│   │   └── websocket.go      # WebSocket dialer with the same network rules
│   ├── socket/               # WebSocket and Socket.IO connections (sockettest: local test servers)
│   ├── runner/
│   │   ├── runner.go
│   │   └── concurrency.go
│   ├── assertions/
│   │   ├── evaluator.go
│   │   └── jsonpath.go
│   ├── regression/
│   │   ├── baseline.go
│   │   ├── changing.go       # fields that change between calls
│   │   └── diff.go
│   ├── compare/              # environment comparison of two run results
│   ├── storage/
│   │   ├── sqlite.go
│   │   └── migrations/
│   ├── environments/
│   │   └── resolver.go
│   ├── reports/
│   │   ├── json.go
│   │   └── html.go
│   ├── service/
│   │   └── app_service.go
│   └── …                 # also appdata, cli, collectionio, fsutil, perf, redact, scripting, security
├── cmd/
│   └── api-test-kits-cli/
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   │   ├── base/         # reusable building blocks, no stores or API calls
│   │   │   ├── layout/
│   │   │   ├── collection/
│   │   │   ├── request/
│   │   │   ├── response/
│   │   │   ├── run/
│   │   │   ├── performance/
│   │   │   ├── security/
│   │   │   ├── socket/       # socket console, steps, messages, frame list
│   │   │   └── variables/
│   │   ├── composables/      # useAsyncAction
│   │   ├── views/            # one per top-bar tab
│   │   ├── stores/           # Pinia
│   │   ├── services/         # domain logic without Vue, Wails bindings
│   │   ├── utils/            # format, object, http helpers
│   │   └── types/
│   ├── package.json
│   └── vite.config.ts
├── tests/
│   ├── fixtures/
│   └── integration/
├── build/
├── docs/
│   ├── architecture.md
│   ├── security.md
│   └── collection-format.md
└── README.md
```

Adjust the layout to the selected Wails version's generated project structure. Avoid creating packages before they have a clear responsibility.

---

## 8. Initial Data Model

Use SQLite migrations from the beginning.

### `collections`

- `id`: text UUID primary key
- `name`: text, required
- `description`: text, optional
- `created_at`: timestamp
- `rules_json`: text — collection rules across requests (see [collection-rules.md](collection-rules.md))
- `updated_at`: timestamp

### `requests`

- `id`: text UUID primary key
- `collection_id`: foreign key
- `parent_id`: optional folder/request grouping identifier
- `name`: text, required
- `method`: text, required
- `url_template`: text, required
- `headers_json`: text
- `query_json`: text
- `body`: text
- `body_type`: text
- `auth_json`: text with secret values excluded or encrypted as designed
- `timeout_ms`: integer
- `kind`: text — `http`, `websocket` or `socketio` (default `http`)
- `socket_json`: text, optional — socket settings: subprotocols, Socket.IO path and auth payload, saved messages,
  test steps
- `created_at`: timestamp
- `updated_at`: timestamp

### `environments`

- `id`: text UUID primary key
- `name`: text, required
- `variables_json`: text, protected according to secret-handling rules
- `created_at`: timestamp
- `updated_at`: timestamp

### `assertions`

- `id`: text UUID primary key
- `request_id`: foreign key
- `kind`: text, required
- `path`: text, optional
- `operator`: text, optional
- `expected_json`: text, optional
- `enabled`: boolean
- `created_at`: timestamp

### `baselines`

- `id`: text UUID primary key
- `request_id`: foreign key
- `environment_id`: optional foreign key
- `response_snapshot`: text or JSON
- `ignored_paths_json`: text
- `approved_by`: text — the name entered when the baseline was approved (empty for older baselines)
- `created_at`: timestamp
- `updated_at`: timestamp

### `runs`

- `id`: text UUID primary key
- `collection_id`: optional foreign key
- `environment_name`: text
- `kind`: text — `run` (sends and collection runs), `race`, `matrix`, or `burst` (race tests and matrices recorded before
  they had their own kinds); only `run` counts towards response-time trends
- `status`: text
- `started_at`: timestamp
- `finished_at`: optional timestamp
- `passed_count`: integer
- `failed_count`: integer
- `error_count`: integer

### `run_results`

- `id`: text UUID primary key
- `run_id`: foreign key
- `request_id`: optional foreign key
- `request_summary_json`: text with secrets removed
- `response_summary_json`: text with secrets removed
- `assertion_results_json`: text
- `socket_json`: text, optional — message transcript of a socket test, within the history body limit
- `duration_ms`: integer
- `status`: text
- `created_at`: timestamp

### `race_tests`

- `request_id`: primary key, foreign key to `requests` (deleted with the request)
- `config_json`: text — count, success rule, unique path, server-error setting, optional state check
- `updated_at`: timestamp

### `permission_matrices`

- `collection_id`: primary key, foreign key to `collections` (deleted with the collection)
- `config_json`: text — roles with their variable overrides, ticked request IDs, expected access per cell
- `updated_at`: timestamp

### `collection_contracts`

- `collection_id`: primary key, foreign key to `collections` (deleted with the collection)
- `spec_json`: text — the OpenAPI or Swagger document as compact JSON
- `source_name`: text — the file it was loaded from
- `enabled`, `allow_undocumented`: integers (booleans)
- `updated_at`: timestamp

Race tests and permission matrices are exported, imported and duplicated with their collection (`raceTests`,
`permissionMatrix` in [collection-format.md](collection-format.md)); request references are remapped.

### `environment_comparisons`

- `collection_id`: primary key, foreign key to `collections` (deleted with the collection)
- `config_json`: text — environments A and B, ignored paths, include requests that may modify data, detect changing
  fields
- `updated_at`: timestamp

Use indexes for foreign keys and common history queries. Define retention and response-body limits so test history cannot silently consume unlimited disk space.

---

## 9. Security and Reliability Requirements

API clients can access sensitive systems, so security must be part of the MVP.

### Secret handling

- Never log passwords, API keys, cookies, or authorization headers.
- Mask secrets in the UI by default.
- Redact secret values from reports and error messages.
- Warn before exporting collections containing secrets.
- Prefer OS-backed secure storage for secrets where supported.
- If encrypted storage is unavailable, explain the limitation and offer safer alternatives rather than claiming plaintext storage is secure.
- Do not embed service credentials in the desktop binary.

### Network safety

- Set request timeouts and response-size limits.
- Avoid unbounded concurrent requests.
- Do not disable TLS certificate verification by default.
- Treat imported collections and environment files as untrusted input.
- Warn before requests target production or perform destructive methods.
- Avoid automatic retries for non-idempotent methods unless explicitly configured.
- Do not automatically follow redirects across origins while forwarding sensitive credentials.
- Protect against accidental requests to local metadata endpoints and other sensitive network destinations when executing untrusted collections.
- Make proxy and certificate behavior explicit.

### Data integrity

- Use database transactions for multi-step writes.
- Version the collection export format.
- Use migrations for schema changes.
- Make baseline updates explicit and reviewable.
- Handle malformed JSON and missing paths without crashing.
- Use bounded history retention and allow users to delete local data.

### Quality gates

- Unit tests for assertion operators and JSON diff logic
- Tests for secret redaction
- Tests for timeout, cancellation, oversized responses, redirects, and TLS failures
- Integration tests using a local mock HTTP server
- Race detector and `govulncheck` in CI where supported
- Build and smoke tests on Windows and Linux
- Verify that no secrets appear in application logs or exported reports

---

## 10. UI Screens

### Main layout

- Left sidebar: collections, folders, and saved requests
- Top bar: environment selector, run button, save button
- Main panel: request editor with Params, Headers, Body, Auth, and Tests tabs
- Bottom or side panel: response status, duration, headers, body, and assertion results
- Separate run-history view with filters
- Regression diff view with changed JSON paths highlighted

### MVP screens

1. Welcome / empty state
2. Collection explorer
3. Request editor
4. Environment editor
5. Assertion editor
6. Response viewer
7. Test-run summary
8. Baseline diff viewer
9. Run history
10. Import/export dialogs
11. Settings and data-management screen
12. Race test tab on the request editor (settings, verdict, status-code counts, every response)
13. Permissions tab in the collection dialog (roles, request × role grid, mismatch list)
14. Socket request editor (type selector, Messages, test steps, socket options) with a live console and test result
15. Compare tab in the collection dialog (environment pickers, options, summary, per-request differences)
16. Changing-field suggestions in the approve-baseline dialog and the Baseline tab
17. Rules tab in the collection dialog (rule builder, check against the last run)
18. Sign-off report dialog (release details, preview, save HTML, print to PDF)

Keep the first version focused. Do not build a complex dashboard before the request and test workflows are reliable.

---

## 11. Development Roadmap

Assumption: one developer working approximately 5–10 hours per week. Estimates are approximate and should be revised after the prototype.

### Phase 0 — Validate the problem (3–5 days)

- Interview 5–10 backend developers or QA testers.
- Ask how they currently test APIs after code, config, or database changes.
- Collect examples of real regression failures.
- Show a simple mockup and test whether users want local-first regression comparisons.
- Define the initial supported use cases.

**Exit criterion:** At least three testers agree to try a working prototype on real, authorized test APIs.

### Phase 1 — Foundation (1 week)

- Create the Wails project.
- Add Vue, TypeScript, Pinia, and the initial layout.
- Define Go domain models.
- Set up SQLite migrations.
- Add CI jobs for Go tests and frontend checks.
- Add a local mock HTTP server for integration testing.

**Exit criterion:** The app launches on Windows and Linux, and automated tests run in CI.

### Phase 2 — HTTP request runner (1–2 weeks)

- Implement request methods, headers, query parameters, body, auth, and timeout.
- Display response status, headers, body, and duration.
- Add cancellation and response-size limits.
- Implement environment variable interpolation.
- Add request and collection persistence.

**Exit criterion:** Users can save, close, reopen, and run a collection successfully.

### Phase 3 — Assertions and regression engine (1–2 weeks)

- Implement status and JSON assertions.
- Implement response time and header assertions.
- Implement baseline creation and explicit approval.
- Implement JSON path diffs and ignored paths.
- Add detailed pass/fail messages.

**Exit criterion:** A deliberate response change causes the expected test failure and a useful diff.

### Phase 4 — History, reports, and security (1–2 weeks)

- Store run history.
- Export self-contained HTML and JSON reports.
- Add secret redaction.
- Add import/export validation.
- Test timeouts, redirects, malformed input, and oversized responses.

**Exit criterion:** A test run can be reproduced and shared without leaking credentials.

### Phase 5 — Private beta (1–2 weeks)

- Package Windows and Linux builds.
- Write installation and quick-start documentation.
- Recruit five testers.
- Collect issues and observe actual use.
- Fix crashes and usability problems before adding more features.

**Exit criterion:** Testers can complete the core workflow without your direct help and want to keep using it.

### Phase 6 — Public release

- Publish release binaries and checksums.
- Add screenshots, demo collections, and a short walkthrough.
- Publish source code if using an open-source model.
- Add issue templates and a public roadmap.
- Track opt-in, privacy-respecting product metrics or rely on direct user feedback.
- Consider a CLI after desktop usage is validated (a first version exists: [cli.md](cli.md)).

---

## 12. Monetization Strategy

The desktop MVP should remain free. Avoid paywalls around basic API requests, assertions, and local regression testing.

Potential future revenue:

### Free

- Unlimited local collections, subject to reasonable resource limits
- HTTP request runner
- Assertions and JSON comparisons
- Local run history
- HTML and JSON reports
- Windows and Linux builds

### Optional paid features

- Hosted run history and scheduled cloud runs
- Team-shared collections
- Centralized dashboards
- Long-term report retention
- Organization access controls
- Managed CI reporting and integrations
- Paid business support

Possible experiments after validation:

- Small team plan: test approximately $5–$10 per user/month.
- Hosted project plan: test approximately $10–$25 per project/month.
- Business support: quote based on support scope.

These are pricing hypotheses, not validated market prices. Interview users and test willingness to pay before implementing billing.

---

## 13. Distribution and Marketing

### Windows

- Publish downloadable installers and portable builds where practical.
- Consider Microsoft Store distribution after packaging and policy checks.
- Sign releases when feasible and provide checksums.
- Document upgrade and uninstall behavior.

### Linux

- Start with a tested AppImage or native package where practical.
- Consider Flatpak/Flathub after the app's permissions and packaging are ready.
- Document supported distributions and dependencies.
- Test on a mainstream Ubuntu release and at least one additional distribution.

### Reach the first users

- Publish technical walkthroughs showing how to catch a real API regression.
- Share with Go, backend development, and QA communities without spamming.
- Offer ready-to-run demo collections.
- Write examples for wallet balances, report totals, pagination, and role-based access.
- Ask early testers which missing feature would make them use the app weekly.

Measure repeat usage and successful test runs rather than downloads alone.

---

## 14. Success Metrics

For the first 30–60 days after beta:

- 5–10 active testers
- At least 3 testers using it repeatedly
- Successful installation on both Windows and Linux
- No unresolved critical data-loss or secret-leak bugs
- Clear evidence of which workflows users repeat
- At least 3 user interviews about paid team or hosted features before implementing monetization

Do not treat downloads or GitHub stars alone as product-market fit.

---

## 15. Definition of Done for the MVP

The MVP is ready for public release when:

- [ ] Users can create and persist collections and requests.
- [ ] Users can define environments and use variables safely.
- [ ] Requests can run with timeouts and cancellation.
- [ ] Assertions cover HTTP status, headers, JSON values, and response time.
- [ ] Users can save, compare, and explicitly update baselines.
- [ ] Regression diffs identify added, removed, and changed fields.
- [ ] Run history persists locally and can be exported.
- [ ] Reports redact secrets.
- [ ] Importing malformed or untrusted data does not crash the app.
- [ ] Unit and integration tests pass.
- [ ] Windows and Linux packages have been smoke-tested.
- [ ] Installation, quick-start, security, and data backup documentation exists.

---

## 16. Recommended First Implementation Task

Start with a narrow vertical slice rather than generating the entire application at once:

1. Create the Wails + Vue + TypeScript project.
2. Add one screen with method, URL, headers, and body inputs.
3. Implement a Go method that sends an HTTP request with a timeout.
4. Display status, headers, response body, and duration.
5. Add tests using a local mock HTTP server.
6. Save and reload one request from SQLite.
7. Build and verify on Windows and Linux.

Once this works, add assertions, then baseline comparison, then reports. This sequence reduces the risk of building a large UI before the core engine is proven.

---