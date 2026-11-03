// Package scripting runs pre-request and post-response scripts written in
// JavaScript with a Postman-compatible API (pm.environment, pm.request,
// pm.response, pm.test, pm.expect, console, require). Bundled libraries:
// crypto-js, lodash and moment, plus uuid, atob/btoa and pm.crypto for RSA.
//
// Each script gets a fresh, isolated runtime with no file, network or process
// access, and is interrupted after Timeout.
package scripting

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/google/uuid"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/environments"
)

// Timeout bounds how long one script may run.
var Timeout = 10 * time.Second

// MaxLogs bounds the console messages kept per script.
const MaxLogs = 200

const maxLogLength = 10_000

type Phase string

const (
	PreRequest   Phase = "pre-request"
	PostResponse Phase = "post-response"
)

// Request is the part of a request that scripts can read and, before it is
// sent, change.
type Request struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  []domain.KeyValue `json:"headers"`
	Query    []domain.KeyValue `json:"query"`
	Body     string            `json:"body"`
	BodyType domain.BodyType   `json:"bodyType"`
}

// Response is what post-response scripts see as pm.response.
type Response struct {
	Code       int
	Status     string
	Header     http.Header
	Body       []byte
	DurationMs int64
	Size       int64
}

// Input describes one script execution.
type Input struct {
	Phase Phase
	// Script is the JavaScript source; Name labels it in error messages.
	Script          string
	Name            string
	RequestName     string
	RequestID       string
	EnvironmentName string
	Request         Request
	// Response is set for post-response scripts only.
	Response *Response
	Vars     *Variables
	// Locals holds pm.variables values. Run updates it in place so a
	// request's scripts share them.
	Locals map[string]string
}

// Test is the outcome of one pm.test().
type Test struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Error  string `json:"error"`
}

// Log is one console message.
type Log struct {
	Level   string
	Message string
}

// Output is what a script produced. Request reflects changes made through
// pm.request (only meaningful for pre-request scripts).
type Output struct {
	Request Request
	Tests   []Test
	Logs    []Log
}

//go:embed sandbox.js
var sandboxSource string

//go:embed libs/crypto-js.min.js
var cryptoJSSource string

//go:embed libs/lodash.min.js
var lodashSource string

//go:embed libs/moment.min.js
var momentSource string

var (
	compileOnce sync.Once
	sandboxProg *goja.Program
	libProgs    map[string]*goja.Program
	compileErr  error
)

// compile parses the sandbox and libraries once; every runtime reuses them.
func compile() error {
	compileOnce.Do(func() {
		sandboxProg, compileErr = goja.Compile("sandbox.js", sandboxSource, false)
		if compileErr != nil {
			return
		}
		libProgs = map[string]*goja.Program{}
		for name, src := range map[string]string{"crypto-js": cryptoJSSource, "lodash": lodashSource, "moment": momentSource} {
			// Wrapped as a CommonJS module so each library exports itself.
			p, err := goja.Compile(name, "(function (module, exports) {"+src+"\n})", false)
			if err != nil {
				compileErr = fmt.Errorf("compile %s: %w", name, err)
				return
			}
			libProgs[name] = p
		}
	})
	return compileErr
}

type inputJSON struct {
	Phase           Phase             `json:"phase"`
	RequestName     string            `json:"requestName"`
	RequestID       string            `json:"requestId"`
	EnvironmentName string            `json:"environmentName"`
	Request         Request           `json:"request"`
	Response        *responseJSON     `json:"response"`
	Locals          map[string]string `json:"locals"`
}

type responseJSON struct {
	Code         int               `json:"code"`
	Status       string            `json:"status"`
	Headers      []domain.KeyValue `json:"headers"`
	Body         string            `json:"body"`
	ResponseTime int64             `json:"responseTime"`
	Size         int64             `json:"size"`
}

type outputJSON struct {
	Request Request           `json:"request"`
	Locals  map[string]string `json:"locals"`
	Tests   []Test            `json:"tests"`
}

