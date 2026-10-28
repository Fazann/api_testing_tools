package collectionio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
)

// FromOpenAPI converts a Swagger 2.0 or OpenAPI 3.x document (JSON) into a
// collection export document. One request is created per operation, grouped
// into folders by their first tag, with a status assertion for the first
// documented 2xx response and JSON checks where a JSON schema is given.
// An environment holding base_url, path parameters and credential
// placeholders is created alongside it.
//
// The result still goes through the normal import validation; the input is
// untrusted, so $ref chains and generated examples are bounded.
func FromOpenAPI(data []byte) (Document, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return Document{}, fmt.Errorf("OpenAPI document is not valid JSON: %w", err)
	}
	c := &converter{root: root, vars: map[string]domain.Variable{}}
	switch {
	case strings.HasPrefix(str(root["openapi"]), "3."):
		c.v3 = true
		c.schemes = obj(obj(root["components"])["securitySchemes"])
	case str(root["swagger"]) == "2.0":
		c.schemes = obj(root["securityDefinitions"])
	default:
		return Document{}, fmt.Errorf("unsupported OpenAPI/Swagger version (openapi %q, swagger %q); supported: 3.x and 2.0",
			str(root["openapi"]), str(root["swagger"]))
	}
	paths := obj(root["paths"])
	if len(paths) == 0 {
		return Document{}, errors.New("the OpenAPI document has no paths")
	}

	info := obj(root["info"])
	title := strings.TrimSpace(singleLine(str(info["title"])))
	if title == "" {
		title = "Imported API"
	}
	name := title
	if v := strings.TrimSpace(singleLine(str(info["version"]))); v != "" {
		name += " " + v
	}
	doc := Document{
		Format:     FormatName,
		Version:    CurrentVersion,
		ExportedAt: time.Now().UTC(),
		Collection: domain.Collection{
			Name:              limit(name, domain.MaxNameLength),
			Description:       limit(str(info["description"]), domain.MaxDescriptionLength),
			RequiredVariables: []string{"base_url"},
		},
		Folders:  []domain.Folder{},
		Requests: []domain.Request{},
	}
	c.addVar("base_url", c.baseURL(), false)
	c.collectionAuth = c.authFor(root["security"])
	doc.Collection.Auth = c.collectionAuth

	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)
	folderIDs := map[string]string{}
	// Declared tags first, so folders follow the document's tag order.
	for _, t := range arr(root["tags"]) {
		c.folder(&doc, folderIDs, str(obj(t)["name"]))
	}
	for _, p := range pathKeys {
		item := c.resolve(paths[p])
		for _, method := range []string{"get", "post", "put", "patch", "delete", "head", "options"} {
			op := obj(item[method])
			if op == nil {
				continue
			}
			if len(doc.Requests) >= MaxRequests {
				return Document{}, fmt.Errorf("the OpenAPI document has more than %d operations", MaxRequests)
			}
			req := c.request(p, strings.ToUpper(method), item, op)
			if tags := arr(op["tags"]); len(tags) > 0 {
				req.ParentID = c.folder(&doc, folderIDs, str(tags[0]))
			}
			req.SortOrder = len(doc.Requests)
			doc.Requests = append(doc.Requests, req)
		}
	}
	if len(doc.Requests) == 0 {
		return Document{}, errors.New("the OpenAPI document has no operations")
	}
	// Drop folders for declared tags that no operation uses.
	used := map[string]bool{}
	for _, r := range doc.Requests {
		used[r.ParentID] = true
	}
	folders := doc.Folders[:0]
	for _, f := range doc.Folders {
		if used[f.ID] {
			folders = append(folders, f)
		}
	}
	doc.Folders = folders

	env := domain.Environment{Name: limit(title+" (from OpenAPI)", domain.MaxNameLength)}
	for _, v := range c.varOrder {
		env.Variables = append(env.Variables, c.vars[v])
	}
	doc.Environments = []domain.Environment{env}
	return doc, nil
}

type converter struct {
	root           map[string]any
	collectionAuth domain.Auth
	v3             bool
	schemes        map[string]any
	vars           map[string]domain.Variable
	varOrder       []string
}

func (c *converter) addVar(name, value string, secret bool) {
	if _, ok := c.vars[name]; ok || len(c.vars) >= domain.MaxVariables {
		return
	}
	c.vars[name] = domain.Variable{Key: name, Value: value, Secret: secret, Enabled: true}
	c.varOrder = append(c.varOrder, name)
}

