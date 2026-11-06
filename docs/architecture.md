# Architecture

```text
frontend (Vue 3 + TypeScript + Pinia)
        │  window.go.main.App.*  (Wails bindings, app.go)
        ▼
internal/service      application facade — no Wails dependency
   ├── runner         resolve → send → assert → compare   (reusable by a future CLI)
   │    ├── environments   {{variable}} resolution
   │    ├── httpclient     timeouts, size limits, redirects, TLS, destination safety
   │    ├── assertions     JSON paths + assertion evaluator
   │    ├── regression     baseline snapshots + order-independent JSON diff
   │    └── redact         secret removal
   ├── storage        SQLite repository + embedded migrations
   ├── reports        JSON and self-contained HTML reports
   └── collectionio   versioned import/export format
```

## Packages

| Package | Responsibility |
| --- | --- |
| `internal/domain` | Typed models and explicit `Validate()` functions. JSON tags define the wire format for the UI and exports. |
| `internal/environments` | Resolves `{{name}}` and built-ins; reports missing and required variables. |
| `internal/httpclient` | Sends a fully resolved request. Never retries. Classifies errors (timeout, cancelled, TLS, blocked, network). `DialWebSocket` opens WebSocket connections with the same TLS, proxy and destination rules. |
| `internal/socket` | WebSocket and Socket.IO v3/v4 (Engine.IO 4) connections behind one `Conn` interface: a channel of received messages, `Send`, `Close`. Handles the Socket.IO handshake, namespaces, ping/pong and acknowledgements. `sockettest` has local servers for tests. See [sockets.md](sockets.md). |
| `internal/assertions` | Path parsing/selection (`a.b[0]`, `[*]`, `["k"]`) and evaluation of every assertion kind. Numbers are decoded as `json.Number` to avoid precision loss. |
| `internal/regression` | Builds baseline snapshots (whole body or selected paths) and diffs them: added / removed / changed / type-changed, with ignored-path globs (`*`, `**`) and numeric tolerance. Schema-mode baselines (`baselines.mode = 'schema'`, `schema.go`) store an inferred JSON-Schema subset (`type`, `properties`, `required`, `items`) and report only structural changes, with array elements reported once as `[*]`. |
| `internal/perf` | Response-time statistics (median, nearest-rank p95) and the "slower than usual" check: a duration ≥ `max(median × factor, median + minDelta)` of the request's recent responses. Pure functions; see [performance.md](performance.md). |
| `internal/redact` | Masks secret values (including URL-encoded forms) and credential headers. |
| `internal/runner` | Runs one request (`Run`) or many with bounded concurrency (`RunAll`), producing already-redacted results. `PreRequest` (pre-request scripts + `Prepare`) is shared with the socket console; socket requests run their test steps in `socket.go`. |
| `internal/storage` | SQLite via the pure-Go `modernc.org/sqlite` driver (no cgo, simple Windows builds). |
| `internal/reports` | Report model plus JSON/HTML writers. |
| `internal/collectionio` | Export (with secret stripping) and validated import. |
| `internal/service` | Preflight checks, protected-environment confirmation, cancellation by operation ID, history, baselines, settings, socket console sessions (`sockets.go`, events on `socket:event`). |
| `app.go`, `main.go` | Wails bindings, native file dialogs, progress events (`run:progress`), atomic file writes. |

## Run flow

1. The UI calls `Preflight…` and shows warnings (missing variables, blocked destinations, disabled TLS, destructive requests). Warnings marked `requiresConfirmation` need explicit approval.
2. `SendRequest`/`RunCollection` re-checks confirmation on the backend, loads baselines for the selected environment and starts an operation that `Cancel(opId)` can stop.
3. For each request the runner resolves variables, sends it, evaluates assertions on the raw body, redacts everything, and compares the **redacted** body with the baseline (baselines are approved from redacted bodies, so both sides match).
4. The run and its results are saved in one transaction, then history is pruned to the retention limit.

## Data model

