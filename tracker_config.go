package sdd

import (
	"fmt"
	"strings"
)

// trackerSettings is a validated tracker block with defaults applied.
type trackerSettings struct {
	Backend   string
	Config    map[string]any
	StatusMap map[string]string
	Comments  string
	Timeout   int
}

// validateTrackerConfig checks the tracker block when a command resolves a
// backend. LoadRepoConfig never calls it, so other commands keep working.
func validateTrackerConfig(c TrackerConfig) (trackerSettings, error) {
	s := trackerSettings{
		Backend:   strings.TrimSpace(c.Backend),
		Config:    c.Config,
		StatusMap: c.StatusMap,
		Comments:  strings.TrimSpace(c.Comments),
		Timeout:   c.Timeout,
	}
	if s.Backend == "" {
		s.Backend = fileBackendName
	}
	if err := checkBackendName(s.Backend); err != nil {
		return s, fmt.Errorf("%s tracker.backend: %w", RepoConfigFile, err)
	}
	switch m := strings.TrimSpace(c.Mirror); m {
	case "", "change":
	default:
		return s, fmt.Errorf("%s tracker.mirror %q: only change is implemented", RepoConfigFile, m)
	}
	switch s.Comments {
	case "":
		s.Comments = "summary"
	case "summary", "minimal", "off":
	default:
		return s, fmt.Errorf("%s tracker.comments %q: use summary, minimal or off", RepoConfigFile, s.Comments)
	}
	for k := range s.StatusMap {
		if !ValidCategory(k) {
			return s, fmt.Errorf("%s tracker.status_map key %q is not one of %v", RepoConfigFile, k, StatusCategories)
		}
	}
	if s.Timeout <= 0 {
		s.Timeout = 30
	}
	return s, nil
}

// loadTrackerSettings reads .sdd.yaml and validates its tracker block.
func loadTrackerSettings(repoRoot string) (trackerSettings, RepoConfig, error) {
	cfg, err := LoadRepoConfig(repoRoot)
	if err != nil {
		return trackerSettings{}, cfg, err
	}
	s, err := validateTrackerConfig(cfg.Tracker)
	return s, cfg, err
}

// fileBackendName is the built-in backend over sdd/issues.jsonl.
const fileBackendName = "file"
