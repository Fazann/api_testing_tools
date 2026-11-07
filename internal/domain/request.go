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

// MayModifyData reports whether running the request may change server state:
// an HTTP request with a destructive method, or a socket test that sends messages.
func (r *Request) MayModifyData() bool {
	if r.Kind.IsSocket() {
		return r.Socket.SendsMessages()
	}
	return IsDestructiveMethod(r.Method)
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
	// BodyFormData sends Form as multipart/form-data; its file fields upload files.
	BodyFormData BodyType = "form-data"
	// BodyURLEncoded sends the text fields of Form as application/x-www-form-urlencoded.
	BodyURLEncoded BodyType = "urlencoded"
	// BodyBinary sends the file at BodyFile as the whole body.
	BodyBinary BodyType = "binary"
)

// FormFieldType says whether a form field sends text or a file.
type FormFieldType string

const (
	FormText FormFieldType = "text"
	FormFile FormFieldType = "file"
)

// FormField is a row of a form-data or URL-encoded body. A file field uploads
// the file at File (an absolute path, may contain {{variables}}); Value is
// not used for it.
type FormField struct {
	Key   string        `json:"key"`
	Value string        `json:"value"`
	Type  FormFieldType `json:"type"`
	File  string        `json:"file,omitempty"`
	// ContentType overrides the part's Content-Type (form-data only). Empty
	// means guess it from the file name.
	ContentType string `json:"contentType,omitempty"`
	Enabled     bool   `json:"enabled"`
}

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
	// MaxUploadBytes caps the files one request uploads, in total.
	MaxUploadBytes    = 100 << 20
	MaxFilePathLength = 4_096
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

// Request is a saved request definition with its assertions. Kind selects
// HTTP (the default), WebSocket or Socket.IO; socket requests use URL,
// headers, query, auth, timeout and the pre-request script like HTTP
// requests, plus Socket. Their test is Socket.Steps; Method is always GET (the
// WebSocket handshake) and Body, Assertions and the post-response script are
// not used.
type Request struct {
	ID           string `json:"id"`
	CollectionID string `json:"collectionId"`
	ParentID     string `json:"parentId"`
	Name         string `json:"name"`
	// Kind is KindHTTP when empty, so older exports keep working.
	Kind        RequestKind `json:"kind"`
	Method      string      `json:"method"`
	URLTemplate string      `json:"url"`
	Headers     []KeyValue  `json:"headers"`
	Query       []KeyValue  `json:"query"`
	Body        string      `json:"body"`
	BodyType    BodyType    `json:"bodyType"`
	// Form holds the fields of a form-data or URL-encoded body.
	Form []FormField `json:"form"`
	// BodyFile is the file a binary body sends (absolute path, may contain {{variables}}).
	BodyFile        string      `json:"bodyFile,omitempty"`
	Auth            Auth        `json:"auth"`
	TimeoutMs       int         `json:"timeoutMs"`
	FollowRedirects bool        `json:"followRedirects"`
	SortOrder       int         `json:"sortOrder"`
	Assertions      []Assertion `json:"assertions"`
	// PreRequestScript runs before variables are resolved and the request is
	// sent; PostResponseScript runs after the response arrives (see internal/scripting).
	PreRequestScript   string `json:"preRequestScript"`
	PostResponseScript string `json:"postResponseScript"`
	// Socket is set for WebSocket and Socket.IO requests.
	Socket    *SocketConfig `json:"socket,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// Normalize fills defaults so that partially specified requests behave predictably.
func (r *Request) Normalize() {
	if r.Kind == "" {
		r.Kind = KindHTTP
	}
	r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
	if r.Method == "" || r.Kind.IsSocket() {
		r.Method = http.MethodGet
	}
	if r.Kind.IsSocket() {
		if r.Socket == nil {
			r.Socket = &SocketConfig{}
		}
		r.Socket.normalize(r.Kind)
	} else {
		r.Socket = nil
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
	if r.Form == nil {
		r.Form = []FormField{}
	}
	for i := range r.Form {
		if r.Form[i].Type == "" {
			r.Form[i].Type = FormText
		}
		if r.Form[i].Type != FormFile {
			r.Form[i].File, r.Form[i].ContentType = "", ""
		}
	}
	r.BodyFile = strings.TrimSpace(r.BodyFile)
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
	switch r.Kind {
	case KindHTTP:
	case KindWebSocket, KindSocketIO:
		if r.Socket == nil {
			return fmt.Errorf("socket settings are missing")
		}
		if err := r.Socket.validate(r.Kind); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported request kind %q", r.Kind)
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
	case BodyNone, BodyJSON, BodyText, BodyFormData, BodyURLEncoded, BodyBinary:
	default:
		return fmt.Errorf("unsupported body type %q", r.BodyType)
	}
	if len(r.Body) > MaxBodyLength {
		return fmt.Errorf("request body must be at most %d bytes", MaxBodyLength)
	}
	if err := validateForm(r.Form); err != nil {
		return err
	}
	if len(r.BodyFile) > MaxFilePathLength {
		return fmt.Errorf("body file path must be at most %d characters", MaxFilePathLength)
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

func validateForm(fields []FormField) error {
	if len(fields) > MaxKeyValuePairs {
		return fmt.Errorf("at most %d form fields are allowed", MaxKeyValuePairs)
	}
	for _, f := range fields {
		switch f.Type {
		case FormText, FormFile:
		default:
			return fmt.Errorf("form field %q: unsupported type %q", f.Key, f.Type)
		}
		if strings.TrimSpace(f.Key) == "" && f.Enabled && (f.Value != "" || f.File != "") {
			return fmt.Errorf("form field name is required")
		}
		if len(f.Value) > MaxBodyLength {
			return fmt.Errorf("form field %q must be at most %d bytes", f.Key, MaxBodyLength)
		}
		if len(f.File) > MaxFilePathLength {
			return fmt.Errorf("form field %q: file path must be at most %d characters", f.Key, MaxFilePathLength)
		}
		if strings.ContainsAny(f.ContentType, "\r\n") {
			return fmt.Errorf("form field %q: content type must not contain line breaks", f.Key)
		}
	}
	return nil
}

// UploadsFiles reports whether sending the request reads files from disk.
func (r *Request) UploadsFiles() bool {
	switch r.BodyType {
	case BodyBinary:
		return r.BodyFile != ""
	case BodyFormData:
		for _, f := range r.Form {
			if f.Enabled && f.Type == FormFile && f.File != "" {
				return true
			}
		}
	}
	return false
}

// ClearFilePaths forgets every file the request uploads, so the user has to
// choose them again, and returns how many were cleared. Imported collections
// are untrusted: a path chosen by someone else could send any local file.
func (r *Request) ClearFilePaths() int {
	n := 0
	if r.BodyFile != "" {
		r.BodyFile = ""
		n++
	}
	for i := range r.Form {
		if r.Form[i].File != "" {
			r.Form[i].File = ""
			n++
		}
	}
	return n
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
