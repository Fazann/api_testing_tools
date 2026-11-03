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
| `internal/httpclient` | Sends a fully resolved request. Never retries. Classifies errors (timeout, cancelled, TLS, blocked, network). |
| `internal/assertions` | Path parsing/selection (`a.b[0]`, `[*]`, `["k"]`) and evaluation of every assertion kind. Numbers are decoded as `json.Number` to avoid precision loss. |
| `internal/regression` | Builds baseline snapshots (whole body or selected paths) and diffs them: added / removed / changed / type-changed, with ignored-path globs (`*`, `**`) and numeric tolerance. Schema-mode baselines (`baselines.mode = 'schema'`, `schema.go`) store an inferred JSON-Schema subset (`type`, `properties`, `required`, `items`) and report only structural changes, with array elements reported once as `[*]`. |
| `internal/redact` | Masks secret values (including URL-encoded forms) and credential headers. |
| `internal/runner` | Runs one request (`Run`) or many with bounded concurrency (`RunAll`), producing already-redacted results. |
| `internal/storage` | SQLite via the pure-Go `modernc.org/sqlite` driver (no cgo, simple Windows builds). |
| `internal/reports` | Report model plus JSON/HTML writers. |
| `internal/collectionio` | Export (with secret stripping) and validated import. |
| `internal/service` | Preflight checks, protected-environment confirmation, cancellation by operation ID, history, baselines, settings. |
| `app.go`, `main.go` | Wails bindings, native file dialogs, progress events (`run:progress`), atomic file writes. |

## Run flow

1. The UI calls `Preflight…` and shows warnings (missing variables, blocked destinations, disabled TLS, destructive requests). Warnings marked `requiresConfirmation` need explicit approval.
2. `SendRequest`/`RunCollection` re-checks confirmation on the backend, loads baselines for the selected environment and starts an operation that `Cancel(opId)` can stop.
3. For each request the runner resolves variables, sends it, evaluates assertions on the raw body, redacts everything, and compares the **redacted** body with the baseline (baselines are approved from redacted bodies, so both sides match).
4. The run and its results are saved in one transaction, then history is pruned to the retention limit.

## Data model

See `internal/storage/migrations/0001_init.sql`. It follows the spec, with these additions: a `folders` table (requests reference it through `parent_id`), `collections.required_vars_json`, `environments.protected`, `assertions.options_json` (tolerance, aggregate function, description), `baselines.selected_paths_json`/`tolerance`/`mode` (0005), and names copied into `runs` so history stays readable after deletions.

## Frontend

- `src/services/api.ts` — typed wrappers over the Wails bindings.
- `src/stores` — `workspace` (collections, open draft, latest results), `environments`, `runner` (send / collection runs / confirmation), `ui` (view, dialogs, toasts).
- `src/views` — Requests, Run, History, Environments, Settings.
- Code style: Prettier with semicolons, single quotes, 120-column width and sorted imports (`npm run format`, `npm run lint`).
