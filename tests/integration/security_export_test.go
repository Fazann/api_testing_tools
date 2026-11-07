package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"api-test-kits/internal/collectionio"
	"api-test-kits/internal/domain"
)

// saveSecurityTests gives the demo collection a race test with a state check
// and a permission matrix with a secret role variable.
func saveSecurityTests(t *testing.T, f *fixture) (balance, health domain.Request) {
	t.Helper()
	ctx := context.Background()
	balance, health = f.request(t, "Get balance"), f.request(t, "Health check")
	if _, err := f.store.SaveRaceTest(ctx, domain.RaceTest{RequestID: balance.ID, Count: 5, SuccessOp: domain.OpLte,
		SuccessCount: 1, UniquePath: "tx",
		Check: &domain.RaceCheck{RequestID: health.ID, Path: "data.balance", DeltaPerSuccess: -10}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SavePermissionMatrix(ctx, domain.PermissionMatrix{
		CollectionID: f.tree.Collection.ID,
		Roles: []domain.Role{
			{ID: "bob", Name: "Bob", Variables: []domain.Variable{
				{Key: "token", Value: bobToken, Secret: true, Enabled: true},
				{Key: "user", Value: "bob", Enabled: true},
			}},
			{ID: "anon", Name: "Anonymous", Anonymous: true},
		},
		RequestIDs: []string{balance.ID, health.ID},
		Expected:   map[string]map[string]domain.Access{balance.ID: {"bob": domain.AccessDeny, "anon": domain.AccessDeny}},
	}); err != nil {
		t.Fatal(err)
	}
	return balance, health
}

func requestByName(t *testing.T, tree domain.CollectionTree, name string) domain.Request {
	t.Helper()
	for _, r := range tree.Requests {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("request %q not found", name)
	return domain.Request{}
}

// checkCopiedSecurity verifies that collection id has the race test and the
// matrix of saveSecurityTests, pointing at its own requests.
func checkCopiedSecurity(t *testing.T, f *fixture, id string, wantSecret string) {
	t.Helper()
	ctx := context.Background()
	tree, err := f.svc.GetCollectionTree(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	balance, health := requestByName(t, tree, "Get balance"), requestByName(t, tree, "Health check")
	rt, err := f.svc.GetRaceTest(ctx, balance.ID)
	if err != nil || rt == nil {
		t.Fatalf("race test not copied: %v", err)
	}
	if rt.Count != 5 || rt.SuccessOp != domain.OpLte || rt.UniquePath != "tx" || rt.Check == nil ||
		rt.Check.RequestID != health.ID || rt.Check.DeltaPerSuccess != -10 {
		t.Fatalf("race test = %+v (check %+v)", rt, rt.Check)
	}
	m, err := f.svc.GetPermissionMatrix(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Roles) != 2 || strings.Join(m.RequestIDs, ",") != balance.ID+","+health.ID ||
		m.Expected[balance.ID]["bob"] != domain.AccessDeny || m.Expected[balance.ID]["anon"] != domain.AccessDeny {
		t.Fatalf("matrix = %+v", m)
	}
	if got := m.Roles[0].Variables[0].Value; got != wantSecret {
		t.Fatalf("Bob's token = %q, want %q", got, wantSecret)
	}
	if m.Roles[0].Variables[1].Value != "bob" {
		t.Fatalf("non-secret role variable lost: %+v", m.Roles[0].Variables)
	}
}

func TestRaceTestsAndMatrixSurviveExportImportAndDuplicate(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	saveSecurityTests(t, f)

	// The export preview warns about the secret role variable.
	findings, err := f.svc.ExportPreview(ctx, f.tree.Collection.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mustJSONString(t, findings), "permission matrix role Bob") {
		t.Fatalf("export preview does not mention the role secret: %+v", findings)
	}

	// Without secrets: the structure is exported, Bob's token is blanked.
	exported, err := f.svc.ExportCollection(ctx, f.tree.Collection.ID, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(exported), bobToken) {
		t.Fatal("export without secrets contains a role secret")
	}
	var doc collectionio.Document
	if err := json.Unmarshal(exported, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.RaceTests) != 1 || doc.PermissionMatrix == nil || doc.PermissionMatrix.CollectionID != "" {
		t.Fatalf("export: %d race tests, matrix %+v", len(doc.RaceTests), doc.PermissionMatrix)
	}
	imp, err := f.svc.ImportCollection(ctx, exported, "")
	if err != nil {
		t.Fatal(err)
	}
	if imp.RaceTests != 1 || !imp.PermissionMatrix {
		t.Fatalf("import summary = %+v", imp)
	}
	checkCopiedSecurity(t, f, imp.Collection.ID, "")

	// With secrets, the token comes along.
	withSecrets, err := f.svc.ExportCollection(ctx, f.tree.Collection.ID, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	imp, err = f.svc.ImportCollection(ctx, withSecrets, "")
	if err != nil {
		t.Fatal(err)
	}
	checkCopiedSecurity(t, f, imp.Collection.ID, bobToken)

	// Duplicating keeps everything, secrets included (nothing leaves the machine).
	dup, err := f.svc.DuplicateCollection(ctx, f.tree.Collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkCopiedSecurity(t, f, dup.ID, bobToken)
	// The original is untouched.
	checkCopiedSecurity(t, f, f.tree.Collection.ID, bobToken)
}

func TestImportRejectsSecurityTestsForUnknownRequests(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	saveSecurityTests(t, f)
	exported, err := f.svc.ExportCollection(ctx, f.tree.Collection.ID, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*collectionio.Document){
		"race test": func(d *collectionio.Document) { d.RaceTests[0].RequestID = "nope" },
		"check":     func(d *collectionio.Document) { d.RaceTests[0].Check.RequestID = "nope" },
		"matrix":    func(d *collectionio.Document) { d.PermissionMatrix.RequestIDs[0] = "nope" },
		"count":     func(d *collectionio.Document) { d.RaceTests[0].Count = 100000 },
	} {
		var doc collectionio.Document
		if err := json.Unmarshal(exported, &doc); err != nil {
			t.Fatal(err)
		}
		edit(&doc)
		data, _ := json.Marshal(doc)
		if _, err := f.svc.ImportCollection(ctx, data, ""); err == nil {
			t.Errorf("%s: import of an inconsistent file succeeded", name)
		}
	}
	// Files from before this feature have neither field and still import.
	var doc collectionio.Document
	_ = json.Unmarshal(exported, &doc)
	doc.RaceTests, doc.PermissionMatrix = nil, nil
	data, _ := json.Marshal(doc)
	imp, err := f.svc.ImportCollection(ctx, data, "")
	if err != nil || imp.RaceTests != 0 || imp.PermissionMatrix {
		t.Fatalf("old-style import: %+v, %v", imp, err)
	}
}

func TestDeleteAllDataClearsContractsToo(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.svc.AttachContract(ctx, f.tree.Collection.ID, []byte(contractSpec), "pets.json"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteAllData(ctx); err != nil {
		t.Fatal(err)
	}
	if c, err := f.store.GetContract(ctx, f.tree.Collection.ID); err != nil || c != nil {
		t.Fatalf("contract left after Delete all data: %+v, %v", c, err)
	}
}

func mustJSONString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
