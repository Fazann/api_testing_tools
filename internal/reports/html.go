package reports

import (
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"
)

// WriteHTML writes a single self-contained HTML file with no external
// resources. html/template escapes all response content.
func WriteHTML(w io.Writer, r Report) error {
	return htmlTemplate.Execute(w, r)
}

var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("2006-01-02 15:04:05 MST")
	},
	"ms": func(v int64) string {
		if v >= 1000 {
			return fmt.Sprintf("%.2f s", float64(v)/1000)
		}
		return fmt.Sprintf("%d ms", v)
	},
	"lower": func(v any) string { return strings.ToLower(fmt.Sprint(v)) },
	"bytes": func(n int64) string {
		switch {
		case n >= 1<<20:
			return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
		case n >= 1<<10:
			return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
		}
		return fmt.Sprintf("%d B", n)
	},
	"truncate": func(s string, n int) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "\n… (truncated in report)"
	},
}).Parse(htmlSource))

const htmlSource = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>API Test Report – {{if .Collection}}{{.Collection}}{{else}}Request{{end}}</title>
<style>
:root{--bg:#fff;--fg:#1b1f24;--muted:#5b6470;--line:#d9dee4;--card:#f6f8fa;--pass:#1a7f37;--fail:#cf222e;--err:#9a6700;--add:#dafbe1;--rem:#ffebe9;--chg:#fff8c5}
@media (prefers-color-scheme:dark){:root{--bg:#0d1117;--fg:#e6edf3;--muted:#8d96a0;--line:#30363d;--card:#161b22;--pass:#3fb950;--fail:#f85149;--err:#d29922;--add:#12261e;--rem:#2d1214;--chg:#272115}}
*{box-sizing:border-box}body{margin:0;padding:24px 16px;background:var(--bg);color:var(--fg);font:14px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
main{max-width:1100px;margin:0 auto}h1{font-size:22px;margin:0 0 4px}h2{font-size:16px;margin:0}
.muted{color:var(--muted)}.meta{display:flex;flex-wrap:wrap;gap:8px 24px;margin:8px 0 20px}
.tiles{display:grid;grid-template-columns:repeat(auto-fit,minmax(120px,1fr));gap:12px;margin-bottom:24px}
.tile{background:var(--card);border:1px solid var(--line);border-radius:8px;padding:12px}.tile b{display:block;font-size:22px}
.badge{display:inline-block;padding:1px 8px;border-radius:999px;font-size:12px;font-weight:600;border:1px solid currentColor}
.passed{color:var(--pass)}.failed{color:var(--fail)}.error,.cancelled{color:var(--err)}
details.result{border:1px solid var(--line);border-radius:8px;margin-bottom:10px;background:var(--card)}
details.result>summary{cursor:pointer;padding:10px 12px;display:flex;gap:10px;align-items:center;flex-wrap:wrap;list-style:none}
details.result>summary::-webkit-details-marker{display:none}
.body{padding:0 12px 12px;border-top:1px solid var(--line)}
.method{font-family:ui-monospace,monospace;font-weight:700}.url{font-family:ui-monospace,monospace;word-break:break-all;color:var(--muted)}
table{width:100%;border-collapse:collapse;margin:8px 0}th,td{text-align:left;padding:6px 8px;border-bottom:1px solid var(--line);vertical-align:top}
th{font-size:12px;color:var(--muted);font-weight:600}td code,pre{font-family:ui-monospace,monospace;font-size:12px}
pre{background:var(--bg);border:1px solid var(--line);border-radius:6px;padding:8px;overflow:auto;max-height:360px;white-space:pre-wrap;word-break:break-all}
tr.added td{background:var(--add)}tr.removed td{background:var(--rem)}tr.changed td,tr.type_changed td{background:var(--chg)}
h3{font-size:13px;margin:14px 0 4px}.wrap{overflow-x:auto}
footer{margin-top:24px;font-size:12px}
</style>
</head>
<body>
<main>
<h1>API Test Report</h1>
<div class="meta muted">
  <span>Collection: <strong>{{if .Collection}}{{.Collection}}{{else}}(single request){{end}}</strong></span>
  <span>Environment: <strong>{{if .Environment}}{{.Environment}}{{else}}(none){{end}}</strong></span>
  <span>Run started: {{fmtTime .Summary.StartedAt}}</span>
  <span>Generated: {{fmtTime .GeneratedAt}}</span>
  <span>{{.AppName}} {{.AppVersion}}</span>
</div>
<div class="tiles">
  <div class="tile"><span class="muted">Status</span><b class="{{lower .Summary.Status}}">{{.Summary.Status}}</b></div>
  <div class="tile"><span class="muted">Requests</span><b>{{.Summary.Total}}</b></div>
  <div class="tile"><span class="muted">Passed</span><b class="passed">{{.Summary.Passed}}</b></div>
  <div class="tile"><span class="muted">Failed</span><b class="failed">{{.Summary.Failed}}</b></div>
  <div class="tile"><span class="muted">Errors</span><b class="error">{{.Summary.Errors}}</b></div>
  <div class="tile"><span class="muted">Assertions failed</span><b>{{.Summary.AssertionsFailed}} / {{.Summary.Assertions}}</b></div>
  <div class="tile"><span class="muted">Duration</span><b>{{ms .Summary.DurationMs}}</b></div>
</div>
{{range .Run.Results}}
<details class="result"{{if ne .Status "passed"}} open{{end}}>
  <summary>
    <span class="badge {{lower .Status}}">{{.Status}}</span>
    <span class="method">{{.Request.Method}}</span>
    <h2>{{.Request.Name}}</h2>
    <span class="url">{{.Request.URL}}</span>
    <span class="muted">{{if .Response}}{{.Response.StatusCode}} · {{end}}{{ms .DurationMs}}</span>
  </summary>
  <div class="body">
    {{if .Error}}<h3>Error</h3><pre>{{.Error}}</pre>{{end}}
    {{if .AssertionResults}}
    <h3>Assertions</h3>
    <div class="wrap"><table>
      <tr><th>Result</th><th>Check</th><th>Expected</th><th>Actual</th><th>Message</th></tr>
      {{range .AssertionResults}}
      <tr><td>{{if .Passed}}<span class="passed">✔ pass</span>{{else}}<span class="failed">✘ fail</span>{{end}}</td>
        <td><code>{{.Kind}}{{if .Path}} {{.Path}}{{end}}{{if .Operator}} {{.Operator}}{{end}}</code>{{if .Description}}<br><span class="muted">{{.Description}}</span>{{end}}</td>
        <td><code>{{.Expected}}</code></td><td><code>{{.Actual}}</code></td><td>{{.Message}}</td></tr>
      {{end}}
    </table></div>
    {{end}}
    {{with .Regression}}
    <h3>Baseline comparison: <span class="{{if .Passed}}passed{{else}}failed{{end}}">{{.Message}}</span></h3>
    {{if .Diffs}}<div class="wrap"><table>
      <tr><th>Change</th><th>Path</th><th>Baseline</th><th>Actual</th></tr>
      {{range .Diffs}}<tr class="{{.Kind}}"><td>{{.Kind}}</td><td><code>{{.Path}}</code></td><td><code>{{.Before}}</code></td><td><code>{{.After}}</code></td></tr>{{end}}
    </table></div>{{end}}
    {{end}}
    {{with .Response}}
    <h3>Response · {{.Status}} · {{bytes .BodyBytes}}{{if .Truncated}} (truncated){{end}}</h3>
    {{if .Redirects}}<p class="muted">Redirects: {{range .Redirects}}<code>{{.}}</code> {{end}}</p>{{end}}
    {{if .Headers}}<div class="wrap"><table>{{range .Headers}}<tr><td><code>{{.Key}}</code></td><td><code>{{.Value}}</code></td></tr>{{end}}</table></div>{{end}}
    {{if .Body}}<pre>{{truncate .Body 20000}}</pre>{{end}}
    {{end}}
  </div>
</details>
{{else}}
<p class="muted">This run has no results.</p>
{{end}}
<footer class="muted">
  TLS certificate verification: {{if .Network.TLSVerification}}on{{else}}<strong class="failed">off</strong>{{end}} · Proxy: {{.Network.ProxyMode}}.
  Secret values, authorization headers and cookies are removed from this report.
</footer>
</main>
</body>
</html>
`
