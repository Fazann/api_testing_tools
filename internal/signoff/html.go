package signoff

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"time"
)

// WriteHTML writes the report as one self-contained HTML file: no scripts,
// no external resources, print styles for A4 and Letter. html/template
// escapes every value.
func WriteHTML(w io.Writer, r Report) error { return htmlTemplate.Execute(w, r) }

var htmlTemplate = template.Must(template.New("signoff").Funcs(template.FuncMap{
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("2006-01-02 15:04 MST")
	},
	"ms": func(v int64) string {
		if v >= 1000 {
			return fmt.Sprintf("%.2f s", float64(v)/1000)
		}
		return fmt.Sprintf("%d ms", v)
	},
	"pct": func(p *float64) string {
		if p == nil {
			return "—"
		}
		sign := "+"
		if *p < 0 {
			sign = "−"
		}
		return fmt.Sprintf("%s%.0f %%", sign, math.Abs(*p))
	},
	"kind": func(k any) string {
		switch fmt.Sprint(k) {
		case "added":
			return "Added"
		case "removed":
			return "Removed"
		case "type_changed":
			return "Type changed"
		}
		return "Changed"
	},
	"or": func(s, alt string) string {
		if s == "" {
			return alt
		}
		return s
	},
}).Parse(htmlSource))

