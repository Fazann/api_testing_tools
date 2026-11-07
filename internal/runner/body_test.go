package runner

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"api-test-kits/internal/domain"
)

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func uploadEnv(base, dir string) *domain.Environment {
	env := testEnv(base)
	env.Variables = append(env.Variables, domain.Variable{Key: "dir", Value: dir, Enabled: true})
	return env
}

// The server sees a well-formed multipart body: text fields, files with their
// names and types, in order; disabled rows are left out.
func TestFormDataUploadsFiles(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake image")
	photo := writeFile(t, "beach photo.png", png)
	notes := writeFile(t, "notes.bin", []byte("# hi"))

	type part struct{ name, file, ctype, data string }
	var got []part
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			data, _ := io.ReadAll(p)
			got = append(got, part{p.FormName(), p.FileName(), p.Header.Get("Content-Type"), string(data)})
		}
		w.WriteHeader(201)
	}))
	defer srv.Close()

	req := domain.Request{
		Name: "upload", Method: "POST", URLTemplate: "{{base_url}}/upload", BodyType: domain.BodyFormData,
		// A user-set multipart Content-Type has no boundary; the app's replaces it.
		Headers: []domain.KeyValue{{Key: "Content-Type", Value: "multipart/form-data", Enabled: true}},
		Form: []domain.FormField{
			{Key: "title", Value: "{{token}}", Enabled: true},
			{Key: "photo", Type: domain.FormFile, File: photo, Enabled: true},
			{Key: "skipped", Value: "x"},
			{Key: "doc", Type: domain.FormFile, File: "{{dir}}/notes.bin", ContentType: "text/markdown", Enabled: true},
		},
		Assertions: []domain.Assertion{{Kind: domain.AssertStatus, Operator: domain.OpEq, Expected: []byte("201"), Enabled: true}},
	}
	res := New(client(t)).Run(context.Background(), req, uploadEnv(srv.URL, filepath.Dir(notes)), nil)
	if res.Status != domain.StatusPassed {
		t.Fatalf("status %s: %s %+v", res.Status, res.Error, res.AssertionResults)
	}
	if mt, params, _ := mime.ParseMediaType(contentType); mt != "multipart/form-data" || params["boundary"] == "" {
		t.Fatalf("content type %q", contentType)
	}
	want := []part{
		{"title", "", "", "tok-secret-1"},
		{"photo", "beach photo.png", "image/png", string(png)},
		{"doc", "notes.bin", "text/markdown", "# hi"},
	}
	if len(got) != len(want) {
		t.Fatalf("parts: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	// History shows files by name and size, never their bytes, and secrets stay redacted.
	sum := res.Request.Body
	if !strings.Contains(sum, "photo=@beach photo.png (18 B, image/png)") || strings.Contains(sum, "PNG") ||
		strings.Contains(sum, "tok-secret-1") {
		t.Fatalf("summary: %q", sum)
	}
}

func TestURLEncodedBody(t *testing.T) {
	req := domain.Request{
		Name: "x", Method: "POST", URLTemplate: "http://h/login", BodyType: domain.BodyURLEncoded,
		Form: []domain.FormField{
			{Key: "user", Value: "ann smith", Enabled: true},
			{Key: "pass", Value: "a&b=c", Enabled: true},
			{Key: "off", Value: "1"},
		},
	}
	p, err := Prepare(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(p.HTTP.Body) != "user=ann+smith&pass=a%26b%3Dc" {
		t.Fatalf("body %q", p.HTTP.Body)
	}
	if ct := p.HTTP.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Fatalf("content type %q", ct)
	}
	req.Form[0].Type, req.Form[0].File = domain.FormFile, "/tmp/x"
	if _, err := Prepare(req, nil); err == nil || !strings.Contains(err.Error(), "use form-data") {
		t.Fatalf("a file in a URL-encoded body should fail, got %v", err)
	}
}

func TestBinaryBody(t *testing.T) {
	data := bytes.Repeat([]byte{0, 1, 2, 255}, 1000)
	path := writeFile(t, "a.pdf", data)
	req := domain.Request{Name: "x", Method: "PUT", URLTemplate: "http://h/f", BodyType: domain.BodyBinary, BodyFile: path}
	p, err := Prepare(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(p.HTTP.Body, data) || p.HTTP.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("body %d bytes, content type %q", len(p.HTTP.Body), p.HTTP.Header.Get("Content-Type"))
	}
	if p.Summary.Body != "@a.pdf (3.9 KB, application/pdf)" {
		t.Fatalf("summary %q", p.Summary.Body)
	}
	// A Content-Type header set by the user wins.
	req.Headers = []domain.KeyValue{{Key: "Content-Type", Value: "application/octet-stream", Enabled: true}}
	if p, _ := Prepare(req, nil); p.HTTP.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("content type %q", p.HTTP.Header.Get("Content-Type"))
	}
	// GET and HEAD never send a body, so the file is not even read.
	req.Method, req.BodyFile = "GET", "/does/not/exist"
	if p, err := Prepare(req, nil); err != nil || p.HTTP.Body != nil {
		t.Fatalf("GET: %v %v", err, p)
	}
}

func TestUploadErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		file string
		want string
	}{
		{"no file chosen", "", "choose a file"},
		{"missing", filepath.Join(dir, "nope.txt"), "cannot open the file"},
		{"relative", "nope.txt", "must be absolute"},
		{"directory", dir, "not a regular file"},
	}
	for _, c := range cases {
		req := domain.Request{Name: "x", Method: "POST", URLTemplate: "http://h", BodyType: domain.BodyBinary, BodyFile: c.file}
		if _, err := Prepare(req, nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
	// An undefined variable in the path is reported like any other.
	req := domain.Request{Name: "x", Method: "POST", URLTemplate: "http://h", BodyType: domain.BodyFormData,
		Form: []domain.FormField{{Key: "f", Type: domain.FormFile, File: "{{fixtures}}/a.png", Enabled: true}}}
	if _, err := Prepare(req, nil); err == nil {
		t.Fatal("expected an error")
	} else if _, ok := err.(*MissingVariablesError); !ok {
		t.Fatalf("expected MissingVariablesError, got %v", err)
	}
}

func TestUploadSizeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// A sparse file: the limit is checked before anything is read.
	if err := f.Truncate(domain.MaxUploadBytes/2 + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	req := domain.Request{Name: "x", Method: "POST", URLTemplate: "http://h", BodyType: domain.BodyFormData,
		Form: []domain.FormField{
			{Key: "a", Type: domain.FormFile, File: path, Enabled: true},
			{Key: "b", Type: domain.FormFile, File: path, Enabled: true},
		}}
	if _, err := Prepare(req, nil); err == nil || !strings.Contains(err.Error(), "in total") {
		t.Fatalf("expected the total size limit, got %v", err)
	}
}

// JSON and text bodies are sent exactly as before.
func TestTextBodiesUnchanged(t *testing.T) {
	for _, c := range []struct {
		typ   domain.BodyType
		ctype string
	}{{domain.BodyJSON, "application/json"}, {domain.BodyText, "text/plain; charset=utf-8"}} {
		req := domain.Request{Name: "x", Method: "POST", URLTemplate: "http://h", BodyType: c.typ, Body: `{"a":"{{token}}"}`}
		p, err := Prepare(req, testEnv("http://h"))
		if err != nil {
			t.Fatal(err)
		}
		if string(p.HTTP.Body) != `{"a":"tok-secret-1"}` || p.HTTP.Header.Get("Content-Type") != c.ctype {
			t.Fatalf("%s: %q %q", c.typ, p.HTTP.Body, p.HTTP.Header.Get("Content-Type"))
		}
		if strings.Contains(p.Summary.Body, "tok-secret-1") || !strings.Contains(p.BodyText, "tok-secret-1") {
			t.Fatalf("%s: summary %q", c.typ, p.Summary.Body)
		}
	}
}
