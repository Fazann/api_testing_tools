package regression

import (
	"encoding/json"
	"reflect"
	"testing"

	"api-test-kits/internal/domain"
)

func schemaBaseline(t *testing.T, body string, ignored ...string) domain.Baseline {
	t.Helper()
	snap, err := SchemaSnapshot([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return domain.Baseline{ID: "b1", Mode: domain.BaselineSchema, Snapshot: snap, IgnoredPaths: ignored}
}

func TestSchemaIgnoresValueChanges(t *testing.T) {
	b := schemaBaseline(t, `{"id":"a1","total":10,"ok":true,"items":[{"amount":1.5,"at":"2026-01-01"}],"meta":null}`)
	res := Compare(b, []byte(`{"total":99.75,"ok":false,"id":"zz","items":[{"amount":3,"at":"x"},{"amount":4,"at":"y"}],"meta":{"x":1}}`), false)
	if !res.Passed {
		t.Fatalf("value-only changes must pass, got %+v", res.Diffs)
	}
	// Empty arrays and then-null values accept anything.
	if res := Compare(b, []byte(`{"total":1,"ok":true,"id":"q","items":[],"meta":"now a string"}`), false); !res.Passed {
		t.Fatalf("got %+v", res.Diffs)
	}
}

func TestSchemaReportsStructuralChanges(t *testing.T) {
	b := schemaBaseline(t, `{"id":"a1","total":10,"items":[{"amount":1,"sku":"x"}],"user":{"name":"n"}}`)
	res := Compare(b, []byte(`{"total":"10","items":[{"amount":"1","sku":"x","extra":1},{"amount":"2","sku":"y"}],"user":null,"new":[1]}`), false)
	if res.Passed {
		t.Fatal("expected failure")
	}
	want := []domain.DiffEntry{
		{Path: "id", Kind: domain.DiffRemoved, Before: "string"},
		{Path: "items[*].amount", Kind: domain.DiffTypeChanged, Before: "number", After: "string"},
		{Path: "items[*].extra", Kind: domain.DiffAdded, After: "number"},
		{Path: "new", Kind: domain.DiffAdded, After: "array of number"},
		{Path: "total", Kind: domain.DiffTypeChanged, Before: "number", After: "string"},
		{Path: "user", Kind: domain.DiffTypeChanged, Before: "object", After: "null"},
	}
	if !reflect.DeepEqual(res.Diffs, want) {
		t.Fatalf("got  %+v\nwant %+v", res.Diffs, want)
	}
}

func TestSchemaMergesArrayElements(t *testing.T) {
	// "note" is missing from one element, "ref" is null in one: both optional / nullable.
	b := schemaBaseline(t, `[{"id":1,"note":"a","ref":"r"},{"id":2,"ref":null}]`)
	var s Shape
	if err := json.Unmarshal(b.Snapshot, &s); err != nil {
		t.Fatal(err)
	}
	if got := s.Items.Required; !reflect.DeepEqual(got, []string{"id", "ref"}) {
		t.Fatalf("required = %v", got)
	}
	if got := s.Items.Properties["ref"].Type; !reflect.DeepEqual(got, []string{"null", "string"}) {
		t.Fatalf("ref type = %v", got)
	}
	if res := Compare(b, []byte(`[{"id":3,"ref":null},{"id":4,"ref":"x","note":"n"}]`), false); !res.Passed {
		t.Fatalf("got %+v", res.Diffs)
	}
	res := Compare(b, []byte(`[{"note":"n","ref":"x"}]`), false)
	if len(res.Diffs) != 1 || res.Diffs[0].Path != "[*].id" || res.Diffs[0].Kind != domain.DiffRemoved {
		t.Fatalf("got %+v", res.Diffs)
	}
}

func TestSchemaIgnoredPaths(t *testing.T) {
	b := schemaBaseline(t, `{"data":{"rows":[{"v":1,"debug":{}}]},"trace":"t"}`, "trace", "data.rows[*].debug", "**.v")
	res := Compare(b, []byte(`{"data":{"rows":[{"v":"s","debug":"gone"}]}}`), false)
	if !res.Passed {
		t.Fatalf("got %+v", res.Diffs)
	}
}

func TestSchemaRootTypeAndCorruptSnapshot(t *testing.T) {
	b := schemaBaseline(t, `{"a":1}`)
	res := Compare(b, []byte(`[1]`), false)
	if len(res.Diffs) != 1 || res.Diffs[0].Path != "$" || res.Diffs[0].Kind != domain.DiffTypeChanged {
		t.Fatalf("got %+v", res.Diffs)
	}
	b.Snapshot = json.RawMessage(`{}`)
	if res := Compare(b, []byte(`{}`), false); res.Passed || res.Message != "stored baseline schema is corrupt" {
		t.Fatalf("got %+v", res)
	}
}
