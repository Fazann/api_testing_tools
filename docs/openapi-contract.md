# Checking responses against the OpenAPI spec

Importing an OpenAPI or Swagger file creates requests from it. A **contract check** goes one step further: it keeps
the spec with the collection and checks every real response against what the spec documents. You find out when the API
and its documentation drift apart, without writing an assertion per field:

| Problem in a response                        | Reported as                                                                 |
| -------------------------------------------- | --------------------------------------------------------------------------- |
| A field has the wrong type                   | `✘ data.price: expected number, got string ("19.90")`                       |
| A required field is missing                  | `✘ data.currency: required field is missing`                                |
| A field the spec does not document           | `✘ data.internal_score: field is not documented`                            |
| A value outside an enum, a format or a limit | `✘ status: "archived" is not one of "available", "sold"`                    |
| A status code the spec does not list         | `✘ status 500 is not documented for GET /pets/{petId} (documented: 200, 404)` |
| A Content-Type the spec does not list        | `✘ Content-Type text/html is not documented (documented: application/json)`  |
| A required response header is missing        | `✘ required response header X-Rate-Limit is missing`                        |

It works next to [schema regression](schema-regression.md): a schema baseline says "the structure has not changed since
I approved it"; the contract says "the structure is what the spec promises".

Find it in a collection's **⋯** menu → **API contract…**.

## Attaching a spec

- **Importing an OpenAPI 3.x or Swagger 2.0 file** (JSON or YAML) attaches it to the new collection automatically, with
  the check turned on.
- For any other collection: **API contract… → Attach spec…** and choose the file. **Replace spec…** loads a newer version
  of it; **Remove** detaches it. Replacing or removing a spec changes no requests.

The spec is stored as JSON in the app's database. It is never fetched from a URL, and external `$ref`s (to other files
or URLs) are not followed: a schema behind one is treated as "anything goes". At most 10 MB.

## When the check runs

