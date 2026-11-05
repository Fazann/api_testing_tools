# Security tests

Two checks for business-security bugs that request-by-request tests miss. Both run against your saved requests and
environments, save every run in History, and never save variable changes made by scripts.

| | [Race test](race-test.md) | [Permission matrix](permission-matrix.md) |
| --- | --- | --- |
| Where | A request's **Race test** tab | A collection's **⋯ → Permission matrix…** |
| What it does | Sends 2–50 copies of one request at the same moment | Runs ticked requests once as every role |
| Finds | Double spend and double redeem, lost updates, duplicate IDs, 5xx under concurrency | Broken access control, IDOR, missing authentication, lost access |
| You define | How many successes are allowed, a unique field, a before/after state check | Roles (variable overrides) and ✓ allow / ✕ deny per cell |
| Stored | Per request (`race_tests`) | Per collection (`permission_matrices`) |

Both guides start with a five-minute walkthrough against the demo API in
[`demo/security_demo_api.py`](demo/security_demo_api.py), which has deliberate bugs next to correct versions of
the same endpoints:

```sh
python3 docs/demo/security_demo_api.py   # http://localhost:8090
```

Only test systems, accounts and data you are authorized to use.
