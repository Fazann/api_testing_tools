package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"api-test-kits/internal/collectionio"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/fsutil"
	"api-test-kits/internal/reports"
	"api-test-kits/internal/rules"
	"api-test-kits/internal/service"
	"api-test-kits/internal/storage"
)

type painter struct{ on bool }

func (p painter) c(code, s string) string {
	if !p.on {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}
func (p painter) green(s string) string  { return p.c("32", s) }
func (p painter) red(s string) string    { return p.c("31", s) }
func (p painter) yellow(s string) string { return p.c("33", s) }
func (p painter) dim(s string) string    { return p.c("2", s) }
func (p painter) bold(s string) string   { return p.c("1", s) }

func runCommand(ctx context.Context, args []string, env Env) (int, error) {
	o, err := parseRunOptions(args, env)
	if err != nil {
		return ExitUsage, err
	}
	paint := painter{on: env.Color && !o.noColor && (env.Getenv == nil || env.Getenv("NO_COLOR") == "")}
	out := env.Stdout

	vars, err := collectVars(o, env)
	if err != nil {
		return ExitUsage, err
	}
	l, err := load(ctx, o.source)
	if err != nil {
		return ExitUsage, err
	}
	imp := l.imp
	if o.noBaselines {
		imp.Baselines = nil
	}
	envName, err := chooseEnvironment(imp, o.env)
	if err != nil {
		return ExitUsage, err
	}
	// --var and APIGUARD_VAR_* override the selected environment; without an
	// environment they become collection variables.
	if envName != "" {
		for i := range imp.Environments {
			if imp.Environments[i].Name == envName {
				imp.Environments[i].Variables = applyVars(imp.Environments[i].Variables, vars)
			}
		}
	} else {
		imp.Tree.Collection.Variables = applyVars(imp.Tree.Collection.Variables, vars)
	}

	store, err := storage.Open(":memory:")
	if err != nil {
		return ExitUsage, err
	}
	defer store.Close()
	svc, err := service.New(store)
	if err != nil {
		return ExitUsage, err
	}
	st, err := o.settings(l.settings)
	if err != nil {
		return ExitUsage, err
	}
	if err := svc.UseSettings(st); err != nil {
		return ExitUsage, err
	}
	res, err := svc.ImportParsed(ctx, imp)
	if err != nil {
		return ExitUsage, err
	}
	envID := ""
	var selected *domain.Environment
	if envName != "" {
		envs, err := svc.ListEnvironments(ctx)
		if err != nil {
			return ExitUsage, err
		}
		for i := range envs {
			if envs[i].Name == envName {
				envID, selected = envs[i].ID, &envs[i]
			}
		}
	}
	tree, err := svc.GetCollectionTree(ctx, res.Collection.ID)
	if err != nil {
		return ExitUsage, err
	}
	folderID := ""
	if o.folder != "" {
		if folderID, err = findFolder(tree, o.folder); err != nil {
			return ExitUsage, err
		}
	}
	reqs := service.OrderedRequests(tree, folderID)
	if len(reqs) == 0 {
		return ExitUsage, usagef("there are no requests to run")
	}

	// ---- header and warnings ----
	baselines := 0
	for _, b := range imp.Baselines {
		if b.Environment == envName {
			baselines++
		}
	}
	if !o.quiet {
		fmt.Fprintf(out, "%s %s\n", paint.bold(service.AppName+" CLI"), service.Version)
		where := "no environment"
		if envName != "" {
			where = fmt.Sprintf("environment %q", envName)
			if selected != nil && selected.Protected {
				where += " (protected)"
			}
		}
		scope := ""
		if o.folder != "" {
			scope = fmt.Sprintf(", folder %q", o.folder)
		}
		bl := fmt.Sprintf("%d baseline(s)", baselines)
		if o.noBaselines {
			bl = "baselines off"
		}
		fmt.Fprintf(out, "Collection %q from %s · %s%s · %d request(s) · %s\n\n",
			tree.Collection.Name, l.label, where, scope, len(reqs), bl)
	}
	warn := func(msg string) { fmt.Fprintf(env.Stderr, "%s %s\n", paint.yellow("warning:"), msg) }
	if res.BaselinesSkipped > 0 {
		warn(fmt.Sprintf("%d baseline(s) belong to environments that are not in the file and were not used", res.BaselinesSkipped))
	}
	for _, name := range emptySecrets(selected, tree.Collection.Variables) {
		warn(fmt.Sprintf("secret variable %q is empty (secrets are not exported); set it with %s%s=…", name, EnvVarPrefix, name))
	}
	if pre, err := svc.PreflightCollection(ctx, tree.Collection.ID, folderID, envID); err == nil {
		for _, w := range pre {
			if w.Level != service.LevelInfo && !w.RequiresConfirmation && w.Code != "missing_required" {
				warn(w.Message)
			}
		}
	}

	// ---- run ----
	if o.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}
	width := 0
	for _, r := range reqs {
		width = max(width, utf8.RuneCountInString(r.Name))
	}
	if len(tree.Collection.Rules) > 0 {
		width = max(width, utf8.RuneCountInString(rules.ResultName))
	}
	width = min(width, 48)
	run, err := svc.RunCollection(ctx, "cli", tree.Collection.ID, folderID, envID, o.confirmProtected,
		func(_, _ int, r domain.RunResult) { printResult(out, paint, o, r, width) })
	var confirm *service.ConfirmationRequiredError
	if errors.As(err, &confirm) {
		for _, w := range confirm.Warnings {
			if w.RequiresConfirmation {
				fmt.Fprintf(env.Stderr, "error: %s\n", w.Message)
			}
		}
		fmt.Fprintln(env.Stderr, "Pass --confirm-protected to run data-modifying requests against this protected environment.")
		return ExitUsage, nil
	}
	if err != nil && run.ID == "" {
		return ExitUsage, err
	}
	if err != nil {
		warn(err.Error())
	}

	// ---- summary and reports ----
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	printSummary(out, paint, run, timedOut)
	type target struct{ path, format string }
	var targets []target
	for _, p := range o.reports {
		f, _ := reportFormat(p) // validated when parsing
		targets = append(targets, target{p, f})
	}
	if o.junit != "" {
		targets = append(targets, target{o.junit, "junit"})
	}
	var written []string
	for _, t := range targets {
		data, err := svc.ExportReport(context.WithoutCancel(ctx), run.ID, t.format)
		if err == nil {
			err = fsutil.WriteFileAtomic(t.path, data)
		}
		if err != nil {
			fmt.Fprintf(env.Stderr, "error: report %s: %v\n", t.path, err)
			return ExitUsage, nil
		}
		written = append(written, t.path)
	}
	if len(written) > 0 && !o.quiet {
		fmt.Fprintf(out, "Reports: %s\n", strings.Join(written, ", "))
	}

	switch run.Status {
	case domain.StatusPassed:
		return ExitPassed, nil
	case domain.StatusFailed:
		return ExitFailed, nil
	case domain.StatusCancelled:
		if timedOut {
			return ExitErrors, nil
		}
		return ExitInterrupted, nil
	}
	return ExitErrors, nil
}

