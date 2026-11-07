"""Demo API for docs/race-test.md, docs/permission-matrix.md and docs/performance.md.

It has deliberate race-condition and access-control bugs next to correct
versions of the same endpoints. Run: python3 docs/demo/security_demo_api.py
Reset its state with: curl -X POST http://localhost:8090/reset
"""
import json, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

lock = threading.Lock()
state = {"balance": 100.0, "racy_used": False, "safe_used": False, "seq": 0}
TOKENS = {"alice-token": "alice", "bob-token": "bob", "admin-token": "admin"}

def next_id():
    with lock:
        state["seq"] += 1
        return state["seq"]

class Handler(BaseHTTPRequestHandler):
    def send(self, code, body):
        data = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def user(self):
        auth = self.headers.get("Authorization", "")
        return TOKENS.get(auth.removeprefix("Bearer "))

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length") or 0))
        if self.path == "/coupons/racy/redeem":      # BUG: check-then-act without a lock
            if state["racy_used"]:
                return self.send(409, {"error": "already redeemed"})
            time.sleep(0.05)
            state["racy_used"] = True
            return self.send(200, {"redeemed": True, "tx": next_id()})
        if self.path == "/coupons/safe/redeem":      # correct: check and act under one lock
            with lock:
                if state["safe_used"]:
                    return self.send(409, {"error": "already redeemed"})
                state["safe_used"] = True
            return self.send(200, {"redeemed": True, "tx": next_id()})
        if self.path == "/wallet/withdraw":          # BUG: lost update (read, wait, write)
            balance = state["balance"]
            time.sleep(0.05)
            state["balance"] = balance - 10
            return self.send(200, {"ok": True})
        if self.path == "/reset":
            state.update(balance=100.0, racy_used=False, safe_used=False)
            return self.send(200, state)
        self.send(404, {"error": "not found"})

    def do_GET(self):
        user = self.user()
        if self.path.startswith("/slow"):            # GET /slow?ms=300 answers after 300 ms (max 10 s)
            q = parse_qs(urlparse(self.path).query)
            raw = (q.get("ms") or ["0"])[0]
            if not raw.isdigit():
                return self.send(400, {"error": "ms must be a whole number of milliseconds"})
            ms = min(int(raw), 10_000)
            time.sleep(ms / 1000)
            return self.send(200, {"data": {"waitedMs": ms}})
        if self.path == "/wallet":
            return self.send(200, {"data": {"balance": state["balance"]}})
        if self.path == "/users/alice/wallet":       # BUG: any signed-in user can read it
            if not user:
                return self.send(401, {"error": "unauthorized"})
            return self.send(200, {"owner": "alice", "balance": 42})
        if self.path == "/users/bob/wallet":         # correct: owner or admin only
            if not user:
                return self.send(401, {"error": "unauthorized"})
            if user not in ("bob", "admin"):
                return self.send(404, {"error": "not found"})
            return self.send(200, {"owner": "bob", "balance": 7})
        if self.path == "/admin/users":              # correct: admin only
            if not user:
                return self.send(401, {"error": "unauthorized"})
            if user != "admin":
                return self.send(403, {"error": "forbidden"})
            return self.send(200, {"users": ["alice", "bob"]})
        self.send(404, {"error": "not found"})

    def log_message(self, *args):
        pass

print("Demo API on http://localhost:8090")
ThreadingHTTPServer(("127.0.0.1", 8090), Handler).serve_forever()
