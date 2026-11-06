"""Demo API for docs/environment-comparison.md and docs/changing-fields.md.

Serves an "old" build on http://localhost:8091 and a "new" build on
http://localhost:8092. The new build has deliberate regressions; both builds
return fields that change on every call (IDs, timestamps, uptime).
Run: python3 docs/demo/compare_demo_api.py
"""
import json, time, uuid
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread

STARTED = time.time()


def now():
    return datetime.now(timezone.utc).isoformat(timespec="microseconds")


def make_handler(new):
    class Handler(BaseHTTPRequestHandler):
        def send(self, code, body, headers=None):
            data = json.dumps(body).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.send_header("X-Request-Id", uuid.uuid4().hex)
            for k, v in (headers or {}).items():
                self.send_header(k, v)
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            path = self.path.split("?")[0]
            if path == "/account":
                # Regression: the new build returns the balance as a string.
                balance = "150.25" if new else 150.25
                self.send(200, {"id": "acc_1", "balance": balance, "currency": "USD",
                                "request_id": uuid.uuid4().hex, "server_time": now()})
            elif path == "/orders":
                # Regression: order 2 lost its status, and caching changed.
                self.send(200, {"orders": [{"id": 1, "status": "paid"},
                                           {"id": 2, "status": "pending" if new else "shipped"}],
                                "generated_at": now()},
                          {"Cache-Control": "no-store" if new else "max-age=60"})
            elif path == "/health":
                self.send(200, {"status": "ok", "version": "1.0",
                                "uptime_s": round(time.time() - STARTED, 6), "checked_at": now()})
            elif path == "/prices":
                if new:
                    time.sleep(0.4)  # Regression: the new build is slow.
                self.send(200, {"items": [{"sku": "A", "price": 10}, {"sku": "B", "price": 25.5}]})
            else:
                self.send(404, {"error": "not found"})

        def do_POST(self):
            if self.path.split("?")[0] == "/orders":
                self.send(201, {"id": uuid.uuid4().hex, "status": "created"})
            else:
                self.send(404, {"error": "not found"})

        def log_message(self, *args):
            pass

    return Handler


if __name__ == "__main__":
    servers = [ThreadingHTTPServer(("127.0.0.1", 8091), make_handler(False)),
               ThreadingHTTPServer(("127.0.0.1", 8092), make_handler(True))]
    for s in servers[1:]:
        Thread(target=s.serve_forever, daemon=True).start()
    print("old build: http://localhost:8091   new build: http://localhost:8092   (Ctrl+C to stop)")
    try:
        servers[0].serve_forever()
    except KeyboardInterrupt:
        pass
