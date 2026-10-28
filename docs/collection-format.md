# Collection Export Format (version 1)

Collections are exported as a single UTF-8 JSON file. The format is versioned so that newer app versions can keep reading older files. The implementation is `internal/collectionio/format.go`.

## Top level

| Field             | Type      | Notes                                                          |
| ----------------- | --------- | -------------------------------------------------------------- |
| `format`          | string    | Always `"api-regression-guard.collection"`                     |
| `version`         | integer   | Format version. This app reads version `1`                       |
| `exportedAt`      | RFC 3339  | Informational                                                  |
| `appVersion`      | string    | Informational                                                  |
| `collection`      | object    | `name`, `description`, `requiredVariables` (string array), `variables`, `auth` |
| `folders`         | array     | `id`, `parentId` (`""` = top level), `name`, `sortOrder`       |
| `requests`        | array     | See below                                                      |
| `environments`    | array     | Optional. `name`, `protected`, `variables`                     |
| `secretsIncluded` | boolean   | `true` only if the user chose to export secret values          |

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
  "assertions": []
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