// Run executes one script. Logs written before a failure are returned along
// with the error.
func Run(ctx context.Context, in Input) (Output, error) {
	out := Output{Request: in.Request}
	if err := compile(); err != nil {
		return out, fmt.Errorf("script engine: %w", err)
	}
	if in.Vars == nil {
		in.Vars = NewVariables(nil, nil)
	}
	if in.Locals == nil {
		in.Locals = map[string]string{}
	}
	if in.Request.Headers == nil {
		in.Request.Headers = []domain.KeyValue{}
	}
	if in.Request.Query == nil {
		in.Request.Query = []domain.KeyValue{}
	}
	data := inputJSON{
		Phase: in.Phase, RequestName: in.RequestName, RequestID: in.RequestID, EnvironmentName: in.EnvironmentName,
		Request: in.Request, Locals: in.Locals,
	}
	if r := in.Response; r != nil {
		data.Response = &responseJSON{Code: r.Code, Status: r.Status, Headers: HeaderRows(r.Header), Body: string(r.Body),
			ResponseTime: r.DurationMs, Size: r.Size}
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return out, err
	}

	vm := goja.New()
	vm.SetMaxCallStackSize(1024)
	stop := time.AfterFunc(Timeout, func() { vm.Interrupt(fmt.Sprintf("script timed out after %s", Timeout)) })
	defer stop.Stop()
	defer context.AfterFunc(ctx, func() { vm.Interrupt("run was cancelled") })()

	n := &natives{vm: vm, in: &in, out: &out}
	if err := vm.Set("__native", n.object()); err != nil {
		return out, err
	}
	if err := vm.Set("__inputJSON", string(payload)); err != nil {
		return out, err
	}
	if _, err := vm.RunProgram(sandboxProg); err != nil {
		return out, fmt.Errorf("script engine: %w", err)
	}
	if _, err := vm.RunScript(in.Name, in.Script); err != nil {
		return out, scriptError(err)
	}
	finish, ok := goja.AssertFunction(vm.Get("__finish"))
	if !ok {
		return out, errors.New("script engine: the script replaced internal state")
	}
	v, err := finish(goja.Undefined())
	if err != nil {
		return out, scriptError(err)
	}
	var res outputJSON
	if err := json.Unmarshal([]byte(v.String()), &res); err != nil {
		return out, fmt.Errorf("script engine: %w", err)
	}
	out.Request, out.Tests = res.Request, res.Tests
	for k := range in.Locals {
		delete(in.Locals, k)
	}
	for k, val := range res.Locals {
		in.Locals[k] = val
	}
	return out, nil
}

// scriptError turns a goja error into a message that points at the script line.
func scriptError(err error) error {
	var ex *goja.Exception
	if errors.As(err, &ex) {
		msg := ex.Value().String()
		if obj, ok := ex.Value().(*goja.Object); ok {
			if m := obj.Get("message"); m != nil && !goja.IsUndefined(m) {
				name := "Error"
				if nm := obj.Get("name"); nm != nil && !goja.IsUndefined(nm) {
					name = nm.String()
				}
				msg = name + ": " + m.String()
			}
		}
		if stack := ex.Stack(); len(stack) > 0 {
			pos := stack[0].Position()
			if pos.Line > 0 && stack[0].SrcName() != "sandbox.js" {
				msg += fmt.Sprintf(" (line %d)", pos.Line)
			}
		}
		return errors.New(msg)
	}
	var ie *goja.InterruptedError
	if errors.As(err, &ie) {
		return fmt.Errorf("%v", ie.Value())
	}
	return err
}

// HeaderRows converts h to rows sorted by name.
func HeaderRows(h http.Header) []domain.KeyValue {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := []domain.KeyValue{}
	for _, k := range keys {
		for _, v := range h[k] {
			rows = append(rows, domain.KeyValue{Key: k, Value: v, Enabled: true})
		}
	}
	return rows
}

// natives are the Go functions the sandbox calls through __native.
type natives struct {
	vm   *goja.Runtime
	in   *Input
	out  *Output
	libs map[string]goja.Value
}

