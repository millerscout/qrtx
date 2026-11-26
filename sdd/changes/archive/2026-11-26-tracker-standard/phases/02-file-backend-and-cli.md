# Phase file backend and tracker CLI

- [x] 2.1 Add the lock helper, with lock files in the state directory: `lock_unix.go` uses `syscall.Flock`, and `lock_windows.go` uses an `O_EXCL` lock file with 10s retry and a 60s stale takeover. verify: `go test . -run TestTrackerLock` shows two goroutines serialize their writes. covers S1
{: id="phase-02-task-1"}
- [x] 2.2 Add `tracker_file.go`, the `file` backend over `sdd/issues.jsonl`:
  - IDs are `sdd-` plus 6 base32 characters of sha256(`client_ref`), lengthened to 8 on a clash.
  - An existing `client_ref` returns the existing issue.
  - The file is kept sorted, with keys in sorted order, and only the changed line is rewritten, via a temp file and rename under the lock.
  - A malformed line is skipped on read with its `path:line`, and refused on write.

  verify: `go test . -run TestTrackerFile` covers three adds and a close changing one line, a create retried with the same `client_ref`, and malformed line 4. covers S1,S2,S16
{: id="phase-02-task-2"}
- [x] 2.3 Add `tracker_merge.go` (`MergeIssues`, including the 8-character re-ID) and `sdd tracker merge-driver %O %A %B`, using D11's rule. verify: `go test ./cli/ -run TestTrackerMerge` builds `cmd/sdd` into a temp dir, registers it as the driver and runs real `git merge` in a temp repo with the driver registered (two branches each add a "Same title" issue → two IDs; a fixture with the same 6-character ID and different `client_ref`s → one issue re-IDed to 8 characters; title vs labels → both kept; title vs title → exit 1 with markers), and once with the driver unregistered, using hand-made fixture lines separated by a base line (no conflict). covers S2,S3,S4
{: id="phase-02-task-3"}
- [x] 2.4 Add `runTracker` to `cli/cli.go`, wired into `Run` and its usage line:
  - Commands: `list [--status <category>] [--change <slug>] [--json]`, `get <ref> [--json]`, `add "<title>" [--description <text>]`, `close <ref> [--canceled]`, `init` (writes `.gitattributes` and registers the merge driver using `os.Executable()`), `merge-driver`.
  - A `<ref>` is `<backend>:<id>`, or a bare ID for the configured backend.

  verify: `go test ./cli/ -run TestTrackerCLI` runs add → list --json → get → close in a temp repo, plus `init` showing `git config merge.sdd-issues.driver`. covers S1
{: id="phase-02-task-4"}
- [x] 2.5 Add `sdd tracker check [--backend <name>] [--allow-writes]`, per D9. With `--allow-writes`, it creates twice with the same `client_ref` and fails the backend if two issues come back. verify: `go test ./cli/ -run TestTrackerCheck` shows the file backend passing every op with `sdd/issues.jsonl` unchanged. covers S20
{: id="phase-02-task-5"}
