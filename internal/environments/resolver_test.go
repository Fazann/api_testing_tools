package environments

import (
	"reflect"
	"strings"
	"testing"

	"api-test-kits/internal/domain"
)

func env() *domain.Environment {
	return &domain.Environment{Name: "Local", Variables: []domain.Variable{
		{Key: "base_url", Value: "http://localhost:8080", Enabled: true},
		{Key: "token", Value: "tok", Secret: true, Enabled: true},
		{Key: "off", Value: "nope", Enabled: false},
		{Key: "empty", Value: "", Enabled: true},
	}}
}

func TestResolve(t *testing.T) {
	r := NewResolver(env())
	if got := r.Resolve("{{base_url}}/x?t={{ token }}"); got != "http://localhost:8080/x?t=tok" {
		t.Fatalf("got %s", got)
	}
	if got := r.Resolve("{{off}}/{{missing}}"); got != "{{off}}/{{missing}}" {
		t.Fatalf("unresolved names must be left as-is, got %s", got)
	}
	if !reflect.DeepEqual(r.Missing(), []string{"missing", "off"}) {
		t.Fatalf("missing: %v", r.Missing())
	}
	if !r.IsSecret("token") || r.IsSecret("base_url") {
		t.Fatal("secret flags wrong")
	}
}

func TestBuiltins(t *testing.T) {
	r := NewResolver(nil)
	if got := r.Resolve("{{$uuid}}"); len(got) != 36 || strings.Contains(got, "{") {
		t.Fatalf("uuid: %s", got)
	}
	if got := r.Resolve("{{$timestamp}}"); strings.Contains(got, "{") {
		t.Fatalf("timestamp: %s", got)
	}
	if len(r.Missing()) != 0 {
		t.Fatal("built-ins are never missing")
	}
}

func TestRequestReferencesAndRequired(t *testing.T) {
	req := &domain.Request{
		URLTemplate: "{{base_url}}/a",
		Headers:     []domain.KeyValue{{Key: "X", Value: "{{h}}", Enabled: true}, {Key: "Y", Value: "{{disabled}}"}},
		Body:        `{"a":"{{b}}","t":"{{$uuid}}"}`,
		Auth:        domain.Auth{Type: domain.AuthBearer, Token: "{{token}}"},
	}
	if got := RequestReferences(req); !reflect.DeepEqual(got, []string{"b", "base_url", "h", "token"}) {
		t.Fatalf("got %v", got)
	}
	if got := MissingRequired(env(), []string{"base_url", "empty", "off", "nope"}); !reflect.DeepEqual(got, []string{"empty", "off", "nope"}) {
		t.Fatalf("got %v", got)
	}
}
