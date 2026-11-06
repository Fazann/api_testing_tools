package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/signoff"
)

// signoffRunScan is how many recent runs of the collection are searched for
// the newest collection run, race tests and permission matrix.
const signoffRunScan = 500

// SignoffInput describes the release being signed off.
type SignoffInput struct {
	CollectionID  string `json:"collectionId"`
	EnvironmentID string `json:"environmentId"`
	Release       string `json:"release"`
	PreparedBy    string `json:"preparedBy"`
	Notes         string `json:"notes"`
}

// SignoffResult is a built report.
type SignoffResult struct {
	HTML     string   `json:"html"`
	FileName string   `json:"fileName"`
	Ready    bool     `json:"ready"`
	Reasons  []string `json:"reasons"`
}

// BuildSignoffReport builds the sign-off report from stored data; it sends no request.
func (s *Service) BuildSignoffReport(ctx context.Context, in SignoffInput) (SignoffResult, error) {
	in.Release, in.PreparedBy = strings.TrimSpace(in.Release), strings.TrimSpace(in.PreparedBy)
	switch {
	case in.Release == "":
		return SignoffResult{}, errors.New("enter the release being signed off, for example v2.3.0")
	case len([]rune(in.Release)) > domain.MaxNameLength || len([]rune(in.PreparedBy)) > domain.MaxNameLength:
		return SignoffResult{}, fmt.Errorf("release and name must be at most %d characters", domain.MaxNameLength)
	case len(in.Notes) > domain.MaxDescriptionLength:
		return SignoffResult{}, fmt.Errorf("notes must be at most %d characters", domain.MaxDescriptionLength)
	}
	tree, err := s.store.GetTree(ctx, in.CollectionID)
	if err != nil {
		return SignoffResult{}, err
	}
	env, err := s.environment(ctx, in.EnvironmentID)
	if err != nil {
		return SignoffResult{}, err
	}
	envID, envName := envKey(env)
	reqs := OrderedRequests(tree, "")

	d := signoff.Data{Release: in.Release, PreparedBy: in.PreparedBy, Notes: in.Notes, Collection: tree.Collection.Name,
		Requests: reqs, EnvironmentName: envName, Races: map[string]*domain.Run{}, Baselines: map[string]*domain.Baseline{},
		AppName: AppName, AppVersion: Version, Secrets: s.allSecrets(ctx), Now: time.Now()}

	runs, err := s.store.ListRuns(ctx, domain.RunFilter{CollectionID: in.CollectionID, Limit: signoffRunScan})
	if err != nil {
		return SignoffResult{}, err
	}
	for _, summary := range runs { // newest first
		if !sameEnvironment(summary, envID, envName) {
			continue
		}
		if summary.Kind == domain.RunKindNormal {
			if d.Run != nil {
				continue
			}
			run, err := s.store.GetRun(ctx, summary.ID)
			if err != nil {
				return SignoffResult{}, err
			}
			d.Run = &run
			continue
		}
		if d.Matrix != nil && summary.Kind == domain.RunKindMatrix {
			continue
		}
		run, err := s.store.GetRun(ctx, summary.ID)
		if err != nil {
			return SignoffResult{}, err
		}
		if id, ok := signoff.IsRaceRun(run); ok {
			if _, seen := d.Races[id]; !seen {
				d.Races[id] = &run
			}
		} else if d.Matrix == nil && signoff.IsMatrixRun(run) {
			d.Matrix = &run
		}
	}
	for _, r := range reqs {
		b, err := s.store.GetBaseline(ctx, r.ID, envID)
		if err != nil {
			return SignoffResult{}, err
		}
		if b != nil {
			d.Baselines[r.ID] = b
		}
	}
	if d.Times, err = s.GetCollectionResponseTimes(ctx, in.CollectionID, envID); err != nil {
		return SignoffResult{}, err
	}

	report := signoff.Build(d)
	var buf bytes.Buffer
	if err := signoff.WriteHTML(&buf, report); err != nil {
		return SignoffResult{}, err
	}
	return SignoffResult{HTML: buf.String(), Ready: report.Ready, Reasons: nonNilStrings(report.Reasons),
		FileName: signoffFileName(tree.Collection.Name, in.Release, d.Now)}, nil
}

// sameEnvironment matches a run to the environment by ID, or by name for runs
// recorded before environment IDs were stored.
func sameEnvironment(r domain.Run, envID, envName string) bool {
	if r.EnvironmentID != "" || envID == "" {
		return r.EnvironmentID == envID
	}
	return r.EnvironmentName == envName
}

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func signoffFileName(collection, release string, now time.Time) string {
	part := func(s string) string { return strings.Trim(unsafeFileChars.ReplaceAllString(s, "-"), "-.") }
	name := "signoff"
	for _, p := range []string{part(collection), part(release)} {
		if p != "" {
			name += "-" + p
		}
	}
	return name + "-" + now.Format("20060102") + ".html"
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
