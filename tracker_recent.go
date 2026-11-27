package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// recentCreateTTL covers the lag between a create and the issue appearing in
// a provider's list (GitHub: 3-4s, measured 2026-11-26).
const recentCreateTTL = 60 * time.Second

type recentCreate struct {
	Backend string    `json:"backend"`
	At      time.Time `json:"at"`
	Issue   Issue     `json:"issue"`
}

func recentCreatesPath(repoRoot string) string {
	dir, _ := trackerStateDir(repoRoot)
	return filepath.Join(dir, "recent-creates-"+rootKey(repoRoot)+".json")
}

func readRecentCreates(path string, now time.Time) []recentCreate {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var all, live []recentCreate
	if json.Unmarshal(b, &all) != nil {
		return nil
	}
	for _, r := range all {
		if now.Sub(r.At) < recentCreateTTL {
			live = append(live, r)
		}
	}
	return live
}

// RecordCreated remembers an issue sdd just created, so the next list shows
// it even before the provider's list does. It is best effort.
func RecordCreated(repoRoot, backend string, is Issue) {
	dir, err := ensureTrackerStateDir(repoRoot)
	if err != nil {
		return
	}
	path := recentCreatesPath(repoRoot)
	now := time.Now()
	list := append(readRecentCreates(path, now), recentCreate{Backend: backend, At: now, Issue: is})
	b, err := json.Marshal(list)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "recent-*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
	}
}

// WithRecentCreates adds issues created in the last minute that list lacks.
// Each is re-read with get, since it may have changed since it was created.
// It only applies to plain status filters.
func WithRecentCreates(repoRoot, backend string, f IssueFilter, list []Issue, get func(id string) (Issue, error)) []Issue {
	if f.Change != "" || len(f.Labels) > 0 || f.Cursor != "" {
		return list
	}
	have := map[string]bool{}
	for _, is := range list {
		have[is.ID] = true
	}
	var add []Issue
	for _, r := range readRecentCreates(recentCreatesPath(repoRoot), time.Now()) {
		if r.Backend != backend || have[r.Issue.ID] {
			continue
		}
		is, err := get(r.Issue.ID)
		if err != nil {
			continue
		}
		if len(f.Categories) > 0 && !containsString(f.Categories, is.Status.Category) {
			continue
		}
		have[is.ID] = true
		add = append([]Issue{is}, add...)
	}
	return append(add, list...)
}

// IsUnavailable reports a transport-level failure, after which a write may
// or may not have reached the provider.
func IsUnavailable(err error) bool { return adapterCode(err) == ErrCodeUnavailable }

