package reports

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"api-test-kits/internal/domain"
)

// JUnit XML is read by most CI systems (GitHub Actions reporters, GitLab,
// Jenkins, Azure DevOps). One test case is written per request result.

type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Name     string       `xml:"name,attr"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Errors   int          `xml:"errors,attr"`
	Time     string       `xml:"time,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name       string          `xml:"name,attr"`
	Tests      int             `xml:"tests,attr"`
	Failures   int             `xml:"failures,attr"`
	Errors     int             `xml:"errors,attr"`
	Skipped    int             `xml:"skipped,attr"`
	Time       string          `xml:"time,attr"`
	Timestamp  string          `xml:"timestamp,attr"`
	Properties []junitProperty `xml:"properties>property,omitempty"`
	Cases      []junitCase     `xml:"testcase"`
}

type junitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitMessage `xml:"failure,omitempty"`
	Error     *junitMessage `xml:"error,omitempty"`
	Skipped   *junitMessage `xml:"skipped,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitMessage struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr,omitempty"`
	Body    string `xml:",chardata"`
}

func seconds(ms int64) string { return fmt.Sprintf("%.3f", float64(ms)/1000) }

// WriteJUnit writes the report's run as JUnit XML. The run in a Report is
// already sanitized by Build.
func WriteJUnit(w io.Writer, r Report) error {
	run := r.Run
	suiteName := run.CollectionName
	if suiteName == "" {
		suiteName = "Requests"
	}
	suite := junitSuite{Name: suiteName, Time: seconds(run.DurationMs), Timestamp: run.StartedAt.UTC().Format("2006-01-02T15:04:05")}
	if run.EnvironmentName != "" {
		suite.Properties = append(suite.Properties, junitProperty{Name: "environment", Value: run.EnvironmentName})
	}
	for _, res := range run.Results {
		tc := junitCase{Name: caseName(res), Classname: suiteName, Time: seconds(res.DurationMs)}
		details := failureDetails(res)
		switch res.Status {
		case domain.StatusFailed:
			suite.Failures++
			tc.Failure = &junitMessage{Message: firstLine(details), Type: "AssertionFailure", Body: details}
		case domain.StatusError:
			suite.Errors++
			tc.Error = &junitMessage{Message: firstLine(res.Error), Type: "RequestError", Body: details}
		case domain.StatusCancelled:
			suite.Skipped++
			tc.Skipped = &junitMessage{Message: "cancelled"}
		}
		if res.Response != nil {
			tc.SystemOut = fmt.Sprintf("%s %s → %d in %d ms", res.Request.Method, res.Request.URL, res.Response.StatusCode, res.DurationMs)
		}
		suite.Cases = append(suite.Cases, tc)
	}
	suite.Tests = len(suite.Cases)
	doc := junitSuites{Name: suiteName, Tests: suite.Tests, Failures: suite.Failures, Errors: suite.Errors,
		Time: suite.Time, Suites: []junitSuite{suite}}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func caseName(res domain.RunResult) string {
	name := res.Request.Name
	if res.Request.Method != "" {
		name = res.Request.Method + " " + name
	}
	return name
}

// FailureDetails lists why a result did not pass, one problem per line.
func FailureDetails(res domain.RunResult) []string {
	var out []string
	if res.Error != "" {
		out = append(out, res.Error)
	}
	for _, a := range res.AssertionResults {
		if a.Passed {
			continue
		}
		label := a.Description
		if label == "" {
			label = string(a.Kind)
			if a.Path != "" {
				label += " " + a.Path
			}
		}
		line := label
		if a.Expected != "" || a.Actual != "" {
			line += fmt.Sprintf(": expected %s, got %s", a.Expected, a.Actual)
		}
		if a.Message != "" {
			line += " — " + a.Message
		}
		out = append(out, line)
	}
	if reg := res.Regression; reg != nil && !reg.Passed {
		msg := reg.Message
		if msg == "" {
			msg = fmt.Sprintf("%d difference(s) from the baseline", len(reg.Diffs))
		}
		out = append(out, "baseline: "+msg)
		const maxDiffs = 20
		for i, d := range reg.Diffs {
			if i == maxDiffs {
				out = append(out, fmt.Sprintf("  … and %d more", len(reg.Diffs)-maxDiffs))
				break
			}
			out = append(out, "  "+DiffLine(d))
		}
	}
	if p := res.Performance; p != nil && p.Slower && p.Mode == domain.PerformanceFail {
		out = append(out, "response time: "+p.Message)
	}
	return out
}

// DiffLine describes one baseline difference.
func DiffLine(d domain.DiffEntry) string {
	switch d.Kind {
	case domain.DiffAdded:
		return fmt.Sprintf("+ %s added: %s", d.Path, d.After)
	case domain.DiffRemoved:
		return fmt.Sprintf("- %s removed (was %s)", d.Path, d.Before)
	case domain.DiffTypeChanged:
		return fmt.Sprintf("~ %s type changed: %s → %s", d.Path, d.Before, d.After)
	default:
		return fmt.Sprintf("~ %s changed: %s → %s", d.Path, d.Before, d.After)
	}
}

func failureDetails(res domain.RunResult) string { return strings.Join(FailureDetails(res), "\n") }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
