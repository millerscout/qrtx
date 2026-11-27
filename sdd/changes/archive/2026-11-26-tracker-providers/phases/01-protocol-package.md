# Phase protocol package

- [x] 1.1 Create package `tracker/` (standard library only) holding everything below, moved out of the root package:
  - from `tracker.go`: `Issue` with its JSON methods, `IssueStatus`, `SDDRef`, `IssueLink`, `IssueFilter`, `CreateIssueInput`, `UpdateIssueInput`, `TrackerCaps`, `IssueTracker`, the categories and the op names;
  - from `adapter.go`: the envelope types, error codes and `ValidBackendName`;
  - exported `CanonicalJSON`, `Issue.Validate`, `Issue.AddRefs`, `AdapterError.Retryable`, `NewClientRef` and `NewNonce` (moved from `tracker_file.go`).

  In root `sdd`, `tracker.go` and `adapter.go` become aliases (`type Issue = tracker.Issue`, `const X = tracker.X`, `var NewClientRef = tracker.NewClientRef`). Internal callers and the two root tests (`tracker_test.go`, `tracker_helper_test.go`) switch to `Validate`/`AddRefs`. verify: `go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./tracker` prints only `github.com/millerscout/sdd/tracker`, and `go test ./... -count=1` passes. covers S1
{: id="phase-01-task-1"}
- [x] 1.2 Move the conformance sequence from `CheckTracker` (`tracker_open.go`) into `tracker.Conformance(impl IssueTracker, allowWrites bool) []CheckResult`. `CheckTracker` keeps its signature and calls it, and `CheckResult` becomes an alias. verify: `go test ./cli/ -count=1 -run 'TestTrackerCheck'` passes unchanged. covers S1
{: id="phase-01-task-2"}
- [x] 1.3 Add `tracker.EncodeMarker(description string, m Marker) string` and `DecodeMarker(body string) (description string, m Marker, ok bool)` for `<!-- sdd:v1 {"client_ref":…,"sdd":[…]} -->` as the last line. Decoding strips the marker, and encoding replaces an existing one. verify: `go test ./tracker/ -run TestMarker` round-trips with and without a marker, a marker in the middle of the text, and a malformed marker (kept as text, `ok` false). covers S1
{: id="phase-01-task-3"}
- [x] 1.4 Make `execTracker.run` (`tracker_exec.go`) set `SDD_TRACKER_DEADLINE` (RFC 3339, now plus the call timeout) in the plugin's environment. Make `QueueDeferredIssues` (`tracker_lifecycle.go`) list the change's issues (`IssueFilter{Change: slug}`) and skip bullets whose `client_ref` is already present; a list failure keeps today's behaviour. verify: `go test . -run 'TestTrackerExec|TestTrackerDeferred'`, where the fake records the deadline variable, and a re-run against a fake that does not deduplicate creates no second issue. covers S11,S19
{: id="phase-01-task-4"}