func (c *converter) folder(doc *Document, ids map[string]string, tag string) string {
	tag = strings.TrimSpace(singleLine(tag))
	if tag == "" {
		return ""
	}
	if id, ok := ids[tag]; ok {
		return id
	}
	if len(doc.Folders) >= MaxFolders {
		return ""
	}
	id := fmt.Sprintf("tag-%d", len(doc.Folders)+1)
	ids[tag] = id
	doc.Folders = append(doc.Folders, domain.Folder{ID: id, Name: limit(tag, domain.MaxNameLength), SortOrder: len(doc.Folders)})
	return id
}

const defaultBaseURL = "http://localhost:8080"

// baseURL derives the server URL, substituting server variable defaults.
func (c *converter) baseURL() string {
	var u string
	if c.v3 {
		servers := arr(c.root["servers"])
		if len(servers) > 0 {
			s := obj(servers[0])
			u = str(s["url"])
			for name, v := range obj(s["variables"]) {
				u = strings.ReplaceAll(u, "{"+name+"}", str(obj(v)["default"]))
			}
		}
	} else {
		host := str(c.root["host"])
		scheme := "https"
		if schemes := arr(c.root["schemes"]); len(schemes) > 0 {
			scheme = str(schemes[0])
		}
		if host != "" {
			u = scheme + "://" + host
		}
		u += str(c.root["basePath"])
	}
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		// Relative server URLs ("/v1") are relative to wherever the API is hosted.
		u = defaultBaseURL + "/" + strings.TrimLeft(u, "/")
		u = strings.TrimRight(u, "/")
	}
	return u
}

var pathParamPattern = regexp.MustCompile(`\{([^{}/]+)\}`)

func (c *converter) request(path, method string, item, op map[string]any) domain.Request {
	params := c.parameters(item, op)
	req := domain.Request{
		Name:            c.operationName(path, method, op),
		Method:          method,
		Headers:         []domain.KeyValue{},
		Query:           []domain.KeyValue{},
		BodyType:        domain.BodyNone,
		Auth:            domain.Auth{Type: domain.AuthNone},
		TimeoutMs:       domain.DefaultTimeoutMs,
		FollowRedirects: true,
	}

	byName := map[string]map[string]any{}
	for _, p := range params {
		if str(p["in"]) == "path" {
			byName[str(p["name"])] = p
		}
	}
	req.URLTemplate = "{{base_url}}" + pathParamPattern.ReplaceAllStringFunc(path, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := c.paramExample(byName[name]); ok {
			return url.PathEscape(v)
		}
		v := variableName(name)
		c.addVar(v, "", false)
		return "{{" + v + "}}"
	})

	var form []string
	for _, p := range params {
		name := str(p["name"])
		required, _ := p["required"].(bool)
		value, _ := c.paramExample(p)
		switch str(p["in"]) {
		case "query":
			req.Query = append(req.Query, domain.KeyValue{Key: name, Value: value, Enabled: required})
		case "header":
			switch strings.ToLower(name) {
			case "accept", "content-type", "authorization":
				continue // set by the body type and auth
			}
			if domain.IsValidHeaderName(name) && !strings.ContainsAny(value, "\r\n") {
				req.Headers = append(req.Headers, domain.KeyValue{Key: name, Value: value, Enabled: required})
			}
		case "body": // Swagger 2
			req.BodyType = domain.BodyJSON
			req.Body = c.exampleJSON(c.resolve(p["schema"]))
		case "formData": // Swagger 2
			form = append(form, url.QueryEscape(name)+"="+url.QueryEscape(value))
		}
	}
	if len(form) > 0 && req.BodyType == domain.BodyNone {
		req.BodyType = domain.BodyText
		req.Body = strings.Join(form, "&")
		req.Headers = append(req.Headers, domain.KeyValue{Key: "Content-Type", Value: "application/x-www-form-urlencoded", Enabled: true})
	}
	if c.v3 {
		c.requestBody(&req, c.resolve(op["requestBody"]))
	}
	if len(req.Body) > domain.MaxBodyLength {
		req.Body = ""
	}
	req.Auth = c.auth(op)
	req.Assertions = c.assertions(method, c.resolve(op["responses"]))
	return req
}