On every **Send** of a request in the collection and on every **Run collection** (and the CLI's `run`), after the
assertions, when the check is turned on and the request **matches an operation** of the spec:

1. **Matching.** The request's method and the path of its final URL (after variables and scripts) are matched against
   the spec's paths. Path parameters (`/pets/{petId}`) match one path segment. The spec's server path (`servers[0].url`
   such as `https://api.example.com/v1`, or Swagger's `basePath`) may precede it, so `/v1/pets/7` matches `/pets/{petId}`.
   When nothing matches that way, an operation whose path *ends* the URL's path is accepted, for gateways that add a
   prefix (`/api/v1/pets/7`). When several paths match, the one with the most fixed segments wins (`/pets/mine` over
   `/pets/{petId}`). The host is not compared, so the same spec works for every environment.
2. **Response.** The documented response is the one for the exact status code, else its range (`4XX`), else `default`.
   With none of those, the status code is reported as undocumented and nothing else is checked.
3. **Headers.** Response headers marked `required: true` must be present (values are not checked).
4. **Content-Type.** When the response documents content types, the response's Content-Type must be one of them
   (parameters such as `charset` are ignored; `application/*` and `*/*` wildcards are honoured).
5. **Body.** When the matched content type is JSON and has a schema, the body is validated against it. A body that is not
   JSON, or is empty, is reported. A body that was **truncated** by the response size limit is not checked, and the
   result says so.

Requests that match no operation are not checked; the **API contract** dialog lists them so you can see what the spec
does not cover. Socket requests are never checked.

## What the schema check supports

The JSON Schema used by OpenAPI 3.0, 3.1 and Swagger 2.0:

- `type` (one type or a list), `nullable` (3.0), `x-nullable` (Swagger 2), `"type": [..., "null"]` (3.1);
  `integer` means a number without a fraction.
- `properties`, `required`, `additionalProperties` (`false`, `true` or a schema), `items`, `minItems`, `maxItems`,
  `uniqueItems`, `minProperties`, `maxProperties`.
- `enum`, `const`, `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum` (3.0's booleans and 3.1's numbers),
  `multipleOf`, `minLength`, `maxLength`, `pattern`.
- `format`: `date-time`, `date`, `email`, `uuid`, `uri`, `ipv4`, `ipv6`. Other formats are not checked.
- `allOf`, `anyOf`, `oneOf` (exactly one must match), `not`, and local `$ref`s (`#/components/schemas/Pet`, also inside
  the spec's paths).
- `readOnly` and `writeOnly` are not enforced; `discriminator` is not needed, because `oneOf` tries every alternative.

A `pattern` that Go's regular expressions cannot compile (look-ahead, back-references) is skipped.

### Undocumented fields

By default, a field the spec does not mention is reported, even though plain JSON Schema allows extra fields; most
"the docs are out of date" problems are new fields. Fields allowed by `additionalProperties` (anything other than
`false`) are documented. Fields defined in any `allOf` part, or in the `oneOf`/`anyOf` alternatives that match, count as
documented.

Turn this off with **Allow undocumented fields** in the dialog if your API adds fields freely.

## Results

The check adds results to the request's **Tests** list, of the kind **Contract**:

- When the response matches: one passing check, `✔ Matches GET /pets/{petId} → 200`.
- Otherwise one failing check per problem, with the JSON path of the field (as in assertions, `$` is the whole body;
  array items by index, `items[3].price`), at most 20 per response, then one more saying how many were left out.

Each check names the operation and status it was checked against (`GET /pets/{petId} → 200`). The CLI prints:

```
✘ FAIL  GET     Get a pet     200      0 ms
        GET /pets/{petId} → 200 price — expected number, got string ("19.90")
        GET /pets/{petId} → 200 nickname — field is not documented
```

A failing contract check fails the request like a failed assertion: it counts in the run summary, is stored in
**History**, appears in HTML, JSON and JUnit reports and the [sign-off report](signoff-report.md), and makes the CLI
exit with code 1. Messages are redacted like assertion results; values are quoted only when they are short (≤ 40
characters).

## The API contract dialog

- The attached spec: its title and version, the file name it came from, when it was attached, the number of operations.
- **Check responses against the spec** (on/off) and **Allow undocumented fields**. Turning the check off keeps the spec.
- **Coverage**, for the selected environment: every request and the operation it matches (or *not in the spec*), and
  every operation that no request matches.
- **Attach spec… / Replace spec… / Remove**.

## Export, duplicate, import

The spec and both settings are part of the collection: they are exported (`contract` in the
[collection format](collection-format.md)), duplicated and imported with it, so the CLI checks the same contract in CI.

## Try it in five minutes

1. Start the demo API: `python3 docs/demo/contract_demo_api.py` (port 8093). It serves its own spec at
   `http://localhost:8093/openapi.json`, and three endpoints that drift from it on purpose.
2. Download the spec (`curl -o pets.json http://localhost:8093/openapi.json`) and **Import** it. The collection
   *Pet store demo 1.0* has four requests, an environment *Pet store demo (from OpenAPI)*, and the contract attached.
3. Select that environment and **Run collection**:
   - **List pets** ✔ *Matches GET /pets → 200*.
   - **Get a pet** ✘ `price: expected number, got string ("19.90")` and ✘ `nickname: field is not documented`.
   - **Create a pet** ✘ `status: "new" is not one of "available", "sold"`.
   - **Delete a pet** ✘ *status 500 is not documented for DELETE /pets/{petId} (documented: 204, 404)*, next to the
     imported status assertion (*expected 204, got 500*).

   The same file runs in CI: `api-test-kits-cli run pets.json` prints these failures and exits with code 1.
4. **⋯ → API contract…**, turn on **Allow undocumented fields**, run again: the `nickname` problem is gone.

## Limits

- One spec per collection, at most 10 MB, at most 5,000 operations.
- Validation stops after 20 problems per response, at a nesting depth of 64 and after 100,000 schema checks.
- External `$ref`s are not followed. Request bodies and parameters are not validated; only responses are.
