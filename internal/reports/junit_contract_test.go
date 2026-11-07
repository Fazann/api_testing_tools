package reports

import (
	"testing"

	"api-test-kits/internal/domain"
)

func TestFailureDetailsShowContractFieldPath(t *testing.T) {
	res := domain.RunResult{AssertionResults: []domain.AssertionResult{
		{Kind: domain.AssertContract, Path: "price", Description: "GET /pets/{petId} → 200", Message: "expected number, got string"},
		{Kind: domain.AssertContract, Description: "DELETE /pets/{petId} → 500", Message: "status 500 is not documented"},
		{Kind: domain.AssertContract, Passed: true, Description: "GET /pets → 200", Message: "Matches GET /pets → 200"},
	}}
	got := FailureDetails(res)
	want := []string{
		"GET /pets/{petId} → 200 price — expected number, got string",
		"DELETE /pets/{petId} → 500 — status 500 is not documented",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("FailureDetails = %q, want %q", got, want)
	}
}
