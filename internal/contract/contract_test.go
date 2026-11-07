package contract

import (
	"net/http"
	"strings"
	"testing"

	"api-test-kits/internal/domain"
)

const petSpec = `{
  "openapi": "3.0.3",
  "info": {"title": "Pets", "version": "1.0"},
  "servers": [{"url": "https://api.example.com/v1"}],
  "paths": {
    "/pets": {
      "get": {"summary": "List pets", "responses": {"200": {"description": "ok",
        "headers": {"X-Rate-Limit": {"required": true, "schema": {"type": "integer"}}},
        "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Pet"}}}}}}}
    },
    "/pets/mine": {"get": {"responses": {"200": {"description": "ok"}}}},
    "/pets/{petId}": {
      "get": {"responses": {
        "200": {"description": "ok", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Pet"}}}},
        "4XX": {"description": "error", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}}}},
      "delete": {"responses": {"204": {"description": "gone"}, "404": {"description": "missing"}}}
    }
  },
  "components": {"schemas": {
    "Pet": {"allOf": [
      {"$ref": "#/components/schemas/NewPet"},
      {"type": "object", "required": ["id"], "properties": {
        "id": {"type": "integer", "minimum": 1},
        "status": {"type": "string", "enum": ["available", "sold"]},
        "born": {"type": "string", "format": "date", "nullable": true},
        "tags": {"type": "array", "items": {"type": "string"}, "maxItems": 2}}}]},
    "NewPet": {"type": "object", "required": ["name", "price"], "properties": {
      "name": {"type": "string", "minLength": 1},
      "price": {"type": "number"},
      "meta": {"type": "object"}}},
    "Error": {"type": "object", "required": ["error"], "properties": {"error": {"type": "string"}}}
  }}
}`