func (n *natives) throw(err error) { panic(n.vm.NewGoError(err)) }

func (n *natives) object() map[string]any {
	return map[string]any{
		"get": func(scope, name string) goja.Value {
			if v, ok := n.in.Vars.Get(Scope(scope), name); ok {
				return n.vm.ToValue(v)
			}
			return goja.Undefined()
		},
		"set": func(scope, name, value string) {
			if err := n.in.Vars.Set(Scope(scope), name, value); err != nil {
				n.throw(err)
			}
		},
		"unset": func(scope, name string) { n.in.Vars.Unset(Scope(scope), name) },
		"all":   func(scope string) map[string]any { return toAny(n.in.Vars.All(Scope(scope))) },
		"replaceIn": func(s string, locals map[string]any) string {
			l := map[string]string{}
			for k, v := range locals {
				l[k] = fmt.Sprint(v)
			}
			return environments.NewResolver(n.in.Vars.Resolved(l)).Resolve(s)
		},
		"log": func(level, msg string) {
			if len(n.out.Logs) >= MaxLogs {
				return
			}
			if len(msg) > maxLogLength {
				msg = msg[:maxLogLength] + "… (truncated)"
			}
			n.out.Logs = append(n.out.Logs, Log{Level: level, Message: msg})
			if len(n.out.Logs) == MaxLogs {
				n.out.Logs = append(n.out.Logs, Log{Level: "warn", Message: fmt.Sprintf("more than %d messages; the rest are not shown", MaxLogs)})
			}
		},
		"uuid": func() string { return uuid.NewString() },
		"randomBytes": func(size int) []any {
			if size < 0 || size > 65536 {
				n.throw(fmt.Errorf("getRandomValues: %d bytes requested, at most 65536 allowed", size))
			}
			b := make([]byte, size)
			_, _ = rand.Read(b)
			out := make([]any, size)
			for i, c := range b {
				out[i] = int(c)
			}
			return out
		},
		"btoa": func(s string) string {
			b := make([]byte, 0, len(s))
			for _, r := range s {
				if r > 0xff {
					n.throw(errors.New("btoa: the string contains characters outside of the Latin1 range"))
				}
				b = append(b, byte(r))
			}
			return base64.StdEncoding.EncodeToString(b)
		},
		"atob": func(s string) string {
			s = strings.Map(func(r rune) rune {
				if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
					return -1
				}
				return r
			}, s)
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				if b, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "=")); err != nil {
					n.throw(errors.New("atob: the string is not valid base64"))
				}
			}
			r := make([]rune, len(b))
			for i, c := range b {
				r[i] = rune(c)
			}
			return string(r)
		},
		"loadLib": func(name string) goja.Value {
			if v, ok := n.libs[name]; ok {
				return v
			}
			v, err := n.loadLib(name)
			if err != nil {
				n.throw(err)
			}
			if n.libs == nil {
				n.libs = map[string]goja.Value{}
			}
			n.libs[name] = v
			return v
		},
		"rsaEncrypt": func(keyPEM, data, padding, hashName, encoding string) string {
			out, err := rsaEncrypt(keyPEM, []byte(data), padding, hashName)
			if err != nil {
				n.throw(fmt.Errorf("pm.crypto.rsaEncrypt: %w", err))
			}
			return encode(out, encoding)
		},
		"rsaSign": func(keyPEM, data, padding, hashName, encoding string) string {
			out, err := rsaSign(keyPEM, []byte(data), padding, hashName)
			if err != nil {
				n.throw(fmt.Errorf("pm.crypto.rsaSign: %w", err))
			}
			return encode(out, encoding)
		},
	}
}

