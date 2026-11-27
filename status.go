package qrtx

import (
	"sync"
	"time"
)

const defaultStatusCacheSec = 30

// Status is the SDD freshness aggregate for APIs.
type Status struct {
	Freshness string   `json:"freshness"`
	Drifted   []string `json:"drifted"`
}

type cacheEntry struct {
	at     time.Time
	status Status
}

var (
	statusMu    sync.Mutex
	statusCache = map[string]cacheEntry{}
)

// StatusForRepo verifies DefaultVerifyPackages and returns aggregate freshness.
// A repo with nothing to verify is "unknown".
func StatusForRepo(repoRoot string, refresh bool) (Status, error) {
	key := repoRoot
	if !refresh {
		statusMu.Lock()
		entry, ok := statusCache[key]
		statusMu.Unlock()
		if ok && time.Since(entry.at).Seconds() < float64(defaultStatusCacheSec) {
			return entry.status, nil
		}
	}
	packages, err := DefaultVerifyPackages(repoRoot)
	if err != nil {
		return Status{Freshness: "unknown", Drifted: []string{}}, err
	}
	if len(packages) == 0 {
		return Status{Freshness: "unknown", Drifted: []string{}}, nil
	}
	issues, err := VerifyPackages(repoRoot, packages)
	if err != nil {
		return Status{Freshness: "unknown", Drifted: []string{}}, err
	}
	st := Status{Freshness: "verified", Drifted: []string{}}
	if len(issues) > 0 {
		st.Freshness = "drift_detected"
		seen := map[string]bool{}
		for _, iss := range issues {
			if !seen[iss.Package] {
				seen[iss.Package] = true
				st.Drifted = append(st.Drifted, iss.Package)
			}
		}
	}
	statusMu.Lock()
	statusCache[key] = cacheEntry{at: time.Now(), status: st}
	statusMu.Unlock()
	return st, nil
}

// StatusMap returns a JSON-friendly map for context payloads.
func StatusMap(repoRoot string, refresh bool) map[string]any {
	st, err := StatusForRepo(repoRoot, refresh)
	if err != nil {
		return map[string]any{
			"freshness": "unknown",
			"drifted":   []string{},
			"error":     err.Error(),
		}
	}
	drifted := st.Drifted
	if drifted == nil {
		drifted = []string{}
	}
	return map[string]any{
		"freshness": st.Freshness,
		"drifted":   drifted,
	}
}

// ClearStatusCache is for tests.
func ClearStatusCache() {
	statusMu.Lock()
	statusCache = map[string]cacheEntry{}
	statusMu.Unlock()
}
