package domain

import (
	"fmt"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

// HTTP methods supported by the request builder.
var SupportedMethods = []string{
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
	http.MethodDelete, http.MethodHead, http.MethodOptions,
}

// IsDestructiveMethod reports whether a method may modify server state.
func IsDestructiveMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

type BodyType string

const (
	BodyNone BodyType = "none"
	BodyJSON BodyType = "json"
	BodyText BodyType = "text"
)

type AuthType string

const (
	AuthNone   AuthType = "none"
	AuthBearer AuthType = "bearer"
	AuthBasic  AuthType = "basic"
	AuthAPIKey AuthType = "apikey"
	// AuthInherit uses the auth configured on the request's collection.
	AuthInherit AuthType = "inherit"
)

// APIKeyLocation says where an API key is sent.
type APIKeyLocation string

const (
	APIKeyHeader APIKeyLocation = "header"
	APIKeyQuery  APIKeyLocation = "query"
)

const (
	DefaultTimeoutMs = 30_000
	MaxTimeoutMs     = 10 * 60_000
	MaxURLLength     = 8_192
	MaxBodyLength    = 5 << 20
	MaxKeyValuePairs = 200
	MaxScriptLength  = 100_000
)

// KeyValue is a header or query parameter row.
type KeyValue struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

// Auth configures request authentication. Credential fields usually hold
// {{variable}} references to secret environment variables.
type Auth struct {
	Type     AuthType       `json:"type"`
	Token    string         `json:"token,omitempty"`
	Username string         `json:"username,omitempty"`
	Password string         `json:"password,omitempty"`
	KeyName  string         `json:"keyName,omitempty"`
	KeyValue string         `json:"keyValue,omitempty"`
	KeyIn    APIKeyLocation `json:"keyIn,omitempty"`
}

// Credentials returns the credential values that must be treated as secret.
func (a Auth) Credentials() []string {
	switch a.Type {
	case AuthBearer:
		return []string{a.Token}
	case AuthBasic:
		return []string{a.Password}
	case AuthAPIKey:
		return []string{a.KeyValue}
	}
	return nil
}

// Effective returns the auth a request actually uses: its own, or the
// collection's when it inherits.
func (a Auth) Effective(collection Auth) Auth {
	if a.Type != AuthInherit {
		return a
	}
	if collection.Type == "" || collection.Type == AuthInherit {
		return Auth{Type: AuthNone}
	}
	return collection
}

// Request is a saved HTTP request definition with its assertions.
type Request struct {
	ID              string      `json:"id"`
	CollectionID    string      `json:"collectionId"`
	ParentID        string      `json:"parentId"`
	Name            string      `json:"name"`
	Method          string      `json:"method"`
	URLTemplate     string      `json:"url"`
	Headers         []KeyValue  `json:"headers"`
	Query           []KeyValue  `json:"query"`
	Body            string      `json:"body"`
	BodyType        BodyType    `json:"bodyType"`
	Auth            Auth        `json:"auth"`
	TimeoutMs       int         `json:"timeoutMs"`
	FollowRedirects bool        `json:"followRedirects"`
	SortOrder       int         `json:"sortOrder"`
	Assertions      []Assertion `json:"assertions"`
	// PreRequestScript runs before variables are resolved and the request is
	// sent; PostResponseScript runs after the response arrives (see internal/scripting).
	PreRequestScript   string    `json:"preRequestScript"`
	PostResponseScript string    `json:"postResponseScript"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// Normalize fills defaults so that partially specified requests behave predictably.
func (r *Request) Normalize() {
	r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	r.URLTemplate = strings.TrimSpace(r.URLTemplate)
	if r.BodyType == "" {
		r.BodyType = BodyNone
	}
	if r.Auth.Type == "" {
		r.Auth.Type = AuthNone
	}
	if r.Auth.Type == AuthAPIKey && r.Auth.KeyIn == "" {
		r.Auth.KeyIn = APIKeyHeader
	}
	if r.TimeoutMs <= 0 {
		r.TimeoutMs = DefaultTimeoutMs
	}
	if r.Headers == nil {
		r.Headers = []KeyValue{}
	}
	if r.Query == nil {
		r.Query = []KeyValue{}
	}
	if r.Assertions == nil {
		r.Assertions = []Assertion{}
	}
}

// Validate checks the request definition. It does not resolve variables, so
// the URL is only checked for length and presence here.
func (r *Request) Validate() error {
	if err := ValidateName("request name", r.Name); err != nil {
		return err
	}
	if !isSupportedMethod(r.Method) {
		return fmt.Errorf("unsupported HTTP method %q", r.Method)
	}
	if r.URLTemplate == "" {
		return fmt.Errorf("request URL is required")
	}
	if len(r.URLTemplate) > MaxURLLength {
		return fmt.Errorf("request URL must be at most %d characters", MaxURLLength)
	}
	switch r.BodyType {
	case BodyNone, BodyJSON, BodyText:
	default:
		return fmt.Errorf("unsupported body type %q", r.BodyType)
	}
	if len(r.Body) > MaxBodyLength {
		return fmt.Errorf("request body must be at most %d bytes", MaxBodyLength)
	}
	if r.TimeoutMs < 0 || r.TimeoutMs > MaxTimeoutMs {
		return fmt.Errorf("timeout must be between 1 and %d ms", MaxTimeoutMs)
	}
	if err := validatePairs("header", r.Headers, true); err != nil {
		return err
	}
	if err := validatePairs("query parameter", r.Query, false); err != nil {
		return err
	}
	if err := r.Auth.Validate(); err != nil {
		return err
	}
	if err := validateScripts(r.PreRequestScript, r.PostResponseScript); err != nil {
		return err
	}
	for i := range r.Assertions {
		if err := r.Assertions[i].Validate(); err != nil {
			return fmt.Errorf("assertion %d: %w", i+1, err)
		}
	}
	return nil
}

func (a *Auth) Validate() error {
	switch a.Type {
	case AuthNone, AuthBearer, AuthBasic, AuthInherit:
	case AuthAPIKey:
		if strings.TrimSpace(a.KeyName) == "" {
			return fmt.Errorf("API key name is required")
		}
		if a.KeyIn != APIKeyHeader && a.KeyIn != APIKeyQuery {
			return fmt.Errorf("API key location must be header or query")
		}
		if a.KeyIn == APIKeyHeader && !isValidHeaderName(a.KeyName) {
			return fmt.Errorf("API key header name %q is invalid", a.KeyName)
		}
	default:
		return fmt.Errorf("unsupported auth type %q", a.Type)
	}
	return nil
}

func validateScripts(pre, post string) error {
	if len(pre) > MaxScriptLength {
		return fmt.Errorf("pre-request script must be at most %d characters", MaxScriptLength)
	}
	if len(post) > MaxScriptLength {
		return fmt.Errorf("post-response script must be at most %d characters", MaxScriptLength)
	}
	return nil
}

func validatePairs(kind string, pairs []KeyValue, header bool) error {
	if len(pairs) > MaxKeyValuePairs {
		return fmt.Errorf("at most %d %ss are allowed", MaxKeyValuePairs, kind)
	}
	for _, p := range pairs {
		if !p.Enabled && p.Key == "" {
			continue
		}
		if strings.TrimSpace(p.Key) == "" {
			if p.Value == "" {
				continue
			}
			return fmt.Errorf("%s name is required", kind)
		}
		// Header names may contain variables, which are checked again after resolution.
		if header && !strings.Contains(p.Key, "{{") && !isValidHeaderName(p.Key) {
			return fmt.Errorf("header name %q is invalid", p.Key)
		}
		if header && strings.ContainsAny(p.Value, "\r\n") {
			return fmt.Errorf("header %q value must not contain line breaks", p.Key)
		}
	}
	return nil
}

func isSupportedMethod(m string) bool {
	for _, s := range SupportedMethods {
		if s == m {
			return true
		}
	}
	return false
}

func isValidHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c > 127 || !isTokenChar(byte(c)) {
			return false
		}
	}
	return textproto.CanonicalMIMEHeaderKey(name) != ""
}

// IsValidHeaderName reports whether name is a legal HTTP header field name.
func IsValidHeaderName(name string) bool { return isValidHeaderName(name) }

func isTokenChar(c byte) bool {
	if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
		return true
	}
	return strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0
}
