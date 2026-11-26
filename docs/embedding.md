# Embedding sdd in a host

`github.com/millerscout/sdd` is a Go library as well as a CLI. A host program, such as an agent server or IDE backend, can run the same commands with its own name and plug in richer data sources.

## Run the CLI under your own name

```go
import (
	"os"

	"github.com/millerscout/sdd"
	"github.com/millerscout/sdd/cli"
)

func main() {
	sdd.CLIName = "mytool sdd"         // shown in templates, errors and usage
	sdd.DefaultProject = "myproject"   // passed to providers when the caller names none
	sdd.RepoEnv = append(sdd.RepoEnv, "MYTOOL_REPO") // extra repo-root variables
	if err := cli.Run(os.Args[2:], os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}
```

Set these once at startup, before serving requests.

## Providers

All providers are optional. Without them, `sdd` still runs; the corresponding sections of `propose-context` are just empty.

### `CodeIndex`

```go
type CodeIndex interface {
	Related(project, query string, limit int) []sdd.CodeHit  // graph neighbours of the request text
	Symbols(project, name string, limit int) []sdd.CodeHit   // definitions of an exact identifier
	Semantic(project, query string, limit int) []sdd.CodeHit // embedding search
}
sdd.SetCodeIndex(myIndex)
```

Return `Path` (repository-relative) and `Line`; `sdd` sets `Source`. The `sdd` binary installs `sdd.TextIndex{Root: repoRoot}`, which scans source files for definitions and returns nothing for `Related` and `Semantic`.

### `ContextProvider`

```go
type ContextProvider interface {
	OpenIssues(project string) (byRule map[string]int, total int, ok bool)
	KnownFixes(project string, limit int) []sdd.KnownFix
}
sdd.SetContextProvider(myContext)
```

The results appear in propose context as `issues` and `known_fixes`, giving the planning agent the project's current health and recent fixes that worked.

### Classifiers

When `sdd` turns a multi-paragraph request into a roadmap, it labels each paragraph as a phase, deferred work or a sequencing note. It labels each follow-up line as a file or an existing phase path. Lines with a clear cue (`later`, `goes first`, a file name) are labelled by rule. The rest go to:

```go
// First reader: label every line with one key of criteria, or "abstain".
sdd.LineClassifier = func(kind string, lines []string, criteria map[string]string) (map[string]string, error) { … }

// Second reader: one LLM completion. It labels lines the first reader
// missed, and settles lines where the two readers disagree.
sdd.SecondReader = func(prompt string) (string, error) { … }
```

With neither set, unlabelled paragraphs become phases.

### `IssueTracker` (unstable)

```go
type IssueTracker interface {
	Capabilities() (sdd.TrackerCaps, error)
	ListIssues(f sdd.IssueFilter) ([]sdd.Issue, error)
	GetIssue(id string) (sdd.Issue, error)
	CreateIssue(in sdd.CreateIssueInput) (sdd.Issue, error)   // idempotent per in.ClientRef
	UpdateIssue(id string, in sdd.UpdateIssueInput) (sdd.Issue, error)
	TransitionIssue(id, category, native string) (sdd.Issue, error)
	Comment(id, markdown, clientRef string) error
}
sdd.SetIssueTracker(myTracker)
```

The protocol types live in package `github.com/millerscout/sdd/tracker` (standard library only). The root package re-exports them as aliases (`sdd.Issue` is `tracker.Issue`). Provider plugins use `providers/kit` and `tracker` without the root package. A host tracker replaces the backend named in `.sdd.yaml` (`tracker.backend`). Without one, sdd uses the built-in `file` backend (`sdd/issues.jsonl`), or runs `sdd-tracker-<name>` from `PATH` as described in [tracker-protocol.md](tracker-protocol.md). The interface and the protocol may change before v1.

## Stability

Before v1, only the core lifecycle API and these host hooks are stable. The package doc lists them. Status payload helpers (`StatusMap`, `ChangesMap`, …), the tracker API (`IssueTracker`, `SetIssueTracker`, `OpenTracker`, …) and the propose-agent helpers (`ProposeBand`, `Regression*`, `WriteRoadmap`, …) may change in any v0 release.
