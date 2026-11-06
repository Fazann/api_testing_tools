package storage

import (
	"context"
	"reflect"
	"testing"

	"api-test-kits/internal/domain"
)

func TestEnvComparisonRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c, err := s.SaveCollection(ctx, domain.Collection{Name: "C"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetEnvComparison(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DetectChanging || got.IncludeModifying || len(got.IgnoredPaths) != 0 {
		t.Fatalf("defaults = %+v", got)
	}

	in := domain.EnvComparison{CollectionID: c.ID, EnvA: "a", EnvB: "b", IncludeModifying: true,
		IgnoredPaths: []string{" meta.id ", "meta.id", "", "header:X-Build"}}
	if _, err := s.SaveEnvComparison(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetEnvComparison(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EnvA != "a" || got.EnvB != "b" || !got.IncludeModifying || got.DetectChanging ||
		!reflect.DeepEqual(got.IgnoredPaths, []string{"meta.id", "header:X-Build"}) || got.UpdatedAt.IsZero() {
		t.Fatalf("got %+v", got)
	}

	if err := s.DeleteCollection(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM environment_comparisons`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("settings must be deleted with the collection: %d %v", n, err)
	}
}

func TestSaveEnvComparisonNeedsCollection(t *testing.T) {
	s := openTest(t)
	if _, err := s.SaveEnvComparison(context.Background(), domain.EnvComparison{CollectionID: "missing"}); err == nil {
		t.Fatal("expected an error for an unknown collection")
	}
}
