package qrtx

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/millerscout/qrtx/tracker"
)

// Tracker is the issue tracker a repository uses: the backend .sdd.yaml
// names (or the built-in file backend, or a host tracker), with its settings.
// Unstable until v1.
type Tracker struct {
	Root    string
	Backend string
	IssueTracker
	settings trackerSettings
	project  string
	others   map[string]IssueTracker
}

// OpenTracker resolves the repository's tracker. It validates the .sdd.yaml
// tracker block and the backend name but runs no plugin.
func OpenTracker(repoRoot string) (*Tracker, error) {
	s, cfg, err := loadTrackerSettings(repoRoot)
	if err != nil {
		return nil, err
	}
	t := &Tracker{Root: repoRoot, Backend: s.Backend, settings: s, project: projectName(repoRoot, cfg), others: map[string]IssueTracker{}}
	impl, err := t.build(s.Backend, true)
	if err != nil {
		return nil, err
	}
	t.IssueTracker = impl
	return t, nil
}

func (t *Tracker) build(name string, configured bool) (IssueTracker, error) {
	if configured && hostTracker != nil {
		return hostTracker, nil
	}
	if name == fileBackendName {
		return newFileTracker(t.Root), nil
	}
	var config map[string]any
	if configured {
		config = t.settings.Config
	}
	return newExecTracker(name, config, time.Duration(t.settings.Timeout)*time.Second)
}

// For returns the tracker for backend, which may differ from the configured
// one (an issue ref such as file:sdd-abc in a repo that uses github).
func (t *Tracker) For(backend string) (IssueTracker, error) {
	if backend == "" || backend == t.Backend {
		return t.IssueTracker, nil
	}
	if err := checkBackendName(backend); err != nil {
		return nil, err
	}
	if impl, ok := t.others[backend]; ok {
		return impl, nil
	}
	impl, err := t.build(backend, false)
	if err != nil {
		return nil, err
	}
	t.others[backend] = impl
	return impl, nil
}

// Native returns the status_map entry for category, or "".
func (t *Tracker) Native(category string) string { return t.settings.StatusMap[category] }

// Comments returns the archive comment mode: summary, minimal or off.
func (t *Tracker) Comments() string { return t.settings.Comments }

// Project returns the project name used in refs.
func (t *Tracker) Project() string { return t.project }

// Warnings reports malformed lines in the file backend's store.
func (t *Tracker) Warnings() []string {
	if f, ok := t.IssueTracker.(*fileTracker); ok {
		return f.Warnings()
	}
	return nil
}

// Ref formats an issue ref for backend and id.
func (t *Tracker) Ref(backend, id string) string {
	if backend == "" {
		backend = t.Backend
	}
	return backend + ":" + id
}

// ParseIssueRef splits "<backend>:<id>"; a bare id uses defaultBackend.
func ParseIssueRef(ref, defaultBackend string) (backend, id string) {
	ref = strings.TrimSpace(ref)
	if i := strings.Index(ref, ":"); i > 0 && ValidBackendName(ref[:i]) && i < len(ref)-1 {
		return ref[:i], ref[i+1:]
	}
	return defaultBackend, ref
}

func projectName(repoRoot string, cfg RepoConfig) string {
	if p := strings.TrimSpace(cfg.Project); p != "" {
		return p
	}
	if DefaultProject != "" {
		return DefaultProject
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return filepath.Base(repoRoot)
	}
	return filepath.Base(abs)
}

// CheckTracker runs the conformance checks against backend ("" = configured).
// The file backend is checked on a temporary copy. Other backends are only
// read unless allowWrites is set.
func CheckTracker(repoRoot, backend string, allowWrites bool) ([]CheckResult, error) {
	t, err := OpenTracker(repoRoot)
	if err != nil {
		return nil, err
	}
	if backend == "" {
		backend = t.Backend
	}
	var impl IssueTracker
	if backend == fileBackendName && (hostTracker == nil || backend != t.Backend) {
		tmp, err := os.MkdirTemp("", "qrtx-tracker-check-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		if raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(IssuesFile))); err == nil {
			dst := filepath.Join(tmp, filepath.FromSlash(IssuesFile))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(dst, raw, 0o644); err != nil {
				return nil, err
			}
		}
		impl = newFileTracker(tmp)
		allowWrites = true
	} else if impl, err = t.For(backend); err != nil {
		return nil, err
	}
	// A plugin backend gets the retried create 10s later, as an outbox retry
	// on a later command would; hosted lists lag behind a new issue.
	delay := time.Duration(0)
	if _, ok := impl.(*execTracker); ok {
		delay = checkRetryDelay
		if d, err := time.ParseDuration(os.Getenv("QRTX_CHECK_RETRY_DELAY")); err == nil {
			delay = d
		}
	}
	return tracker.ConformanceWith(impl, allowWrites, delay), nil
}

// checkRetryDelay is how long `tracker check` waits before retrying a create
// against a plugin backend. Tests shorten it.
var checkRetryDelay = 10 * time.Second

const maxBacklog = 20

// proposeBacklog lists open issues for propose-context. A repo with no
// tracker block and no issues file has no backlog and gets no warning.
func proposeBacklog(repoRoot string) ([]Issue, string) {
	cfg, err := LoadRepoConfig(repoRoot)
	if err != nil {
		return nil, ""
	}
	if strings.TrimSpace(cfg.Tracker.Backend) == "" && hostTracker == nil {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(IssuesFile))); err != nil {
			return nil, ""
		}
	}
	t, err := OpenTracker(repoRoot)
	if err != nil {
		return nil, "tracker: " + err.Error()
	}
	if e, ok := t.IssueTracker.(*execTracker); ok && e.timeout > 10*time.Second {
		e.timeout = 10 * time.Second
	}
	list, err := t.ListIssues(IssueFilter{Categories: []string{CategoryBacklog, CategoryUnstarted, CategoryStarted}, Limit: maxBacklog})
	if err != nil {
		return nil, "tracker backlog unavailable: " + err.Error()
	}
	if len(list) > maxBacklog {
		list = list[:maxBacklog]
	}
	return list, ""
}
