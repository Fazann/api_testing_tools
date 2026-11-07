package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"api-test-kits/internal/cli"
	"api-test-kits/internal/domain"
)

// uploadServer answers 201 with the uploaded file's name and content, or 400.
func uploadServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, hdr, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		data, _ := io.ReadAll(file)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]string{"name": hdr.Filename, "content": string(data), "title": r.FormValue("title")})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func uploadCollection(file, bodyFile string) string {
	doc := map[string]any{
		"format": "api-test-kits.collection", "version": 1,
		"collection": map[string]any{"name": "Uploads", "variables": []any{}},
		"requests": []any{
			map[string]any{
				"id": "r1", "name": "Upload", "method": "POST", "url": "{{base_url}}/upload", "bodyType": "form-data",
				"form": []any{
					map[string]any{"key": "title", "value": "hello", "type": "text", "enabled": true},
					map[string]any{"key": "file", "type": "file", "file": file, "enabled": true},
				},
				"assertions": []any{
					map[string]any{"kind": "status", "operator": "eq", "expected": 201, "enabled": true},
					map[string]any{"kind": "json_path", "path": "content", "operator": "eq", "expected": "file body", "enabled": true},
					map[string]any{"kind": "json_path", "path": "title", "operator": "eq", "expected": "hello", "enabled": true},
				},
			},
		},
	}
	if bodyFile != "" {
		doc["requests"] = append(doc["requests"].([]any), map[string]any{"id": "r2", "name": "Binary", "method": "PUT",
			"url": "{{base_url}}/blob", "bodyType": "binary", "bodyFile": bodyFile})
	}
	data, _ := json.Marshal(doc)
	return string(data)
}

// Importing someone else's collection must not keep paths that would upload
// local files; the fields stay so the user only re-picks the files.
func TestImportClearsUploadPaths(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	res, err := f.svc.ImportCollection(ctx, []byte(uploadCollection("/home/me/.ssh/id_rsa", "/etc/passwd")), "x.json")
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesCleared != 2 {
		t.Fatalf("files cleared: %d", res.FilesCleared)
	}
	tree, err := f.svc.GetCollectionTree(ctx, res.Collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range tree.Requests {
		if r.BodyFile != "" {
			t.Fatalf("%s kept its body file", r.Name)
		}
		for _, field := range r.Form {
			if field.File != "" {
				t.Fatalf("%s kept the file of field %s", r.Name, field.Key)
			}
		}
		if r.Name == "Upload" && (len(r.Form) != 2 || r.Form[1].Type != domain.FormFile || r.BodyType != domain.BodyFormData) {
			t.Fatalf("form fields were not kept: %+v", r.Form)
		}
	}
}

// A request with uploads is saved, reloaded, exported and sent intact.
func TestUploadRequestRoundTripAndSend(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	srv := uploadServer(t)
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte("file body"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := domain.Request{CollectionID: f.tree.Collection.ID, Name: "Upload", Method: "POST",
		URLTemplate: srv.URL + "/upload", BodyType: domain.BodyFormData,
		Form: []domain.FormField{
			{Key: "title", Value: "hello", Type: domain.FormText, Enabled: true},
			{Key: "file", Type: domain.FormFile, File: path, ContentType: "text/plain", Enabled: true},
		}}
	saved, err := f.svc.SaveRequest(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.GetRequest(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Form) != 2 || got.Form[1].File != path || got.Form[1].ContentType != "text/plain" {
		t.Fatalf("form not stored: %+v", got.Form)
	}
	res, err := f.svc.SendRequest(ctx, "", got, f.env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Response == nil || res.Response.StatusCode != 201 || !strings.Contains(res.Response.Body, `"content":"file body"`) {
		t.Fatalf("send: %+v %s", res.Response, res.Error)
	}
	if !strings.Contains(res.Request.Body, "file=@report.txt (9 B, text/plain)") {
		t.Fatalf("history body: %q", res.Request.Body)
	}
	data, err := f.svc.ExportCollection(ctx, f.tree.Collection.ID, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"bodyType": "form-data"`) || !strings.Contains(string(data), "report.txt") {
		t.Fatal("export lost the form body")
	}
}

// The CLI trusts the collection file it is given and reads relative upload
// paths from the file's directory, so tests and fixtures can live together.
func TestCLIUploadsRelativeFiles(t *testing.T) {
	srv := uploadServer(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "files", "report.txt"), []byte("file body"), 0o600); err != nil {
		t.Fatal(err)
	}
	collection := filepath.Join(dir, "uploads.collection.json")
	if err := os.WriteFile(collection, []byte(uploadCollection("files/report.txt", "")), 0o600); err != nil {
		t.Fatal(err)
	}
	res := runCLI(t, nil, "run", collection, "--var", "base_url="+srv.URL)
	if res.code != cli.ExitPassed {
		t.Fatalf("exit %d\n%s\n%s", res.code, res.stdout, res.stderr)
	}
}
