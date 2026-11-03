package scripting

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
)

func run(t *testing.T, in Input) Output {
	t.Helper()
	if in.Name == "" {
		in.Name = "test script"
	}
	if in.Phase == "" {
		in.Phase = PreRequest
	}
	out, err := Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out
}

// value runs expr in a script and returns what it logged.
func value(t *testing.T, expr string) string {
	t.Helper()
	out := run(t, Input{Script: "console.log(" + expr + ")"})
	if len(out.Logs) != 1 {
		t.Fatalf("logs = %+v", out.Logs)
	}
	return out.Logs[0].Message
}

func TestCryptoJS(t *testing.T) {
	cases := map[string]string{
		// Known test vectors.
		`CryptoJS.MD5("abc").toString()`:    "900150983cd24fb0d6963f7d28e17f72",
		`CryptoJS.SHA256("abc").toString()`: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		`CryptoJS.HmacSHA256("The quick brown fox jumps over the lazy dog", "key").toString()`:                            "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8",
		`CryptoJS.HmacSHA256("abc", "key").toString(CryptoJS.enc.Base64)`:                                                 "nBluMtwBdfhvSxy4konWYZ3mvuaZ5MN45oMJ7Zehpqs=",
		`CryptoJS.enc.Base64.stringify(CryptoJS.enc.Utf8.parse("hello"))`:                                                 "aGVsbG8=",
		`require('crypto-js').SHA1("abc").toString()`:                                                                     "a9993e364706816aba3e25717850c26c9cd0d89d",
		`CryptoJS.AES.decrypt(CryptoJS.AES.encrypt("secret msg", "pass").toString(), "pass").toString(CryptoJS.enc.Utf8)`: "secret msg",
	}
	for expr, want := range cases {
		if got := value(t, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
	// AES with an explicit key and IV (CBC, PKCS7) matches a known ciphertext.
	got := value(t, `CryptoJS.AES.encrypt("hello", CryptoJS.enc.Utf8.parse("0123456789abcdef"), {iv: CryptoJS.enc.Utf8.parse("abcdef9876543210")}).toString()`)
	if got != "rkbumbtCClkN+Jkds7bQJw==" {
		t.Errorf("AES-CBC = %q", got)
	}
}

func TestLibraries(t *testing.T) {
	cases := map[string]string{
		`_.sum([1, 2, 3])`:                                      "6",
		`require('lodash').get({a: {b: 7}}, 'a.b')`:             "7",
		`moment.utc("2024-01-02T03:04:05Z").format("YYYYMMDD")`: "20240102",
		`require('moment')(0).utc().year()`:                     "1970",
		`btoa("hello")`:                                         "aGVsbG8=",
		`atob("aGVsbG8=")`:                                      "hello",
		`require('uuid').v4().length`:                           "36",
		`typeof crypto.getRandomValues(new Uint8Array(4))[0]`:   "number",
	}
	for expr, want := range cases {
		if got := value(t, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
	_, err := Run(context.Background(), Input{Name: "s", Script: "require('fs')"})
	if err == nil || !strings.Contains(err.Error(), "module 'fs' is not available") {
		t.Errorf("require('fs') error = %v", err)
	}
}

func TestVariablesAndRequest(t *testing.T) {
	env := &domain.Environment{ID: "e", Variables: []domain.Variable{
		{Key: "secret", Value: "s3cret", Enabled: true, Secret: true},
		{Key: "shared", Value: "from-env", Enabled: true},
	}}
	vars := NewVariables(env, []domain.Variable{{Key: "shared", Value: "from-collection", Enabled: true}, {Key: "base", Value: "http://h", Enabled: true}})
	locals := map[string]string{}
	out := run(t, Input{Vars: vars, Locals: locals, Script: `
		const body = pm.request.body.raw;
		const sig = CryptoJS.HmacSHA256(pm.variables.replaceIn(body), pm.environment.get("secret")).toString();
		pm.variables.set("signature", sig);
		pm.request.headers.upsert({key: "X-Signature", value: "{{signature}}"});
		pm.request.headers.remove("x-remove-me");
		pm.request.url.query.add({key: "ts", value: "1"});
		pm.environment.set("token", "abc");
		pm.collectionVariables.set("counter", 1);
		console.log(pm.variables.get("shared"), pm.collectionVariables.get("shared"), pm.request.url.toString());
		pm.request.method = "put";
	`, Request: Request{Method: "POST", URL: "{{base}}/pay", Body: `{"amount":{{amount}}}`, BodyType: domain.BodyJSON,
		Headers: []domain.KeyValue{{Key: "X-Remove-Me", Value: "1", Enabled: true}}}})

	if out.Request.Method != "PUT" || len(out.Request.Headers) != 1 || out.Request.Headers[0].Key != "X-Signature" {
		t.Fatalf("request = %+v", out.Request)
	}
	if len(out.Request.Query) != 1 || out.Request.Query[0].Key != "ts" {
		t.Errorf("query = %+v", out.Request.Query)
	}
	if got := out.Logs[0].Message; got != "from-env from-collection {{base}}/pay?ts=1" {
		t.Errorf("log = %q", got)
	}
	if len(locals["signature"]) != 64 {
		t.Errorf("locals = %v", locals)
	}
	if e, changed := vars.Environment(); !changed || e.Variables[2].Key != "token" || e.Variables[2].Secret {
		t.Errorf("environment = %+v changed=%v", e, changed)
	}
	if c, changed := vars.Collection(); !changed || c[2].Value != "1" {
		t.Errorf("collection = %+v changed=%v", c, changed)
	}
	// Locals override environment values and keep the secret flag.
	resolved := vars.Resolved(map[string]string{"secret": "x", "signature": "y"})
	for _, v := range resolved.Variables {
		if v.Key == "secret" && (v.Value != "x" || !v.Secret) {
			t.Errorf("local secret = %+v", v)
		}
	}
	if err := vars.Set(ScopeEnvironment, "bad name", "v"); err == nil {
		t.Error("invalid variable name accepted")
	}
}

func TestEnvironmentWithoutSelection(t *testing.T) {
	vars := NewVariables(nil, nil)
	run(t, Input{Vars: vars, Script: `pm.environment.set("a", "1"); console.log(pm.environment.get("missing") === undefined)`})
	e, changed := vars.Environment()
	if !changed || e == nil || e.ID != "" || e.Variables[0].Value != "1" {
		t.Errorf("environment = %+v changed=%v", e, changed)
	}
}

func TestTestsAndExpect(t *testing.T) {
	out := run(t, Input{Phase: PostResponse, Script: `
		pm.test("status is 201", () => pm.response.to.have.status(201));
		pm.test("status is 200", () => pm.response.to.have.status(200));
		pm.test("json", () => {
			const data = pm.response.json();
			pm.expect(data.id).to.eql(7);
			pm.expect(data.tags).to.include("a").and.have.lengthOf(2);
			pm.expect(data).to.have.property("name", "x");
			pm.expect(data.name).to.be.a("string").and.not.be.empty;
			pm.expect(data.nested).to.deep.equal({k: [1]});
			pm.expect(pm.response.responseTime).to.be.below(1000);
			pm.response.to.have.header("content-type");
			pm.response.to.be.json;
			pm.response.to.not.be.ok;
		});
		pm.test("fails", () => pm.expect(data => 1).to.be.a("string"));
		pm.test("not", () => pm.expect(1).to.not.equal(1));
		tests["legacy"] = responseCode.code === 201;
	`, Response: &Response{Code: 201, Status: "201 Created", Header: http.Header{"Content-Type": {"application/json"}},
		Body: []byte(`{"id":7,"name":"x","tags":["a","b"],"nested":{"k":[1]}}`), DurationMs: 12}})

	want := []struct {
		name, err string
		passed    bool
	}{
		{"status is 201", "", true},
		{"status is 200", "expected response to have status 200 but got 201 Created", false},
		{"json", "expected response not to be 2xx (got 201)", false},
		{"fails", "expected [Function] to be a 'string'", false},
		{"not", "expected 1 not to equal 1", false},
		{"legacy", "", true},
	}
	if len(out.Tests) != len(want) {
		t.Fatalf("tests = %+v", out.Tests)
	}
	for i, w := range want {
		got := out.Tests[i]
		if got.Name != w.name || got.Passed != w.passed || got.Error != w.err {
			t.Errorf("test %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestErrors(t *testing.T) {
	_, err := Run(context.Background(), Input{Name: "pre-request script", Script: "const a = 1;\nundefinedFn();"})
	if err == nil || err.Error() != "ReferenceError: undefinedFn is not defined (line 2)" {
		t.Errorf("error = %v", err)
	}
	_, err = Run(context.Background(), Input{Name: "s", Script: "throw new Error('boom')"})
	if err == nil || err.Error() != "Error: boom (line 1)" {
		t.Errorf("error = %v", err)
	}
	_, err = Run(context.Background(), Input{Name: "s", Script: "pm.environment.set('a b', 1)"})
	if err == nil || !strings.Contains(err.Error(), `variable name "a b" is invalid`) {
		t.Errorf("error = %v", err)
	}
	_, err = Run(context.Background(), Input{Name: "s", Script: "function ("})
	if err == nil || !strings.Contains(err.Error(), "SyntaxError") {
		t.Errorf("syntax error = %v", err)
	}
}

func TestTimeoutAndCancel(t *testing.T) {
	old := Timeout
	Timeout = 200 * time.Millisecond
	defer func() { Timeout = old }()
	start := time.Now()
	out, err := Run(context.Background(), Input{Name: "s", Script: "console.log('before'); while (true) {}"})
	if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(start) > 3*time.Second {
		t.Errorf("timeout error = %v after %s", err, time.Since(start))
	}
	if len(out.Logs) != 1 {
		t.Errorf("logs before timeout were lost: %+v", out.Logs)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	if _, err := Run(ctx, Input{Name: "s", Script: "while (true) {}"}); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("cancel error = %v", err)
	}
}

func TestRSA(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	privPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	pubDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	// Keys stored in a variable often have literal \n instead of line breaks.
	vars := NewVariables(&domain.Environment{Variables: []domain.Variable{
		{Key: "pub", Value: strings.ReplaceAll(pubPEM, "\n", `\n`), Enabled: true},
		{Key: "priv", Value: privPEM, Enabled: true},
	}}, nil)
	out := run(t, Input{Vars: vars, Script: `
		console.log(pm.crypto.rsaEncrypt(pm.environment.get("pub"), "card=4111"));
		console.log(pm.crypto.rsaEncrypt(pm.environment.get("pub"), "card=4111", {padding: "oaep", hash: "sha256"}));
		console.log(pm.crypto.rsaSign(pm.environment.get("priv"), "payload"));
	`})
	pkcs1, _ := base64.StdEncoding.DecodeString(out.Logs[0].Message)
	if plain, err := rsa.DecryptPKCS1v15(nil, key, pkcs1); err != nil || string(plain) != "card=4111" {
		t.Errorf("PKCS1 decrypt = %q, %v", plain, err)
	}
	oaep, _ := base64.StdEncoding.DecodeString(out.Logs[1].Message)
	if plain, err := rsa.DecryptOAEP(sha256.New(), nil, key, oaep, nil); err != nil || string(plain) != "card=4111" {
		t.Errorf("OAEP decrypt = %q, %v", plain, err)
	}
	sig, _ := base64.StdEncoding.DecodeString(out.Logs[2].Message)
	digest := sha256.Sum256([]byte("payload"))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Errorf("signature does not verify: %v", err)
	}
}
