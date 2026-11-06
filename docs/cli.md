# Running tests from the command line and CI

`api-test-kits-cli` runs a collection without the desktop app: every request, its tests, and the comparison with
approved baselines. It exits non-zero when something fails, so a pipeline can block a deploy. It writes JUnit XML for
CI dashboards and the same HTML and JSON reports as the app.

It is a single static binary with no GUI libraries and no CGO, so it runs on minimal CI images (Alpine, distroless,
Windows and macOS runners).

```sh
APIGUARD_VAR_token="$API_TOKEN" \
  api-test-kits-cli run wallet.collection.json --env Staging --junit junit.xml --report report.html
```

```
API Regression Guard CLI 1.2.0
Collection "Wallet demo" from wallet.collection.json · environment "Staging" · 6 request(s) · 1 baseline(s)

✔ PASS  GET     Health check                                   200      12 ms
✔ PASS  GET     Profile hides sensitive fields                 200      18 ms
✔ PASS  GET     Unauthenticated request is rejected            401       9 ms
✔ PASS  GET     Daily report totals                            200      25 ms
✘ FAIL  GET     Get balance                                    200      21 ms
        baseline: 1 structural difference(s) from the baseline
          ~ data.balance type changed: number → string
✔ PASS  POST    Rejected transaction does not change balance   422      30 ms

FAILED — 5 passed, 1 failed, 0 errors in 140ms
Reports: junit.xml, report.html
```

## Install

- **From a release:** download `api-test-kits-cli-<version>-<os>-<arch>` (`.tar.gz`, or `.zip` for Windows) and
  check it against `SHA256SUMS-cli-<version>.txt`.
- **From source** (Go 1.26+): `go build -o api-test-kits-cli ./cmd/api-test-kits-cli`, or `make build-cli` →
  `build/bin/api-test-kits-cli`. It does not need Node.js or Wails.

`make release-cli` (or `scripts/build-cli.sh`) cross-compiles and packages it for Linux, Windows and macOS on amd64
and arm64 from any host.

## From the app to CI in three steps

1. **Approve baselines in the app** for the requests whose responses must not change (see
   [schema-regression.md](schema-regression.md)), against the environment CI will use, for example `Staging`.
2. **Export the collection:** collection **⋯ → Export…**. Tick the environment, and tick **Include baselines**. Leave
   **Include secret values** off: secrets are left blank in the file, and CI supplies them. Commit the file.
3. **Run it in the pipeline** with the secrets from the CI secret store (see the examples below).

When you approve a new baseline or change a test, export again and commit the new file. The diff in the pull request
shows what changed.

## Variables and secrets

Exports never contain secret values unless you chose to include them. Supply them when running:

| How | Example | Use for |
| --- | --- | --- |
| `APIGUARD_VAR_<name>` environment variable | `APIGUARD_VAR_token=$API_TOKEN` | **Secrets.** Marked secret, so they are redacted from the console output and reports, and they do not appear in the process list. |
| `--var KEY=VALUE` (repeatable) | `--var base_url=https://staging.example.com` | Non-secret values: URLs, IDs, feature flags. |

Both override the selected environment's value of the same name, and `--var` wins over `APIGUARD_VAR_*`. Without an
environment they become collection variables. A variable that was already secret in the app stays secret even when
set with `--var`.

If a secret variable is still empty when the run starts, the CLI warns, for example
`secret variable "token" is empty …; set it with APIGUARD_VAR_token=…`. If the collection lists it as a **required
variable**, the run does not start (exit code 3).

## Choosing what to run

| Flag | Meaning |
| --- | --- |
| `FILE` | A collection export (`*.collection.json`), or an OpenAPI 3 / Swagger 2 document, which runs as imported: requests without tests or baselines. |
| `-e`, `--env NAME` | The environment to run against. Matching ignores case. It is required when the file has several environments. When the file has exactly one, it is used automatically. A name that is not in the file gives an empty environment of that name, to fill with `--var` / `APIGUARD_VAR_*`; baselines approved for that name still apply. |
| `--folder PATH` | Run only one folder and its subfolders: `Wallet`, or `Wallet/Admin` when the name is not unique. |
| `--collection NAME` | Instead of a file, use a collection in the desktop app's local database (see below). |

Run `api-test-kits-cli list FILE` to see the environments (with empty secrets marked), folders, requests and the
baselines in a file.

## Reports