See `internal/storage/migrations/0001_init.sql`. It follows the spec, with these additions: a `folders` table (requests reference it through `parent_id`), `collections.required_vars_json`, `environments.protected`, `assertions.options_json` (tolerance, aggregate function, description), `baselines.selected_paths_json`/`tolerance`/`mode` (0005), `runs.environment_id`/`kind`, `run_results.performance_json` and `performance_checks` for response-time trends (0007, see [performance.md](performance.md)), `requests.kind`/`socket_json` and `run_results.socket_json` for WebSocket and Socket.IO requests (0008, see [sockets.md](sockets.md)), and names copied into `runs` so history stays readable after deletions.

## Frontend

```text
frontend/src/
  App.vue, main.ts, style.css   shell, global tokens and shared classes
  views/                        one per top-bar tab: Request, Run, History, Environments, Settings
  components/
    base/                       reusable building blocks, no stores or API calls (see below)
    layout/                     TopBar, AppDialog, AppToasts
    collection/                 sidebar tree, collection settings, endpoints, import/export dialogs
    request/                    request editor and its tabs: auth, tests, scripts, key/value rows
    response/                   response viewer, test results, baseline diff and approval
    run/                        run summary
    performance/                response-time chart, Performance tab, collection overview
    security/                   race test, permission matrix
    socket/                     WebSocket / Socket.IO console, test steps, saved messages, message list
    variables/                  variable input, popover, table, secret input
  composables/                  useAsyncAction (busy flag + error toast around backend calls)
  services/                     domain logic without Vue: api (Wails bindings), model, curl, variables…
  stores/                       Pinia: workspace, environments, runner, ui
  utils/                        generic helpers: format (bytes, durations, times, changes), object, http
  types/                        types shared with the Go backend
```

**Base components** (`components/base/`). Use these instead of hand-written markup, so look, accessibility and
behaviour stay the same everywhere:

| Component | Use for |
| --- | --- |
| `BaseButton` | Every button. `variant` = `default` / `primary` / `danger` / `ghost`, `small`, `type` (default `button`) |
| `BaseInput` | Bare input or textarea (`multiline`) without a label, for toolbars, table cells and inline editors. All attributes go to the control; exposes `focus()` / `select()` |
| `TextField` | Labelled input or textarea (`multiline`). `v-model`; `type="number"` emits numbers. `class` styles the wrapper, `input-class` the input |
| `SelectField` | `<select>` from `options` (`{ value, label, disabled? }`). Without `label` it renders a bare select for toolbars |
| `CheckboxField`, `RadioGroup` | Checkbox with a label (boolean, or an array with `value`); without a label it renders the bare checkbox for table cells, radio buttons from `options` |
| `FormField` | Label + hint around a custom control |
| `DataTable` | Every table. `columns` (`{ key, label, class, headClass }`), `rows`, `row-key`; `#cell-<key>` slots, `clickable` + `@row-click`, `empty`, `#after-row` for detail rows |
| `TabBar` | Tabs from `{ id, label, count?, alert?, hidden?, disabled? }` with `v-model` |
| `SelectableList` | A list with one selected item (environments, runs) |
| `StatTile`, `StatusBadge`, `MethodBadge`, `FilterChip`, `CalloutBox`, `EmptyState` | Headline numbers, run status, HTTP method, filter toggles, notes, empty panels |
| `ModalFrame`, `MenuButton`, `CodeEditor` | Dialog frame (`stacked` puts it above other modals), ⋯ menu, script editor |

**Conventions**

- `<script setup lang="ts">`, then `<template>`, then `<style scoped>`. Styles that reach into a child component use
  `:deep()`.
- Imports: `@/…` for anything outside the component's own folder, `./…` for siblings. Prettier sorts them in three
  groups: packages, `@/` and relative (`npm run format`).
- Backend calls go through `useAsyncAction()`: `const { busy, run } = useAsyncAction()` and
  `await run(() => api.save(x))` shows errors as a toast and resolves to `undefined` on failure.
- Display formatting lives in `utils/format.ts` and is not repeated in components.
- Code style: Prettier with semicolons, single quotes, a 120-column width and sorted imports. `npm run lint` also fails
  on any line longer than 120 characters, including template text Prettier cannot wrap.
- Tests sit next to the code (`*.test.ts`); base components are covered in `components/base/BaseComponents.test.ts`.
