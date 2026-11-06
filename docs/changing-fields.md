# Detecting changing fields

Most responses contain values that are different on every call: request IDs, timestamps, nonces, generated tokens,
counters. A value baseline that includes them fails on the next run, so they have to be listed as **ignored paths**.
**Detect changing fields** finds them for you: it sends the request a few more times, compares the responses with
each other, and suggests every field whose value changed.

| Field in the responses                         | Example over three calls                   | Suggested ignore         | Ticked |
| ---------------------------------------------- | ------------------------------------------ | ------------------------ | ------ |
| A value that differs between calls             | `meta.request_id`: `a1`, `b7`, `c3`        | `meta.request_id`        | yes    |
| The same field in every list item              | `items[0].updated_at`, `items[1].updated_at` | `items[*].updated_at`  | yes    |
| A field present in some responses only         | `data.promo` appears once                  | `data.promo`             | yes    |
| A list whose length changes                    | `events` has 3, 4, then 3 items            | `events`                 | no     |
| A field whose JSON type changes                | `score`: `1`, then `null`                  | `score`                  | yes    |
| A value that stays the same                    | `data.currency`: `USD` each time           | —                        | —      |

A list whose length changes is suggested but not ticked: ignoring it ignores everything inside it. Consider a schema
baseline ([schema-regression.md](schema-regression.md)) for such responses.

## Where to find it

- **Approve as baseline…** dialog: **Detect changing fields** next to *Ignored paths*.
- The request's **Baseline** tab, when a baseline exists: **Detect changing fields** next to its ignored paths.
- **Compare environments** uses the same detection to leave changing fields out of the comparison
  ([environment-comparison.md](environment-comparison.md)).

The button keeps the name-based **Add likely dynamic fields** suggestion (fields named like `*_at`, `request_id`,
`token`), which sends nothing.

## How it works

1. Press **Detect changing fields**. The saved request (or the open draft, with its unsaved edits) is sent **3**
   times in a row to the selected environment, with the same pre-request scripts, auth and variables as **Send**.
2. The responses are compared with each other, never with the baseline:
   - only JSON bodies are compared; the request fails with a message if a response is not JSON, is truncated or the
     request fails;
   - leaf values (strings, numbers, booleans, `null`) are compared at each path; numbers are equal when their values
     are equal;
   - array indices are generalized to `[*]`, so a field that changes in any item is suggested once for all items;
   - a path that is missing from some responses, or whose type changes, is suggested;
   - an array whose length changes is suggested (unticked) and its items are compared up to the shortest length.
3. The result lists each suggestion with up to three example values (secrets redacted) and the reason. A
   different status code between calls is shown as a warning, because the request may not be repeatable.
4. Tick or untick suggestions and press **Add to ignored paths**. Suggestions that are already ignored (by an existing
   pattern such as `**.timestamp`) are not listed. Nothing is saved until you approve the baseline or **Save rules**.

### Safety rules

- A request that may modify data (`POST`, `PUT`, `PATCH`, `DELETE`) is sent three more times only after you confirm
  it, because each call can create or change data. A protected environment always asks for confirmation for such a
  request, as **Send** does.
- Variables changed by scripts during detection are **not** saved.
- The calls are not saved in history and do not affect response-time trends.
- **Cancel** stops detection; nothing is suggested from an incomplete set of responses.

## Try it

With the comparison demo running (`python3 docs/demo/compare_demo_api.py`), send
`GET http://localhost:8091/health`, choose **Approve as baseline…** and press **Detect changing fields**. It suggests
`uptime_s` and `checked_at`; `status` and `version` stay compared. For `GET http://localhost:8091/orders` it suggests
only `generated_at`: the orders themselves are stable.

## Limits

- Three calls find values that change on every call. A value that changes rarely (a daily date, a cache that expires
  every few minutes) may look stable; add it by hand when a later run reports it.
- Ordering changes in a list show up as changed item fields (`items[*].id`). If order does not matter, ignore the
  list or use a schema baseline.
- At most 200 suggestions are listed.