func (c *converter) operationName(path, method string, op map[string]any) string {
	for _, k := range []string{"summary", "operationId"} {
		if s := strings.TrimSpace(singleLine(str(op[k]))); s != "" {
			return limit(s, domain.MaxNameLength)
		}
	}
	return limit(method+" "+path, domain.MaxNameLength)
}

// parameters merges path-level and operation-level parameters; operation
// parameters override path ones with the same name and location.
func (c *converter) parameters(item, op map[string]any) []map[string]any {
	var out []map[string]any
	index := map[string]int{}
	for _, list := range [][]any{arr(item["parameters"]), arr(op["parameters"])} {
		for _, raw := range list {
			p := c.resolve(raw)
			if p == nil || str(p["name"]) == "" {
				continue
			}
			key := str(p["in"]) + ":" + str(p["name"])
			if i, ok := index[key]; ok {
				out[i] = p
				continue
			}
			index[key] = len(out)
			out = append(out, p)
		}
	}
	return out
}

// paramExample returns an example value for a parameter as text.
func (c *converter) paramExample(p map[string]any) (string, bool) {
	if p == nil {
		return "", false
	}
	candidates := []any{p["example"], p["default"]}
	if enum := arr(p["enum"]); len(enum) > 0 {
		candidates = append(candidates, enum[0])
	}
	if s := c.resolve(p["schema"]); s != nil {
		candidates = append(candidates, s["example"], s["default"])
		if enum := arr(s["enum"]); len(enum) > 0 {
			candidates = append(candidates, enum[0])
		}
	}
	for _, ex := range obj(p["examples"]) {
		candidates = append(candidates, c.resolve(ex)["value"])
		break
	}
	for _, v := range candidates {
		if s, ok := scalarText(v); ok {
			return s, true
		}
	}
	return "", false
}

func (c *converter) requestBody(req *domain.Request, body map[string]any) {
	content := obj(body["content"])
	if len(content) == 0 {
		return
	}
	types := make([]string, 0, len(content))
	for t := range content {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		if strings.Contains(t, "json") {
			media := obj(content[t])
			req.BodyType = domain.BodyJSON
			if ex, ok := c.mediaExample(media); ok {
				req.Body = prettyJSON(ex)
			} else {
				req.Body = c.exampleJSON(c.resolve(media["schema"]))
			}
			if t != "application/json" {
				req.Headers = append(req.Headers, domain.KeyValue{Key: "Content-Type", Value: t, Enabled: true})
			}
			return
		}
	}
	t := types[0]
	req.BodyType = domain.BodyText
	if t == "application/x-www-form-urlencoded" {
		schema := c.resolve(obj(content[t])["schema"])
		var fields []string
		for _, k := range sortedKeys(obj(schema["properties"])) {
			fields = append(fields, url.QueryEscape(k)+"=")
		}
		req.Body = strings.Join(fields, "&")
	}
	if !strings.ContainsAny(t, "\r\n") {
		req.Headers = append(req.Headers, domain.KeyValue{Key: "Content-Type", Value: t, Enabled: true})
	}
}

func (c *converter) mediaExample(media map[string]any) (any, bool) {
	if ex, ok := media["example"]; ok {
		return ex, true
	}
	examples := obj(media["examples"])
	for _, k := range sortedKeys(examples) {
		if v, ok := c.resolve(examples[k])["value"]; ok {
			return v, true
		}
	}
	return nil, false
}

// auth returns the operation's own auth when it declares "security";
// otherwise it inherits the collection auth built from the document-wide
// security requirement.
func (c *converter) auth(op map[string]any) domain.Auth {
	if security, ok := op["security"]; ok {
		return c.authFor(security)
	}
	if c.collectionAuth.Type != domain.AuthNone {
		return domain.Auth{Type: domain.AuthInherit}
	}
	return domain.Auth{Type: domain.AuthNone}
}

