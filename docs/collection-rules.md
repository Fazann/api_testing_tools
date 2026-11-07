# Rules across requests

A request's assertions check one response. Business logic usually spans several: money that leaves one account must
arrive in another, an order you just created must show up in the order list, a deleted item must be gone. **Collection
rules** check exactly that. They run after a collection run and compare values taken from *different* requests of the
same run, without writing a script.

| Rule                                                        | How to write it                                                                                       |
| ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| The balance after a transfer is the balance before minus the amount | `Balance after` → `data.balance` **=** `Balance before` → `data.balance` **−** `Transfer` (request body) → `amount` |
| An order created in step 1 appears in the list in step 3    | `Create order` → `id` **appears in** `List orders` → `orders[*].id`                                   |
| A deleted item is gone                                      | `Delete item` (request body) → `id` **does not appear in** `List items` → `items[*].id`               |
| The list grew by one                                        | `List after` → **count** `items` **=** `List before` → **count** `items` **+** constant `1`           |
| The invoice total matches the cart                          | `Get invoice` → `total` **=** `Get cart` → **sum** `items[*].price` (tolerance `0.005`)                |
| Both services agree on the status                           | `Order service` → `status` **=** `Shipping service` → `order_status`                                  |

Find them in a collection's **⋯** menu → **Rules…**.

## Writing a rule

A rule has a **name**, a **kind**, a **left value**, an **operator** and a **right side**.

**A value** is taken from one request of the collection:

| Source            | Path                                    | Example value                       |
| ----------------- | --------------------------------------- | ----------------------------------- |
| **Response body** | a JSON path, as in assertions            | `data.balance`, `orders[*].id`      |
| **Request body**  | a JSON path into the JSON body that was sent | `amount`                       |
| **Status code**   | —                                       | `201`                               |
| **Header**        | a response header name (any case)       | `Location`                          |
| **Constant**      | the value itself: a number, `true`, `null`, `"text"` or plain text | `1`, `paid` |

Response and request bodies can be reduced with **count**, **sum**, **min** or **max**, as in aggregate assertions:
`count` of `items` is the number of items (`items` may be an array or a wildcard path such as `items[*].id`).

**Compare values** (`=`, `≠`, `>`, `≥`, `<`, `≤`): the right side is one value, or several values added and
subtracted (`Balance before − Transfer amount − Fee`). With one value and `=` or `≠`, any JSON values are compared
(strings, numbers, booleans, `null`); otherwise every value must be a number and the right side is calculated. The
**tolerance** is the absolute difference allowed between numbers.

**Value appears in a list** (*appears in*, *does not appear in*): the right side is one value that selects a list,
usually a wildcard path (`orders[*].id`) or an array (`orders`). Numbers and strings are compared by value, so `42`
appears in `["42", "43"]`.

Values are read from the responses and request bodies as stored in the run, so secret values are already redacted;
do not compare secrets.

## When rules run

- After every **Run collection** (and the CLI's `run`), once all requests have finished, regardless of their order or
  of the **Parallel requests** setting.
- A rule runs only when every request it uses was part of the run. Running a folder therefore skips rules that use
  requests outside it; a single **Send** runs no rules.
- Disabled rules never run.

Results are added to the run as one more result named **Collection rules**, with one check per rule. Each check shows
the values it used, for example:

```
✔ Balance after transfer: 90 = 100 − 10
✘ Order appears in list: "ord_7" does not appear in List orders → orders[*].id (2 values: "ord_1", "ord_2")
```

A failed rule fails the run like a failed assertion: it is counted in the summary, stored in **History**, included in
HTML, JSON and JUnit reports, and makes the CLI exit with code 1.

A rule **fails** when its check is false, and also when a value cannot be read: the request had no response, the body
is not JSON, the path is missing, a path matches several values without count/sum/min/max, or a calculation meets a
value that is not a number. The message says which.

## Checking rules while writing them

**Check against last run** evaluates the rules being edited against the most recent run of the collection in
History, without sending anything, and shows the outcome next to each rule. Save the rules to use them in the next run.

## Export, duplicate, import

Rules are part of the collection: they are included when the collection is exported (`rules` in the
[collection format](collection-format.md)), duplicated with it and imported with it. References to requests are kept
when IDs are regenerated on import or duplication. A rule whose request was deleted is shown with a warning and skipped.

## Try it in five minutes

With the race-test demo API running (`python3 docs/demo/security_demo_api.py`, see [race-test.md](race-test.md);
reset it with `curl -X POST http://localhost:8090/reset`):

1. Create a collection with three saved requests, in this order:
   - `Balance before`: `GET http://localhost:8090/wallet`
   - `Withdraw 10`: `POST http://localhost:8090/wallet/withdraw`, JSON body `{"amount": 10}`
   - `Balance after`: `GET http://localhost:8090/wallet`
2. **⋯ → Rules… → Add rule**: name `Withdrawal is deducted`, *Compare values*:
   `Balance after` → Response body → `data.balance` **=** `Balance before` → Response body → `data.balance` **−**
   `Withdraw 10` → Request body → `amount`. **Save rules**.
3. **Run collection**. The results end with **Collection rules**: *✔ Withdrawal is deducted: 90 = 100 − 10*.
4. Change the last value of the right side to the constant `20` and press **Check against last run**: it now shows
   *✘ Withdrawal is deducted: 90 ≠ 100 − 20 = 80*, without sending a request.

## Limits

- At most 50 rules per collection and 10 values on the right side of a rule.
- A value comes from the request's single result in the run. To read the same endpoint twice (before and after),
  save it as two requests.
- Socket requests cannot be used in rules.
