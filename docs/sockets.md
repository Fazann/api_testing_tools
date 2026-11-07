# WebSocket and Socket.IO requests

A request's **type** selector (next to the URL) switches it between **HTTP**, **WebSocket** and **Socket.IO**. Socket
requests live in collections like HTTP requests and share their URL, params, headers, auth (including inherited
collection auth), timeout, variables and pre-request scripts. They have two extra parts:

- a **console** to connect, send messages and watch what arrives (exploring an API), and
- a **test**: steps that send, expect and wait on one connection. **Run test** (Ctrl+Enter) runs it, records the
  result in history, and collection runs, reports and the CLI run it like any other request.

Socket.IO v3 and v4 servers are supported (Engine.IO protocol 4) over the WebSocket transport. Long-polling and binary
attachments are not supported; a binary event shows up as a message saying so.

## URLs

| Type | URL | Notes |
| --- | --- | --- |
| WebSocket | `wss://api.example.com/chat` or `ws://localhost:8080/ws` | `https://` and `http://` work too and are sent as `wss://` and `ws://`. |
| Socket.IO | `https://api.example.com/admin` | Like the official client: the URL's path is the **namespace** (`/admin`; none means `/`). The server path is set in **Options** and defaults to `/socket.io/`. |

Params, auth and headers are part of the WebSocket handshake: a bearer token becomes `Authorization: Bearer …`, an API
key in the query is added to the URL. Many Socket.IO servers read credentials from the **auth payload** instead
(**Options → Auth payload**, a JSON object such as `{"token": "{{token}}"}`, available on the server as
`socket.handshake.auth`). A refused Socket.IO connection (`connect_error`) shows the server's message.

The network rules are the same as for HTTP: TLS is verified (with the extra CA certificates from Settings), the proxy
setting applies, and link-local and cloud metadata addresses are blocked unless allowed in Settings. Redirects are
not followed during the handshake.

## Messages

A message has a **body**, sent as **JSON** or **Text**:

- **WebSocket:** the body is sent as one text frame. JSON bodies are only checked to be valid JSON.
- **Socket.IO:** a message is an **event** with one argument: a JSON body is sent as that JSON value, a text body as
  a string, an empty body as no argument. **Ask for acknowledgement** requests the server's callback; its reply
  arrives as a message marked *(ack)*.

Received Socket.IO events show the event name and the argument as JSON, or a JSON array when the server sent several
arguments. Binary WebSocket frames are shown base64-encoded.

Bodies, event names, the auth payload and subprotocols support `{{variables}}`. Values of secret variables are
replaced by `[REDACTED]` everywhere a message is shown or stored: the console, test results, history and reports.

**Saved messages** (the **Messages** tab, or **Save message…** in the console) are kept with the request. Pick one in
the console to send it again, or add it to the test as a send step.

## The console

Below the editor, the **Console** tab connects the request (**Connect**). Before connecting, the same checks as Send
run (missing variables, blocked destinations, TLS warnings), and the pre-request scripts run. Variables they set are
saved. In a **protected environment** you confirm first, because messages sent from the console may change data.

The log shows sent (↑), received (↓) and connection (·) messages with the time since connecting. Filter it by
direction or search it; click a message to see it pretty-printed. It keeps the last 1,000 messages per request and
stays while you switch requests; connections stay open until you disconnect, the server closes them or the app quits.
The console is for exploring: its messages are not saved to history. Use a test for repeatable checks.

## Tests

The **Tests** tab of a socket request holds **steps**, run in order on one connection:

| Step | Does |
| --- | --- |
| **Send** | Sends a message (event, body, acknowledgement). Fails if the connection is closed. |
| **Expect** | Waits up to *Within (ms)* for a received message that matches: the **event** (Socket.IO; empty = any), whether it is an **acknowledgement**, and every **check**. Without checks any message matches. |
| **Wait** | Pauses, for servers that need time between messages. |

Checks are the familiar ones applied to the message body: JSON path, body text, valid JSON, required fields and
aggregates (count/sum/min/max). Status, header and response-time checks do not apply. Expected values are JSON and may
use `{{variables}}` inside strings, as in HTTP tests.

Replies may arrive in any order. An expect step looks at every message received since the connection opened that no
earlier expect step has used, so *send A, send B, expect reply B, expect reply A* works. Each received message
satisfies at most one expect step.

The request's **timeout** (Options) bounds connecting and the whole test, including waits. When it runs out, or the
server closes the connection, the remaining steps are reported as not run. A test without steps checks that the
connection can be opened.

The result shows one line per step (with the reason an expect step failed, including why the closest message did not
match), the handshake headers, and every message in the **Messages** tab. A test **fails** when a step fails and is an
**error** when the connection cannot be opened.

## Limits and what is not supported

- Each transcript keeps up to 1,000 messages and 64 KB per message; history also applies the **history body limit**
  from Settings to the transcript as a whole. Larger messages than the response size limit close the connection.
- Post-response scripts, baselines, response-time checks, race tests, the permission matrix and *Copy as code* are for
  HTTP requests; socket requests do not have them. Collection post-response scripts are skipped for socket requests.
- Socket.IO: no long-polling transport, no binary attachments, no Engine.IO 3 (Socket.IO v2) servers. Server events
  that ask the client for an acknowledgement are shown but not acknowledged.
- At most 20 console connections can be open at once.
