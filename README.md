# API Regression Guard

A free, local-first desktop app for testing HTTP APIs, checking business rules, and catching regressions by comparing responses with an approved baseline. No account, no cloud, no telemetry.

- **Requests** — GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS, headers, query, JSON/text bodies, Bearer/Basic/API-key auth (set once per collection and inherited, or overridden per request), timeouts, redirect control, cancellation, response size limit.
- **Environments** — `{{variables}}` everywhere, secret variables (masked, redacted, not exported), protected environments that require confirmation for destructive requests, required variables per collection.
- **Assertions** — status, headers, JSON fields (`eq`, `≠`, `>`, `≥`, `<`, `≤`, contains, regex, type), response time, valid JSON, required fields, body text, and aggregates (`count`/`sum`/`min`/`max`) with tolerance — e.g. “item amounts sum to `total` within 0.005”.
- **Regression comparison** — approve a baseline per request and environment (whole body or selected fields), ignore dynamic fields (`meta.request_id`, `items[*].updated_at`, `**.timestamp`), numeric tolerance, order-independent diff of added / removed / changed / type-changed fields, and comparison with the previous run.
- **History & reports** — SQLite history with filters by collection, request, status and date; self-contained HTML and JSON reports with secrets removed.
- **Import/export** — documented, versioned JSON format; also imports **OpenAPI 3 / Swagger 2** (`swagger.json`, `openapi.yaml`) as a ready-to-run collection. Imports are treated as untrusted.

See [docs/architecture.md](docs/architecture.md), [docs/security.md](docs/security.md) and [docs/collection-format.md](docs/collection-format.md).

## Quick start

1. **Environments → ＋** — create `Local` with `base_url` and a `token` variable marked **Secret**.
2. **Requests → Create collection**. In its **⋯ → Settings & auth…**, set Authorization to **Bearer token** with
   `{{token}}`. New requests use **Inherit auth from collection**, so they all send that token.
3. Add a request such as `GET {{base_url}}/health`, and add tests in the **Tests** tab.
4. Pick the environment in the top bar and press **Send** (`Ctrl+Enter`). Save with `Ctrl+S`.
5. When the response is correct, choose **Approve as baseline…** and mark dynamic fields as ignored.
6. After a code, config or database change, press **▶** on the collection. Failures and baseline diffs are listed in the **Run** view; export an HTML or JSON report from there or from **History**.

A demo collection with wallet, report-total and authorization checks is in [`tests/fixtures/wallet-demo.collection.json`](tests/fixtures/wallet-demo.collection.json) (**Import** in the sidebar).

> Only test systems and accounts you are authorized to use. Use dedicated test accounts and controlled data; never run destructive tests against real customer data.

## Building

Requirements: Go 1.26+, Node.js 20+, and the [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation):

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
```

### Linux

```sh
# Debian/Ubuntu 24.04+ (WebKitGTK 4.1)
sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
wails build -tags webkit2_41          # → build/bin/api-regression-guard
```

On distributions that still ship WebKitGTK 4.0 (`libwebkit2gtk-4.0-dev`), omit `-tags webkit2_41`. Fedora: `sudo dnf install gtk3-devel webkit2gtk4.1-devel`.

### Windows

Requires the WebView2 runtime (preinstalled on Windows 10/11).

```powershell
wails build                           # → build\bin\api-regression-guard.exe
wails build -nsis                     # optional installer (requires NSIS)
```

### App icon

The icon source is [`build/appicon.svg`](build/appicon.svg) (also the frontend favicon). After changing it, update the
matching geometry in `scripts/render-icon.py` and run `python3 scripts/render-icon.py` to regenerate
`build/appicon.png` (Linux window icon) and `build/windows/icon.ico` (embedded in the Windows executable).

### Development

```sh
wails dev -tags webkit2_41            # hot-reloading app (omit the tag on Windows)
```

## Testing

```sh
go test -race ./...                   # unit + integration tests (local mock HTTP server)
cd frontend && npm ci && npm run lint && npm test && npm run build
```

`go test` compiles `main.go`, which embeds `frontend/dist`, so build the frontend once first. CI (`.github/workflows/ci.yml`) runs all of this on Linux and Windows, plus `govulncheck` and packaged desktop builds.

## Data and backups

Everything is stored in one SQLite file:

| OS      | Location                                            |
| ------- | --------------------------------------------------- |
| Linux   | `~/.config/api-regression-guard/data.db`            |
| Windows | `%AppData%\api-regression-guard\data.db`            |

Set `APIGUARD_DATA_DIR` to use another directory (e.g. a portable install). To back up, close the app and copy `data.db`, or export collections. **Settings → Local data** shows the path and size, compacts the database, and can delete all local data. Uninstalling the app does not remove this folder; delete it manually to remove your data.

Secrets are stored unencrypted in this file with owner-only permissions — read [docs/security.md](docs/security.md).

## Project layout

```text
main.go, app.go          Wails entry point and bindings
internal/                engine (domain, httpclient, runner, assertions, regression,
                         environments, redact, storage, reports, collectionio, service)
frontend/                Vue 3 + TypeScript + Pinia UI
tests/integration        end-to-end tests against a mock API
tests/fixtures           demo collection
docs/                    spec, architecture, security, collection format
```

## Status

This implements the MVP (P0) from [docs/spece.md](docs/spece.md). OpenAPI import (P1) is also done. Not yet done: other P1 items (CLI, scheduled runs, webhooks, OS-backed encrypted secrets, request chaining) and packaged-app smoke tests on real Windows/Linux machines.
