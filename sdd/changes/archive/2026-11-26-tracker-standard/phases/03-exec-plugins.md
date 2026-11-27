# Phase exec plugin backends

- [x] 3.1 Add `tracker_exec.go`, `execTracker`, implementing `IssueTracker`:
  - It resolves `sdd-tracker-<name>` with `exec.LookPath` after `ValidBackendName`.
  - It calls `capabilities` first, picks the highest shared protocol, and caches the result for the process.
  - It writes one envelope to stdin and reads one response from stdout, with a 30s timeout (`tracker.config.timeout` overrides it) and stderr passed through.
  - It never calls an op that `capabilities` did not declare.

  verify: `go test . -run TestTrackerExec`, using a helper plugin built from the test binary (`TestMain` re-exec as `sdd-tracker-fake` in a temp `PATH`), checks list, the v2-only refusal, the timeout, non-JSON output, the `auth` error with backend/op/code in the message, an invalid name (`../evil`, `a/b`) and an undeclared `comment`. covers S6,S7,S8,S9
{: id="phase-03-task-1"}
- [x] 3.2 Add the backend selection: `SetIssueTracker` when a host set one, otherwise `.sdd.yaml` `tracker.backend`, otherwise `file`. `tracker check` gains the read-only exec path. verify: `go test ./cli/ -run TestTrackerCheckExec` runs only `capabilities` and `list_issues` without `--allow-writes`. covers S20
{: id="phase-03-task-2"}
