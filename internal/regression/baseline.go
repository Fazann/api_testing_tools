package regression

import (
	"encoding/json"
	"fmt"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
)

// Snapshot builds the JSON stored in a baseline. With no selected paths the
// whole document is stored; otherwise an object maps each selected path to
// the value it selects (wildcard paths store an array of all matches).
func Snapshot(body []byte, selectedPaths []string) (json.RawMessage, error) {
	doc, err := assertions.DecodeJSON(body)
	if err != nil {
		return nil, fmt.Errorf("baseline requires a JSON response: %w", err)
	}
	if len(selectedPaths) == 0 {
		return json.Marshal(doc)
	}
	out := map[string]any{}
	for _, p := range selectedPaths {
		v, ok, err := selectValue(doc, p)
		if err != nil {
			return nil, err
		}
		if ok {
			out[p] = v
		}
	}
	return json.Marshal(out)
}

// Compare checks a response body against an approved baseline.
func Compare(b domain.Baseline, body []byte, truncated bool) domain.RegressionResult {
	res := domain.RegressionResult{BaselineID: b.ID, Diffs: []domain.DiffEntry{}}
	if truncated {
		res.Message = "response was truncated at the size limit, so it cannot be compared with the baseline"
		return res
	}
	actual, err := assertions.DecodeJSON(body)
	if err != nil {
		res.Message = "response is not valid JSON: " + err.Error()
		return res
	}
	snap, err := assertions.DecodeJSON(b.Snapshot)
	if err != nil {
		res.Message = "stored baseline is corrupt: " + err.Error()
		return res
	}
	opts := Options{IgnoredPaths: b.IgnoredPaths, Tolerance: b.Tolerance}
	if len(b.SelectedPaths) == 0 {
		res.Diffs = Diff(snap, actual, opts)
	} else {
		snapMap, _ := snap.(map[string]any)
		d := newDiffer(opts)
		for _, p := range b.SelectedPaths {
			before, beforeOK := snapMap[p]
			after, afterOK, err := selectValue(actual, p)
			if err != nil {
				res.Message = err.Error()
				return res
			}
			d.walkAt(p, before, beforeOK, after, afterOK)
		}
		res.Diffs = d.out
	}
	res.Passed = len(res.Diffs) == 0
	if res.Passed {
		res.Message = "response matches the baseline"
	} else {
		res.Message = fmt.Sprintf("%d difference(s) from the baseline", len(res.Diffs))
	}
	return res
}

func selectValue(doc any, path string) (any, bool, error) {
	segs, err := assertions.ParsePath(path)
	if err != nil {
		return nil, false, err
	}
	matches := assertions.SelectSegments(doc, segs)
	if assertions.HasWildcard(segs) {
		vals := make([]any, len(matches))
		for i, m := range matches {
			vals[i] = m.Value
		}
		return vals, true, nil
	}
	if len(matches) == 0 {
		return nil, false, nil
	}
	return matches[0].Value, true, nil
}