func mustSpec(t *testing.T, doc string) *Spec {
	t.Helper()
	s, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func jsonResp(code int, body string) Response {
	h := http.Header{}
	h.Set("Content-Type", "application/json; charset=utf-8")
	return Response{StatusCode: code, Header: h, Body: []byte(body)}
}

func messages(res []domain.AssertionResult) []string {
	var out []string
	for _, r := range res {
		if !r.Passed {
			p := r.Path
			if p != "" {
				p += ": "
			}
			out = append(out, p+r.Message)
		}
	}
	return out
}

func TestMatchUsesServerPathAndPrefersLiteralSegments(t *testing.T) {
	s := mustSpec(t, petSpec)
	cases := map[string]string{
		"GET https://staging.local/v1/pets":     "GET /pets",
		"GET http://localhost:8080/v1/pets/7":   "GET /pets/{petId}",
		"GET http://localhost/v1/pets/mine?x=1": "GET /pets/mine",
		"DELETE http://localhost/pets/7":        "DELETE /pets/{petId}",
		"GET http://gateway/api/v1/pets/7":      "GET /pets/{petId}", // prefix before the server path
		"GET http://localhost/v1/pets/a%2Fb":    "GET /pets/{petId}",
		"POST http://localhost/v1/pets":         "",
		"GET http://localhost/v1/owners":        "",
	}
	for in, want := range cases {
		method, u, _ := strings.Cut(in, " ")
		op := s.Match(method, u)
		got := ""
		if op != nil {
			got = op.Label()
		}
		if got != want {
			t.Errorf("%s: matched %q, want %q", in, got, want)
		}
	}
}

func TestValidResponsePasses(t *testing.T) {
	c := &Checker{Spec: mustSpec(t, petSpec)}
	resp := jsonResp(200, `{"id": 7, "name": "Rex", "price": 19.9, "status": "sold", "born": null, "meta": {"any": 1}}`)
	res := c.Check("GET", "http://localhost/v1/pets/7", resp)
	if len(res) != 1 || !res[0].Passed || res[0].Kind != domain.AssertContract {
		t.Fatalf("results = %+v", res)
	}
	if res[0].Message != "Matches GET /pets/{petId} → 200" {
		t.Errorf("message = %q", res[0].Message)
	}
}

func TestSchemaProblemsAreReportedWithPaths(t *testing.T) {
	c := &Checker{Spec: mustSpec(t, petSpec)}
	resp := jsonResp(200, `{"id": 0, "name": "", "price": "19.90", "status": "archived", "born": "yesterday",
		"tags": ["a", 2, "c"], "nickname": "R"}`)
	got := messages(c.Check("GET", "http://localhost/v1/pets/7", resp))
	want := []string{
		`price: expected number, got string ("19.90")`,
		`name: "" is shorter than 1 characters`,
		`born: "yesterday" is not a valid date`,
		`id: 0 is less than the minimum 1`,
		`status: "archived" is not one of "available", "sold"`,
		`tags: has 3 items, expected at most 2`,
		`tags[1]: expected string, got integer (2)`,
		`nickname: field is not documented`,
	}
	for _, w := range want {
		if !contains(got, w) {
			t.Errorf("missing %q in\n%s", w, strings.Join(got, "\n"))
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d problems, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
}

func TestAllowUndocumentedAndRequiredFields(t *testing.T) {
	c := &Checker{Spec: mustSpec(t, petSpec), AllowUndocumented: true}
	got := messages(c.Check("GET", "http://localhost/v1/pets/7", jsonResp(200, `{"id": 1, "nickname": "R"}`)))
	want := []string{"name: required field is missing", "price: required field is missing"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStatusRangeContentTypeHeadersAndBodies(t *testing.T) {
	c := &Checker{Spec: mustSpec(t, petSpec)}
	if got := messages(c.Check("GET", "http://x/v1/pets/9", jsonResp(404, `{"error": "nope"}`))); len(got) != 0 {
		t.Errorf("4XX response: %q", got)
	}
	got := messages(c.Check("DELETE", "http://x/v1/pets/9", jsonResp(500, `{}`)))
	if len(got) != 1 || got[0] != "status 500 is not documented for DELETE /pets/{petId} (documented: 204, 404)" {
		t.Errorf("undocumented status: %q", got)
	}
	html := Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html"}}, Body: []byte("<p>")}
	got = messages(c.Check("GET", "http://x/v1/pets/9", html))
	if len(got) != 1 || got[0] != "Content-Type text/html is not documented (documented: application/json)" {
		t.Errorf("content type: %q", got)
	}
	got = messages(c.Check("GET", "http://x/v1/pets", jsonResp(200, `[]`)))
	if len(got) != 1 || got[0] != "required response header X-Rate-Limit is missing" {
		t.Errorf("header: %q", got)
	}
	got = messages(c.Check("GET", "http://x/v1/pets/1", Response{StatusCode: 200, Header: http.Header{}}))
	if len(got) != 1 || got[0] != "$: the body is empty, but the spec documents a JSON schema" {
		t.Errorf("empty body: %q", got)
	}
	got = messages(c.Check("GET", "http://x/v1/pets/1", jsonResp(200, `{not json`)))
	if len(got) != 1 || !strings.HasPrefix(got[0], "$: the body is not valid JSON") {
		t.Errorf("invalid JSON: %q", got)
	}
	trunc := jsonResp(200, `{"id": "x"`)
	trunc.Truncated = true
	res := c.Check("GET", "http://x/v1/pets/1", trunc)
	if len(res) != 1 || !res[0].Passed || !strings.Contains(res[0].Message, "truncated") {
		t.Errorf("truncated: %+v", res)
	}
	if res := c.Check("GET", "http://x/v1/owners", jsonResp(200, `{}`)); res != nil {
		t.Errorf("unmatched request should not be checked: %+v", res)
	}
}

func TestOneOfAnyOfNotAndProblemLimit(t *testing.T) {
	doc := `{"swagger": "2.0", "info": {"title": "T", "version": "1"}, "basePath": "/api",
	  "paths": {
	    "/shape": {"get": {"responses": {"200": {"description": "", "schema": {"oneOf": [
	      {"type": "object", "required": ["radius"], "properties": {"radius": {"type": "number"}}},
	      {"type": "object", "required": ["side"], "properties": {"side": {"type": "number"}}}]}}}}},
	    "/list": {"get": {"responses": {"200": {"description": "", "schema": {"type": "array",
	      "items": {"type": "object", "x-nullable": true, "properties": {"n": {"type": "integer"}}}}}}}},
	    "/free": {"get": {"responses": {"default": {"description": "", "schema": {"type": "object",
	      "properties": {"code": {"not": {"type": "string"}}}, "additionalProperties": {"type": "string"}}}}}}
	  }}`
	c := &Checker{Spec: mustSpec(t, doc)}
	if got := messages(c.Check("GET", "http://x/api/shape", jsonResp(200, `{"side": 2}`))); len(got) != 0 {
		t.Errorf("oneOf match: %q", got)
	}
	got := messages(c.Check("GET", "http://x/api/shape", jsonResp(200, `{"side": "2"}`)))
	if len(got) == 0 || got[0] != "$: matches none of the 2 alternatives of oneOf" {
		t.Errorf("oneOf none: %q", got)
	}
	got = messages(c.Check("GET", "http://x/api/shape", jsonResp(200, `{"side": 2, "radius": 1}`)))
	if len(got) != 1 || got[0] != "$: matches 2 alternatives of oneOf, expected exactly one" {
		t.Errorf("oneOf both: %q", got)
	}
	got = messages(c.Check("GET", "http://x/api/free", jsonResp(201, `{"code": "x", "other": "s", "bad": 1}`)))
	want := []string{`bad: expected string, got integer (1)`, `code: the value must not match the schema under "not"`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("free: %q", got)
	}

	var items []string
	for i := 0; i < 30; i++ {
		items = append(items, `{"n": 1.5}`)
	}
	items = append(items, "null")
	res := c.Check("GET", "http://x/api/list", jsonResp(200, "["+strings.Join(items, ",")+"]"))
	got = messages(res)
	if len(got) != MaxProblems+1 || got[MaxProblems] != "…and 10 more problems" || got[0] != "[0].n: expected integer, got number (1.5)" {
		t.Errorf("limit: %d problems, first %q, last %q", len(got), got[0], got[len(got)-1])
	}
}

func TestRecursiveAndHostileSchemasTerminate(t *testing.T) {
	doc := `{"openapi": "3.1.0", "info": {"title": "T", "version": "1"},
	  "paths": {"/tree": {"get": {"responses": {"200": {"description": "", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Node"}}}}}}},
	           "/loop": {"get": {"responses": {"200": {"description": "", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/A"}}}}}}}},
	  "components": {"schemas": {
	    "Node": {"type": ["object", "null"], "properties": {"name": {"type": "string", "pattern": "(?=x)"}, "child": {"$ref": "#/components/schemas/Node"}}},
	    "A": {"$ref": "#/components/schemas/B"}, "B": {"$ref": "#/components/schemas/A"}}}}`
	c := &Checker{Spec: mustSpec(t, doc)}
	body := strings.Repeat(`{"name": "n", "child": `, 100) + "null" + strings.Repeat("}", 100)
	if got := messages(c.Check("GET", "http://x/tree", jsonResp(200, body))); len(got) != 0 {
		t.Errorf("deep tree: %q", got)
	}
	if got := messages(c.Check("GET", "http://x/loop", jsonResp(200, `{"a": 1}`))); len(got) != 0 {
		t.Errorf("ref cycle should accept anything: %q", got)
	}
}

func TestParseRejectsNonSpecs(t *testing.T) {
	for _, doc := range []string{`{"openapi": "4.0"}`, `{"swagger": "2.0", "paths": {}}`, `not json`} {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("Parse(%s) succeeded", doc)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
