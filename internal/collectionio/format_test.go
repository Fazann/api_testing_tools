package collectionio

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
)

func sampleTree() domain.CollectionTree {
	return domain.CollectionTree{
		Collection: domain.Collection{ID: "c1", Name: "Wallet",
			PreRequestScript: "pm.variables.set('a', 1);", PostResponseScript: "pm.test('ok', () => {});",
			Auth: domain.Auth{Type: domain.AuthBearer, Token: "collection-literal-token"},
			Variables: []domain.Variable{
				{Key: "base_url", Value: "http://collection-base", Enabled: true},
				{Key: "api_secret", Value: "collection-var-secret", Secret: true, Enabled: true},
			}},
		Folders: []domain.Folder{{ID: "f1", CollectionID: "c1", Name: "Folder"}},
		Requests: []domain.Request{
			{ID: "r1", CollectionID: "c1", ParentID: "f1", Name: "Literal", Method: "GET", URLTemplate: "http://x",
				Auth:    domain.Auth{Type: domain.AuthBearer, Token: "literal-token-123"},
				Headers: []domain.KeyValue{{Key: "Cookie", Value: "sid=abc", Enabled: true}, {Key: "Accept", Value: "*/*", Enabled: true}}},
			{ID: "r2", CollectionID: "c1", Name: "Var", Method: "GET", URLTemplate: "http://x",
				Auth: domain.Auth{Type: domain.AuthBearer, Token: "{{token}}"}, PreRequestScript: "console.log(1);"},
		},
	}
}

func sampleEnv() domain.Environment {
	return domain.Environment{ID: "e1", Name: "Local", Variables: []domain.Variable{
		{Key: "base_url", Value: "http://localhost", Enabled: true},
		{Key: "token", Value: "env-secret-xyz", Secret: true, Enabled: true},
	}}
}

func TestScanSecrets(t *testing.T) {
	f := ScanSecrets(sampleTree(), []domain.Environment{sampleEnv()})
	if len(f) != 5 {
		t.Fatalf("expected collection token and variable, literal token, cookie header and secret variable, got %+v", f)
	}
}

func TestExportStripsSecretsByDefault(t *testing.T) {
	var buf bytes.Buffer
	doc := Export(sampleTree(), []domain.Environment{sampleEnv()}, false, "test", time.Now())
	if err := Write(&buf, doc); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, secret := range []string{"literal-token-123", "sid=abc", "env-secret-xyz", "collection-literal-token",
		"collection-var-secret"} {
		if strings.Contains(out, secret) {
			t.Fatalf("export leaked %q", secret)
		}
	}
	for _, keep := range []string{"{{token}}", "http://localhost", "http://collection-base", `"secret": true`} {
		if !strings.Contains(out, keep) {
			t.Fatalf("export lost %q", keep)
		}
	}

	buf.Reset()
	_ = Write(&buf, Export(sampleTree(), []domain.Environment{sampleEnv()}, true, "test", time.Now()))
	if !strings.Contains(buf.String(), "env-secret-xyz") || !strings.Contains(buf.String(), `"secretsIncluded": true`) {
		t.Fatal("explicit opt-in should include secrets and say so")
	}
}

func TestRoundTripAssignsFreshIDs(t *testing.T) {
	var buf bytes.Buffer
	_ = Write(&buf, Export(sampleTree(), []domain.Environment{sampleEnv()}, false, "test", time.Now()))
	imp, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if imp.Tree.Collection.ID == "c1" || imp.Tree.Requests[0].ID == "r1" || imp.Tree.Folders[0].ID == "f1" {
		t.Fatal("imports must never reuse IDs")
	}
	if imp.Tree.Requests[0].ParentID != imp.Tree.Folders[0].ID {
		t.Fatal("folder relationship lost")
	}
	if v := imp.Tree.Collection.Variables; len(v) != 2 || v[0].Value != "http://collection-base" || v[1].Value != "" {
		t.Fatalf("collection variables should round trip with secrets blanked: %+v", v)
	}
	if c := imp.Tree.Collection; c.PreRequestScript == "" || c.PostResponseScript == "" || imp.Tree.Requests[1].PreRequestScript != "console.log(1);" {
		t.Fatalf("scripts should round trip: %+v", c)
	}
	if len(imp.Environments) != 1 || imp.Environments[0].ID != "" {
		t.Fatal("environments should import without IDs")
	}
}

