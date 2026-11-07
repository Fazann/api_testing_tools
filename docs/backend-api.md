# Backend API

The app will talk to a separate backend server. The Vue frontend never calls that server
directly. It calls Go through the standard Wails bindings, and Go calls the server:

```
Vue (frontend/src/api)  ──Wails bindings──▶  Go App (app.go, app_backend.go)  ──HTTP──▶  backend server
                                              internal/backend client
```

The backend server does not exist yet. Until it does, the app runs in **mock mode**: Go answers
backend calls with mock data, in-process, without network access. Code built on the client doesn't
change when the real server arrives. Setting the URL switches it to **live mode**.

## Modes

| Mode | When | Where calls go |
| --- | --- | --- |
| `mock` | No URL (today), or the URL is malformed | `backend.DefaultMock()` in `internal/backend/mock.go` |
| `live` | A valid URL is set | The server, through the cache, the offline breaker and retries |
| `off` | No URL and no mock (only when built that way) | Nowhere: `ErrNotConfigured` |

`BackendStatus()` (Go, bound to Vue as `api.backendStatus()`) reports `mode`, `reachable`,
`offline`, `latencyMs` and `error`. A malformed URL is logged at startup and shown in `error`. The
app keeps running on the mock.

## Never blocking the app

- **Mock mode** answers immediately. `Mock.WithDelay(d)` adds a delay so you can try loading
  states.
- **Cache first (live).** Successful GET answers are kept in `backend-cache.json` in the data
  directory. An entry younger than 5 minutes is returned without a request.
- **Stale while down.** When the server fails (network error, timeout or 5xx), the last good
  answer, up to 7 days old, is returned instead of an error. Cached responses carry
  `X-Backend-Cache: fresh|stale`.
- **Fail fast while offline.** After the server fails, calls stop going to it for 30 s. They fail
  at once with `backend.ErrOffline`, or are served from the cache. They do not wait on timeouts
  again. After the cool-down, the next call tries the server.
- **Bounded waits.** Each call has a 15 s timeout, including retries. Idempotent requests are
  tried up to 3 times on network errors and 502/503/504.
- **Async.** Wails runs each bound Go method on its own goroutine, and the Vue call is a promise,
  so a slow call never freezes the UI.

What is not cached:
- Writes (POST, PUT, PATCH, DELETE)
- Requests with `Request{NoCache: true}`, such as the health check
- Bodies over 1 MB

Cache entries are kept per user, because the key includes a hash of the `Authorization` header.
Call `Cache.Clear()` on sign-out.

## Go client: `internal/backend`

| File | What it holds |
| --- | --- |
| `endpoints.go` | Every route as `Endpoint{Method, Path}`, e.g. `EndpointHealth`. Paths may use `{name}` placeholders. |
| `client.go` | `Client`, `New` / `NewOrDisabled`, modes, `Do`, the generic `Call[T]` helper, `APIError` |
| `interceptors.go` | `Interceptor` type, `UserAgent`, `BearerAuth` (retries are `Options.Retries`) |
| `mock.go` | `Mock`, `DefaultMock()`: mock answers per endpoint |
| `cache.go` | `Cache`: cache-first and stale-while-down for GET |
| `breaker.go` | Fails fast with `ErrOffline` after the server fails |
| `services.go` | Typed calls, e.g. `Health`, `CheckStatus` |

Request order, outermost first: your interceptors → cache → offline breaker → retry → network.
Headers your interceptors set, such as `Authorization`, are therefore part of the cache key, and
the cache can stand in after the retries and the breaker give up.

### Configuration

- At build time: `make build BACKEND_URL=https://api.example.com`. This sets
  `-X api-test-kits/internal/backend.BaseURL=…`. `scripts/build.sh` and
  `scripts/build-debian.sh` read `BACKEND_URL` from the environment too.
- At run time: `API_TEST_KITS_BACKEND_URL=http://localhost:8080 make dev`. This overrides the
  build value.
- With neither, the app uses mock mode.

The client is created in `app_backend.go` (`newBackendClient`):
- `UserAgent("api-test-kits/<version>")`
- `Retries: 3`
- `DefaultMock()`
- The disk cache

Add `BearerAuth(tokenFunc)` there once the server has sign-in.

### Adding an endpoint

1. Add the route to `endpoints.go`:
   ```go
   EndpointGetTeam = Endpoint{Method: "GET", Path: "/teams/{id}"}
   ```
2. Add its mock to `DefaultMock()` in `mock.go`, so it works before the server exists:
   ```go
   Handle(EndpointGetTeam, func(r *http.Request) (any, error) {
       if r.PathValue("id") == "missing" {
           return nil, &APIError{Status: 404, Code: "not_found", Message: "team not found"}
       }
       return Team{ID: r.PathValue("id"), Name: "Demo team"}, nil
   })
   ```
3. Add a typed function to `services.go`:
   ```go
   func (c *Client) GetTeam(ctx context.Context, id string) (Team, error) {
       return Call[Team](ctx, c, EndpointGetTeam, Request{PathParams: map[string]string{"id": id}})
   }
   ```
4. Expose it to the frontend as an `App` method, e.g. in `app_backend.go`. Then regenerate
   `frontend/wailsjs` with `wails dev`, `wails build` or `wails generate module`.
5. Add it to the frontend, as described below.

### Requests and errors

- `Request` has `PathParams` (escaped into `{name}`), `Query`, `Body` (sent as JSON), `Header` and
  `NoCache`.
- A 2xx answer is decoded into `T`. An empty body (e.g. 204) leaves `T` at its zero value.
- Any other status returns `*APIError{Endpoint, Status, Code, Message}`. `Code` and `Message`
  come from a JSON body like `{"code": "...", "message": "..."}` or `{"error": "..."}`. Mocks
  return the same shape.
- `errors.Is(err, backend.ErrOffline)` means the call failed fast because the server is down.
  `errors.Is(err, backend.ErrNotConfigured)` means the mode is `off`.
- Responses over 32 MB are rejected.
- Interceptors must not log bodies, headers or URLs with query strings, because these can carry
  tokens and user data.

## Frontend: `frontend/src/api`

The frontend follows the standard Wails pattern. Services call the generated functions in
`frontend/wailsjs/go/main/App`, and events use `EventsOn` from `frontend/wailsjs/runtime`. The
generated `wailsjs/` folder is committed, so CI can typecheck and build the frontend without the
Wails CLI. Commit it again whenever Go method signatures change.

| File | What it holds |
| --- | --- |
| `endpoints.ts` | `Endpoint` enum (Go method names) and `BackendEvent` enum (event names) |
| `request.ts` | `request<T>(endpoint, () => App.Method(...))`, `wire()`, `BackendUnavailableError` |
| `interceptors.ts` | `interceptors.request / .response / .error` (`use()` returns a remover) |
| `services/*.ts` | One service per feature area, e.g. `backendService.backendStatus()` |
| `events.ts` | `onRunProgress`, `onSocketEvent`, … |
| `index.ts` | `api` (all services merged) and everything above |

Adding a call for a new Go method:

```ts
// endpoints.ts
GetTeam = 'GetTeam',
// services/teams.ts
getTeam: (id: string) => request<Team>(Endpoint.GetTeam, () => App.GetTeam(id)),
```

The generated parameter types are the model classes in `wailsjs/go/models.ts`. The app's own
types in `@/types` describe the same JSON as plain interfaces, so object arguments go through
`wire(value)`, which changes only the compile-time type. Typecheck still checks each call's
argument count and primitive types against the Go signature.
