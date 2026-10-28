package collectionio

import (
	"os"
	"strings"
	"testing"

	"api-test-kits/internal/domain"
)

func requestNamed(t *testing.T, imp *Imported, name string) domain.Request {
	t.Helper()
	for _, r := range imp.Tree.Requests {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no request named %q", name)
	return domain.Request{}
}

func kinds(r domain.Request) []string {
	var out []string
	for _, a := range r.Assertions {
		out = append(out, string(a.Kind)+":"+string(a.Operator)+":"+string(a.Expected))
	}
	return out
}

func TestSwagger2Import(t *testing.T) {
	f, err := os.Open("../../tests/fixtures/petstore.swagger.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	imp, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	if imp.Tree.Collection.Name != "Petstore 1.0.0" || len(imp.Tree.Requests) != 5 {
		t.Fatalf("collection %q with %d requests", imp.Tree.Collection.Name, len(imp.Tree.Requests))
	}
	if len(imp.Tree.Folders) != 1 || imp.Tree.Folders[0].Name != "pet" {
		t.Fatalf("expected only the used 'pet' folder, got %+v", imp.Tree.Folders)
	}

	get := requestNamed(t, imp, "Find pet by ID")
	if get.URLTemplate != "{{base_url}}/pet/{{petId}}" || get.ParentID != imp.Tree.Folders[0].ID {
		t.Fatalf("url %q parent %q", get.URLTemplate, get.ParentID)
	}
	ca := imp.Tree.Collection.Auth
	if ca.Type != domain.AuthAPIKey || ca.KeyName != "api_key" || ca.KeyValue != "{{api_key}}" {
		t.Fatalf("document-wide security should become the collection auth: %+v", ca)
	}
	if get.Auth.Type != domain.AuthInherit {
		t.Fatalf("operations without their own security should inherit: %+v", get.Auth)
	}
	if len(get.Headers) != 1 || get.Headers[0].Key != "X-Trace-Id" || get.Headers[0].Enabled {
		t.Fatalf("optional header should be present but disabled: %+v", get.Headers)
	}
	if got := strings.Join(kinds(get), " "); got != `status:eq:200 valid_json:: required_fields::["name","photoUrls"]` {
		t.Fatalf("assertions: %s", got)
	}

	if del := requestNamed(t, imp, "deletePet"); del.Auth.Type != domain.AuthNone || len(del.Assertions) != 1 {
		t.Fatalf("security: [] means no auth; 204 has no body checks: %+v", del)
	}

	post := requestNamed(t, imp, "Add a pet")
	if post.BodyType != domain.BodyJSON || !strings.Contains(post.Body, `"name": "doggie"`) ||
		strings.Contains(post.Body, `"id"`) || !strings.Contains(post.Body, `"status": "available"`) {
		t.Fatalf("body example (readOnly id omitted, enum and example used):\n%s", post.Body)
	}

	find := requestNamed(t, imp, "Finds pets by status")
	if len(find.Query) != 2 || find.Query[0].Value != "available" || !find.Query[0].Enabled ||
		find.Query[1].Value != "20" || find.Query[1].Enabled {
		t.Fatalf("query: %+v", find.Query)
	}

	inv := requestNamed(t, imp, "GET /store/inventory")
	if got := strings.Join(kinds(inv), " "); got != "status:gte:200 status:lt:300" {
		t.Fatalf("no 2xx documented → status range check, got %s", got)
	}

	env := imp.Environments[0]
	vars := map[string]domain.Variable{}
	for _, v := range env.Variables {
		vars[v.Key] = v
	}
	if vars["base_url"].Value != "https://petstore.example.com/v2" || !vars["api_key"].Secret {
		t.Fatalf("environment %+v", env)
	}
	if _, ok := vars["petId"]; !ok {
		t.Fatal("path parameter without example should become a variable")
	}
}

const openAPI3YAML = `
openapi: 3.0.3
info:
  title: Wallet
  version: "2"
servers:
  - url: https://{region}.wallet.test/api
    variables:
      region: { default: eu }
components:
  securitySchemes:
    bearer: { type: http, scheme: bearer }
  schemas:
    Balance:
      type: object
      required: [balance, currency]
      properties:
        balance: { type: number }
        currency: { type: string, example: USD }
security:
  - bearer: []
paths:
  /accounts/{id}/balance:
    get:
      summary: Get balance
      parameters:
        - { name: id, in: path, required: true, schema: { type: string, example: acc-1 } }
      responses:
        200:
          description: ok
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Balance' }
  /transactions:
    post:
      summary: Create transaction
      requestBody:
        content:
          application/json:
            examples:
              small: { value: { amount: 5 } }
      responses:
        '201': { description: created }
`

func TestOpenAPI3YAMLImport(t *testing.T) {
	imp, err := Read(strings.NewReader(openAPI3YAML))
	if err != nil {
		t.Fatal(err)
	}
	get := requestNamed(t, imp, "Get balance")
	if get.URLTemplate != "{{base_url}}/accounts/acc-1/balance" || get.Auth.Type != domain.AuthInherit {
		t.Fatalf("url %q auth %+v", get.URLTemplate, get.Auth)
	}
	if ca := imp.Tree.Collection.Auth; ca.Type != domain.AuthBearer || ca.Token != "{{token}}" {
		t.Fatalf("collection auth %+v", ca)
	}
	if got := strings.Join(kinds(get), " "); got != `status:eq:200 valid_json:: required_fields::["balance","currency"]` {
		t.Fatalf("assertions: %s", got)
	}
	post := requestNamed(t, imp, "Create transaction")
	if post.Body != "{\n  \"amount\": 5\n}" {
		t.Fatalf("named example should be used, got %q", post.Body)
	}
	if imp.Environments[0].Variables[0].Value != "https://eu.wallet.test/api" {
		t.Fatalf("server variables not substituted: %+v", imp.Environments[0].Variables)
	}
}

func TestOpenAPIRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"no paths":    `{"openapi":"3.0.0","info":{"title":"x"},"paths":{}}`,
		"old version": `{"swagger":"1.2","paths":{"/a":{"get":{}}}}`,
		"empty file":  `{}`,
		"bad yaml":    "openapi: [unclosed",
	}
	for name, in := range cases {
		if _, err := Read(strings.NewReader(in)); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestOpenAPIRefCyclesAndRelativeServer(t *testing.T) {
	in := `{"openapi":"3.1.0","info":{"title":"Loop"},"servers":[{"url":"/v1"}],
	  "components":{"schemas":{"A":{"$ref":"#/components/schemas/B"},"B":{"$ref":"#/components/schemas/A"},
	    "Node":{"type":["object","null"],"properties":{"child":{"$ref":"#/components/schemas/Node"}}}}},
	  "paths":{"/a":{"post":{"requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Node"}}}},
	    "responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/A"}}}}}}}}}`
	imp, err := Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if imp.Environments[0].Variables[0].Value != "http://localhost:8080/v1" {
		t.Fatalf("relative server: %+v", imp.Environments[0].Variables[0])
	}
	if r := imp.Tree.Requests[0]; !strings.Contains(r.Body, `"child"`) {
		t.Fatalf("recursive schema should produce a bounded example, got %q", r.Body)
	}
}