func (n *natives) loadLib(name string) (goja.Value, error) {
	prog, ok := libProgs[name]
	if !ok {
		return nil, fmt.Errorf("library %q is not bundled", name)
	}
	wrapper, err := n.vm.RunProgram(prog)
	if err != nil {
		return nil, err
	}
	fn, ok := goja.AssertFunction(wrapper)
	if !ok {
		return nil, fmt.Errorf("library %q did not load", name)
	}
	module := n.vm.NewObject()
	exports := n.vm.NewObject()
	if err := module.Set("exports", exports); err != nil {
		return nil, err
	}
	if _, err := fn(goja.Undefined(), module, exports); err != nil {
		return nil, fmt.Errorf("load %s: %w", name, err)
	}
	return module.Get("exports"), nil
}

func toAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func encode(b []byte, encoding string) string {
	if strings.EqualFold(encoding, "hex") {
		return hex.EncodeToString(b)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// ---- RSA ----

func hashFor(name string) (crypto.Hash, func() hash.Hash, error) {
	switch strings.ToLower(strings.ReplaceAll(name, "-", "")) {
	case "sha1":
		return crypto.SHA1, sha1.New, nil
	case "sha256", "":
		return crypto.SHA256, sha256.New, nil
	case "sha384":
		return crypto.SHA384, sha512.New384, nil
	case "sha512":
		return crypto.SHA512, sha512.New, nil
	}
	return 0, nil, fmt.Errorf("unsupported hash %q (use sha1, sha256, sha384 or sha512)", name)
}

// pemBlock decodes a PEM key. Keys pasted into variables often have literal
// "\n" sequences or no header at all, so both are accepted.
func pemBlock(key string) ([]byte, string, error) {
	key = strings.TrimSpace(strings.ReplaceAll(key, `\n`, "\n"))
	if block, _ := pem.Decode([]byte(key)); block != nil {
		return block.Bytes, block.Type, nil
	}
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(key), ""))
	if err != nil {
		return nil, "", errors.New("the key is not PEM or base64 DER")
	}
	return der, "", nil
}

func parsePublicKey(key string) (*rsa.PublicKey, error) {
	der, typ, err := pemBlock(key)
	if err != nil {
		return nil, err
	}
	if typ == "CERTIFICATE" {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		if pub, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return pub, nil
		}
		return nil, errors.New("the certificate does not contain an RSA key")
	}
	if pub, err := x509.ParsePKIXPublicKey(der); err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
		return nil, errors.New("the key is not an RSA key")
	}
	if pub, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return pub, nil
	}
	return nil, errors.New("could not parse the RSA public key (expected PUBLIC KEY, RSA PUBLIC KEY or CERTIFICATE)")
}

func parsePrivateKey(key string) (*rsa.PrivateKey, error) {
	der, _, err := pemBlock(key)
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if rsaKey, ok := k.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("the key is not an RSA key")
	}
	if k, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return k, nil
	}
	return nil, errors.New("could not parse the RSA private key (expected unencrypted PRIVATE KEY or RSA PRIVATE KEY)")
}

func rsaEncrypt(keyPEM string, data []byte, padding, hashName string) ([]byte, error) {
	pub, err := parsePublicKey(keyPEM)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(padding) {
	case "pkcs1", "":
		return rsa.EncryptPKCS1v15(rand.Reader, pub, data)
	case "oaep":
		_, newHash, err := hashFor(hashName)
		if err != nil {
			return nil, err
		}
		return rsa.EncryptOAEP(newHash(), rand.Reader, pub, data, nil)
	}
	return nil, fmt.Errorf("unsupported padding %q (use pkcs1 or oaep)", padding)
}

func rsaSign(keyPEM string, data []byte, padding, hashName string) ([]byte, error) {
	priv, err := parsePrivateKey(keyPEM)
	if err != nil {
		return nil, err
	}
	h, newHash, err := hashFor(hashName)
	if err != nil {
		return nil, err
	}
	hasher := newHash()
	hasher.Write(data)
	digest := hasher.Sum(nil)
	switch strings.ToLower(padding) {
	case "pkcs1", "":
		return rsa.SignPKCS1v15(rand.Reader, priv, h, digest)
	case "pss":
		return rsa.SignPSS(rand.Reader, priv, h, digest, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	}
	return nil, fmt.Errorf("unsupported padding %q (use pkcs1 or pss)", padding)
}
