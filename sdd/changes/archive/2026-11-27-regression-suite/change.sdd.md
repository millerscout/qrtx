```change
name: regression-suite
status: complete
packages: []
```

## Why

qrtx's tracker, providers, sync and global hooks shipped in v0.5.0. The unit tests and local fakes all passed, yet real use found four bugs they missed:
- **Plugin not found:** a hook-started daemon cannot find `qrtx-tracker-github` under the desktop app's `PATH`.
- **Wrong worktree:** task events are credited to the root that scanned them, not to the worktree they came from.
- **Duplicate `claim_expired`:** claims stored under the old key format expire twice.
- **Unidentified sessions:** a command run without a session id records a `<path>:<pid>` session and gives no warning.

Several tests that would have caught these were never run:
- CI never runs on Windows;
- there is no race detector and no fuzzing;
- nothing drives the built binaries the way Claude Code does (hook JSON on stdin, a minimal `PATH`, `CLAUDE_ENV_FILE`);
- no test pins the JSON that agents and scripts read;
- nothing checks a real tracker.

The user asked for ways to test these features so they don't regress. This change builds the test layers. The fixes come in `sync-hardening`, which removes each known-bug guard as its fix lands. Alternatives go in Decisions.

## User request

After the v0.5.0 work, the user asked to "plan next steps for improvements", adding: "also we need ways to test those features to prevent regression". The approved plan puts the tests first (`regression-suite`), then the fixes (`sync-hardening`).

## Acceptance

- **CI matrix:** every push and pull request runs the full test suite on Ubuntu, macOS and Windows, a race-detector run on Ubuntu, a short fuzz run, and an `e2e` job that drives the built binaries with a minimal `PATH`.
- **e2e scenarios:** `go test -tags e2e ./e2e/` reproduces how Claude Code drives qrtx. It covers hook-driven session identity, claims across worktrees and subprojects, the inbox, the cross-worktree task watch, the lease lifecycle with daemon exit and restart, outbox draining, and plugin lookup with a minimal `PATH`.
- **Known bugs:** the four known bugs each have an e2e scenario. Each is skipped with a message naming `sync-hardening`. Setting `QRTX_E2E_KNOWN_BUGS=1` runs them, and each one fails today.
- **JSON contracts:** a change to the JSON of `sync status`, `sync inbox`, `tracker list`, `tracker check`, the `propose-context` `backlog` key or `events.jsonl` lines fails a golden test. `QRTX_UPDATE_GOLDEN=1` regenerates the goldens on purpose.
- **Nightly:** a scheduled workflow, also runnable by hand, runs `qrtx tracker check --allow-writes` against the real GitHub scratch repo and a 5-minute multi-session soak that checks sync invariants.

## Requirements

