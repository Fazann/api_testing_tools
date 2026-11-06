package storage

import (
	"context"
	"testing"

	"api-test-kits/internal/domain"
)

func sampleRule(left, right string) domain.Rule {
	return domain.Rule{ID: "rule-1", Name: "Balance deducted", Enabled: true, Kind: domain.RuleCompare, Operator: domain.RuleEq,
		Left:  domain.RuleValue{Source: domain.SourceResponse, RequestID: left, Path: "data.balance"},
		Right: []domain.RuleTerm{{Sign: "+", Value: domain.RuleValue{Source: domain.SourceResponse, RequestID: right, Path: "data.balance"}}}}
}

func TestCollectionRulesSaveDuplicate(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c, err := s.SaveCollection(ctx, domain.Collection{Name: "Wallet"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "Before", Method: "GET", URLTemplate: "http://x/a"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.SaveRequest(ctx, domain.Request{CollectionID: c.ID, Name: "After", Method: "GET", URLTemplate: "http://x/b"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetCollection(ctx, c.ID); got.Rules == nil || len(got.Rules) != 0 {
		t.Fatalf("a new collection has an empty rule list: %#v", got.Rules)
	}
	if _, err := s.SaveCollectionRules(ctx, c.ID, []domain.Rule{sampleRule(after.ID, before.ID)}); err != nil {
		t.Fatal(err)
	}

	// Saving the collection's settings from a stale copy keeps the rules.
	stale := c
	stale.Description = "edited"
	if _, err := s.SaveCollection(ctx, stale); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCollection(ctx, c.ID)
	if err != nil || len(got.Rules) != 1 || got.Description != "edited" {
		t.Fatalf("rules lost by SaveCollection: %+v %v", got, err)
	}

	// A duplicate's rules point at the duplicate's requests.
	dup, err := s.DuplicateCollection(ctx, c.ID, "Copy")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := s.GetTree(ctx, dup.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, r := range tree.Requests {
		ids[r.Name] = r.ID
	}
	rule := tree.Collection.Rules[0]
	if rule.Left.RequestID != ids["After"] || rule.Right[0].Value.RequestID != ids["Before"] || ids["After"] == after.ID {
		t.Fatalf("rule not remapped: %+v ids %v", rule, ids)
	}

	if _, err := s.SaveCollectionRules(ctx, c.ID, []domain.Rule{{ID: "x", Name: "Bad", Kind: "nope"}}); err == nil {
		t.Fatal("an invalid rule must be refused")
	}
	if _, err := s.SaveCollectionRules(ctx, "missing", nil); err == nil {
		t.Fatal("unknown collection must be refused")
	}
}
