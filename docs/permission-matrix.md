# Testing access control with the permission matrix

The **permission matrix** runs a collection's endpoints once as every **role** (admin, user A, user B, signed out,
…) and shows a grid of who could call what, next to who *should* be able to. You approve the grid once, and every
later run tells you when access changes.

It finds the most common serious API bugs, which ordinary tests (written as a single, usually all-powerful user)
never see:

| Bug | Example | In the matrix |
| --- | --- | --- |
| **Broken access control** | A normal user can call an admin endpoint | ✕ expected, `200` received |
| **IDOR** (insecure direct object reference) | Bob can read Alice's wallet by changing the ID in the URL | ✕ expected for Bob on Alice's resource, `200` received |
| **Missing authentication** | An endpoint answers without a token | ✕ expected for the anonymous role, `200` received |
| **Lost access (regression)** | A refactor makes a valid user get `403` | ✓ expected, `403` received |
| **Authorization checked too late** | The server validates the body before checking permissions | ✕ expected, `400`/`422` received (*suspicious*) |

Open it from a collection's **⋯ → Permission matrix…**, or the **Permissions** tab of the collection dialog.

> Only test systems and accounts you are authorized to use, with dedicated test accounts for each role.

## Try it in five minutes

The demo API knows three users (`alice-token`, `bob-token`, `admin-token`). `/users/alice/wallet` has an IDOR bug;
`/users/bob/wallet` and `/admin/users` are correct.

### 1. Start the demo API

```sh
python3 docs/demo/security_demo_api.py      # http://localhost:8090, Python 3.9+, no dependencies
```

### 2. Set up the environment and collection

1. **Environments → ＋** `Demo`, with these variables. Mark the tokens **Secret**.

   | Variable | Value | Secret |
   | --- | --- | --- |
   | `base_url` | `http://localhost:8090` | |
   | `token` | `alice-token` | ✔ |
   | `bob_token` | `bob-token` | ✔ |
   | `admin_token` | `admin-token` | ✔ |

   Select `Demo` in the top bar.
2. **Requests → Create collection** `Access demo`. In **⋯ → Settings & auth…** choose **Bearer token** with
   `{{token}}`.
3. Add and save three `GET` requests: `Alice wallet` (`{{base_url}}/users/alice/wallet`), `Bob wallet`
   (`{{base_url}}/users/bob/wallet`) and `List users` (`{{base_url}}/admin/users`).

### 3. Add the roles

Open **⋯ → Permission matrix…**.

1. **＋ Role**, name it `Alice`. Leave its variables empty: Alice uses the environment's `token` as it is.
2. **＋ Role** `Bob`. Open **Variables** and add `token` = `{{bob_token}}`.
3. **＋ Role** `Admin`, with `token` = `{{admin_token}}`.
4. **＋ Anonymous**. It sends requests with no credentials.

### 4. Run first, then decide

Next to **Requests × roles**, choose **Tick: All** and press **Run matrix**. Every cell now shows the status code
it got:

| Request | Alice | Bob | Admin | Anonymous |
| --- | --- | --- | --- | --- |
| Alice wallet | · `200` | · `200` | · `200` | · `401` |
| Bob wallet | · `404` | · `200` | · `200` | · `401` |
| List users | · `403` | · `403` | · `200` | · `401` |

`·` means *no expectation yet*, so nothing fails. One cell is already wrong: **Bob can read Alice's wallet**.

### 5. Set the expectations

Click a cell to cycle **✓ allow → ✕ deny → · none**. Set:

| Request | Alice | Bob | Admin | Anonymous |
| --- | --- | --- | --- | --- |
| Alice wallet | ✓ | **✕** | ✓ | ✕ |
| Bob wallet | ✕ | ✓ | ✓ | ✕ |
| List users | ✕ | ✕ | ✓ | ✕ |

The colours update as you click. *Alice wallet × Bob* turns red. The summary says **✘ 1 access mismatch**, and
below it:

> **Alice wallet** as **Bob**: Bob should be denied but got 200 — broken access control

*Bob wallet × Alice* got `404`, which counts as denied: many APIs answer "not found" for other people's resources
so they don't reveal that the resource exists.

Press **Save**. Expectations, roles and ticked requests are stored with the collection.

### 6. Use it as a regression check

After every change to authentication, roles or middleware, open the matrix and press **Run matrix**. A green grid
means access is unchanged; any red cell is a new hole or a lost permission.

> **Approve results as expected** is a shortcut for step 5: it copies the last run into the expectations
> (2xx/3xx → ✓, 401/403/404 → ✕). Here it would have approved Bob reading Alice's wallet as ✓. Use it only after
> checking every `200` in the grid.

## Roles

A role is a name plus variables that **override** the environment and collection variables while that role runs.
Requests then send whatever identity the variables produce.

A new role has no variables, so it behaves exactly like the selected environment until you add an override. An
override with an empty value really is empty: `token` = *(empty)* sends `Bearer ` with no token, which is a valid way
to test a blank credential.

- **Bearer or API-key auth on the collection** (`{{token}}`, `{{api_key}}`): give each role its own value for that
  variable.