- Q1 CI SHALL run gofmt, vet and `go test ./...` on `ubuntu-latest`, `macos-latest` and `windows-latest`, plus `go test -race ./...` on Ubuntu. Every CI job SHALL use `bash` as its shell, with `core.autocrlf false` set before checkout. A test that cannot run on Windows SHALL skip, checked with `runtime.GOOS == "windows"`, with a reason naming the missing feature, never pass silently.
- Q2 The `e2e` package SHALL build `qrtx`, `qrtx-tracker-github` and a fake tracker plugin into a temporary bin directory, and SHALL run each scenario against temp git repositories with worktrees. Each repo is a qrtx project: `.sdd.yaml` and a `demo` change with task 1.1 are committed before `git worktree add`, and `svc/` has its own `.sdd.yaml`. Scenarios run with `HOME`, `QRTX_CREDENTIALS`, `CLAUDE_CONFIG_DIR` and the state directory isolated from the developer's machine.
- Q3 Each e2e scenario SHALL drive qrtx like Claude Code: hook commands receive hook JSON on stdin with `CLAUDE_ENV_FILE` set, and agent commands run in a shell that sourced that env file.
- Q4 Each e2e scenario that starts a daemon SHALL read its pid from `daemon.json`, stop it on cleanup, and SHALL fail if that pid is still alive (signal 0) afterwards.
- Q5 A scenario for a known, unfixed bug SHALL skip with `known bug, fixed by sync-hardening: <one line>` unless `QRTX_E2E_KNOWN_BUGS=1` is set, in which case it SHALL run and fail until the fix lands.
- Q6 Golden tests SHALL compare the JSON output of `sync status --json`, `sync inbox --json`, `tracker list --json`, `tracker check --json`, the `propose-context` `backlog` key and `events.jsonl` lines against files under `testdata/golden/`, after normalizing times, pids, generated ids and paths. Each temp path is replaced in its raw, symlink-resolved (`/private/var`) and JSON-escaped forms, and `<path>:<pid>` session ids are replaced too. `QRTX_UPDATE_GOLDEN=1` SHALL rewrite them.
- Q7 Fuzz targets SHALL exist for `MergeIssues`, `tracker.EncodeMarker`/`DecodeMarker`, the Jira `MarkdownToADF`/`ADFToMarkdown` pair and `setManifestIssues`. CI SHALL run each for 10s, and a crasher SHALL fail the job with its input saved as an artifact. A crasher seed SHALL be committed only together with its fix, so `go test ./...` on `main` stays green.
- Q8 A `nightly` workflow (schedule plus `workflow_dispatch`) SHALL run `qrtx tracker check --allow-writes` against `millerscout/sdd-tracker-scratch`, using the repo secret `TRACKER_SCRATCH_TOKEN` bound in a temp credentials file. It SHALL skip with a notice when the secret is absent, as on forks.
- Q9 The nightly soak SHALL run 5 simulated sessions for 5 minutes, doing random heartbeats, claims, releases and task flips across two worktrees with a short lease. It SHALL fail when any of these breaks: event seqs are strictly increasing with no duplicates; after a heartbeat, begin or claim (not a release), a live lease has a daemon within 1s; no claim outlives its lease by more than 1s; and no daemon is left running at the end.
- Q10 Nothing in this change SHALL alter qrtx's runtime behaviour. Only tests, CI workflows, test helpers and docs change.

## Scenarios

- S1 (Q1) WHEN the CI workflow runs on a pull request THEN a `test` job for each OS (Ubuntu, macOS, Windows), plus the `race`, `fuzz` and `e2e` jobs, all appear and pass on this branch.
- S2 (Q1) WHEN a test needs `sh` or Unix signals and runs on Windows THEN it reports `SKIP` with a reason containing "windows", and `go test` still exits 0.
- S3 (Q2, Q3) WHEN the e2e session-identity scenario runs a `SessionStart` hook (`qrtx sync begin --hook`, stdin `{"session_id":"s-1","cwd":…}`) and then, in a shell that sourced `CLAUDE_ENV_FILE`, `qrtx sync claim demo/1.1` THEN `qrtx sync status --json` lists the claim under session `s-1`.
- S4 (Q2, Q3) WHEN session A in the main worktree claims `demo/1.1` and session B in a second worktree claims it THEN B exits 1 naming A, while a claim of the same key in subproject `svc/` succeeds.
- S5 (Q2, Q3) WHEN the daemon has snapshotted B's change file (its path is in `state.json` `watch`), A has run `inbox` once, and task 1.1 is then checked in worktree B THEN A's next `inbox --hook --brief` prints `demo/1.1 done in <base name of B> (<B branch>)` within 2s.
- S6 (Q2, Q4) WHEN every lease ends (lease 2s, tick 100ms) THEN `sync status` shows no daemon and `sync.log` holds `exiting: no live sessions`. A later heartbeat starts a new daemon, `sync stop` returns in under 1s, and cleanup finds no live daemon pid.
- S7 (Q2) WHEN a create is queued while the fake plugin is down and the plugin recovers THEN the daemon drains the outbox with no `tracker` command, and the fake records the create.
- S8 (Q5) WHEN the e2e suite runs without `QRTX_E2E_KNOWN_BUGS` THEN four scenarios skip with `known bug, fixed by sync-hardening`: minimal-`PATH` plugin lookup, watch attribution, legacy claim migration and the unattributed-session warning. WHEN it runs with `QRTX_E2E_KNOWN_BUGS=1` THEN those four fail with the bug's symptom: `qrtx-tracker-fake not found on PATH`; a wrong `root` (only A has a live session, so only A's root scans); two `claim_expired` events (the test seeds an old-format key, then the same session claims the same key again in the new format); and no warning.
- S9 (Q6) WHEN a field is renamed in `SyncStatus` or `Issue` JSON THEN the golden test fails with a diff naming the field. WHEN `QRTX_UPDATE_GOLDEN=1` is set THEN the golden is rewritten, and the next run passes.
- S10 (Q7) WHEN `go test -run '^$' -fuzz FuzzMergeIssues -fuzztime 10s .` runs on this branch THEN it finds no crasher. Seed corpora live under `testdata/fuzz/`. The properties match the code: `MergeIssues` yields valid JSON lines only when there is no error and no conflict, and is idempotent on canonical output; the marker round-trips up to trimming, nil/empty slices and UTF-8 normalization; `MarkdownToADF` yields a valid doc and `ADFToMarkdown` never panics; `setManifestIssues` preserves parsing for parseable input and refs without backticks or newlines.
- S11 (Q8) WHEN `nightly` is dispatched by hand with `TRACKER_SCRATCH_TOKEN` set THEN every `tracker check` op passes against the scratch repo. WHEN the secret is absent THEN the job prints a notice and succeeds without contacting GitHub.
- S12 (Q9) WHEN the soak runs for 5 minutes THEN it reports the operation count and zero invariant violations, and leaves no daemon running. Each invariant has a unit test that feeds a hand-made event log breaking it and expects the checker to report it.
- S13 (Q10) WHEN `git diff main --stat` runs at the end of this change THEN only `*_test.go` files (including test helpers), `e2e/` (with its test-only `fakeplugin` command), `testdata/` and each package's `testdata/fuzz/`, `.github/workflows/`, `docs/`, `CHANGELOG.md` and this change's folder are touched.