// authFor maps the first security requirement to an auth configuration that
// references placeholder variables.
func (c *converter) authFor(security any) domain.Auth {
	for _, req := range arr(security) {
		names := sortedKeys(obj(req))
		if len(names) == 0 {
			return domain.Auth{Type: domain.AuthNone} // {} means "no auth" is acceptable
		}
		scheme := c.resolve(c.schemes[names[0]])
		switch strings.ToLower(str(scheme["type"])) {
		case "http":
			if strings.EqualFold(str(scheme["scheme"]), "basic") {
				return c.basicAuth()
			}
			return c.bearerAuth()
		case "basic":
			return c.basicAuth()
		case "oauth2", "openidconnect":
			return c.bearerAuth()
		case "apikey":
			keyName := str(scheme["name"])
			switch str(scheme["in"]) {
			case "header":
				if domain.IsValidHeaderName(keyName) {
					c.addVar("api_key", "", true)
					return domain.Auth{Type: domain.AuthAPIKey, KeyName: keyName, KeyValue: "{{api_key}}", KeyIn: domain.APIKeyHeader}
				}
			case "query":
				if keyName != "" {
					c.addVar("api_key", "", true)
					return domain.Auth{Type: domain.AuthAPIKey, KeyName: keyName, KeyValue: "{{api_key}}", KeyIn: domain.APIKeyQuery}
				}
			}
		}
	}
	return domain.Auth{Type: domain.AuthNone}
}

func (c *converter) bearerAuth() domain.Auth {
	c.addVar("token", "", true)
	return domain.Auth{Type: domain.AuthBearer, Token: "{{token}}"}
}

func (c *converter) basicAuth() domain.Auth {
	c.addVar("username", "", false)
	c.addVar("password", "", true)
	return domain.Auth{Type: domain.AuthBasic, Username: "{{username}}", Password: "{{password}}"}
}

const maxRequiredFields = 20

func (c *converter) assertions(method string, responses map[string]any) []domain.Assertion {
	codes := sortedKeys(responses)
	var code string
	for _, k := range codes {
		if strings.HasPrefix(k, "2") {
			code = k
			break
		}
	}
	var out []domain.Assertion
	if n, err := strconv.Atoi(code); err == nil {
		out = append(out, domain.Assertion{Kind: domain.AssertStatus, Operator: domain.OpEq, Expected: json.RawMessage(strconv.Itoa(n)), Enabled: true})
	} else {
		out = append(out,
			domain.Assertion{Kind: domain.AssertStatus, Operator: domain.OpGte, Expected: json.RawMessage("200"), Enabled: true},
			domain.Assertion{Kind: domain.AssertStatus, Operator: domain.OpLt, Expected: json.RawMessage("300"), Enabled: true},
		)
	}
	if code == "" || code == "204" || method == "HEAD" {
		return out
	}
	resp := c.resolve(responses[code])
	var schema map[string]any
	isJSON := false
	if c.v3 {
		for t, m := range obj(resp["content"]) {
			if strings.Contains(t, "json") {
				isJSON = true
				schema = c.resolve(obj(m)["schema"])
				break
			}
		}
	} else if s, ok := resp["schema"]; ok {
		isJSON = true
		schema = c.resolve(s)
	}
	if !isJSON {
		return out
	}
	out = append(out, domain.Assertion{Kind: domain.AssertValidJSON, Enabled: true})
	var fields []string
	for _, f := range arr(schema["required"]) {
		if name := str(f); name != "" && len(fields) < maxRequiredFields {
			fields = append(fields, assertions.JoinKey("", name))
		}
	}
	if len(fields) > 0 {
		raw, _ := json.Marshal(fields)
		out = append(out, domain.Assertion{Kind: domain.AssertRequiredFields, Expected: raw, Enabled: true,
			Description: "Required fields from the response schema"})
	}
	return out
}

// ---- schema examples ----

const (
	maxSchemaDepth = 8
	maxSchemaNodes = 2000
)

func (c *converter) exampleJSON(schema map[string]any) string {
	if schema == nil {
		return ""
	}
	budget := maxSchemaNodes
	return prettyJSON(c.example(schema, 0, &budget))
}