- **Basic auth** (`{{username}}` / `{{password}}`): override both.
- **Logins in a pre-request script** that read `pm.environment.get("username")`: override the inputs, and the
  script logs in as that role. Each role starts from a fresh copy of the variables, so one role's token never leaks
  into the next role's requests.
- **IDs in URLs** can also be role variables, for example `my_wallet_id`. The usual IDOR setup, though, is a fixed
  ID owned by one user (`{{alice_wallet_id}}`) and ✕ for everyone except that user.

Values may reference other variables: `token` = `{{bob_token}}` is resolved against the environment and collection.
A value that references a **secret** variable is treated as secret and redacted from History and reports. Prefer
references over pasting tokens into a role. Role variables marked **Secret** are also redacted, but they are stored
in the local database like collection variables.

**No credentials** (the *Anonymous* role) removes the request's auth and any `Authorization` or `Cookie` header
rows. Other headers, such as an API key in a custom header, are still sent; give the role an empty value for that
variable to remove it as well.

Up to 20 roles; role names must be unique.

## How a cell is judged

| Response | Outcome | Expected ✓ allow | Expected ✕ deny | No expectation |
| --- | --- | --- | --- | --- |
| 2xx, 3xx | allowed | pass | **fail**: broken access control | shown |
| 401, 403 | denied | **fail**: access lost | pass | shown |
| 404 | not found (hidden) | **fail** | pass | shown |
| other 4xx (400, 405, 409, 422) | rejected | **fail** | **suspicious** | shown |
| 5xx | server error | **fail** | **fail** | shown |
| no response | error | error | error | error |

- **Suspicious** (yellow) does not fail the run. It means the request got past the permission check and was
  stopped by validation or business rules, so with a valid body it might have succeeded. Check the endpoint, or
  give the request a valid body so the result is clear-cut.
- A **3xx** counts as allowed, and with **Follow redirects** on (the default) the final response is judged. If
  your API answers signed-out users with a redirect to a login page, that looks like success to the matrix; leave
  those cells without an expectation for now.
- **Errors** (dashed border: timeout, connection refused, script error) fail the run, because the access was not
  tested.

Hover a cell to see its expectation, the last status code and the explanation.

## Requests

Only **ticked** requests run; unticked rows are greyed out. **Tick: Read-only** selects `GET` and `HEAD` requests,
which is the safe default.

Ticked requests that may change data (`POST`, `PUT`, `PATCH`, `DELETE`) run **once per role**, so a `DELETE` in a
matrix of four roles is sent four times. Testing that a normal user *cannot* delete something is valuable, but use
throwaway test data, and expect that the role that is allowed will delete it. A banner warns you when such requests
are ticked, and the confirmation dialog lists them.

## Running

- **Run matrix** saves pending changes, asks for confirmation (with the number of requests, any requests that may
  modify data, and the environment's protection), and runs role by role. Progress is shown as *done / total*;
  **Cancel** stops it.
- Requests within a role run with the **Concurrency** setting from **Settings**.
- Each role uses a fresh copy of the variables, and variables changed by scripts are **not saved** after the run.
- The request's own **Tests** and **Baseline** are not checked. In History each result shows the access check
  instead.
- Clicking a request name opens it in the editor (you are asked first if the matrix has unsaved changes).

Every run is saved in **History** under the collection, with one result per cell named `[Role] Request`, for
example `[Bob] Alice wallet`. A result is failed when its cell failed, and its check shows the expected access, the
actual status and the explanation. HTML and JSON reports include them, with tokens removed.

## Tips

- **One account per role, with known data.** Seed Alice's and Bob's resources once and keep their IDs in
  environment variables.
- **Cover every resource type twice**: once as its owner (✓) and once as another user of the same level (✕). The
  second one is the IDOR test, and it is the one most often missing.
- **Include write endpoints on resources that belong to others** (`PUT /users/alice/profile` as Bob → ✕). These are
  the costliest IDORs. Use test data.
- **Keep the anonymous role.** It catches endpoints that were added without authentication.
- **Expect what the API should do, not what it does.** Use **Approve results as expected** only for cells you have
  checked.

## Stored format

The matrix is stored per collection in the local database (table `permission_matrices`, migration `0006`) as JSON,
and deleted with the collection:

```json
{
  "collectionId": "…",
  "roles": [
    { "id": "…", "name": "Bob", "anonymous": false,
      "variables": [{ "key": "token", "value": "{{bob_token}}", "secret": false, "enabled": true }] },
    { "id": "…", "name": "Anonymous", "anonymous": true, "variables": [] }
  ],
  "requestIds": ["…"],
  "expected": { "<request id>": { "<role id>": "allow" } }
}
```

Expectations are `"allow"` or `"deny"`; cells without an expectation are left out. Expectations for deleted roles
are dropped on save. The matrix is not yet part of collection export or duplication.

## Automated tests (for contributors)

```sh
go test ./internal/security -run 'Classify|Judge' -v           # outcome and verdict rules
go test ./tests/integration -run TestPermissionMatrix -v       # IDOR found, anonymous strips the token, no token in History
cd frontend && npx vitest run src/components/PermissionMatrix.test.ts   # grid, cycling, run, approve
```

The code is in `internal/security/matrix.go` (judging), `internal/service/security.go` (roles and running) and
`frontend/src/components/PermissionMatrix.vue`.