## Decisions

- D1 e2e lives in its own package behind the build tag `e2e`, with its own CI job. Rejected: putting it in `go test ./...`, because e2e builds binaries and spawns daemons, which makes the default suite slower and timing-sensitive.
- D2 Known-bug scenarios skip with a message naming the fixing change, unless `QRTX_E2E_KNOWN_BUGS=1`. Rejected: committing failing tests, because CI on `main` must stay green; and writing the tests only alongside the fixes, because the user asked for regression protection first, and a test is only trusted once it has failed against the bug.
- D3 Goldens normalize volatile values (`<TIME>`, `<PID>`, `<ROOT>`, `<ID>`) and reuse the existing `QRTX_UPDATE_GOLDEN=1` convention (`propose_context_json_golden_test.go`). Rejected: snapshotting raw output, because times and paths change on every run.
- D4 The real-GitHub check runs only nightly and on demand, with a fine-grained token scoped to the scratch repo, held in a repo secret. Rejected: running on every PR, because it writes real issues and forks have no secret.
- D5 The soak lives in the e2e package, gated by `QRTX_SOAK=5m`, and only the nightly workflow sets it. Rejected: a separate tool, because the e2e helpers already build binaries and set up worktrees.
- D7 The `e2e` and `race` jobs use Go `stable` (not 1.22.x with cgo on macOS), so the binaries they build avoid the macOS 26 `LC_UUID` abort. Rejected: copying the `GOFLAGS` linker workaround, because those jobs don't need a Go 1.22 build.
- D6 The Windows job runs the full suite. A test that needs `sh`, Unix signals or `flock` semantics skips with a reason, and each such skip is listed in this change's Observed. Rejected: a build-only Windows job, because it would leave the Windows lock and detach code untested.

## Risks

- e2e timing flakes on slow CI runners → short leases with generous waits (`waitFor` up to 10s), daemon tests never depending on wall-clock order, and the e2e job retrying once on failure (`go test -count=1` run twice only when the first run fails, with both logs kept).
- The Windows suite reveals real Windows bugs → each becomes a task here (fix in a test helper) or a Deferred item for `sync-hardening` when the fix is in runtime code. The test skips with the deferral named, never silently.
- The scratch-repo token leaks → it is a fine-grained token with Issues read and write on one repo only, and the workflow never prints it.
- Fuzzing finds a crash in shipped code → the crasher is saved as a CI artifact and filed for `sync-hardening` (or a tracker change). The seed is committed only with its fix (Q7).
- `claim --release` extends the lease but does not start a daemon (`sync.go` `Claim`) → the soak invariant excludes releases (Q9), and whether a release should start a daemon is filed under Deferred for `sync-hardening`.

