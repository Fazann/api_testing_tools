# Testing with schema regression

A **schema baseline** (choose **Compare structure only** when you approve a baseline) records the *shape* of a
response, not its values. Later runs fail only when that shape changes:

| Change in a later response                                   | Result                        |
| ------------------------------------------------------------ | ----------------------------- |
| Different values (balance, IDs, timestamps, list length)     | passes                        |
| A field's JSON type changes (`number` → `string`, `object` → `null`) | fails — **Type changed** |
| A field that was always present is missing                   | fails — **Removed**           |
| A field that was not there before appears                    | fails — **Added**             |
| A field that was missing from some list items is missing again | passes (it is optional)     |

Use it for endpoints whose values change on every call (live balances, search results, reports, lists), where a
value baseline would need a long list of ignored paths. Use a normal value baseline when the exact values matter
(for example a fixed test account's balance), and assertions for business rules (for example "items sum to total").
You can have one baseline per request and environment, so pick the mode that fits each request.

## Try it in five minutes

This walkthrough uses a static JSON file served locally, so you can change the "API" by editing the file.

### 1. Start a fake API

```sh
mkdir -p /tmp/schema-demo && cd /tmp/schema-demo
cat > balance.json <<'EOF'
{
  "data": {
    "balance": 150.25,
    "currency": "USD",
    "promo": null,
    "transactions": [
      { "id": "t1", "amount": 10, "note": "coffee" },
      { "id": "t2", "amount": 5.5 }
    ]
  },
  "meta": { "request_id": "a1", "generated_at": "2026-11-03T10:00:00Z" }
}
EOF
python3 -m http.server 8080
```

### 2. Approve a schema baseline

1. **Requests → Create collection**, then add a request `GET http://localhost:8080/balance.json` and save it
   (`Ctrl+S`). Baselines only work on saved requests.
2. Press **Send**. Check that the response is correct — a baseline is only as good as the response you approve.
3. Choose **Approve as baseline…**, select **Compare structure only (field names and types)** and press
   **Approve baseline**.

The request's **Baseline** tab now says it compares *the structure of the response*. Open **Approved schema** to
see what was recorded:

- `data.balance` is a `number`, `data.currency` a `string`, `meta` an `object`;
- `transactions[*].id` and `amount` are **required** (every item had them), `note` is **optional** (only one did);
- `data.promo` was `null`, so its type is unknown and any value is accepted later.

### 3. Change values — the run should pass

Edit `balance.json`: change the balance, the IDs and the timestamp, add a third transaction, and set
`"promo": "SUMMER"`. Press **Send**. The **Baseline diff** tab shows *response matches the baseline structure*.
An empty list (`"transactions": []`) also passes.

### 4. Break the contract — the run should fail

Replace the file with:

```json
{
  "data": {
    "balance": "150.25",
    "promo": null,
    "transactions": [{ "amount": "10" }, { "id": "t2", "amount": "5.5", "fee": 0.1 }]
  },
  "meta": null
}
```

Press **Send**. The request is marked failed and **Baseline diff** lists
*6 structural difference(s) from the baseline*:

| Change       | Path                          | Baseline | Actual   |
| ------------ | ----------------------------- | -------- | -------- |
| Type changed | `data.balance`                | number   | string   |
| Removed      | `data.currency`               | string   |          |
| Type changed | `data.transactions[*].amount` | number   | string   |
| Removed      | `data.transactions[*].id`     | string   |          |
| Added        | `data.transactions[*].fee`    |          | number   |
| Type changed | `meta`                        | object   | null     |

Problems inside list items are reported once with `[*]`, however many items have them.

### 5. Ignore a part of the response

On the **Baseline** tab, add `meta` to **Ignored paths** and press **Save rules**. On the next **Send**, the `meta`
row is gone. Ignored paths skip the field and everything below it, and accept the same patterns as value
baselines: `meta.request_id`, `items[*].debug`, `**.timestamp`.

### 6. Run it as a regression check

Press **▶** on the collection after each code, config or database change. Requests whose structure changed are
listed as failures in the **Run** view, and the diff is included in HTML and JSON reports.

## When the change is intended

Baselines never update on their own. If the new structure is correct, review the response and choose
**Approve as baseline…** again with **Compare structure only**. Re-approving **replaces** the schema with the shape
of that one response, so approve from a response that is representative.

## Tips for a good schema baseline

- **Approve from a full response.** A field that is `null`, or a list that is empty, when you approve is recorded
  without a type and accepts anything later. Use test data where optional fields and lists are filled in.
- **Lists teach optional fields.** If some items lack a field, it becomes optional; if some items have `null`, it
  becomes nullable (`null | string`). A list with a single item makes all of that item's fields required.
- **Integers and decimals are both `number`.** `10` → `10.5` is not a structural change.
- **No numeric tolerance or selected fields.** Values are never compared, and the whole body is checked; use
  ignored paths to leave parts out.
- **JSON only.** Non-JSON or truncated responses cannot be approved or compared.

## Stored format

The approved schema is a subset of [JSON Schema](https://json-schema.org/) with `type` (always a list),
`properties`, `required` and `items`, so it can be read by other tools:

```json
{
  "type": ["object"],
  "properties": {
    "balance": { "type": ["number"] },
    "promo": { "type": ["null", "string"] },
    "transactions": {
      "type": ["array"],
      "items": {
        "type": ["object"],
        "properties": { "id": { "type": ["string"] }, "note": { "type": ["string"] } },
        "required": ["id"]
      }
    }
  },
  "required": ["balance", "promo", "transactions"]
}
```

It is stored in `baselines.response_snapshot` with `baselines.mode = 'schema'` (see
[architecture.md](architecture.md)). Baselines are local and are not part of collection exports.

## Automated tests (for contributors)

```sh
go test ./internal/regression -run Schema -v                          # inference, merging, diffs, ignored paths
go test ./tests/integration -run TestSchemaBaselineDetectsContractBreak -v   # approve → value change → type change
make test                                                             # full suite
```

The engine is in `internal/regression/schema.go`. The integration test uses the mock wallet API in
`tests/integration/mockserver_test.go`, whose `balanceAsString` switch simulates the contract break.