func (c *converter) example(schema map[string]any, depth int, budget *int) any {
	*budget--
	if schema == nil || depth > maxSchemaDepth || *budget < 0 {
		return nil
	}
	for _, k := range []string{"example", "default"} {
		if v, ok := schema[k]; ok {
			return v
		}
	}
	if enum := arr(schema["enum"]); len(enum) > 0 {
		return enum[0]
	}
	if all := arr(schema["allOf"]); len(all) > 0 {
		merged := map[string]any{}
		for _, part := range all {
			if m, ok := c.example(c.resolve(part), depth+1, budget).(map[string]any); ok {
				for k, v := range m {
					merged[k] = v
				}
			}
		}
		return merged
	}
	for _, k := range []string{"oneOf", "anyOf"} {
		if alts := arr(schema[k]); len(alts) > 0 {
			return c.example(c.resolve(alts[0]), depth+1, budget)
		}
	}
	typ := schemaType(schema)
	switch {
	case typ == "object" || (typ == "" && schema["properties"] != nil):
		out := map[string]any{}
		props := obj(schema["properties"])
		for _, k := range sortedKeys(props) {
			p := c.resolve(props[k])
			if ro, _ := p["readOnly"].(bool); ro {
				continue
			}
			out[k] = c.example(p, depth+1, budget)
		}
		return out
	case typ == "array":
		return []any{c.example(c.resolve(schema["items"]), depth+1, budget)}
	case typ == "string":
		switch str(schema["format"]) {
		case "date-time":
			return "2026-01-01T00:00:00Z"
		case "date":
			return "2026-01-01"
		case "email":
			return "user@example.com"
		case "uuid":
			return "00000000-0000-0000-0000-000000000000"
		case "uri", "url":
			return "https://example.com"
		}
		return "string"
	case typ == "integer", typ == "number":
		return 0
	case typ == "boolean":
		return false
	}
	return nil
}

// schemaType handles both "type": "x" and OpenAPI 3.1's "type": ["x", "null"].
func schemaType(schema map[string]any) string {
	if s := str(schema["type"]); s != "" {
		return s
	}
	for _, t := range arr(schema["type"]) {
		if s := str(t); s != "" && s != "null" {
			return s
		}
	}
	return ""
}

// ---- $ref resolution ----

const maxRefHops = 20

// resolve follows local "$ref" pointers ("#/components/schemas/X"). External
// references are not fetched; they resolve to nil.
func (c *converter) resolve(v any) map[string]any {
	m := obj(v)
	for hops := 0; m != nil && hops < maxRefHops; hops++ {
		ref, ok := m["$ref"].(string)
		if !ok {
			return m
		}
		m = obj(c.pointer(ref))
	}
	if m != nil {
		if _, ok := m["$ref"]; ok {
			return nil // reference cycle
		}
	}
	return m
}

func (c *converter) pointer(ref string) any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	var cur any = c.root
	for _, part := range strings.Split(ref[2:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		if p, err := url.PathUnescape(part); err == nil {
			part = p
		}
		switch node := cur.(type) {
		case map[string]any:
			cur = node[part]
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(node) {
				return nil
			}
			cur = node[i]
		default:
			return nil
		}
	}
	return cur
}

// ---- YAML ----

// looksLikeYAML reports whether data is not JSON (JSON documents start with
// '{' or '[' after optional whitespace and a byte-order mark).
func looksLikeYAML(data []byte) bool {
	t := bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), " \t\r\n")
	return len(t) > 0 && t[0] != '{' && t[0] != '['
}

func yamlToJSON(data []byte) ([]byte, error) {
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return json.Marshal(stringKeys(v, 0))
}

// stringKeys converts YAML maps with non-string keys (such as response codes
// written as 200:) into JSON-compatible maps.
func stringKeys(v any, depth int) any {
	if depth > 200 {
		return nil
	}
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			t[k] = stringKeys(child, depth+1)
		}
		return t
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[fmt.Sprint(k)] = stringKeys(child, depth+1)
		}
		return out
	case []any:
		for i := range t {
			t[i] = stringKeys(t[i], depth+1)
		}
		return t
	}
	return v
}

// ---- small helpers ----

func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }
func arr(v any) []any          { a, _ := v.([]any); return a }
func str(v any) string         { s, _ := v.(string); return s }

func scalarText(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(t), true
	}
	return "", false
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

var invalidVarChars = regexp.MustCompile(`[^A-Za-z0-9_.\-]`)

// variableName turns a parameter name into a valid {{variable}} name.
func variableName(name string) string {
	v := invalidVarChars.ReplaceAllString(name, "_")
	if v == "" || !(v[0] == '_' || v[0] >= 'A' && v[0] <= 'Z' || v[0] >= 'a' && v[0] <= 'z') {
		v = "p_" + v
	}
	return limit(v, 128)
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r < 32 {
			return ' '
		}
		return r
	}, s)), " ")
}

// limit truncates s to at most n bytes without splitting a UTF-8 character.
func limit(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
