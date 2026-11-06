package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.1.1", "0.1.0", true},
		{"v0.2.0", "0.1.9", true},
		{"1.0.0", "0.99.99", true},
		{"0.1.10", "0.1.9", true},
		{"0.1.0", "0.1.0", false},
		{"0.1.0", "0.1.1", false},
		{"0.1.0", "0.1.0-dev", false},       // a dev build of 0.1.0 is not offered 0.1.0
		{"0.1.0", "0.1.0-3-gabc123", false}, // nor is a later untagged commit
		{"0.1.1", "0.1.0+git1d0c261", true},
		{"0.1.1", "1d0c261", true}, // untagged builds count as oldest
		{"garbage", "0.1.0", false},
		{"0.1", "0.0.1", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// feed serves latest.json plus the given files.
func feed(t *testing.T, m Manifest, files map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/releases/")
		if name == "latest.json" {
			_ = json.NewEncoder(w).Encode(m)
			return
		}
		b, ok := files[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeExe(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "api-test-kits")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestCheckReportsAvailableUpdate(t *testing.T) {
	bin := []byte("new binary")
	srv := feed(t, Manifest{
		Version: "0.1.1", Notes: "fixes",
		Assets: map[string]Asset{"linux-amd64": {URL: "bin", SHA256: sum(bin)}},
	}, map[string][]byte{"bin": bin})
	u := &Updater{Feed: srv.URL + "/releases/latest.json", Current: "0.1.0", Platform: "linux/amd64", Exe: fakeExe(t)}
	info, _, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !info.Enabled || !info.Available || info.Latest != "0.1.1" || info.Method != MethodReplace || info.Notes != "fixes" {
		t.Fatalf("unexpected info %+v", info)
	}

	u.Current = "0.1.1"
	info, _, err = u.Check(context.Background())
	if err != nil || info.Available || info.Method != "" {
		t.Fatalf("same version must not be offered: %+v %v", info, err)
	}
}

func TestCheckDisabledWithoutFeed(t *testing.T) {
	info, m, err := (&Updater{Current: "0.1.0"}).Check(context.Background())
	if err != nil || m != nil || info.Enabled || info.Available {
		t.Fatalf("got %+v %v %v", info, m, err)
	}
}

func TestCheckRejectsBadManifest(t *testing.T) {
	srv := feed(t, Manifest{Version: "not-a-version"}, nil)
	u := &Updater{Feed: srv.URL + "/releases/latest.json", Current: "0.1.0"}
	if _, _, err := u.Check(context.Background()); err == nil {
		t.Fatal("expected an error for an invalid manifest version")
	}
}

func TestApplyReplacesExecutable(t *testing.T) {
	bin := []byte("new binary")
	m := Manifest{Version: "0.1.1", Assets: map[string]Asset{"linux-amd64": {URL: "bin", SHA256: sum(bin)}}}
	srv := feed(t, m, map[string][]byte{"bin": bin})
	exe := fakeExe(t)
	u := &Updater{Feed: srv.URL + "/releases/latest.json", Current: "0.1.0", Platform: "linux/amd64", Exe: exe}

	var lastDone int64
	method, err := u.Apply(context.Background(), &m, func(done, _ int64) { lastDone = done })
	if err != nil || method != MethodReplace {
		t.Fatalf("Apply = %q, %v", method, err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new binary" {
		t.Fatalf("exe = %q", got)
	}
	if fi, _ := os.Stat(exe); fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("updated exe is not executable: %v", fi.Mode())
	}
	if lastDone != int64(len(bin)) {
		t.Fatalf("progress ended at %d", lastDone)
	}
	assertNoTempFiles(t, filepath.Dir(exe))
}

func TestApplyChecksumMismatchKeepsOldExecutable(t *testing.T) {
	m := Manifest{Version: "0.1.1", Assets: map[string]Asset{"linux-amd64": {URL: "bin", SHA256: sum([]byte("other"))}}}
	srv := feed(t, m, map[string][]byte{"bin": []byte("tampered")})
	exe := fakeExe(t)
	u := &Updater{Feed: srv.URL + "/releases/latest.json", Current: "0.1.0", Platform: "linux/amd64", Exe: exe}
	if _, err := u.Apply(context.Background(), &m, nil); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum error, got %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatalf("exe changed to %q", got)
	}
	assertNoTempFiles(t, filepath.Dir(exe))
}

func TestApplyMissingAssetIsManual(t *testing.T) {
	m := Manifest{Version: "0.1.1", PageURL: "https://example.com/r", Assets: map[string]Asset{"windows-amd64": {URL: "x", SHA256: "y"}}}
	u := &Updater{Feed: "https://example.com/latest.json", Current: "0.1.0", Platform: "darwin/universal", Exe: fakeExe(t)}
	if method, err := u.Apply(context.Background(), &m, nil); method != MethodManual || !errors.Is(err, ErrManual) {
		t.Fatalf("Apply = %q, %v", method, err)
	}
}

func TestPickByPackage(t *testing.T) {
	m := &Manifest{Assets: map[string]Asset{
		"linux-amd64":             {URL: "a", SHA256: "1"},
		"linux-amd64-deb":         {URL: "b.deb", SHA256: "2"},
		"windows-amd64-installer": {URL: "c.exe", SHA256: "3"},
	}}
	u := &Updater{Platform: "linux/amd64", Pkg: "deb"}
	if method, a := u.pick(m); method != MethodDeb || a.URL != "b.deb" {
		t.Fatalf("deb: %q %+v", method, a)
	}
	u = &Updater{Platform: "linux/amd64"}
	if method, a := u.pick(m); method != MethodReplace || a.URL != "a" {
		t.Fatalf("portable: %q %+v", method, a)
	}
	u = &Updater{Platform: "windows/amd64", Pkg: "installer"}
	if method, a := u.pick(m); method != MethodInstaller || a.URL != "c.exe" {
		t.Fatalf("installer: %q %+v", method, a)
	}
	// An asset without a checksum is never used.
	m.Assets["linux-arm64"] = Asset{URL: "d"}
	u = &Updater{Platform: "linux/arm64"}
	if method, _ := u.pick(m); method != MethodManual {
		t.Fatalf("missing checksum: %q", method)
	}
}

func TestResolveRefusesHTTPSDowngrade(t *testing.T) {
	u := &Updater{Feed: "https://example.com/releases/latest.json"}
	if _, err := u.resolve("http://evil.example/bin"); err == nil {
		t.Fatal("expected an error for an http asset on an https feed")
	}
	got, err := u.resolve("api-test-kits-0.1.1-linux-amd64")
	if err != nil || got.String() != "https://example.com/releases/api-test-kits-0.1.1-linux-amd64" {
		t.Fatalf("resolve = %v, %v", got, err)
	}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(dir, ".update-*"))
	if len(left) > 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
}
