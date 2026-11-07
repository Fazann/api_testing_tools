# API test kits

A free desktop app for testing HTTP APIs and catching regressions.

- **Local-first** — no account, no cloud, no telemetry.
- **Runs on** Linux, Windows and macOS, plus a headless CLI for CI.

> Only test systems and accounts you are authorized to use. Never run destructive tests against real customer data.

---

## Contents

- [Features](#features)
- [Quick start](#quick-start)
- [Building](#building)
- [Testing](#testing)
- [Releasing](#releasing)
- [Data and backups](#data-and-backups)
- [Project layout](#project-layout)
- [Documentation](#documentation)

---

## Features

### Send requests

| Feature | What it does |
| --- | --- |
| **Requests** | All HTTP methods, headers, query, JSON/text body, timeouts, redirects, cancel. |
| **Auth** | Bearer, Basic or API key. Set it once on the collection, or override per request. |
| **Collections** | Create, rename, duplicate, export and delete from the sidebar **⋯** menu. |
| **Environments** | `{{variables}}` everywhere. Secrets are masked and never exported. |
| **WebSocket / Socket.IO** | Live console, and send / expect / wait test steps. |
| **cURL** | Paste a cURL command to create a request. **Copy as code** exports to cURL, JS, Python, Go, PHP, Java, C#, Dart, Swift, Rust and more. |
| **Import** | Our JSON format, plus **OpenAPI 3 / Swagger 2**. |
| **Scripts** | Pre-request and post-response JavaScript with a Postman-compatible `pm` API. |

### Check results

| Feature | What it does |
| --- | --- |
| **Assertions** | Status, headers, JSON fields, response time, regex, and sums/counts with tolerance. |
| **Baselines** | Approve a good response, then flag any later change. Ignore dynamic fields like `**.timestamp`. |
| **Schema regression** | Compare structure only (field names and types), not values. |
| **Detect changing fields** | Sends the request 3 times and suggests fields to ignore (IDs, timestamps). |
| **Rules across requests** | E.g. *balance after = balance before − amount*. No scripts needed. |
| **Compare environments** | Run against two environments at once and show only what differs. |
| **Performance** | Charts response times (median, p95) and flags requests that got slower. |
| **Race tests** | Send 2–50 copies at once to catch double-spend and lost updates. |
| **Permission matrix** | Run endpoints as every role and spot broken access control / IDOR. |

### Share results

| Feature | What it does |
| --- | --- |
| **History** | Every run is saved, with filters by collection, status and date. |
| **Reports** | HTML and JSON reports, with secrets removed. |
| **Sign-off report** | A *Ready / Not ready* summary for QA hand-off. Save as HTML or print to PDF. |
| **Endpoints summary** | List every request in a collection, filter by method or API version. |
| **CLI** | Run a collection in CI and get JUnit/HTML/JSON reports. See [docs/cli.md](docs/cli.md). |

---

## Quick start

1. **Create a collection.** Open **⋯ → Settings & auth…** and:
   - add a `base_url` variable (e.g. `http://localhost:8080`)
   - add a `token` variable and mark it **Secret**
   - set **Authorization** to **Bearer token** with `{{token}}`
2. **(Optional) Add environments** like `Staging`. Their variables override the collection's.
3. **Add a request**, e.g. `GET {{base_url}}/health`, and add checks in the **Tests** tab.
4. **Send** with `Ctrl+Enter`. Save with `Ctrl+S`.
5. **Approve as baseline** once the response looks right.
6. **Run the collection** with **▶** after each change. Failures and diffs show in the **Run** view.

### Try the demo

**Import** [`docs/demo/jsonplaceholder.collection.json`](docs/demo/jsonplaceholder.collection.json), pick the
**JSONPlaceholder** environment and press **▶**. It runs 10 requests with tests against the free
[JSONPlaceholder](https://jsonplaceholder.typicode.com) fake API. No setup or token needed.

Or from the command line:

```sh
api-test-kits-cli run docs/demo/jsonplaceholder.collection.json --env JSONPlaceholder
```

### Keyboard shortcuts

| Keys | Action |
| --- | --- |
| `Ctrl+Enter` | Send request |
| `Ctrl+S` | Save |
| `Ctrl+W` | Close tab |

---

## Building

### Requirements

- Go 1.26+
- Node.js 20+
- Wails v2 CLI:

  ```sh
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
  ```

### Common commands

```sh
make deps     # install dependencies
make dev      # run with hot reload
make run      # build and start the app
make build    # build → build/bin/api-test-kits
make test     # run all tests
make check    # lint, typecheck, format check and tests
make help     # list every target
```

### Platform builds

```sh
make build-linux     # Linux binary
make build-debian    # .deb for Debian 12+ / Ubuntu 22.04+ (needs Docker)
make build-windows   # Windows amd64 + arm64
make build-darwin    # macOS .app (macOS host only)
make build-cli       # headless CLI → build/bin/api-test-kits-cli
```

<details>
<summary><b>Linux notes</b></summary>

Install system packages first:

```sh
# Debian / Ubuntu 24.04+
sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev

# Fedora
sudo dnf install gtk3-devel webkit2gtk4.1-devel
```

- A plain Linux binary only runs on systems with the same or newer glibc.
  For a package that works on Debian 12+ / Ubuntu 22.04+, use `make build-debian`, then:

  ```sh
  sudo apt install ./release/api-test-kits_*.deb
  ```

- On older systems with WebKitGTK 4.0, drop `-tags webkit2_41` when calling `wails` directly.

</details>

<details>
<summary><b>Windows notes</b></summary>

Needs the WebView2 runtime (already on Windows 10/11).

```powershell
wails build         # → build\bin\api-test-kits.exe
wails build -nsis   # optional installer (needs NSIS)
```

Run `scripts/*.sh` from Git Bash.

</details>

<details>
<summary><b>App icon</b></summary>

Edit [`build/appicon.svg`](build/appicon.svg), update the matching geometry in `scripts/render-icon.py`, then run:

```sh
make icons
```

</details>

---

## Testing

```sh
make test     # frontend + Go tests
make check    # full regression check
make hooks    # run the check before every commit (skip with --no-verify)
```

> `go test` needs `frontend/dist`, so build the frontend once first (`make frontend`).

CI runs everything on Linux and Windows ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)).

---

## Releasing

Installed apps update themselves. Users never need to reinstall.

```sh
make release-patch NOTES="Fix header parsing"   # 0.1.0 → 0.1.1 (bug fixes)
make release-minor NOTES="New feature"          # 0.1.1 → 0.2.0 (new features)
make release-major NOTES="Breaking change"      # 0.2.0 → 1.0.0
git push --follow-tags                          # publishes the GitHub release
```

Each command bumps the version, tags it, builds every package and writes `latest.json`.

### How the app updates

| Installed from | Update method |
| --- | --- |
| `.deb` | Installs the new `.deb` (asks for password) |
| Windows installer | Runs the new installer silently |
| Portable binary / `.zip` | Replaces itself and restarts |
| macOS / other | Opens the download page |

- Apps read `latest.json` from `UPDATE_URL` (default: this repo's latest GitHub release).
- The repo or host **must be public**.
- Build with `UPDATE_URL=` to turn updates off.

---

## Data and backups

All data is in one SQLite file:

| OS | Location |
| --- | --- |
| Linux | `~/.config/api-test-kits/data.db` |
| macOS | `~/Library/Application Support/api-test-kits/data.db` |
| Windows | `%AppData%\api-test-kits\data.db` |

- **Custom folder:** set `APIGUARD_DATA_DIR`.
- **Backup:** close the app and copy `data.db`, or export your collections.
- **Manage:** **Settings → Local data** shows size, compacts, or deletes all data.
- **Uninstall** keeps this folder. Delete it by hand to remove your data.

> Secrets are stored **unencrypted** (owner-only file permissions). See [docs/security.md](docs/security.md).

---

## Project layout

```text
main.go, app.go      Wails entry point and bindings
cmd/                 CLI entry point
internal/            core engine (runner, assertions, regression, storage, reports…)
frontend/            Vue 3 + TypeScript + Pinia UI
scripts/             build, release and check scripts
tests/integration/   end-to-end tests against a mock API
tests/fixtures/      OpenAPI fixtures for tests
docs/demo/           JSONPlaceholder demo collection
docs/                guides and specs
```

---

## Documentation

| Topic | Doc |
| --- | --- |
| Collections | [collections.md](docs/collections.md) |
| Collection file format & scripts | [collection-format.md](docs/collection-format.md) |
| Rules across requests | [collection-rules.md](docs/collection-rules.md) |
| Schema regression | [schema-regression.md](docs/schema-regression.md) |
| Detect changing fields | [changing-fields.md](docs/changing-fields.md) |
| Compare environments | [environment-comparison.md](docs/environment-comparison.md) |
| Performance | [performance.md](docs/performance.md) |
| Race tests | [race-test.md](docs/race-test.md) |
| Permission matrix | [permission-matrix.md](docs/permission-matrix.md) |
| WebSocket / Socket.IO | [sockets.md](docs/sockets.md) |
| Sign-off report | [signoff-report.md](docs/signoff-report.md) |
| Security tests | [security-tests.md](docs/security-tests.md) |
| CLI | [cli.md](docs/cli.md) |
| Security | [security.md](docs/security.md) |
| Architecture | [architecture.md](docs/architecture.md) |
| Spec | [spec.md](docs/spec.md) |
