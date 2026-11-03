# API Regression Guard

A free, local-first desktop app for testing HTTP APIs, checking business rules, and catching regressions by comparing responses with an approved baseline. No account, no cloud, no telemetry.

- **Requests** — GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS, headers, query, JSON/text bodies, Bearer/Basic/API-key auth (set once per collection and inherited, or overridden per request), timeouts, redirect control, cancellation, response size limit.
- **Endpoints summary** — a collection's **⋯ → Endpoints summary…** lists every request with its method, URL, folder, test count and last result; filter by method or API version (`/v1/`, `/api/v2.1/` or an `api-version` / `version` query parameter, with collection variables such as `{{base_url}}` filled in), or search, and click a row to open that request.
- **Environments** — `{{variables}}` everywhere, secret variables (masked, redacted, not exported), protected environments that require confirmation for destructive requests, required variables per collection.
- **Assertions** — status, headers, JSON fields (`eq`, `≠`, `>`, `≥`, `<`, `≤`, contains, regex, type), response time, valid JSON, required fields, body text, and aggregates (`count`/`sum`/`min`/`max`) with tolerance — e.g. “item amounts sum to `total` within 0.005”.
- **Regression comparison** — approve a baseline per request and environment (whole body, selected fields, or **structure only**), ignore dynamic fields (`meta.request_id`, `items[*].updated_at`, `**.timestamp`), numeric tolerance, order-independent diff of added / removed / changed / type-changed fields, and comparison with the previous run.
- **Schema regression** — a structure-only baseline records field names, JSON types, optional and nullable fields (merged across array items) and fails a run only when a field appears, disappears or changes type, so endpoints whose values change on every call need no ignore list. See [docs/schema-regression.md](docs/schema-regression.md).
- **History & reports** — SQLite history with filters by collection, request, status and date; self-contained HTML and JSON reports with secrets removed.
- **Import/export** — documented, versioned JSON format; also imports **OpenAPI 3 / Swagger 2** (`swagger.json`, `openapi.yaml`) as a ready-to-run collection. Imports are treated as untrusted.

See [docs/architecture.md](docs/architecture.md), [docs/security.md](docs/security.md), [docs/collection-format.md](docs/collection-format.md) and [docs/schema-regression.md](docs/schema-regression.md).

## Quick start

1. **Requests → Create collection**. In its **⋯ → Settings & auth…**:
   - under **Variables**, add `base_url` (e.g. `http://localhost:8080`) and a `token` marked **Secret**;
   - under **Authorization**, choose **Bearer token** with `{{token}}`.

   Every request in the collection follows these: change the URL or token once and all requests use the new value.
   New requests use **Inherit auth from collection**; tick **Use this auth for all requests** to switch existing ones.
2. Optional: **Environments → ＋** — create `Staging` etc. Its variables override collection variables of the same
   name, so the same collection runs against each environment.
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

### With make

```sh
make help              # list all targets
make deps              # install frontend dependencies, download Go modules
make dev               # run with hot reload
make run               # build for this machine and start the app
make build             # build for this machine → build/bin/test-tool-kit
make test              # frontend + Go tests
make build-linux       # Linux binary for this architecture
make build-debian      # .deb for Debian 12+/Ubuntu 22.04+ (built in a Debian 12 container, needs Docker)
make build-windows     # Windows amd64 + arm64 .exe (cross-compiles from Linux, no C toolchain needed)
make release           # tests, then packaged builds of every platform this host can build → release/
```

`make release` calls [`scripts/build.sh`](scripts/build.sh), which can also be used directly:

```sh
scripts/build.sh --help
scripts/build.sh -v 1.2.0 linux windows      # → release/test-tool-kit-1.2.0-<os>-<arch>.{tar.gz,zip}
scripts/build.sh --nsis windows/amd64        # plus an NSIS installer (needs makensis)
```

Each release also gets a `SHA256SUMS-<version>.txt`. The version is stamped into the binary (default: `git describe`).
Linux builds use CGO with GTK/WebKitGTK, so they only target the host architecture; macOS builds need a macOS host.
On Windows, run the script from Git Bash, or use the `wails build` commands below.

### Linux

```sh
# Debian/Ubuntu 24.04+ (WebKitGTK 4.1)
sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
wails build -tags webkit2_41          # → build/bin/test-tool-kit
```

A binary built with `wails build` or `make build-linux` only runs on distributions with the same or a newer glibc than
the build machine. To build one package for Debian 12+, Ubuntu 22.04+ and their derivatives (Mint, Pop!_OS, ...), use:

```sh
make build-debian                     # → release/test-tool-kit_<version>_<arch>.deb
sudo apt install ./release/test-tool-kit_*.deb
test-tool-kit                         # or open it from the app menu
```

This builds inside a Debian 12 container ([`build/linux/debian.Dockerfile`](build/linux/debian.Dockerfile)), so only
Docker is needed on the host. The package declares its GTK/WebKitGTK dependencies, so `apt` installs them, and it adds
a menu entry and icon.

On distributions that still ship WebKitGTK 4.0 (`libwebkit2gtk-4.0-dev`), omit `-tags webkit2_41`. Fedora: `sudo dnf install gtk3-devel webkit2gtk4.1-devel`.

### Windows

Requires the WebView2 runtime (preinstalled on Windows 10/11).

```powershell
wails build                           # → build\bin\test-tool-kit.exe
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

## Scripts

Requests and collections can have **pre-request** and **post-response** scripts in JavaScript with a
Postman-compatible `pm` API — sign or encrypt requests (`CryptoJS`, `pm.crypto.rsaSign`/`rsaEncrypt`), write
`pm.test` checks, and pass values such as tokens between requests with `pm.environment.set`. `lodash`, `moment`,
`uuid` and `atob`/`btoa` are bundled. Scripts run in an isolated runtime with no file or network access. See
[docs/collection-format.md#scripts](docs/collection-format.md#scripts) for the full API.

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
internal/                engine (domain, httpclient, runner, assertions, regression, scripting,
                         environments, redact, storage, reports, collectionio, service)
frontend/                Vue 3 + TypeScript + Pinia UI
tests/integration        end-to-end tests against a mock API
tests/fixtures           demo collection
docs/                    spec, architecture, security, collection format
```

## Status

This implements the MVP (P0) from [docs/spece.md](docs/spece.md). OpenAPI import and request chaining through scripts (P1) are also done. Not yet done: other P1 items (CLI, scheduled runs, webhooks, OS-backed encrypted secrets) and packaged-app smoke tests on real Windows/Linux machines.
