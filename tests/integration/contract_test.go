package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"api-test-kits/internal/domain"
)

const contractSpec = `{
  "openapi": "3.0.3",
  "info": {"title": "Pets", "version": "1.0"},
  "servers": [{"url": "http://localhost:9/v1"}],
  "paths": {
    "/pets/{petId}": {"get": {"summary": "Get a pet", "parameters": [{"name": "petId", "in": "path", "required": true, "example": 7}],
      "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Pet"}}}}}}},
    "/owners": {"get": {"summary": "List owners", "responses": {"200": {"description": "ok"}}}}
  },
  "components": {"schemas": {"Pet": {"type": "object", "required": ["id", "price"], "properties": {
    "id": {"type": "integer"}, "price": {"type": "number"}, "status": {"type": "string", "enum": ["available", "sold"]}}}}}
}`

// contractResults returns the contract findings of a run as "Request: path: message".
func contractResults(run domain.Run) (passed, failed []string) {
	for _, r := range run.Results {
		for _, a := range r.AssertionResults {
			if a.Kind != domain.AssertContract {
				continue
			}
			line := r.Request.Name + ": " + a.Message
			if a.Path != "" {
				line = r.Request.Name + ": " + a.Path + ": " + a.Message
			}
			if a.Passed {
				passed = append(passed, line)
			} else {
				failed = append(failed, line)
			}
		}
	}
	return passed, failed
}

