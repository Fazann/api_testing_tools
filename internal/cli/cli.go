// Package cli is the headless command-line runner used in CI pipelines and
// scheduled jobs. It runs collections through the same service as the
// desktop app, against an in-memory database, so it never changes the app's
// data and needs no GUI libraries.
package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"api-test-kits/internal/appdata"
	"api-test-kits/internal/collectionio"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

// Exit codes.
const (
	ExitPassed      = 0
	ExitFailed      = 1   // assertions or baselines failed
	ExitErrors      = 2   // requests could not be completed (network, script, missing variable)
	ExitUsage       = 3   // bad arguments, unreadable file, unknown environment, …
	ExitInterrupted = 130 // cancelled with Ctrl+C / SIGTERM
)

// EnvVarPrefix marks environment variables that set collection variables:
// APIGUARD_VAR_token=… sets {{token}}. Values set this way are secret.
const EnvVarPrefix = "APIGUARD_VAR_"

// Env abstracts the process environment for tests.
type Env struct {
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	Environ        func() []string
	// Color enables ANSI colours on Stdout.
	Color bool
	Now   func() time.Time
}

type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error { return &usageError{fmt.Sprintf(format, args...)} }

// Main runs the CLI and returns the process exit code.
func Main(ctx context.Context, args []string, env Env) int {
	if env.Now == nil {
		env.Now = time.Now
	}
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, mainUsage)
		return ExitUsage
	}
	var err error
	code := ExitPassed
	switch args[0] {
	case "run":
		code, err = runCommand(ctx, args[1:], env)
	case "list", "ls":
		err = listCommand(ctx, args[1:], env)
	case "version", "--version", "-v":
		fmt.Fprintf(env.Stdout, "%s CLI %s\n", service.AppName, service.Version)
	case "help", "--help", "-h":
		if len(args) > 1 && args[1] == "run" {
			fmt.Fprint(env.Stdout, runUsage)
		} else {
			fmt.Fprint(env.Stdout, mainUsage)
		}
	default:
		err = usagef("unknown command %q", args[0])
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitPassed
		}
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		var ue *usageError
		if errors.As(err, &ue) {
			fmt.Fprintf(env.Stderr, "Run \"%s help\" for usage.\n", progName)
		}
		if ctx.Err() != nil {
			return ExitInterrupted
		}
		return ExitUsage
	}
	return code
}

const progName = "api-test-kits-cli"

const mainUsage = `Usage: api-test-kits-cli <command> [arguments]

Commands:
  run    Run a collection and exit non-zero when a test fails
  list   Show a collection's environments, folders, requests and baselines
  version
  help [run]

Examples:
  api-test-kits-cli run wallet.collection.json --env Staging --junit junit.xml
  APIGUARD_VAR_token=$API_TOKEN api-test-kits-cli run wallet.collection.json --env Staging
  api-test-kits-cli run --collection "Wallet demo" --env Local --report report.html
`

const runUsage = `Usage: api-test-kits-cli run [FILE] [flags]

Runs every request of a collection (or one folder), evaluates its tests and
compares responses with the approved baselines.

Source (one of):
  FILE                      a collection export (File → Export, tick "Include baselines"
                            for regression checks) or an OpenAPI / Swagger document
  --collection NAME         a collection in the desktop app's local database
  --data-dir DIR            app data directory for --collection (default: the app's, or $APIGUARD_DATA_DIR)

Selection:
  -e, --env NAME            environment to run against (required when the file has several)
  --folder PATH             run only this folder, e.g. "Wallet" or "Wallet/Admin"

Variables:
  --var KEY=VALUE           set a variable (repeatable); overrides the environment's value
  APIGUARD_VAR_KEY=VALUE    same, from the process environment, and marked secret (redacted
                            from output and reports) — use this for tokens and passwords

Reports:
  --report FILE             write a report; format from the extension: .html, .json, .xml (JUnit)
                            (repeatable)
  --junit FILE              write a JUnit XML report

Behaviour:
  --no-baselines            do not compare with baselines
  --confirm-protected       allow running data-modifying requests against a protected environment
  --concurrency N           requests in parallel, 1–8 (default 1: in order)
  --timeout DURATION        stop the whole run after this long, e.g. 5m (default: no limit)
  --insecure                do not verify TLS certificates
  --ca-cert FILE            trust the CA certificates in this PEM file
  --proxy none|system|URL   proxy to use (default: system, from HTTPS_PROXY / HTTP_PROXY)

Output:
  -q, --quiet               print only failures and the summary
  --verbose                 also print the response body of failing requests
  --no-color                disable colours (also: NO_COLOR=1)

Exit codes: 0 all passed · 1 tests failed · 2 requests errored · 3 usage or setup error · 130 interrupted
`

