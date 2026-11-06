// Package updater keeps an installed desktop app up to date in place, so users
// never uninstall and reinstall to get a new version.
//
// Every release publishes a small manifest (latest.json) next to its files:
//
//	{
//	  "version": "0.1.2",
//	  "notes": "Fixes …",
//	  "pageUrl": "https://…/releases/tag/v0.1.2",
//	  "assets": {
//	    "linux-amd64":             {"url": "api-test-kits-0.1.2-linux-amd64", "sha256": "…"},
//	    "linux-amd64-deb":         {"url": "api-test-kits_0.1.2_amd64.deb", "sha256": "…"},
//	    "windows-amd64":           {"url": "api-test-kits-0.1.2-windows-amd64.exe", "sha256": "…"},
//	    "windows-amd64-installer": {"url": "api-test-kits-0.1.2-windows-amd64-installer.exe", "sha256": "…"}
//	  }
//	}
//
// Asset URLs may be relative to the manifest URL. The app downloads the asset
// that matches how it was installed, verifies its SHA-256, and then either
// swaps its own executable (portable installs), or hands the package to the
// system installer (.deb via pkexec/apt, Windows NSIS installer). User data
// lives outside the install directory and is migrated by the storage layer on
// the next start, so it survives every update.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// FeedURL is the manifest location, set at build time with
// -ldflags "-X api-test-kits/internal/updater.FeedURL=https://…/latest.json".
// Builds without it (local `wails dev`, tests) never check for updates.
var FeedURL = ""

// Package says how this build was distributed; "deb" for the Debian package,
// empty for the portable binary and the Windows builds (which detect at run
// time whether they were installed by the NSIS installer).
var Package = ""

// Methods describe how an update is applied.
const (
	MethodReplace   = "replace"   // swap the executable in place, then restart
	MethodDeb       = "deb"       // install the new .deb with pkexec apt-get, then restart
	MethodInstaller = "installer" // run the Windows installer silently, then restart
	MethodManual    = "manual"    // no matching asset: the user downloads it from PageURL
)

const (
	maxManifestBytes = 1 << 20
	maxAssetBytes    = 512 << 20
)

// Asset is one downloadable file in the manifest.
type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// Manifest is the published latest.json.
type Manifest struct {
	Version string           `json:"version"`
	Notes   string           `json:"notes"`
	PageURL string           `json:"pageUrl"`
	Assets  map[string]Asset `json:"assets"`
}

// Info is the result of an update check.
type Info struct {
	Enabled   bool   `json:"enabled"` // false when this build has no update feed
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Notes     string `json:"notes"`
	PageURL   string `json:"pageUrl"`
	Method    string `json:"method"`
}

// Updater checks a feed and applies updates for one executable.
type Updater struct {
	Feed    string
	Current string
	Client  *http.Client
	// Exe is the running executable; defaults to os.Executable.
	Exe string
	// Platform overrides runtime.GOOS/GOARCH ("linux/amd64"), for tests.
	Platform string
	// Pkg overrides Package, for tests.
	Pkg string
}

// New returns an updater for the running app using the build-time feed.
func New(current string) *Updater {
	return &Updater{Feed: FeedURL, Current: current, Pkg: Package, Client: &http.Client{Timeout: 10 * time.Minute}}
}

func (u *Updater) platform() (goos, goarch string) {
	if u.Platform != "" {
		goos, goarch, _ = strings.Cut(u.Platform, "/")
		return goos, goarch
	}
	return runtime.GOOS, runtime.GOARCH
}

func (u *Updater) exe() (string, error) {
	if u.Exe != "" {
		return u.Exe, nil
	}
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p, nil
}

// Check fetches the manifest and reports whether a newer version exists.
func (u *Updater) Check(ctx context.Context) (Info, *Manifest, error) {
	info := Info{Current: u.Current, Enabled: u.Feed != ""}
	if !info.Enabled {
		return info, nil, nil
	}
	m, err := u.fetchManifest(ctx)
	if err != nil {
		return info, nil, err
	}
	info.Latest = m.Version
	info.Notes = m.Notes
	info.PageURL = m.PageURL
	info.Available = Newer(m.Version, u.Current)
	if info.Available {
		info.Method, _ = u.pick(m)
	}
	return info, m, nil
}

func (u *Updater) fetchManifest(ctx context.Context) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.Feed, nil)
	if err != nil {
		return nil, fmt.Errorf("update feed: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := u.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach the update server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update server answered %s", resp.Status)
	}
	var m Manifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestBytes)).Decode(&m); err != nil {
		return nil, fmt.Errorf("update manifest is not valid JSON: %w", err)
	}
	if _, ok := parseVersion(m.Version); !ok {
		return nil, fmt.Errorf("update manifest has an invalid version %q", m.Version)
	}
	return &m, nil
}

func (u *Updater) client() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return http.DefaultClient
}

// pick chooses how to apply the update and which asset to download.
func (u *Updater) pick(m *Manifest) (string, Asset) {
	goos, goarch := u.platform()
	key := goos + "-" + goarch
	pkg := u.Pkg
	if pkg == "" && goos == "windows" {
		if exe, err := u.exe(); err == nil && !dirWritable(filepath.Dir(exe)) {
			pkg = "installer" // e.g. C:\Program Files, installed by the NSIS installer
		}
	}
	try := func(method, k string) (string, Asset, bool) {
		a, ok := m.Assets[k]
		return method, a, ok && a.URL != "" && a.SHA256 != ""
	}
	var method string
	var a Asset
	var ok bool
	switch pkg {
	case "deb":
		method, a, ok = try(MethodDeb, key+"-deb")
	case "installer":
		method, a, ok = try(MethodInstaller, key+"-installer")
	default:
		method, a, ok = try(MethodReplace, key)
	}
	if !ok {
		return MethodManual, Asset{}
	}
	return method, a
}

