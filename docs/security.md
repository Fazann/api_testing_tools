# Security Model

API clients reach sensitive systems, so these protections are part of the MVP. Each item links to where it is enforced.

## Secrets

| Protection | Where |
| --- | --- |
| Environment and collection variables can be marked **secret**; they are masked in the UI with a show/hide toggle. Collection secrets are merged under the environment's before a run, so they get every protection below. | `frontend/src/components/SecretInput.vue` |
| Secret values and resolved auth credentials are redacted from request/response summaries, assertion messages, diffs and errors **before** anything is stored in history. | `internal/runner/runner.go`, `internal/redact` |
| `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, the configured API-key header and any header whose value references a secret variable are always masked. | `internal/redact/redact.go` |
| Reports are redacted a second time with every environment's and collection's current secrets, and credential headers are removed entirely. | `internal/reports/json.go` |
| Collection exports blank secrets unless the user confirms a warning listing each one. | `internal/collectionio/format.go`, `ExportDialog.vue` |
| The app logs only warnings/errors from the desktop framework and never logs request data. | `main.go` |

Secrets shorter than 4 characters are only masked where they form a whole value (a header, query value or variable), because replacing them everywhere would corrupt unrelated text. Use real-length secrets.

### Storage limitation — read this

Secret values are stored **unencrypted** in the local SQLite database (`data.db`). The file and its directory are created with owner-only permissions (`0600`/`0700` on Linux; on Windows the file lives in your per-user `%AppData%` folder). This protects against other local users, **not** against malware running as you, backups that copy the file, or someone with access to your account.

Safer alternatives until OS-backed secret storage (P1) lands:

- keep production credentials out of the app and use short-lived test tokens;
- use dedicated test accounts with minimal permissions;
- use full-disk encryption.

## Network safety

| Protection | Where |
| --- | --- |
| Every request has a timeout (default 30 s, max 10 min) and can be cancelled. | `internal/httpclient/client.go` |
| Response bodies are read up to a limit (default 10 MB) and marked truncated; truncated bodies are never parsed or used as baselines. | `client.go`, `assertions`, `regression` |
| Collection runs default to one request at a time, configurable up to 8. | `internal/runner/concurrency.go` |
| TLS certificates are verified (TLS 1.2+). Disabling verification is an explicit setting, shown as a warning before runs and recorded in reports. Extra CA bundles can be added. | `client.go`, Settings |
| No automatic retries for any method. | `client.go` |
| Redirects are capped at 10 and can be disabled per request. When a redirect changes origin (scheme, host or port), `Authorization`, `Cookie`, `Proxy-Authorization` and API-key/secret headers are dropped. | `client.go` |
| Link-local and cloud metadata destinations (`169.254.0.0/16`, `fe80::/10`, `fd00:ec2::/32`, `metadata.google.internal`, …) are blocked by default — checked on the URL, on every redirect, and after DNS resolution (so DNS rebinding is caught). | `internal/httpclient/safety.go` |
| Proxy behavior is explicit: system variables, none, or a custom URL. | Settings |
| Environments can be marked **protected**; POST/PUT/PATCH/DELETE against them is refused by the backend unless the user confirms. | `internal/service/app_service.go` |

Loopback and private-network addresses are allowed because testing local and internal APIs is the main use case. When a proxy is used, the post-DNS check applies to the proxy connection; the URL and redirect checks still apply.

## Untrusted input

Imported collections are validated in full before anything is written and always receive fresh IDs (see [collection-format.md](collection-format.md)). Malformed JSON in responses, baselines or damaged database rows produces a clear failure message instead of a crash. Response content in HTML reports is escaped by `html/template`, and reports load no external resources.

## Data integrity

- Schema changes use numbered, embedded migrations, each applied in a transaction.
- Multi-step writes (request + assertions, runs + results, imports) use transactions.
- SQLite runs in WAL mode, which survives application crashes.
- Exported files are written to a temporary file and renamed into place.
- Baselines change only through explicit approval.
- History is bounded (default 500 runs, 256 KB of body per result) and can be cleared from Settings or History.

## Reporting a vulnerability

Please report security issues privately to the maintainers rather than in a public issue.
