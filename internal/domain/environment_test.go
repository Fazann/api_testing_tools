package domain

import "testing"

func TestWithCollectionVariables(t *testing.T) {
	collection := []Variable{
		{Key: "base_url", Value: "http://collection", Enabled: true},
		{Key: "token", Value: "collection-token", Secret: true, Enabled: true},
		{Key: "off", Value: "x", Enabled: false},
	}
	if got := WithCollectionVariables(nil, nil); got != nil {
		t.Fatal("nothing defined should stay nil")
	}
	only := WithCollectionVariables(nil, collection)
	if len(only.Variables) != 2 || only.SecretValues()[0] != "collection-token" {
		t.Fatalf("collection variables alone: %+v", only.Variables)
	}

	env := &Environment{ID: "e", Name: "Staging", Protected: true, Variables: []Variable{
		{Key: "base_url", Value: "http://staging", Enabled: true},
		{Key: "token", Value: "", Enabled: false},
	}}
	merged := WithCollectionVariables(env, collection)
	if merged.Name != "Staging" || !merged.Protected || merged.ID != "e" {
		t.Fatalf("environment identity lost: %+v", merged)
	}
	values := map[string]string{}
	for _, v := range merged.Variables {
		if v.Enabled {
			values[v.Key] = v.Value
		}
	}
	if values["base_url"] != "http://staging" {
		t.Fatalf("environment should override the collection: %+v", merged.Variables)
	}
	if values["token"] != "collection-token" {
		t.Fatalf("a disabled environment variable should not hide the collection one: %+v", merged.Variables)
	}
	if len(env.Variables) != 2 {
		t.Fatal("the environment passed in must not be modified")
	}
}

func TestCollectionValidatesVariables(t *testing.T) {
	c := Collection{Name: "C", Variables: []Variable{{Key: "a b", Value: "1"}}}
	if err := c.Validate(); err == nil {
		t.Fatal("invalid collection variable name must fail")
	}
	c.Variables = []Variable{{Key: "a", Value: "1"}, {Key: "a", Value: "2"}}
	if err := c.Validate(); err == nil {
		t.Fatal("duplicate collection variable must fail")
	}
}