func TestReadRejectsUntrustedInput(t *testing.T) {
	cases := map[string]string{
		"not json":      `{{{`,
		"wrong format":  `{"format":"other","version":1}`,
		"future":        `{"format":"api-test-kits.collection","version":99}`,
		"empty name":    `{"format":"api-test-kits.collection","version":1,"collection":{"name":""}}`,
		"bad method":    `{"format":"api-test-kits.collection","version":1,"collection":{"name":"x"},"requests":[{"name":"r","method":"TRACE","url":"http://x"}]}`,
		"bad assertion": `{"format":"api-test-kits.collection","version":1,"collection":{"name":"x"},"requests":[{"name":"r","method":"GET","url":"http://x","assertions":[{"kind":"json_path","path":"a","operator":"matches","expected":"\"(\"","enabled":true}]}]}`,
		"bad header":    `{"format":"api-test-kits.collection","version":1,"collection":{"name":"x"},"requests":[{"name":"r","method":"GET","url":"http://x","headers":[{"key":"Bad Header","value":"v","enabled":true}]}]}`,
		"wrong types":   `{"format":"api-test-kits.collection","version":1,"collection":{"name":5}}`,
	}
	for name, input := range cases {
		if _, err := Read(strings.NewReader(input)); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
	huge := strings.NewReader(strings.Repeat(" ", MaxFileBytes+10))
	if _, err := Read(huge); err == nil {
		t.Fatal("oversized file must be rejected")
	}
}

func TestReadDropsUnknownFolderParents(t *testing.T) {
	in := `{"format":"api-test-kits.collection","version":1,"collection":{"name":"x"},
	  "requests":[{"name":"r","method":"GET","url":"http://x","parentId":"ghost"}]}`
	imp, err := Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if imp.Tree.Requests[0].ParentID != "" {
		t.Fatal("requests with unknown parents should move to the top level")
	}
}

func TestBaselinesRoundTripWithNewRequestIDs(t *testing.T) {
	doc := Export(sampleTree(), nil, false, "test", time.Now())
	doc.Baselines = []Baseline{
		{RequestID: "r1", Environment: "Staging", Mode: domain.BaselineSchema, Snapshot: []byte(`{"type":["object"]}`)},
		{RequestID: "r2", Snapshot: []byte(`{"a":1}`), IgnoredPaths: []string{"meta.id"}, Tolerance: 0.01},
	}
	var buf bytes.Buffer
	if err := Write(&buf, doc); err != nil {
		t.Fatal(err)
	}
	imp, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Baselines) != 2 {
		t.Fatalf("expected 2 baselines, got %d", len(imp.Baselines))
	}
	b0, b1 := imp.Baselines[0], imp.Baselines[1]
	if b0.Baseline.RequestID != imp.Tree.Requests[0].ID || b1.Baseline.RequestID != imp.Tree.Requests[1].ID {
		t.Fatal("baselines must point at the regenerated request IDs")
	}
	if b0.Environment != "Staging" || b0.Baseline.Mode != domain.BaselineSchema || b1.Baseline.Mode != domain.BaselineValues {
		t.Fatalf("unexpected baselines %+v", imp.Baselines)
	}
	if b1.Baseline.Tolerance != 0.01 || b1.Baseline.IgnoredPaths[0] != "meta.id" {
		t.Fatalf("rules lost: %+v", b1.Baseline)
	}
}

func TestBaselinesAreValidated(t *testing.T) {
	cases := map[string]Baseline{
		"does not match a request": {RequestID: "nope", Snapshot: []byte(`{}`)},
		"unknown mode":             {RequestID: "r1", Mode: "magic", Snapshot: []byte(`{}`)},
		"valid JSON":               {RequestID: "r1", Snapshot: []byte(`{`)},
		"tolerance":                {RequestID: "r1", Snapshot: []byte(`{}`), Tolerance: -1},
	}
	for want, b := range cases {
		doc := Export(sampleTree(), nil, false, "test", time.Now())
		doc.Baselines = []Baseline{b}
		var buf bytes.Buffer
		Write(&buf, doc)
		if _, err := Read(&buf); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
	}
	doc := Export(sampleTree(), nil, false, "test", time.Now())
	doc.Baselines = []Baseline{{RequestID: "r1", Snapshot: []byte(`{}`)}, {RequestID: "r1", Snapshot: []byte(`{}`)}}
	var buf bytes.Buffer
	Write(&buf, doc)
	if _, err := Read(&buf); err == nil || !strings.Contains(err.Error(), "two baselines") {
		t.Errorf("duplicate baseline: got %v", err)
	}
}

func TestImportAcceptsLegacyFormatNames(t *testing.T) {
	for _, format := range LegacyFormatNames {
		in := `{"format":"` + format + `","version":1,"collection":{"name":"old export"}}`
		got, err := Read(strings.NewReader(in))
		if err != nil {
			t.Fatalf("%s export must still import: %v", format, err)
		}
		if got.Tree.Collection.Name != "old export" {
			t.Fatalf("name = %q", got.Tree.Collection.Name)
		}
	}
}
