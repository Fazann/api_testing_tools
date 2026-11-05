# Collection Export Format (version 1)

Collections are exported as a single UTF-8 JSON file. The format is versioned so that newer app versions can keep reading older files. The implementation is `internal/collectionio/format.go`.

## Top level

| Field             | Type      | Notes                                                          |
| ----------------- | --------- | -------------------------------------------------------------- |
| `format`          | string    | Always `"api-regression-guard.collection"`                     |
| `version`         | integer   | Format version. This app reads version `1`                       |
| `exportedAt`      | RFC 3339  | Informational                                                  |
| `appVersion`      | string    | Informational                                                  |
| `collection`      | object    | `name`, `description`, `requiredVariables` (string array), `variables`, `auth`, `preRequestScript`, `postResponseScript` |
| `folders`         | array     | `id`, `parentId` (`""` = top level), `name`, `sortOrder`       |
| `requests`        | array     | See below                                                      |
| `environments`    | array     | Optional. `name`, `protected`, `variables`                     |
| `secretsIncluded` | boolean   | `true` only if the user chose to export secret values          |
| `baselines`       | array     | Optional. Approved baselines, when **Include baselines** was ticked; see [Baselines](#baselines) |

IDs in a file only link folders and requests to each other. **Every ID is regenerated on import**, so importing can never overwrite existing data.

## Request

```json
{
  "id": "r-balance",
  "parentId": "f-wallet",
  "name": "Get balance",
  "method": "GET",
  "url": "{{base_url}}/wallet/balance",
  "headers": [{ "key": "Accept", "value": "application/json", "enabled": true }],
  "query": [{ "key": "currency", "value": "USD", "enabled": true }],
  "body": "",
  "bodyType": "none",
  "auth": { "type": "bearer", "token": "{{token}}" },
  "timeoutMs": 5000,
  "followRedirects": true,
  "assertions": [],
  "preRequestScript": "",
  "postResponseScript": ""
}
```

- `method`: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`
- `bodyType`: `none`, `json`, `text`
- `auth.type`: `none`; `bearer` (`token`); `basic` (`username`, `password`); `apikey` (`keyName`, `keyValue`, `keyIn`: `header` | `query`); `inherit` — use the collection's `auth`

### Collection auth

`collection.auth` has the same shape as a request's `auth` (except `inherit`). Requests whose `auth.type` is
`inherit` use it, so a token is configured once:

```json
"collection": { "name": "Wallet", "auth": { "type": "bearer", "token": "{{token}}" } },
"requests": [{ "name": "Get balance", "auth": { "type": "inherit" }, "...": "..." }]
```

A request with any other auth type overrides the collection auth. Builds made before collection auth was added
reject files that use `inherit`. **Settings & auth… → Use this auth for all requests** switches every request in the
collection to `inherit`.

### Collection variables

`collection.variables` has the same shape as an environment's `variables` and is shared by every request in the
collection, so values such as the base URL or token are set once:

```json
"collection": {
  "name": "Wallet",
  "variables": [
    { "key": "base_url", "value": "https://staging.wallet.test", "secret": false, "enabled": true },
    { "key": "token", "value": "", "secret": true, "enabled": true }
  ],
  "auth": { "type": "bearer", "token": "{{token}}" }
}
```

An enabled variable of the same name in the selected environment takes precedence, so one collection can target
Local, Staging and Production. Secret collection variables are redacted like environment secrets and their values
are blanked on export unless secrets are explicitly included. Required variables are satisfied by either.

- `{{name}}` references a collection or environment variable in the URL, headers, query, body, auth and assertion expected values. Built-ins: `{{$uuid}}`, `{{$timestamp}}` (Unix seconds), `{{$isoTimestamp}}`.

## Assertion

```json
{ "kind": "json_path", "path": "data.currency", "operator": "eq", "expected": "USD", "tolerance": 0, "enabled": true }
```

| `kind`            | `path`          | Operators                                                                  | `expected`                       |
| ----------------- | --------------- | -------------------------------------------------------------------------- | -------------------------------- |
| `status`          | —               | `eq` `neq` `gt` `gte` `lt` `lte`                                           | number                           |
| `header`          | header name     | `exists` `not_exists` `eq` `neq` `contains` `not_contains` `matches`       | string                           |
| `json_path`       | JSON path       | `exists` `not_exists` `eq` `neq` `gt` `gte` `lt` `lte` `contains` `not_contains` `matches` `type` | any JSON value |
| `response_time`   | —               | `lt` `lte`                                                                 | milliseconds                     |
| `valid_json`      | —               | `""`                                                                       | —                                |
| `required_fields` | —               | `""`                                                                       | array of JSON paths              |
| `body`            | —               | `contains` `not_contains` `matches` `eq`                                   | string                           |
| `aggregate`       | JSON path       | `eq` `neq` `gt` `gte` `lt` `lte`; plus `function`: `count` `sum` `min` `max` | number                         |

- **JSON paths**: `data.items[0].id`, `$.data.total`, `items[-1]` (last), `items[*].amount` (every item), `meta["request-id"]`. With a wildcard, every matched value must satisfy the operator.
- **Comparing two fields**: `"expected": {"$path": "data.total"}` compares against another value in the same response.
- **Types** for `type`: `string`, `number` (includes integers), `integer`, `boolean`, `object`, `array`, `null`.
- **Patterns** for `matches` use RE2 syntax (linear-time, no catastrophic backtracking), max 512 characters.
- `tolerance` is an absolute tolerance for numeric equality (`eq`/`neq`, `aggregate`).

## Scripts

`preRequestScript` and `postResponseScript` hold JavaScript (ES5.1 plus most of ES2015, run by
[goja](https://github.com/dop251/goja)) with a Postman-compatible `pm` API. They exist on the collection and on each
request, at most 100,000 characters each. For every request the order is:

1. collection pre-request script, then the request's pre-request script;
2. `{{variables}}` are resolved and the request is sent;
3. collection post-response script, then the request's post-response script;
4. the request's saved assertions are evaluated.

```js
// Pre-request: sign the body with HMAC-SHA256
const ts = Date.now().toString();
const sig = CryptoJS.HmacSHA256(ts + pm.request.body.raw, pm.environment.get('api_secret')).toString(CryptoJS.enc.Hex);
pm.request.headers.upsert({ key: 'X-Timestamp', value: ts });
pm.request.headers.upsert({ key: 'X-Signature', value: sig });

// Post-response: check the response and keep a token for later requests
pm.test('status is 200', () => pm.response.to.have.status(200));
pm.environment.set('token', pm.response.json().data.token);
```

| API | Notes |
| --- | --- |
| `pm.environment`, `pm.collectionVariables` | `get` `set` `unset` `has` `toObject` `replaceIn`. Changes are seen by later requests in the run and **saved when the run ends** (only the variables a script changed; a variable keeps its secret flag; new ones are not secret) |
| `pm.variables` | Reads local → environment → collection. `set` creates a local value that lasts for this request only (both its scripts and `{{name}}` resolution) |
| `pm.request` | `method`, `url` (assign a string; `url.query` is a list), `headers`, `body.raw` / `body.update()`, `addHeader` `upsertHeader` `removeHeader`. Changes apply only in pre-request scripts; in post-response scripts it is the request that was sent |
| `pm.response` | `code` `status` `responseTime` `responseSize` `headers` `text()` `json()`, and `pm.response.to.have.status/header/body/jsonBody`, `to.be.ok/success/clientError/serverError/json` |
| `pm.test(name, fn)`, `pm.expect(v)` | Each test becomes a result of kind `script`. `pm.expect` covers the common Chai BDD chain: `equal` `eql` `deep` `above` `below` `within` `a` `include` `match` `oneOf` `property` `lengthOf` `keys` `members` `ok` `true` `exist` `empty` `not` … |
| `pm.info` | `eventName` (`prerequest` / `test`), `requestName`, `requestId` |
| `pm.crypto.rsaEncrypt(publicKeyPem, data, opts)` | `padding`: `pkcs1` (default) or `oaep`; `hash` for OAEP: `sha1` `sha256` (default) `sha384` `sha512`; `encoding`: `base64` (default) or `hex` |
| `pm.crypto.rsaSign(privateKeyPem, data, opts)` | `padding`: `pkcs1` (default) or `pss`; `hash` and `encoding` as above. PKCS#1 and PKCS#8 PEM keys |
| `console.log/info/warn/error` | Shown in the response's **Console** tab and stored with the run, with secret values redacted |
| Libraries | Globals `CryptoJS`, `_` (lodash), `moment`, `atob`, `btoa`; or `require('crypto-js' \| 'lodash' \| 'moment' \| 'uuid' \| 'atob' \| 'btoa')`. `crypto.getRandomValues` and `crypto.randomUUID` are available |
| Legacy | `tests["name"] = bool`, `postman.setEnvironmentVariable`, `responseBody`, `responseCode`, `responseTime`, `responseHeaders` |

Not supported: `pm.sendRequest`, `pm.globals`, other `require` modules, and file, network or process access. Each
script runs in a fresh runtime and is stopped after 10 seconds.

A pre-request script that throws stops the request (result `error`). A post-response script that throws marks the
result `error`; tests recorded before the throw are kept. A request may use variables that are missing from the
environment when it or its collection has a pre-request script, since the script may set them.

Scripts are exported as written. Do not put literal secrets in them — reference variables instead
(`pm.environment.get('api_secret')`), because the export secret scan does not look inside scripts.

## Environments and secrets

```json
{ "name": "Staging", "protected": true,
  "variables": [{ "key": "token", "value": "", "secret": true, "enabled": true }] }
```

Unless the user explicitly opts in (after a warning that lists them), exports blank:

- values of variables marked `secret`;
- values of secret collection variables;
- literal (non-`{{variable}}`) `auth.token`, `auth.password` and `auth.keyValue`;
- literal values of credential headers (`Authorization`, `Cookie`, `X-Api-Key`, …).

## Baselines

Added when **Include baselines** is ticked in the export dialog, so a regression check can run on another machine or
in CI with [`test-tool-kit-cli`](cli.md). Files without this field are still version 1; builds that do not know it
ignore it.

```json
"baselines": [
  {
    "requestId": "r-balance",
    "environment": "Staging",
    "mode": "values",
    "snapshot": { "data": { "balance": 150.25, "currency": "USD" } },
    "selectedPaths": [],
    "ignoredPaths": ["meta.request_id", "meta.generated_at"],
    "tolerance": 0.005
  }
]
```

| Field | Notes |
| --- | --- |
| `requestId` | ID of a request in the same file. |
| `environment` | The environment's **name**, or `""` for a baseline approved without an environment. Names are used because IDs differ between machines. |
| `mode` | `values` (default) or `schema`. In schema mode `snapshot` is the inferred structure (see [schema-regression.md](schema-regression.md#stored-format)). |
| `snapshot` | The approved JSON. With `selectedPaths` it maps each path to its approved value. Bodies were redacted when they were approved. |
| `selectedPaths`, `ignoredPaths`, `tolerance` | The comparison rules, as on the request's **Baseline** tab. |

On import a baseline is attached to the environment with that name, either from the same file or one that already
exists. Baselines whose environment exists in neither place are skipped and counted in the import message. The CLI
attaches baselines for `--env NAME` even when the file has no environment of that name. At most 20,000 baselines; a
request may have one per environment.

## Import validation

Imported files are untrusted. The importer rejects files larger than 20 MB, unknown `format` values, unsupported versions, more than 5,000 requests / 1,000 folders / 50 environments, and any request, assertion or environment that fails validation (bad methods, header names, regex patterns, variable names, …). All problems are reported together; nothing is written unless the whole file is valid. Requests pointing at unknown folders are moved to the top level.

## OpenAPI / Swagger import

The importer also accepts **OpenAPI 3.x** and **Swagger 2.0** documents in JSON or YAML (`swagger.json`, `openapi.yaml`, …). They are converted into a collection (`internal/collectionio/openapi.go`) and then validated like any other import:

| OpenAPI | Becomes |
| --- | --- |
| `info.title` + `info.version` | Collection name; a new environment “<title> (from OpenAPI)” |
| `servers[0].url` (server variables use their defaults) / Swagger `schemes`+`host`+`basePath` | `base_url` variable. Relative URLs are prefixed with `http://localhost:8080` |
| Each path + method | One request, named from `summary`, else `operationId`, else `METHOD /path` |
| First tag | Folder |
| Path parameters | The example/default value, or a `{{param}}` variable added to the environment |
| Query and header parameters | Rows filled with example/default/first enum value; enabled only when `required` |
| Request body (`application/json`, Swagger `in: body`) | JSON body from the media example, a named example, or one generated from the schema |
| Security (`http` bearer, `oauth2`, `openIdConnect`, `http` basic, `apiKey` in header or query) | Auth referencing `{{token}}`, `{{username}}`/`{{password}}` or `{{api_key}}` — added to the environment as empty **secret** variables. Document-wide `security` becomes the **collection auth** and operations inherit it; an operation's own `security` overrides it (`security: []` = no auth) |
| First documented `2xx` response | `status = code` (or `200 ≤ status < 300` when no 2xx code is listed) |
| JSON response schema | `valid_json`, plus `required_fields` from the schema's top-level `required` list |

Fill in `base_url` and the credentials in the generated environment, then run. Requests are ordered by path. Local `$ref`s are followed (bounded, cycles are cut); external `$ref`s are ignored and no network requests are made during import.
