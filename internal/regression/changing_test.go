package regression

import (
	"reflect"
	"testing"

	"api-test-kits/internal/assertions"
)

func decodeAll(t *testing.T, bodies ...string) []any {
	t.Helper()
	out := make([]any, len(bodies))
	for i, b := range bodies {
		v, err := assertions.DecodeJSON([]byte(b))
		if err != nil {
			t.Fatal(err)
		}
		out[i] = v
	}
	return out
}

func byPath(fields []ChangingField) map[string]ChangingField {
	m := map[string]ChangingField{}
	for _, f := range fields {
		m[f.Path] = f
	}
	return m
}

func TestChangingFields(t *testing.T) {
	docs := decodeAll(t,
		`{"meta":{"request_id":"a1","at":"10:00"},"data":{"currency":"USD","n":1,"items":[{"id":1,"updated_at":"x"},{"id":2,"updated_at":"y"}],"events":[1,2,3]}}`,
		`{"meta":{"request_id":"b7","at":"10:01"},"data":{"currency":"USD","n":1.0,"items":[{"id":1,"updated_at":"z"},{"id":2,"updated_at":"y"}],"events":[1,2,3,4],"promo":"X"}}`,
		`{"meta":{"request_id":"c3","at":"10:02"},"data":{"currency":"USD","n":1,"items":[{"id":1,"updated_at":"x"},{"id":2,"updated_at":"w"}],"events":[1,2,3],"score":null}}`,
	)
	got := byPath(ChangingFields(docs, nil, 0))

	want := map[string]struct {
		reason    string
		suggested bool
	}{
		"meta.request_id":          {ChangedValue, true},
		"meta.at":                  {ChangedValue, true},
		"data.items[*].updated_at": {ChangedValue, true},
		"data.events":              {ChangedLength, false},
		"data.promo":               {ChangedMissing, true},
		"data.score":               {ChangedMissing, true},
	}
	for path, w := range want {
		f, ok := got[path]
		if !ok {
			t.Errorf("missing %s; got %v", path, got)
			continue
		}
		if f.Reason != w.reason || f.Suggested != w.suggested {
			t.Errorf("%s = %+v, want reason %s suggested %v", path, f, w.reason, w.suggested)
		}
	}
	for _, stable := range []string{"data.currency", "data.n", "data.items[*].id", "data.events[*]"} {
		if _, ok := got[stable]; ok {
			t.Errorf("%s is stable but was reported", stable)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d fields, want %d: %v", len(got), len(want), got)
	}
	if ex := got["meta.request_id"].Examples; !reflect.DeepEqual(ex, []string{`"a1"`, `"b7"`, `"c3"`}) {
		t.Errorf("examples = %v", ex)
	}
	if ex := got["data.events"].Examples; !reflect.DeepEqual(ex, []string{"3 items", "4 items"}) {
		t.Errorf("length examples = %v", ex)
	}
}

func TestChangingFieldsTypeChange(t *testing.T) {
	got := ChangingFields(decodeAll(t, `{"score":1}`, `{"score":null}`), nil, 0)
	if len(got) != 1 || got[0].Path != "score" || got[0].Reason != ChangedType {
		t.Fatalf("got %+v", got)
	}
}

func TestChangingFieldsRespectsIgnoredAndMax(t *testing.T) {
	docs := decodeAll(t, `{"a":1,"b":{"ts":1},"c":[{"x":1}]}`, `{"a":2,"b":{"ts":2},"c":[{"x":2}]}`)
	got := ChangingFields(docs, []string{"**.ts", "c[*].x"}, 0)
	if len(got) != 1 || got[0].Path != "a" {
		t.Fatalf("ignored patterns must hide fields: %+v", got)
	}
	if got := ChangingFields(docs, nil, 2); len(got) != 2 {
		t.Fatalf("max not applied: %+v", got)
	}
}

func TestChangingFieldsStableAndSingle(t *testing.T) {
	same := decodeAll(t, `{"a":[1,{"b":true}]}`, `{"a":[1,{"b":true}]}`)
	if got := ChangingFields(same, nil, 0); len(got) != 0 {
		t.Fatalf("identical responses: %+v", got)
	}
	if got := ChangingFields(same[:1], nil, 0); len(got) != 0 {
		t.Fatalf("one response cannot change: %+v", got)
	}
	root := ChangingFields(decodeAll(t, `"x"`, `"y"`), nil, 0)
	if len(root) != 1 || root[0].Path != "$" {
		t.Fatalf("root value: %+v", root)
	}
}

func TestChangingPaths(t *testing.T) {
	got := ChangingPaths([]ChangingField{{Path: "a", Suggested: true}, {Path: "list", Suggested: false}})
	if !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("got %v", got)
	}
}