// ---- options ----

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

type sourceOptions struct {
	file       string
	collection string
	dataDir    string
}

type runOptions struct {
	source           sourceOptions
	env              string
	folder           string
	vars             multiFlag
	reports          multiFlag
	junit            string
	noBaselines      bool
	confirmProtected bool
	concurrency      int
	timeout          time.Duration
	insecure         bool
	caCert           string
	proxy            string
	quiet, verbose   bool
	noColor          bool
	set              map[string]bool
}

// parseInterleaved parses flags that may come before or after positional
// arguments, which the standard flag package does not allow on its own.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

func addSourceFlags(fs *flag.FlagSet, s *sourceOptions) {
	fs.StringVar(&s.collection, "collection", "", "")
	fs.StringVar(&s.dataDir, "data-dir", "", "")
}

func (s *sourceOptions) finish(positional []string) error {
	switch {
	case len(positional) > 1:
		return usagef("expected one collection file, got %d arguments: %s", len(positional), strings.Join(positional, " "))
	case len(positional) == 1 && s.collection != "":
		return usagef("give either a collection file or --collection, not both")
	case len(positional) == 1:
		s.file = positional[0]
	case s.collection == "":
		return usagef("give a collection file, or --collection NAME to use the desktop app's database")
	}
	return nil
}

func parseRunOptions(args []string, env Env) (*runOptions, error) {
	o := &runOptions{set: map[string]bool{}}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	addSourceFlags(fs, &o.source)
	fs.StringVar(&o.env, "env", "", "")
	fs.StringVar(&o.env, "e", "", "")
	fs.StringVar(&o.folder, "folder", "", "")
	fs.Var(&o.vars, "var", "")
	fs.Var(&o.reports, "report", "")
	fs.StringVar(&o.junit, "junit", "", "")
	fs.BoolVar(&o.noBaselines, "no-baselines", false, "")
	fs.BoolVar(&o.confirmProtected, "confirm-protected", false, "")
	fs.IntVar(&o.concurrency, "concurrency", 0, "")
	fs.DurationVar(&o.timeout, "timeout", 0, "")
	fs.BoolVar(&o.insecure, "insecure", false, "")
	fs.StringVar(&o.caCert, "ca-cert", "", "")
	fs.StringVar(&o.proxy, "proxy", "", "")
	fs.BoolVar(&o.quiet, "quiet", false, "")
	fs.BoolVar(&o.quiet, "q", false, "")
	fs.BoolVar(&o.verbose, "verbose", false, "")
	fs.BoolVar(&o.noColor, "no-color", false, "")
	help := false
	fs.BoolVar(&help, "help", false, "")
	fs.BoolVar(&help, "h", false, "")
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return nil, usagef("%v", err)
	}
	if help {
		fmt.Fprint(env.Stdout, runUsage)
		return nil, flag.ErrHelp
	}
	fs.Visit(func(f *flag.Flag) { o.set[f.Name] = true })
	if err := o.source.finish(positional); err != nil {
		return nil, err
	}
	if o.set["concurrency"] && (o.concurrency < 1 || o.concurrency > 8) {
		return nil, usagef("--concurrency must be between 1 and 8")
	}
	if o.timeout < 0 {
		return nil, usagef("--timeout must not be negative")
	}
	for _, r := range o.reports {
		if _, err := reportFormat(r); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func reportFormat(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm":
		return "html", nil
	case ".json":
		return "json", nil
	case ".xml":
		return "junit", nil
	}
	return "", usagef("cannot tell the report format of %q: use .html, .json or .xml (JUnit)", path)
}