const htmlSource = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sign-off – {{.Collection}} {{.Release}}</title>
<style>
:root{--bg:#fff;--fg:#1b1f24;--muted:#5b6470;--line:#d9dee4;--card:#f6f8fa;--pass:#1a7f37;--fail:#cf222e;--warn:#9a6700;--add:#dafbe1;--rem:#ffebe9;--chg:#fff8c5}
@media screen and (prefers-color-scheme:dark){:root{--bg:#0d1117;--fg:#e6edf3;--muted:#8d96a0;--line:#30363d;--card:#161b22;--pass:#3fb950;--fail:#f85149;--warn:#d29922;--add:#12261e;--rem:#2d1214;--chg:#272115}}
*{box-sizing:border-box}body{margin:0;padding:24px 16px;background:var(--bg);color:var(--fg);font:13px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
main{max-width:960px;margin:0 auto}h1{font-size:22px;margin:0}h2{font-size:15px;margin:22px 0 6px;padding-bottom:4px;border-bottom:1px solid var(--line)}
.muted{color:var(--muted)}.meta{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:4px 24px;margin:10px 0 16px}
.meta b{display:block;font-size:11px;color:var(--muted);font-weight:600;text-transform:uppercase;letter-spacing:.03em}
.verdict{border:2px solid currentColor;border-radius:8px;padding:12px 14px;margin:8px 0 4px}.verdict strong{font-size:18px;display:block}
.verdict ul{margin:6px 0 0;padding-left:20px;color:var(--fg)}
.pass{color:var(--pass)}.fail{color:var(--fail)}.warn{color:var(--warn)}
.tiles{display:grid;grid-template-columns:repeat(auto-fit,minmax(110px,1fr));gap:8px;margin:6px 0}
.tile{background:var(--card);border:1px solid var(--line);border-radius:6px;padding:8px 10px}.tile b{display:block;font-size:18px}
table{width:100%;border-collapse:collapse;margin:6px 0}th,td{text-align:left;padding:5px 8px;border-bottom:1px solid var(--line);vertical-align:top}
th{font-size:11px;color:var(--muted);font-weight:600;text-transform:uppercase;letter-spacing:.03em}
code,.mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px;word-break:break-all}
.method{font-family:ui-monospace,monospace;font-weight:700;font-size:11px;margin-right:6px}
tr.added td{background:var(--add)}tr.removed td{background:var(--rem)}tr.changed td,tr.type_changed td{background:var(--chg)}
.item{border:1px solid var(--line);border-radius:6px;padding:8px 10px;margin:6px 0;break-inside:avoid}
.item ul{margin:4px 0 0;padding-left:18px}.notes{white-space:pre-wrap;background:var(--card);border:1px solid var(--line);border-radius:6px;padding:8px 10px}
.sign{display:grid;grid-template-columns:2fr 1fr 2fr;gap:24px;margin-top:28px}.sign div{border-top:1px solid var(--fg);padding-top:4px;font-size:11px;color:var(--muted)}
section{break-inside:auto}h2{break-after:avoid}thead{display:table-header-group}tr,.verdict{break-inside:avoid}footer{margin-top:24px;font-size:11px}
@media print{body{padding:0;font-size:11px}main{max-width:none}@page{margin:16mm 14mm}.tile,.notes{background:none}a{color:inherit}}
</style>
</head>
<body>
<main>
<h1>Release sign-off: {{.Release}}</h1>
<div class="meta">
<div><b>Collection</b>{{.Collection}}</div>
<div><b>Environment</b>{{.Environment}}</div>
<div><b>Prepared by</b>{{or .PreparedBy "—"}}</div>
<div><b>Generated</b>{{fmtTime .GeneratedAt}}</div>
</div>

{{if .Ready}}
<div class="verdict pass"><strong>✔ Ready for release</strong>
<span class="muted">The newest run passed, nothing differs from its baseline and the newest security checks passed.</span></div>
{{else}}
<div class="verdict fail"><strong>✘ Not ready</strong>
<ul>{{range .Reasons}}<li>{{.}}</li>{{end}}</ul></div>
{{end}}

<section>
<h2>Test run</h2>
{{with .Run}}
<p class="muted">Newest collection run: {{fmtTime .StartedAt}} — covered {{.Covered}} of {{.Requests}} requests.</p>
<div class="tiles">
<div class="tile"><b class="pass">{{.Passed}}</b>passed</div>
<div class="tile"><b class="{{if .Failed}}fail{{end}}">{{.Failed}}</b>failed</div>
<div class="tile"><b class="{{if .Errors}}warn{{end}}">{{.Errors}}</b>errors</div>
<div class="tile"><b>{{.Total}}</b>results</div>
</div>
{{range .Failures}}<div class="item"><span class="method">{{.Method}}</span><strong>{{.Name}}</strong> <span class="{{if eq (print .Status) "error"}}warn{{else}}fail{{end}}">{{.Status}}</span>
{{if .Details}}<ul>{{range .Details}}<li>{{.}}</li>{{end}}</ul>{{end}}</div>{{end}}
{{else}}
<p class="fail">No collection run in this environment. Run the collection, then create the report again.</p>
{{end}}
</section>

<section>
<h2>Regressions</h2>
{{if .Regressions}}{{range .Regressions}}<div class="item"><span class="method">{{.Method}}</span><strong>{{.Name}}</strong> <span class="muted">— {{.Message}}</span>
<table><thead><tr><th>Change</th><th>Path</th><th>Baseline</th><th>Actual</th></tr></thead>
{{range .Diffs}}<tr class="{{.Kind}}"><td>{{kind .Kind}}</td><td class="mono">{{.Path}}</td><td class="mono">{{.Before}}</td><td class="mono">{{.After}}</td></tr>{{end}}
</table>{{if .More}}<p class="muted">… and {{.More}} more difference(s).</p>{{end}}</div>{{end}}
{{else}}<p class="muted">None{{if .Run}}: every compared response matches its baseline{{end}}.</p>{{end}}
</section>

<section>
<h2>Security checks</h2>
<table><thead><tr><th>Check</th><th>Last run</th><th>Result</th></tr></thead>
{{range .Races}}<tr><td>Race test — {{.Name}}</td><td>{{fmtTime .StartedAt}}</td><td>{{if .Passed}}<span class="pass">passed</span>{{else}}<span class="fail">{{.Status}}</span>{{range .Details}}<div class="muted">{{.}}</div>{{end}}{{end}}</td></tr>{{end}}
{{with .Matrix}}<tr><td>Permission matrix</td><td>{{fmtTime .StartedAt}}</td><td>{{if .Passed}}<span class="pass">passed</span>{{else}}<span class="fail">{{.Status}}</span>{{range .Details}}<div class="muted">{{.}}</div>{{end}}{{end}}</td></tr>
{{else}}<tr><td>Permission matrix</td><td>—</td><td class="muted">not run</td></tr>{{end}}
{{if not .Races}}<tr><td>Race tests</td><td>—</td><td class="muted">not run</td></tr>{{end}}
</table>
</section>

<section>
<h2>Performance</h2>
{{if .Performance}}<table><thead><tr><th>Request</th><th>Median</th><th>Latest</th><th>Median change</th><th></th></tr></thead>
{{range .Performance}}<tr><td><span class="method">{{.Method}}</span>{{.Name}}</td><td>{{ms .MedianMs}}</td><td>{{ms .LastMs}}</td><td>{{pct .ChangePct}}</td><td>{{if .Slower}}<span class="warn">slower than usual</span>{{end}}</td></tr>{{end}}
</table>{{else}}<p class="muted">No request was slower than usual, and no median changed by 20 % or more.</p>{{end}}
</section>

<section>
<h2>Baselines</h2>
<table><thead><tr><th>Request</th><th>Compares</th><th>Approved</th><th>Approved by</th></tr></thead>
{{range .Baselines}}<tr><td><span class="method">{{.Method}}</span>{{.Name}}</td>
{{if .Has}}<td>{{.Mode}}{{if .Ignored}} <span class="muted">({{.Ignored}} ignored)</span>{{end}}</td><td>{{fmtTime .ApprovedAt}}</td><td>{{or .ApprovedBy "—"}}</td>
{{else}}<td class="muted" colspan="3">no baseline</td>{{end}}</tr>{{end}}
</table>
<p class="muted">Approver names are as entered when approving; the app has no accounts to verify them.</p>
</section>

<section>
<h2>Coverage</h2>
<table>
<tr><th>Without tests</th><td>{{if .NoTests}}{{range $i, $n := .NoTests}}{{if $i}}, {{end}}{{$n}}{{end}}{{else}}<span class="muted">none</span>{{end}}</td></tr>
<tr><th>Without a baseline</th><td>{{if .NoBaseline}}{{range $i, $n := .NoBaseline}}{{if $i}}, {{end}}{{$n}}{{end}}{{else}}<span class="muted">none</span>{{end}}</td></tr>
<tr><th>Not in the run</th><td>{{if .NotRun}}{{range $i, $n := .NotRun}}{{if $i}}, {{end}}{{$n}}{{end}}{{else}}<span class="muted">none</span>{{end}}</td></tr>
</table>
</section>

<section>
<h2>Sign-off</h2>
{{if .Notes}}<div class="notes">{{.Notes}}</div>{{end}}
<div class="sign"><div>Approved by</div><div>Date</div><div>Signature</div></div>
</section>

<footer class="muted">{{.AppName}} {{.AppVersion}} · built from stored results; no request was sent to create this report.</footer>
</main>
</body>
</html>
`
