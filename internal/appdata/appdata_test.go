package appdata

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDB(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.db"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readDB(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestResolveMovesEachLegacyDir(t *testing.T) {
	for _, name := range LegacyDirNames {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			writeDB(t, filepath.Join(base, name), name)
			got := resolve(base)
			if want := filepath.Join(base, DirName); got != want {
				t.Fatalf("resolve = %s, want %s", got, want)
			}
			if readDB(t, got) != name {
				t.Fatal("data not moved")
			}
			if _, err := os.Stat(filepath.Join(base, name)); !os.IsNotExist(err) {
				t.Fatalf("legacy dir still exists: %v", err)
			}
		})
	}
}

func TestResolvePrefersNewestLegacyDir(t *testing.T) {
	base := t.TempDir()
	writeDB(t, filepath.Join(base, "test-tool-kit"), "newer")
	writeDB(t, filepath.Join(base, "api-regression-guard"), "older")
	if got := readDB(t, resolve(base)); got != "newer" {
		t.Fatalf("moved %q, want the newest legacy dir", got)
	}
}

func TestResolvePrefersNewDir(t *testing.T) {
	base := t.TempDir()
	writeDB(t, filepath.Join(base, DirName), "current")
	writeDB(t, filepath.Join(base, LegacyDirNames[0]), "legacy")
	if got := readDB(t, resolve(base)); got != "current" {
		t.Fatalf("resolve picked %q", got)
	}
	if _, err := os.Stat(filepath.Join(base, LegacyDirNames[0])); err != nil {
		t.Fatalf("legacy dir must be left alone when the new one exists: %v", err)
	}
}

func TestResolveFreshInstall(t *testing.T) {
	base := t.TempDir()
	if got := resolve(base); got != filepath.Join(base, DirName) {
		t.Fatalf("resolve = %s", got)
	}
}