Deploy: n/a (tests, CI and docs only; nothing ships in the binaries).

Rollback: n/a (revert the workflow files; no runtime state).

## Observed

- `.github/workflows/ci.yml`:
  - **Matrix:** `os: [ubuntu-latest, macos-latest]` × `go: ["1.22.x", "stable"]` × `cgo: ["0","1"]`.
  - **Steps:** gofmt, `go mod tidy` diff, vet, `go test ./...`, and the example plan's verify.
  - **macOS 26 workaround:** Go 1.22 + cgo + macOS uses `-ldflags=-linkmode=external` because of `LC_UUID`.
- There are no `func Fuzz` tests and no `e2e` package.
- Golden pattern: `propose_context_json_golden_test.go:36` rewrites when `QRTX_UPDATE_GOLDEN=1`.
- Existing daemon tests (`cli/daemon_test.go`) build the binary through `buildBin`, filter `CI=` and `QRTX_NO_DAEMON=` out of the child env, and stop daemons on cleanup with `stopDaemonCleanup`. These are the helpers to reuse in `e2e/`.
- Real-use evidence (2026-11-27):
  - `sdd-product/.git/sdd/sync.log` repeats `poll …: tracker github capabilities: unavailable: qrtx-tracker-github not found on PATH` although `/Users/aurora/go/bin/qrtx-tracker-github` exists.
  - In `sdd/.git/sdd/events.jsonl`: seq 15–21 have `root` `qrtx-hooks` with `worktree` `qrtx-review`; seq 6 and 7 are duplicate `claim_expired` entries; seq 9 and 22 have session `…/qrtx-hooks:70881`.
- `tracker_exec.go:64` resolves plugins with `exec.LookPath` only. `sync_daemon.go:56-70` spawns the daemon with `os.Environ()`.
- `sync.go` `ResolveSession` falls back to `<worktree>:<ppid>` with no warning.
- `cli/sync.go` `hookSilent` and `verify.go` `findRoot`: hooks act only when the session's folder and root form a qrtx project. Worktrees only see committed files, and `findRoot` stops at the nearest `.sdd.yaml`.
- `sync_watch.go`: the first scan of a file is silent, and `Root` is whichever live root scanned. `pollRoot`'s first poll only records. `SyncEvent.Summary` prints the base name of the worktree.
- `tracker_merge.go`: output under conflict holds markers, and output is canonicalized and re-sorted. `tracker/marker.go` `EncodeMarker` strips an existing trailing marker first.
- `cli/daemon_test.go` `stopDaemonCleanup` checks only `status.Daemon == nil`, not the pid.
- Windows runners default to pwsh and check out with `autocrlf=true`, and Git Bash puts `sh` on `PATH`.
- `daemonDisabled` treats any `CI` value as "no daemon", so CI e2e runs must clear `CI` in the child environment, as the existing tests do.
- Windows skips added (`skipOnWindows`, task 1.2; still to be confirmed on a real Windows runner once the branch is pushed):
  - **symlinks:** `manifest_quality_grounding_test.go` (2 sites), `cli/tracker_driver_test.go`, `skills/skills_test.go`;
  - **test-binary plugins via symlinks:** `tracker_helper_test.go` `installHelper`, which covers every root test that uses the fake plugin;
  - **sh-script plugins:** `cli/tracker_test.go` (2), `cli/tracker_precedence_test.go`, `cli/daemon_test.go` `downPlugin`.
- CI on PR #3 (2026-11-27):
  - **First run:** Windows failed 17 tests, mostly because the built binaries had no `.exe` suffix, and `race` failed because race-built plugins sleep 1s at exit.
  - **Second run:** 3 Windows failures remained: the merge-driver path quoting, `TestWatch`'s fixed wait, and the killed-daemon lock limitation, which is filed for `sync-hardening`.
  - **Third run:** all 13 jobs green.