func TestContractCheckOnImportRunExportDuplicateAndRemove(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Drifted from the spec: price is a string and there is an extra field.
		_, _ = w.Write([]byte(`{"id": 7, "price": "19.90", "status": "sold", "nickname": "Rex", "secret_note": "` + testToken + `"}`))
	}))
	t.Cleanup(srv.Close)

	imp, err := f.svc.ImportCollection(ctx, []byte(contractSpec), "pets.json")
	if err != nil {
		t.Fatal(err)
	}
	if !imp.Contract {
		t.Fatal("importing an OpenAPI file should attach it as the contract")
	}
	info, err := f.svc.GetContract(ctx, imp.Collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Attached || !info.Enabled || info.AllowUndocumented || info.SourceName != "pets.json" || info.Title != "Pets" || info.Operations != 2 {
		t.Fatalf("contract info = %+v", info)
	}

	env, err := f.svc.SaveEnvironment(ctx, domain.Environment{Name: "Contract", Variables: []domain.Variable{
		{Key: "base_url", Value: srv.URL + "/v1", Enabled: true},
		{Key: "note", Value: testToken, Secret: true, Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.svc.RunCollection(ctx, "", imp.Collection.ID, "", env.ID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, failed := contractResults(run)
	want := []string{
		`Get a pet: nickname: field is not documented`,
		`Get a pet: price: expected number, got string ("19.90")`,
		`Get a pet: secret_note: field is not documented`,
	}
	// "/owners" documents no content, so its body is not checked: it only gets a passing result.
	if strings.Join(failed, "\n") != strings.Join(want, "\n") {
		t.Fatalf("contract failures:\n%s\nwant:\n%s", strings.Join(failed, "\n"), strings.Join(want, "\n"))
	}
	if run.Status != domain.StatusFailed {
		t.Fatalf("a contract failure should fail the run, status %s", run.Status)
	}
	stored, err := f.svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, failedStored := contractResults(stored); len(failedStored) != 3 {
		t.Fatalf("history should keep the contract results: %v", failedStored)
	}
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), testToken) {
		t.Fatal("history contains a secret")
	}

	// Allow undocumented fields: only the type problem is left.
	if _, err := f.svc.SetContractOptions(ctx, imp.Collection.ID, true, true); err != nil {
		t.Fatal(err)
	}
	run, _ = f.svc.RunCollection(ctx, "", imp.Collection.ID, "", env.ID, false, nil)
	if _, failed := contractResults(run); len(failed) != 1 || !strings.Contains(failed[0], "price") {
		t.Fatalf("with undocumented fields allowed: %v", failed)
	}

	// Coverage relates requests to operations without sending anything.
	cov, err := f.svc.GetContractCoverage(ctx, imp.Collection.ID, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cov.Requests) != 2 || len(cov.Operations) != 2 {
		t.Fatalf("coverage = %+v", cov)
	}
	for _, r := range cov.Requests {
		if r.Operation == "" {
			t.Errorf("request %q matched no operation (%s)", r.Name, r.Problem)
		}
	}

	// Export and import keep the spec and its options; the CLI therefore checks it too.
	exported, err := f.svc.ExportCollection(ctx, imp.Collection.ID, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.svc.ImportCollection(ctx, exported, "ignored.json")
	if err != nil {
		t.Fatal(err)
	}
	info, _ = f.svc.GetContract(ctx, again.Collection.ID)
	if !info.Attached || !info.AllowUndocumented || info.SourceName != "pets.json" || info.Operations != 2 {
		t.Fatalf("imported export contract = %+v", info)
	}

	dup, err := f.svc.DuplicateCollection(ctx, imp.Collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := f.svc.GetContract(ctx, dup.ID); !info.Attached || !info.AllowUndocumented {
		t.Fatalf("duplicate should copy the contract: %+v", info)
	}

	// Turning it off keeps the spec; removing it detaches it. Neither checks anything.
	if _, err := f.svc.SetContractOptions(ctx, imp.Collection.ID, false, true); err != nil {
		t.Fatal(err)
	}
	run, _ = f.svc.RunCollection(ctx, "", imp.Collection.ID, "", env.ID, false, nil)
	if p, fl := contractResults(run); len(p)+len(fl) != 0 {
		t.Fatalf("disabled contract still checked: %v %v", p, fl)
	}
	if err := f.svc.RemoveContract(ctx, dup.ID); err != nil {
		t.Fatal(err)
	}
	if info, _ := f.svc.GetContract(ctx, dup.ID); info.Attached {
		t.Fatal("contract was not removed")
	}
	if err := f.svc.DeleteCollection(ctx, again.Collection.ID); err != nil {
		t.Fatal(err)
	}
	if info, _ := f.svc.GetContract(ctx, again.Collection.ID); info.Attached {
		t.Fatal("deleting the collection should delete its contract")
	}
}

func TestContractAttachToExistingCollectionAndSendRequest(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	data, err := os.ReadFile("../fixtures/wallet-mock.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AttachContract(ctx, f.tree.Collection.ID, []byte("not: [a spec"), "x.yaml"); err == nil {
		t.Fatal("attaching an invalid file should fail")
	}
	info, err := f.svc.AttachContract(ctx, f.tree.Collection.ID, data, "wallet-mock.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Enabled || info.Operations == 0 {
		t.Fatalf("info = %+v", info)
	}
	// Replacing keeps the options.
	if _, err := f.svc.SetContractOptions(ctx, f.tree.Collection.ID, true, true); err != nil {
		t.Fatal(err)
	}
	if info, _ = f.svc.AttachContract(ctx, f.tree.Collection.ID, data, "v2.yaml"); !info.AllowUndocumented || info.SourceName != "v2.yaml" {
		t.Fatalf("replace lost the options: %+v", info)
	}

	cov, err := f.svc.GetContractCoverage(ctx, f.tree.Collection.ID, f.env.ID)
	if err != nil {
		t.Fatal(err)
	}
	var matched []string
	for _, r := range cov.Requests {
		if r.Operation != "" {
			matched = append(matched, r.Name)
		}
	}
	if len(matched) == 0 {
		t.Fatalf("no demo request matched the wallet spec: %+v", cov.Requests)
	}

	// A single Send of a matching request is checked too.
	req := f.request(t, matched[0])
	res, err := f.svc.SendRequest(ctx, "", req, f.env.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(res.AssertionResults, func(a domain.AssertionResult) bool { return a.Kind == domain.AssertContract }) {
		t.Fatalf("Send did not run the contract check: %+v", res.AssertionResults)
	}
}
