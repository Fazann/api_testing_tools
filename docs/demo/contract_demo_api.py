"""Demo API for docs/openapi-contract.md.

Serves a small pet store on http://localhost:8093 and its own OpenAPI spec at
/openapi.json. Three endpoints drift from the spec on purpose:
  GET    /pets/{id}  price is a string, and an undocumented "nickname" field
  POST   /pets       status "new" is not in the documented enum
  DELETE /pets/{id}  answers 500, which the spec does not document
Run: python3 docs/demo/contract_demo_api.py
"""
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = 8093

PET = {"$ref": "#/components/schemas/Pet"}
SPEC = {
    "openapi": "3.0.3",
    "info": {"title": "Pet store demo", "version": "1.0"},
    "servers": [{"url": f"http://localhost:{PORT}"}],
    "paths": {
        "/pets": {
            "get": {
                "summary": "List pets",
                "responses": {"200": {"description": "All pets", "content": {
                    "application/json": {"schema": {"type": "array", "items": PET}}}}},
            },
            "post": {
                "summary": "Create a pet",
                "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/NewPet"}}}},
                "responses": {"201": {"description": "Created", "content": {"application/json": {"schema": PET}}}},
            },
        },
        "/pets/{petId}": {
            "parameters": [{"name": "petId", "in": "path", "required": True, "schema": {"type": "integer"}, "example": 1}],
            "get": {
                "summary": "Get a pet",
                "responses": {
                    "200": {"description": "The pet", "content": {"application/json": {"schema": PET}}},
                    "404": {"description": "Not found", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}},
                },
            },
            "delete": {
                "summary": "Delete a pet",
                "responses": {"204": {"description": "Deleted"}, "404": {"description": "Not found"}},
            },
        },
    },
    "components": {"schemas": {
        "NewPet": {"type": "object", "required": ["name", "price"], "properties": {
            "name": {"type": "string", "minLength": 1},
            "price": {"type": "number", "minimum": 0},
            "status": {"type": "string", "enum": ["available", "sold"]},
        }},
        "Pet": {"allOf": [
            {"$ref": "#/components/schemas/NewPet"},
            {"type": "object", "required": ["id"], "properties": {"id": {"type": "integer"}}},
        ]},
        "Error": {"type": "object", "required": ["error"], "properties": {"error": {"type": "string"}}},
    }},
}

PETS = [{"id": 1, "name": "Rex", "price": 19.9, "status": "available"},
        {"id": 2, "name": "Tom", "price": 12, "status": "sold"}]


class Handler(BaseHTTPRequestHandler):
    def send(self, code, body=None):
        data = b"" if body is None else json.dumps(body, indent=2).encode()
        self.send_response(code)
        if body is not None:
            self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        path = self.path.split("?")[0].rstrip("/")
        if path == "/openapi.json":
            self.send(200, SPEC)
        elif path == "/pets":
            self.send(200, PETS)
        elif path.startswith("/pets/"):
            # Drift: price as a string, plus a field the spec does not document.
            self.send(200, {"id": 1, "name": "Rex", "price": "19.90", "status": "available", "nickname": "Rexy"})
        else:
            self.send(404, {"error": "not found"})

    def do_POST(self):
        if self.path.split("?")[0].rstrip("/") == "/pets":
            length = int(self.headers.get("Content-Length") or 0)
            try:
                body = json.loads(self.rfile.read(length) or b"{}")
            except ValueError:
                body = {}
            # Drift: new pets get a status the spec does not list.
            self.send(201, {"id": 3, "name": body.get("name") or "New", "price": body.get("price", 0), "status": "new"})
        else:
            self.send(404, {"error": "not found"})

    def do_DELETE(self):
        # Drift: the spec documents 204 and 404 only.
        self.send(500, {"error": "database is read-only"})

    def log_message(self, fmt, *args):
        pass


if __name__ == "__main__":
    print(f"Pet store demo on http://localhost:{PORT} (spec: http://localhost:{PORT}/openapi.json)")
    ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