- Apply runs (2026-11-27):
  - **e2e:** 6 scenarios pass, twice in a row, and 4 known-bug scenarios skip. With `QRTX_E2E_KNOWN_BUGS=1` all four fail with their symptoms: `qrtx-tracker-fake not found on PATH` in `sync.log`; a `task_progress` root of `…/main` instead of the worktree; 2 `claim_expired` for one claim; no warning.
  - **Goldens:** stable over 10 runs. Renaming `SyncStatus`'s `outbox` tag fails only `sync_status.json`, with a diff naming the field.
  - **Fuzz:** each target ran 10s clean after correcting two properties, and found the real `setManifestIssues` multi-line-list bug (Deferred).
  - **Soak:** 30s, 100 operations, 0 violations.
- `qrtx change propose-context --change regression-suite --json`: score `simple`, no overlaps (2026-11-27).

## Assumptions

- GitHub-hosted `windows-latest` runners have `git` and Go. `sh` may be absent there, so tests that need it skip.
- The user will add the `TRACKER_SCRATCH_TOKEN` repo secret. Until then, the nightly GitHub step skips with a notice (S11).

## Non-goals

- Fixing the four known bugs (`sync-hardening`).
- A real Jira site check (`tracker-jira-live-check`).
- Coverage targets or badges.

## Deferred

- `nightly-secret-check`: run V1.7 once the user adds `TRACKER_SCRATCH_TOKEN`.
- `sync-hardening`: on Windows a killed daemon's `daemon.lock` (an `O_EXCL` file holding `pid nonce`) looks held until the 60s stale rule, so `sync status` reports a dead daemon as running and no new daemon starts for up to a minute. `tryLockFile` should treat the lock as stale when the owner pid is gone. Found by `TestDaemonAutostart` on `windows-latest` (2026-11-27); that step skips on Windows until fixed. Also, `setManifestIssues` replaces only the `issues:` line, so a hand-written multi-line list (`issues:` with `- ref` lines, or a flow list wrapped over lines) is left half in place and breaks the manifest's YAML on the next `tracker start`. `FuzzSetManifestIssues` found it on 2026-11-27; the property skips multi-line lists, and the seed is committed with the fix. Also, `sync status` reports `outbox: 0` while writes are pending, until sync has state (`ReadSyncStatus` returns before counting the outbox when `state.json` is missing; found by `TestOutboxDrain`, 2026-11-27); also decide whether `claim --release` should start the daemon (it extends the lease without one); also the plugin lookup fix, watch attribution, legacy claim migration and the unattributed-session warning. Each removes its known-bug guard.
- `tracker-jira-live-check`: a nightly Jira step, once a Jira sandbox exists.
- `manifest-fence-consistency`: `ParseChangeMarkdown` accepts "```change" with text on the same line (`extractFencedBlock` trims), but `setManifestIssues` treats that text as part of the fence line. `FuzzSetManifestIssues` found it on 2026-11-27 with the input "```changename: 0\n\v```"; the property is limited to well-formed fences until this is fixed.

## Tasks

### Phase CI matrix
{: id="phase-01" status="accepted" file="phases/01-ci-matrix.md"}

### Phase e2e harness and scenarios
{: id="phase-02" status="accepted" file="phases/02-e2e.md"}

### Phase JSON goldens and fuzzing
{: id="phase-03" status="accepted" file="phases/03-goldens-fuzz.md"}

### Phase nightly workflow and soak
{: id="phase-04" status="accepted" file="phases/04-nightly.md"}

## Validate