// settings applies command-line overrides to base.
func (o *runOptions) settings(base domain.Settings) (domain.Settings, error) {
	st := base
	if o.set["concurrency"] {
		st.Concurrency = o.concurrency
	}
	if o.insecure {
		st.InsecureSkipVerify = true
	}
	if o.caCert != "" {
		if _, err := os.Stat(o.caCert); err != nil {
			return st, usagef("--ca-cert %s: %v", o.caCert, unwrapPathError(err))
		}
		st.CACertFile = o.caCert
	}
	switch o.proxy {
	case "":
	case "none", "system":
		st.ProxyMode, st.ProxyURL = o.proxy, ""
	default:
		st.ProxyMode, st.ProxyURL = "custom", o.proxy
	}
	if err := st.Validate(); err != nil {
		return st, usagef("%v", err)
	}
	return st, nil
}

// emptySecrets lists secret variables that have no value, usually because
// exports leave secrets out and nothing on the command line set them.
func emptySecrets(env *domain.Environment, collection []domain.Variable) []string {
	defined := map[string]bool{}
	var out []string
	check := func(vars []domain.Variable) {
		for _, v := range vars {
			if !v.Enabled || v.Key == "" || defined[v.Key] {
				continue
			}
			defined[v.Key] = true
			if v.Secret && v.Value == "" {
				out = append(out, v.Key)
			}
		}
	}
	if env != nil {
		check(env.Variables)
	}
	check(collection)
	sort.Strings(out)
	return out
}

