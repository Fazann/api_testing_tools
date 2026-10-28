package regression

import (
	"testing"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	v, err := assertions.DecodeJSON([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func kinds(diffs []domain.DiffEntry) map[string]domain.DiffKind {
	out := map[string]domain.DiffKind{}
	for _, d := range diffs {
		out[d.Path] = d.Kind
	}
	return out
}

func TestDiffIgnoresKeyOrder(t *testing.T) {
	a := decode(t, `{"a":1,"b":{"x":true,"y":[1,2]}}`)
	b := decode(t, `{"b":{"y":[1,2],"x":true},"a":1}`)
	if d := Diff(a, b, Options{}); len(d) != 0 {
		t.Fatalf("expected no diffs, got %+v", d)
	}
}

func TestDiffKinds(t *testing.T) {
	a := decode(t, `{"keep":1,"gone":"x","change":"old","typ":5,"arr":[1,2,3],"n":1}`)
	b := decode(t, `{"keep":1,"new":true,"change":"new","typ":"5","arr":[1,2],"n":1.5}`)
	got := kinds(Diff(a, b, Options{}))
	want := map[string]domain.DiffKind{
		"gone":   domain.DiffRemoved,
		"new":    domain.DiffAdded,
		"change": domain.DiffChanged,
		"typ":    domain.DiffTypeChanged,
		"arr[2]": domain.DiffRemoved,
		"n":      domain.DiffChanged, // integer → decimal is a value change, not a type change
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for p, k := range want {
		if got[p] != k {
			t.Fatalf("%s: got %s want %s (all: %v)", p, got[p], k, got)
		}
	}
}

func TestDiffIgnoredPaths(t *testing.T) {
	a := decode(t, `{"meta":{"request_id":"a","ts":1},"items":[{"id":1,"updated_at":"x"},{"id":2,"updated_at":"y"}],"deep":{"x":{"timestamp":1}}}`)
	b := decode(t, `{"meta":{"request_id":"b","ts":2},"items":[{"id":1,"updated_at":"z"},{"id":2,"updated_at":"w"}],"deep":{"x":{"timestamp":2}}}`)
	d := Diff(a, b, Options{IgnoredPaths: []string{"meta.request_id", "$.meta.ts", "items[*].updated_at", "**.timestamp"}})
	if len(d) != 0 {
		t.Fatalf("all dynamic fields should be ignored, got %+v", d)
	}
	d = Diff(a, b, Options{IgnoredPaths: []string{"meta"}})
	if len(d) != 3 {
		t.Fatalf("ignoring a parent ignores children; expected 3 remaining diffs, got %+v", d)
	}
}

func TestDiffTolerance(t *testing.T) {
	a := decode(t, `{"total":100.0001}`)
	b := decode(t, `{"total":100.0002}`)
	if d := Diff(a, b, Options{}); len(d) != 1 {
		t.Fatalf("without tolerance expected a diff, got %+v", d)
	}
	if d := Diff(a, b, Options{Tolerance: 0.001}); len(d) != 0 {
		t.Fatalf("with tolerance expected no diff, got %+v", d)
	}
}

func TestDiffRootTypeChange(t *testing.T) {
	d := Diff(decode(t, `{"a":1}`), decode(t, `[1]`), Options{})
	if len(d) != 1 || d[0].Kind != domain.DiffTypeChanged || d[0].Path != "$" {
		t.Fatalf("got %+v", d)
	}
}

func TestDiffMaxEntries(t *testing.T) {
	d := Diff(decode(t, `[1,2,3,4,5]`), decode(t, `[]`), Options{MaxDiffs: 2})
	if len(d) != 2 {
		t.Fatalf("expected cap of 2, got %d", len(d))
	}
}

func TestBaselineSelectedPaths(t *testing.T) {
	snap, err := Snapshot([]byte(`{"data":{"balance":10,"currency":"USD","ts":1}}`), []string{"data.balance", "data.currency"})
	if err != nil {
		t.Fatal(err)
	}
	b := domain.Baseline{ID: "b1", Snapshot: snap, SelectedPaths: []string{"data.balance", "data.currency"}}

	same := Compare(b, []byte(`{"data":{"currency":"USD","balance":10,"ts":999,"extra":1}}`), false)
	if !same.Passed {
		t.Fatalf("unselected fields must not matter: %+v", same.Diffs)
	}
	changed := Compare(b, []byte(`{"data":{"currency":"EUR"}}`), false)
	got := kinds(changed.Diffs)
	if changed.Passed || got["data.balance"] != domain.DiffRemoved || got["data.currency"] != domain.DiffChanged {
		t.Fatalf("got %+v", changed.Diffs)
	}
}

func TestBaselineWholeDocumentAndErrors(t *testing.T) {
	snap, err := Snapshot([]byte(`{"a":1}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	b := domain.Baseline{Snapshot: snap}
	if r := Compare(b, []byte(`{"a":2}`), false); r.Passed || len(r.Diffs) != 1 {
		t.Fatalf("got %+v", r)
	}
	if r := Compare(b, []byte(`not json`), false); r.Passed || r.Message == "" {
		t.Fatalf("malformed JSON must fail with a message: %+v", r)
	}
	if r := Compare(b, []byte(`{"a":1}`), true); r.Passed {
		t.Fatal("truncated responses must not pass a baseline comparison")
	}
	if _, err := Snapshot([]byte(`<html>`), nil); err == nil {
		t.Fatal("non-JSON snapshot must fail")
	}
}