- [x] V1.1 `go test ./... -count=1` covers S2,S13
- [x] V1.2 `go test -tags e2e ./e2e/ -count=1` covers S3,S4,S5,S6,S7,S8
- [x] V1.3 `go test ./... -count=1 -run 'Golden'` covers S9
- [x] V1.4 `go test -run '^$' -fuzz FuzzMergeIssues -fuzztime 10s .` covers S10
- [x] V1.5 `go test -tags e2e ./e2e/ -count=1 -run TestSoakInvariants` covers S12
- [x] V1.6 manual: PR millerscout/qrtx#3 ran all 13 CI jobs green (`test` on Ubuntu, macOS and Windows, plus `race`, `fuzz` and `e2e`) before merging as `2c50967`, on 2026-11-27. The `nightly` workflow was dispatched by hand on `main` (run 46057035106): the 5-minute soak made 1510 operations, 751 events and 0 violations, and the GitHub step skipped with `TRACKER_SCRATCH_TOKEN not set`. covers S1,S11
- [ ] V1.7 manual: dispatch `nightly` with the `TRACKER_SCRATCH_TOKEN` secret set and confirm every `tracker check` op passes against the scratch repo. Not run: the secret is not added yet. covers S11

## Constitution

- C1 Rule 1 → planned here with `qrtx change new`, `propose-context` and `finish-propose` before any code, then applied and archived through the skills. The fixes are a separate slug, `sync-hardening`, in this repository.

## Review

- R1 Q1, S1, task 1.1: the gofmt step fails under pwsh on Windows, and `autocrlf` breaks gofmt and the goldens → Q1: `bash` shell and `core.autocrlf false` before checkout, with no `.gitattributes` needed.
- R2 S1 vs task 1.1: matrix job names include every matrix value → S1 now says "a test job for each OS".
- R3 S2, D6: Git Bash provides `sh` on Windows, so a `LookPath` check would run it instead of skipping → Q1: skips check `runtime.GOOS == "windows"`.
- R4 tasks 2.1, S3–S5 vs `hookSilent`/`findRoot`: hooks act only inside qrtx projects → Q2: `newRepo` commits `.sdd.yaml` and the `demo` change before adding the worktree, and gives `svc/` its own `.sdd.yaml`.
- R5 S5 vs the silent first scan and the first inbox only setting the cursor → S5 waits for the watch snapshot and for A's first inbox, and expects the base name of B.
- R6 S8 legacy migration: seeding only the old key gives one event today → the test re-claims the same key in the new format after seeding (S8).
- R7 S8 watch attribution: flaky if B's root scans first → only A has a live session in that test (S8).
- R8 task 2.3 plugin `PATH`: the first poll only records → the fake bumps `updated_at` after the first poll, and the test waits one interval (task 2.3).
- R9 Q6: the goldens miss `/private/var`, JSON-escaped and `<path>:<pid>` forms → Q6 normalizes all of them.
- R10 Q7 `MergeIssues`: conflict output is not JSON, and identity is false → JSON only when there is no error and no conflict, and the identity property becomes idempotence (S10).
- R11 Q7 marker: stripping, nil/empty slices and UTF-8 → the round-trip property is up to those normalizations (S10).
- R12 Q7 ADF: the property mixed up the types → `MarkdownToADF` yields a valid doc, and `ADFToMarkdown` never panics (S10).
- R13 Q7 `setManifestIssues`: arbitrary input never parsed, and backticks break the block → the property is limited to parseable input and refs without backticks or newlines (S10).
- R14 K4 vs D2: a committed crasher seed turns `main` red → Q7: seeds are committed only with their fix, and crashers go to artifacts.
- R15 Q9: `claim --release` creates a live lease with no daemon, and 200ms is tight → the invariant counts heartbeat, begin and claim only, with a 1s bound; the release behaviour is filed under Deferred for `sync-hardening`.
- R16 S13 vs helpers and `.gitattributes` → S13 allows test helpers and `e2e/fakeplugin`, and R1 avoids `.gitattributes`.
- R17 Q4 vs `stopDaemonCleanup` checking only status → Q4: `e2e` reads the pid from `daemon.json` and checks it with signal 0.
- R18 task 1.1: e2e on macOS with Go 1.22 + cgo hits `LC_UUID` → D7: the `e2e` and `race` jobs use Go `stable`.
- R19 Q2 vs `daemonDisabled` (`CI`) → not a conflict: `session()` removes `CI` and `QRTX_NO_DAEMON`, and spawned daemons inherit that environment. `agent()` overrides `QRTX_SESSION` for the unattributed test (task 2.3).
- R20 C1 vs Rule 1 → not a conflict: planned here, with fixes in the separate slug `sync-hardening`.