| Flag | Output |
| --- | --- |
| `--junit FILE` | JUnit XML: one test case per request, with the failed assertions and baseline differences in the failure text. Read by GitHub Actions reporters, GitLab, Jenkins and Azure DevOps. |
| `--report FILE` | The format follows the extension: `.html` (self-contained, like the app's), `.json`, or `.xml` (JUnit). Repeatable. |

Parent directories are created. Reports go through the same redaction as the app: secret values, `Authorization`,
`Cookie` and similar headers are removed.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Every request passed. |
| `1` | At least one assertion or baseline comparison failed. |
| `2` | At least one request could not be completed: connection refused, timeout, TLS error, script error, missing variable. The `--timeout` limit was reached. |
| `3` | Usage or setup error: bad flag, unreadable or invalid file, unknown folder, several environments without `--env`, missing required variables, or a protected environment without `--confirm-protected`. |
| `130` | Interrupted with Ctrl+C or SIGTERM. |

Failures (`1`) mean the API is wrong. Errors (`2`) usually mean the environment is unreachable. Many pipelines treat
both as a failed job but report them differently.

## Other flags

| Flag | Meaning |
| --- | --- |
| `--no-baselines` | Skip baseline comparisons; only tests run. |
| `--confirm-protected` | Allow `POST`/`PUT`/`PATCH`/`DELETE` requests against an environment marked **protected**. Without it such a run is refused, as the app asks for confirmation. |
| `--concurrency N` | Requests in parallel, 1–8. The default is 1, in collection order, which business flows that depend on earlier requests need. |
| `--timeout DURATION` | Stop the whole run after `30s`, `5m`, …; the remaining requests are skipped and the exit code is 2. Each request also has its own timeout from the collection. |
| `--insecure` | Do not verify TLS certificates. Prefer `--ca-cert`. |
| `--ca-cert FILE` | Also trust the CA certificates in this PEM file, for example an internal CA. |
| `--proxy none\|system\|URL` | `system` (default) uses `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY`. |
| `-q`, `--quiet` | Print only failing requests and the summary. |
| `--verbose` | Also print the response body (first 2,000 characters) of failing requests. |
| `--no-color` | No ANSI colours. Colours are also off when the output is not a terminal or `NO_COLOR` is set. |

The usual safety rules apply as in the app. Cloud metadata and link-local addresses are blocked, response sizes are
limited, and credentials are not forwarded on redirects to another origin.

## Examples

### GitHub Actions

```yaml
name: API regression
on: [pull_request, workflow_dispatch]

jobs:
  api-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - name: Build the CLI
        run: go build -o api-test-kits-cli ./cmd/api-test-kits-cli
      - name: Run API tests against staging
        env:
          APIGUARD_VAR_token: ${{ secrets.STAGING_API_TOKEN }}
        run: ./api-test-kits-cli run tests/api/wallet.collection.json --env Staging --junit results/junit.xml --report results/report.html
      - name: Upload the report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: api-report
          path: results/
```

If the collection lives in another repository, download a release archive instead of building. To show the
failures as annotations on the pull request, feed `results/junit.xml` to a JUnit report action.

### GitLab CI

```yaml
api-tests:
  image: golang:1.26
  script:
    - go build -o api-test-kits-cli ./cmd/api-test-kits-cli
    - APIGUARD_VAR_token="$STAGING_API_TOKEN" ./api-test-kits-cli run tests/api/wallet.collection.json --env Staging --junit junit.xml --report report.html
  artifacts:
    when: always
    reports:
      junit: junit.xml
    paths: [report.html]
```

### Smoke test after a deploy

```sh
api-test-kits-cli run api.collection.json --env Production --folder Smoke --no-baselines --timeout 2m -q
```

Keep production runs read-only. A protected environment refuses data-modifying requests unless you pass
`--confirm-protected`.

### A scheduled check on your own machine

`--collection` runs a collection straight from the desktop app's database, with its secrets, environments and
baselines, so there is nothing to export:

```sh
# crontab: every morning at 07:00, keep a dated HTML report
0 7 * * * api-test-kits-cli run --collection "Wallet demo" --env Staging -q --report "$HOME/api-reports/$(date +\%F).html"
```

The database is found the same way as the app does it, or at `$APIGUARD_DATA_DIR` / `--data-dir DIR`. The app can
stay open meanwhile.

`NAME` is matched against the collection's **current** name, ignoring case. If you rename the collection in the app
(**⋯ → Rename**), update the cron job too. If two collections have the same name, the CLI stops and asks you to rename
one. See [collections.md](collections.md#rename).

## How it works

The CLI loads the collection into an **in-memory database** and runs it through the same engine as the app:
variables, collection auth, pre-request and post-response scripts, assertions and baselines. Consequences:

- Results are identical to running the collection in the app with the same environment and baselines.
- **Nothing is written back.** Variables that scripts set (`pm.environment.set(...)`) last only for the run;
  `--collection` reads the app's database but never changes it, and CLI runs do not appear in the app's History.
  Keep the reports instead.
- Race tests and permission matrices are not run by the CLI yet.
- **No response-time regression check.** The in-memory database has no earlier responses to compare with, so a slowdown
  never fails a CLI run. Use **Response time** tests for a fixed budget in CI. See [performance.md](performance.md#limits).

## Export format

**Include baselines** adds an optional `baselines` list to the collection file; see
[collection-format.md](collection-format.md#baselines). Builds of the app without this feature ignore it.

## Automated tests (for contributors)

```sh
go test ./tests/integration -run CLI -v          # file and --collection modes, baselines, reports, exit codes
go test ./internal/collectionio -run Baselines   # baselines in the export format
go test ./internal/reports -run JUnit            # JUnit XML
```

The code is in `cmd/api-test-kits-cli` (entry point), `internal/cli` (flags, output, exit codes) and
`internal/reports/junit.go`.