// ---- loading ----

type loaded struct {
	imp *collectionio.Imported
	// settings are the desktop app's settings in --collection mode, else defaults.
	settings domain.Settings
	label    string
}

// load reads the collection from a file or from the app database. Secrets
// and baselines come along in --collection mode; the app database is only read.
func load(ctx context.Context, s sourceOptions) (*loaded, error) {
	if s.file != "" {
		st, err := os.Stat(s.file)
		if err != nil {
			return nil, usagef("cannot read %s: %v", s.file, unwrapPathError(err))
		}
		if st.IsDir() {
			return nil, usagef("%s is a directory, not a collection file", s.file)
		}
		if st.Size() > collectionio.MaxFileBytes {
			return nil, usagef("%s is larger than %d MB", s.file, collectionio.MaxFileBytes>>20)
		}
		data, err := os.ReadFile(s.file)
		if err != nil {
			return nil, usagef("cannot read %s: %v", s.file, unwrapPathError(err))
		}
		imp, err := collectionio.Read(bytes.NewReader(data))
		if err != nil {
			return nil, usagef("%s: %v", s.file, err)
		}
		absolutizeUploads(imp, filepath.Dir(s.file))
		return &loaded{imp: imp, settings: domain.DefaultSettings(), label: s.file}, nil
	}

	path := ""
	if s.dataDir != "" {
		path = filepath.Join(s.dataDir, "data.db")
	} else {
		p, err := appdata.DatabasePath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	if _, err := os.Stat(path); err != nil {
		return nil, usagef("no app database at %s (start the desktop app once, or use --data-dir)", path)
	}
	store, err := storage.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open app database: %w", err)
	}
	defer store.Close()
	app, err := service.New(store)
	if err != nil {
		return nil, err
	}
	cols, err := app.ListCollections(ctx)
	if err != nil {
		return nil, err
	}
	var match []domain.Collection
	var names []string
	for _, c := range cols {
		names = append(names, c.Name)
		if strings.EqualFold(c.Name, s.collection) {
			match = append(match, c)
		}
	}
	switch len(match) {
	case 0:
		return nil, usagef("no collection named %q in %s; collections: %s", s.collection, path, quoteList(names))
	case 1:
	default:
		return nil, usagef("%d collections are named %q; rename one in the app", len(match), s.collection)
	}
	envs, err := app.ListEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	envIDs := make([]string, len(envs))
	for i, e := range envs {
		envIDs[i] = e.ID
	}
	data, err := app.ExportCollection(ctx, match[0].ID, envIDs, true, true)
	if err != nil {
		return nil, err
	}
	imp, err := collectionio.Read(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &loaded{imp: imp, settings: app.GetSettings(), label: fmt.Sprintf("app collection %q", match[0].Name)}, nil
}

// absolutizeUploads makes relative upload paths in a collection file relative
// to the file's directory, so a collection and its test files can be checked
// in together. Paths starting with a {{variable}} are left alone.
func absolutizeUploads(imp *collectionio.Imported, dir string) {
	abs := func(p string) string {
		if p == "" || strings.HasPrefix(p, "{{") || filepath.IsAbs(p) {
			return p
		}
		if d, err := filepath.Abs(dir); err == nil {
			return filepath.Join(d, p)
		}
		return p
	}
	for i := range imp.Tree.Requests {
		r := &imp.Tree.Requests[i]
		r.BodyFile = abs(r.BodyFile)
		for j := range r.Form {
			r.Form[j].File = abs(r.Form[j].File)
		}
	}
}

func unwrapPathError(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

func quoteList(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}

// ---- variables ----

type cliVar struct {
	key, value string
	secret     bool
	source     string
}

func collectVars(o *runOptions, env Env) ([]cliVar, error) {
	var out []cliVar
	if env.Environ != nil {
		for _, kv := range env.Environ() {
			if !strings.HasPrefix(kv, EnvVarPrefix) {
				continue
			}
			k, v, _ := strings.Cut(strings.TrimPrefix(kv, EnvVarPrefix), "=")
			if k == "" {
				continue
			}
			if !domain.IsValidVariableName(k) {
				return nil, usagef("%s%s: %q is not a valid variable name", EnvVarPrefix, k, k)
			}
			out = append(out, cliVar{key: k, value: v, secret: true, source: EnvVarPrefix + k})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	for _, kv := range o.vars {
		k, v, ok := strings.Cut(kv, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, usagef("--var %q: expected KEY=VALUE", kv)
		}
		if !domain.IsValidVariableName(k) {
			return nil, usagef("--var %q: %q is not a valid variable name", kv, k)
		}
		out = append(out, cliVar{key: k, value: v, source: "--var " + k})
	}
	return out, nil
}

// applyVars sets vars in list, keeping an existing variable's secret flag.
func applyVars(list []domain.Variable, vars []cliVar) []domain.Variable {
	out := append([]domain.Variable(nil), list...)
	for _, cv := range vars {
		found := false
		for i := range out {
			if out[i].Key == cv.key {
				out[i].Value, out[i].Enabled = cv.value, true
				out[i].Secret = out[i].Secret || cv.secret
				found = true
			}
		}
		if !found {
			out = append(out, domain.Variable{Key: cv.key, Value: cv.value, Secret: cv.secret, Enabled: true})
		}
	}
	return out
}

// ---- environment and folder selection ----

func chooseEnvironment(imp *collectionio.Imported, name string) (string, error) {
	var names []string
	for _, e := range imp.Environments {
		names = append(names, e.Name)
	}
	if name != "" {
		for _, e := range imp.Environments {
			if e.Name == name {
				return name, nil
			}
		}
		for _, e := range imp.Environments {
			if strings.EqualFold(e.Name, name) {
				return e.Name, nil
			}
		}
		// Not in the file: run with an empty environment of that name, filled by
		// --var / APIGUARD_VAR_*, so baselines approved for it still apply.
		imp.Environments = append(imp.Environments, domain.Environment{Name: name, Variables: []domain.Variable{}})
		return name, nil
	}
	switch len(imp.Environments) {
	case 0:
		return "", nil
	case 1:
		return imp.Environments[0].Name, nil
	}
	return "", usagef("the collection has %d environments; choose one with --env: %s", len(names), quoteList(names))
}

func findFolder(tree domain.CollectionTree, path string) (string, error) {
	byID := map[string]domain.Folder{}
	for _, f := range tree.Folders {
		byID[f.ID] = f
	}
	fullPath := func(f domain.Folder) string {
		parts := []string{f.Name}
		for p, ok := byID[f.ParentID]; ok && len(parts) < 100; p, ok = byID[p.ParentID] {
			parts = append([]string{p.Name}, parts...)
		}
		return strings.Join(parts, "/")
	}
	want := strings.Trim(path, "/ ")
	var byPath, byName []domain.Folder
	var all []string
	for _, f := range tree.Folders {
		fp := fullPath(f)
		all = append(all, fp)
		if strings.EqualFold(fp, want) {
			byPath = append(byPath, f)
		}
		if strings.EqualFold(f.Name, want) {
			byName = append(byName, f)
		}
	}
	switch {
	case len(byPath) == 1:
		return byPath[0].ID, nil
	case len(byPath) == 0 && len(byName) == 1:
		return byName[0].ID, nil
	case len(byPath)+len(byName) > 1:
		return "", usagef("--folder %q matches several folders; use the full path: %s", path, quoteList(all))
	}
	sort.Strings(all)
	return "", usagef("no folder %q; folders: %s", path, quoteList(all))
}