func pad(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n > width {
		r := []rune(s)
		return string(r[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-n)
}

func printResult(w io.Writer, p painter, o *runOptions, r domain.RunResult, width int) {
	failed := r.Status != domain.StatusPassed
	if o.quiet && !failed {
		return
	}
	var mark string
	switch r.Status {
	case domain.StatusPassed:
		mark = p.green("✔ PASS ")
	case domain.StatusFailed:
		mark = p.red("✘ FAIL ")
	case domain.StatusCancelled:
		mark = p.yellow("- SKIP ")
	default:
		mark = p.yellow("! ERROR")
	}
	code := "   —"
	if r.Response != nil {
		code = fmt.Sprintf("%4d", r.Response.StatusCode)
	}
	fmt.Fprintf(w, "%s %-7s %s %s %s\n", mark, r.Request.Method, pad(r.Request.Name, max(width, 4)), code,
		p.dim(fmt.Sprintf("%6d ms", r.DurationMs)))
	if !failed {
		return
	}
	for _, line := range reports.FailureDetails(r) {
		fmt.Fprintf(w, "        %s\n", line)
	}
	if o.verbose && r.Response != nil && r.Response.Body != "" {
		body := r.Response.Body
		const limit = 2000
		if len(body) > limit {
			body = body[:limit] + "\n… (truncated)"
		}
		fmt.Fprintf(w, "        %s\n", p.dim("response body:"))
		for _, line := range strings.Split(body, "\n") {
			fmt.Fprintf(w, "          %s\n", p.dim(line))
		}
	}
}

func printSummary(w io.Writer, p painter, run domain.Run, timedOut bool) {
	var verdict string
	switch run.Status {
	case domain.StatusPassed:
		verdict = p.green(p.bold("PASSED"))
	case domain.StatusFailed:
		verdict = p.red(p.bold("FAILED"))
	case domain.StatusCancelled:
		verdict = p.yellow(p.bold("CANCELLED"))
		if timedOut {
			verdict = p.yellow(p.bold("TIMED OUT"))
		}
	default:
		verdict = p.yellow(p.bold("ERRORS"))
	}
	skipped := len(run.Results) - run.PassedCount - run.FailedCount - run.ErrorCount
	parts := []string{fmt.Sprintf("%d passed", run.PassedCount), fmt.Sprintf("%d failed", run.FailedCount),
		fmt.Sprintf("%d errors", run.ErrorCount)}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	fmt.Fprintf(w, "\n%s — %s in %s\n", verdict, strings.Join(parts, ", "),
		(time.Duration(run.DurationMs) * time.Millisecond).Round(10*time.Millisecond))
}

// ---- list ----

func listCommand(ctx context.Context, args []string, env Env) error {
	var src sourceOptions
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addSourceFlags(fs, &src)
	positional, err := parseInterleaved(fs, args)
	if err != nil {
		return usagef("%v", err)
	}
	if err := src.finish(positional); err != nil {
		return err
	}
	l, err := load(ctx, src)
	if err != nil {
		return err
	}
	printList(env.Stdout, l.imp, l.label)
	return nil
}

func printList(w io.Writer, imp *collectionio.Imported, label string) {
	t := imp.Tree
	fmt.Fprintf(w, "Collection %q (%s)\n", t.Collection.Name, label)

	fmt.Fprintln(w, "\nEnvironments:")
	if len(imp.Environments) == 0 {
		fmt.Fprintln(w, "  (none in the file; use --env NAME with --var / APIGUARD_VAR_* to define one)")
	}
	for _, e := range imp.Environments {
		flags := ""
		if e.Protected {
			flags = " (protected)"
		}
		fmt.Fprintf(w, "  %s%s: %s\n", e.Name, flags, describeVars(e.Variables))
	}
	if len(t.Collection.Variables) > 0 {
		fmt.Fprintf(w, "  collection variables: %s\n", describeVars(t.Collection.Variables))
	}

	envsOf := map[string][]string{}
	for _, b := range imp.Baselines {
		name := b.Environment
		if name == "" {
			name = "(no environment)"
		}
		envsOf[b.Baseline.RequestID] = append(envsOf[b.Baseline.RequestID], name)
	}
	fmt.Fprintf(w, "\nRequests (%d):\n", len(t.Requests))
	children := map[string][]domain.Folder{}
	for _, f := range t.Folders {
		children[f.ParentID] = append(children[f.ParentID], f)
	}
	byParent := map[string][]domain.Request{}
	for _, r := range t.Requests {
		byParent[r.ParentID] = append(byParent[r.ParentID], r)
	}
	// Same order as a run: sort order, then name.
	for k := range children {
		sort.SliceStable(children[k], func(i, j int) bool {
			a, b := children[k][i], children[k][j]
			return a.SortOrder < b.SortOrder || a.SortOrder == b.SortOrder && strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})
	}
	for k := range byParent {
		sort.SliceStable(byParent[k], func(i, j int) bool {
			a, b := byParent[k][i], byParent[k][j]
			return a.SortOrder < b.SortOrder || a.SortOrder == b.SortOrder && strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})
	}
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		if depth > 50 {
			return
		}
		indent := strings.Repeat("  ", depth+1)
		for _, r := range byParent[parent] {
			line := fmt.Sprintf("%s%-7s %s", indent, r.Method, r.Name)
			if b := envsOf[r.ID]; len(b) > 0 {
				line += "  [baseline: " + strings.Join(b, ", ") + "]"
			}
			fmt.Fprintln(w, line)
		}
		for _, f := range children[parent] {
			fmt.Fprintf(w, "%s%s/\n", indent, f.Name)
			walk(f.ID, depth+1)
		}
	}
	walk("", 0)
	fmt.Fprintf(w, "\nBaselines: %d\n", len(imp.Baselines))
}

func describeVars(vars []domain.Variable) string {
	if len(vars) == 0 {
		return "no variables"
	}
	parts := make([]string, 0, len(vars))
	for _, v := range vars {
		if !v.Enabled || v.Key == "" {
			continue
		}
		s := v.Key
		if v.Secret {
			s += " (secret"
			if v.Value == "" {
				s += ", empty"
			}
			s += ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}