// ErrManual means the update cannot be applied automatically.
var ErrManual = errors.New("this install cannot update itself; download the new version from the release page")

// Apply downloads and installs the update described by m. On success the
// caller should restart (see Restart) or quit, depending on the method.
// progress, if set, receives downloaded and total bytes (total may be -1).
func (u *Updater) Apply(ctx context.Context, m *Manifest, progress func(done, total int64)) (string, error) {
	method, a := u.pick(m)
	if method == MethodManual {
		return method, ErrManual
	}
	src, err := u.resolve(a.URL)
	if err != nil {
		return method, err
	}
	exe, err := u.exe()
	if err != nil {
		return method, fmt.Errorf("cannot locate the running executable: %w", err)
	}

	// Portable installs download next to the executable so the final rename is
	// atomic and on the same file system; packages go to the temp directory.
	dir := os.TempDir()
	if method == MethodReplace {
		dir = filepath.Dir(exe)
	}
	f, err := os.CreateTemp(dir, ".update-*"+filepath.Ext(src.Path))
	if err != nil {
		return method, fmt.Errorf("cannot write the update next to %s: %w", exe, err)
	}
	tmp := f.Name()
	keep := false
	defer func() {
		if !keep {
			os.Remove(tmp)
		}
	}()
	err = u.download(ctx, src.String(), a.SHA256, f, progress)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return method, err
	}

	switch method {
	case MethodReplace:
		if err := replaceExecutable(exe, tmp); err != nil {
			return method, err
		}
	case MethodDeb:
		if err := installDeb(ctx, tmp); err != nil {
			return method, err
		}
	case MethodInstaller:
		// The installer runs after this process quits (it must replace our exe),
		// so the downloaded file has to outlive this function.
		if err := runInstaller(tmp, exe); err != nil {
			return method, err
		}
		keep = true
	}
	return method, nil
}

func (u *Updater) resolve(ref string) (*url.URL, error) {
	base, err := url.Parse(u.Feed)
	if err != nil {
		return nil, fmt.Errorf("update feed: %w", err)
	}
	r, err := url.Parse(ref)
	if err != nil {
		return nil, fmt.Errorf("update asset URL: %w", err)
	}
	abs := base.ResolveReference(r)
	// Never let a manifest downgrade the transport (an https feed must serve https assets).
	if base.Scheme == "https" && abs.Scheme != "https" {
		return nil, fmt.Errorf("update asset %s is not served over https", abs)
	}
	return abs, nil
}

func (u *Updater) download(ctx context.Context, src, wantSum string, w io.Writer, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return err
	}
	resp, err := u.client().Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	if resp.ContentLength > maxAssetBytes {
		return fmt.Errorf("update is larger than %d MB", maxAssetBytes>>20)
	}
	h := sha256.New()
	pw := &progressWriter{total: resp.ContentLength, fn: progress}
	n, err := io.Copy(io.MultiWriter(w, h, pw), io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if n > maxAssetBytes {
		return fmt.Errorf("update is larger than %d MB", maxAssetBytes>>20)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, strings.TrimSpace(wantSum)) {
		return fmt.Errorf("update checksum mismatch (got %s, expected %s); nothing was changed", got, wantSum)
	}
	return nil
}

type progressWriter struct {
	done, total int64
	last        time.Time
	fn          func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.fn != nil && (time.Since(p.last) > 100*time.Millisecond || p.done == p.total) {
		p.last = time.Now()
		p.fn(p.done, p.total)
	}
	return len(b), nil
}

// replaceExecutable atomically puts newPath in place of exe. Running
// executables cannot be overwritten on Windows but they can be renamed, so the
// old one is moved aside and removed on the next start (CleanupOld).
func replaceExecutable(exe, newPath string) error {
	if err := os.Chmod(newPath, 0o755); err != nil {
		return err
	}
	old := exe + ".old"
	if runtime.GOOS == "windows" {
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return fmt.Errorf("cannot move the current executable aside: %w", err)
		}
		if err := os.Rename(newPath, exe); err != nil {
			_ = os.Rename(old, exe)
			return fmt.Errorf("cannot install the update: %w", err)
		}
		return nil
	}
	if err := os.Rename(newPath, exe); err != nil {
		return fmt.Errorf("cannot install the update: %w", err)
	}
	return nil
}

// CleanupOld removes leftovers of a previous update. Call it on startup.
func CleanupOld() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	os.Remove(exe + ".old")
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".update-*"))
	for _, m := range matches {
		os.Remove(m)
	}
}

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// Newer reports whether version a is newer than b. Versions are
// MAJOR.MINOR.PATCH with an optional leading "v" and an optional suffix
// ("-dev", "+git…", "-3-gabc123") that is ignored for ordering, so a dev build
// of 0.1.2 is not offered 0.1.2 again. An unparsable b counts as oldest.
func Newer(a, b string) bool {
	va, ok := parseVersion(a)
	if !ok {
		return false
	}
	vb, ok := parseVersion(b)
	if !ok {
		return true
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+~"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// Restart starts exe again with the current arguments. The caller quits right
// after. It is not used for the Windows installer, which relaunches the app
// itself once it has finished.
func Restart(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("updated, but could not restart; please start the app again: %w", err)
	}
	return cmd.Process.Release()
}

// Executable returns the path the updater installs to.
func (u *Updater) Executable() (string, error) { return u.exe() }
