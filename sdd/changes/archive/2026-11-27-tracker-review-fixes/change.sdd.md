```change
name: tracker-review-fixes
status: complete
packages: []
```

## Why

A hands-on review of the tracker (`tracker-standard`, `tracker-providers`, `tracker-sync`) in a scratch repo found three rough edges, and the release ships them to every user.
1. The `issues.jsonl` merge driver records whatever binary ran `init`. When that file is replaced, upgraded or removed, merges conflict.
2. `tracker get` repeats the context line.
3. `tracker start` silently links an issue that another active change already works on.

The fixes are small and local to `cli/tracker.go`, `cli/setup.go` and `tracker_lifecycle.go`.

## User request

After reviewing the new backlog feature, the user approved a plan that ships it after the qrtx rename. As its first step, they asked for the three review findings to be fixed as their own qrtx change:
- the merge driver path that dies with a temp or `go run` build;
- the duplicate context line in `tracker get`;
- no warning when one issue is started on a second change.

## Acceptance

- After `qrtx tracker init` (or `qrtx init`) run with `qrtx` on `PATH`, merging two branches that both added issues still works after the installed binary is upgraded or replaced behind the same `PATH` name, and its old file is gone.
- Running `init` from a binary that `PATH` does not resolve to still registers a driver when none works yet, and prints a warning that names the path and says how to make it permanent.
- Running `init` from a temporary binary in a repo whose driver already uses the `PATH` name keeps that driver.
- `qrtx tracker get <id>` on an issue that was added and then started no longer prints the change-less context line for that repo and project.
- `qrtx tracker start <id> <slug>` on an issue that an active change in this repo or a sibling worktree already links prints a warning naming that change, still links the issue, and exits 0.

## Requirements

- Q1 The issues merge driver SHALL keep working when the binary behind the `qrtx` name on `PATH` is upgraded or replaced after it registered the driver.
- Q2 Registering the driver from a binary that `PATH` does not resolve to SHALL still work when no `PATH`-form driver exists, and SHALL say that the recorded path may be temporary.
- Q3 Re-running `init` SHALL upgrade an older absolute-path driver to the `PATH` form when it can, SHALL never replace an existing `PATH`-form driver with an absolute path, and SHALL otherwise report `unchanged`.
- Q4 `tracker get` SHALL drop a context line that names no change when another ref for the same repo and project names one, without changing `--json` output.
- Q5 `tracker start` SHALL warn when the issue is already linked from another active change in any worktree of the repository, and SHALL still link it.

## Scenarios

- S1 (Q1) WHEN `PATH` holds a symlink `qrtx` → `v1/qrtx`, `qrtx tracker init` runs through it, the symlink is repointed to `v2/qrtx` and `v1/` is removed THEN `git config merge.sdd-issues.driver` is `qrtx tracker merge-driver %O %A %B` (no directory), and a two-branch merge of `sdd/issues.jsonl` succeeds.
- S2 (Q2) WHEN `init` runs from a binary that `PATH` does not resolve to, in a repo with no `PATH`-form driver THEN the driver records the absolute path as today, and the output contains `warning:` and that path.
- S3 (Q3) WHEN a repo's driver holds an old absolute path and `qrtx tracker init` runs again from a `qrtx` on `PATH` THEN the driver becomes the `PATH` form and the output says `registered`. A further re-run reports `unchanged`.
- S4 (Q4) WHEN an issue is added and then started on `dark-mode` THEN `tracker get` prints one `sdd:` line for that repo and project, which names `dark-mode`, and `tracker get --json` still lists both refs.
- S5 (Q4) WHEN an issue is only added THEN `tracker get` still prints its single context line.
- S6 (Q5) WHEN an issue started on `dark-mode` is started again on `dark-mode-2` while `dark-mode` is active THEN stderr has `warning:` naming `dark-mode`, the exit code is 0, and both manifests list the issue under `issues:`.
- S7 (Q5) WHEN the earlier change is archived THEN starting the issue on a new change prints no warning.
- S8 (Q3) WHEN the driver is already `qrtx tracker merge-driver %O %A %B` and `init` runs from a binary that `PATH` does not resolve to THEN the driver is kept, the step reports `unchanged`, and the output carries a `warning:` that this binary is not the one on `PATH`.
- S9 (Q5) WHEN `dark-mode` is active only in a sibling git worktree of the same repository THEN starting its issue on `dark-mode-2` in this worktree warns, naming `dark-mode`.

## Decisions

- D1 The driver's `PATH` form is the full `qrtx.CLIName` followed by ` tracker merge-driver %O %A %B`, chosen when `exec.LookPath` of the first field of `CLIName` and `os.Executable()` resolve through `filepath.EvalSymlinks` to the same file (embedded hosts set `CLIName = "mytool qrtx"`, `docs/embedding.md`, so the full name keeps the word their CLI needs); otherwise the absolute path form plus a `warning:`, except that an existing `PATH`-form driver is kept (S8); `registerMergeDriver` gains a `temporary bool` result and `trackerInit` and `qrtx init` print the warning on stdout, the only writer they have. Rejected: always writing the bare name because an uninstalled `go run` or test binary would then register a driver git can't find at all.
- D2 Fix the duplicate line in the display only: `tracker get` skips a ref with an empty `Change` when another ref for the same `Repo` and `Project` names a change, and leaves empty fields out of the `sdd:` line; `--json` returns before the display loop and keeps every ref. Rejected: removing the change-less ref on `start` because refs live in the backend (the GitHub issue body, Jira fields) and an extra update round-trip per start is a bigger change than the bug.
- D3 Warn without refusing on a double start, found from active manifests in every worktree of the repository (`worktreesOf(root)` in `sync_watch.go`, then `ListChanges` and `ParseChangeMarkdown(...).Issues` normalized with `normalizeRef`, excluding the slug being started here), returned as `StartResult.AlsoActive` (slug and worktree) and printed by the CLI on stderr. Rejected: refusing the start because the tracker design allows several changes per issue (archive keeps an issue open while another active change links it).
- D4 Read other changes from local manifests, not from the issue's stored refs. Rejected: reading `is.Refs` because those include other repos' changes and stale ones from changes that were never archived.

## Risks

- A `PATH`-form driver depends on `qrtx` being on `PATH` when git merges → this is the same dependency as installing the tool. D1 only chooses it when `PATH` already resolves to the running binary, and a later temporary binary never replaces it (S8).
- `worktreesOf` runs `git worktree list` on every `start` → this is one short git call, and `start` already shells out to git for the branch name.
- A warning on stderr could be missed by scripts → the exit code stays 0 by design (D3), and the text names the other change and its worktree.

Deploy: ship the new binary. Existing repos keep their old absolute driver until `qrtx tracker init` or `qrtx init` runs again (S3); nothing else persists.

Rollback: run the previous binary's `qrtx tracker init` in an affected repo, which rewrites `merge.sdd-issues.driver` to that binary's absolute path. The display and warning changes leave no state.

## Observed

- `cli/tracker.go` `registerMergeDriver` (l.253–289) writes `os.Executable()` `%q`-quoted into `merge.sdd-issues.driver`, compares that exact string for `unchanged` (l.276), and returns `created` on a rewrite.
  - `trackerInit` (l.227–249) has only `stdout`, and maps `created` to `registered merge driver`. `cli/setup.go:278` (`qrtx init`) prints the action as is.
- Reproduced with a qrtx build at `d48d725`: `init` from a copy `bin/qrtx-tmp`, two branches each `tracker add`, `rm bin/qrtx-tmp`, then `git merge a`. Output: `.../qrtx-tmp: No such file or directory` and `CONFLICT (content): Merge conflict in sdd/issues.jsonl`. The same merge with the binary present succeeds.
- `cli/tracker.go` `case "get"` (l.71–94) returns `--json` before printing, then prints `  sdd: %s %s %s %s` for every `is.Refs` entry. After `tracker add` then `tracker start ID dark-mode`, it printed `sdd: repo repo` (no change, trailing spaces) and then `sdd: repo repo dark-mode`.
- `tracker_lifecycle.go` `StartFromIssue` appends the ref to the manifest and queues `AddRefs` plus a `started` transition with no check of other changes. `tracker start ID dark-mode-2` after `tracker start ID dark-mode` printed `created …/dark-mode-2/` and `linked …` with exit 0 and no warning. Callers: `cli/tracker.go:149` and `tracker_lifecycle_test.go`.
- `change.go` `ListChanges` returns active slugs and skips `archive/`.
- `sync_watch.go:30` `worktreesOf(root)` lists the repository's worktrees from `git worktree list --porcelain`.
- `docs/embedding.md:16`: hosts set `qrtx.CLIName = "mytool qrtx"` and run `cli.Run(os.Args[2:], …)`.
- `cli/tracker_test.go`: `buildBin`/`qrtxBinary` (binary in a temp dir), `gitEnv` (appends `os.Environ()`, so `t.Setenv("PATH", …)` reaches git's driver), `newGitRepo`, `qrtxRun` (CombinedOutput), and `TestTrackerMerge` to extend. `cli/cli_test.go` `run` returns stdout and stderr separately.

## Assumptions

- Git runs merge drivers through the shell with the user's `PATH`, so a `PATH`-form driver resolves the same way it did at `init` time.

## Non-goals

- Keeping an off-`PATH` binary's driver working after that binary is removed. The `go run` or temp-build case is warned about (S2), not fixed.
- Changing how refs are stored or deduplicated in backends (D2 is display-only).
- Blocking or ordering between issues. That is `tracker-precedence`.

## Deferred

- tracker-precedence: `tracker link`, a blocked `start` that refuses, and priority/blocked marks in `list`.

## Tasks

- [x] 1.1 Implement D1 (`PATH`-form detection through the first field of `CLIName`, never downgrading a `PATH`-form driver, a `temporary` result, and `warning:` lines from `tracker init` and `qrtx init`). Extend `TestTrackerMerge` with four cases:
  - a `PATH` symlink repointed from `v1` to `v2` with `v1` removed, after which the merge succeeds;
  - an off-`PATH` binary, which gives the absolute driver plus a warning;
  - an old absolute driver upgraded on re-run, then `unchanged`;
  - an existing `PATH` driver kept when `init` runs off `PATH`.
  `TestInit`'s re-run check now accepts `warning:` lines (they change nothing).
  verify: `go test ./cli -run TestTrackerMerge -count=1`. covers S1,S2,S3,S8
- [x] 1.2 Implement D2 in `tracker get` (skip the change-less ref when a sibling names a change; leave out empty fields). Add `TestTrackerGetContext`: add then start, one `sdd:` line naming the slug, `--json` with both refs; plus the add-only case. verify: `go test ./cli -run TestTrackerGetContext -count=1`. covers S4,S5
- [x] 1.3 Implement D3 (`StartResult.AlsoActive` across `worktreesOf`, CLI warning on stderr). Add `TestStartFromIssueAlsoActive` (same worktree, sibling worktree, archived), and `TestTrackerStartWarns`, which asserts on `run`'s stderr, exit 0, and both manifests. verify: `go test . ./cli -run 'TestStartFromIssueAlsoActive|TestTrackerStartWarns' -count=1`. covers S6,S7,S9

## Validate

- [x] V1.1 `go test . ./cli -run 'TestTrackerMerge|TestTrackerGetContext|TestStartFromIssueAlsoActive|TestTrackerStartWarns' -count=1` covers S1,S2,S3,S4,S5,S6,S7,S8,S9
- [x] V1.2 `go test ./... -count=1` covers S1,S4,S6

## Constitution

- C1 Rule 1 → this change is planned here with `qrtx change new` and `finish-propose` before any code. It is then applied and archived with the same tool (tasks 1.1–1.3, V1.1–V1.2).

## Review

- R1 A1,Q1,S1 The off-`PATH` repro can't be fixed by a `PATH` check, and the old S1 never removed anything → Q1, A1 and S1 now cover an upgrade behind a `PATH` symlink with the old file removed. The off-`PATH` case is warned (S2) and named in Non-goals.
- R2 Q3,D1 vs `cli/tracker.go:276` Any string mismatch rewrote the driver, so a temp binary would replace a working `PATH` driver → D1 keeps an existing `PATH`-form driver, and S8 was added.
- R3 S3 vs `cli/setup.go:278` Only `tracker init` maps `created` to `registered` → S3 now names `qrtx tracker init`. `qrtx init` keeps printing `created`.
- R4 D1 vs `docs/embedding.md:16` `CLIName` can be `mytool qrtx` → D1 looks up the first field and writes the full `CLIName`.
- R5 S1 `cli/tracker_test.go:47-52` → not a conflict because `gitEnv` appends `os.Environ()`, so `t.Setenv("PATH", …)` reaches git's driver, and `EvalSymlinks` handles macOS `/var` → `/private/var`.
- R6 D1, task 1.1 vs `cli/tracker.go:238` `trackerInit` has no stderr → D1 names stdout as the stream and adds the `temporary` result.
- R7 Q4 vs D2 After S6, two `sdd:` lines are correct → Q4 reworded to drop only the change-less line.
- R8 D2,S4 `cli/tracker.go:85-87` → not a conflict because `--json` returns before the display loop.
- R9 S4 `cli/tracker.go:92` An empty role left trailing spaces → D2 leaves out empty fields, and S4 says "names `dark-mode`".
- R10 D3,S6,S7 `change.go:90-113` → not a conflict because the new slug is excluded, `archive/` is skipped, and `run` returns stderr for the S6 assertion.
- R11 Q5,D3 vs `sync_watch.go:91-93` `ListChanges` only sees this worktree → D3 scans `worktreesOf(root)`, and S9 was added.
- R12 callers → not a conflict because `registerMergeDriver` (`cli/tracker.go:239`, `cli/setup.go:278`) and `StartFromIssue` (`cli/tracker.go:149`, tests) are all covered.
- R13 C1 → not a conflict because C1 covers Rule 1.
- R14 `cli/cli.go:411` → not a conflict because the skills dir is unrelated data.
- R15 `cli/cli.go:142` → not a conflict because it is only the `change new` output text.
- R16 `cmd/qrtx/main.go:1` → not a conflict because it is a doc comment.
- R17 `doc.go:2` → not a conflict because it is a doc comment.
- R18 `living.go:13` → not a conflict because archive moves changes under `archive/`, which agrees with S7.
- R19 `propose_context.go:96` → not a conflict because that not-found path is unaffected.
- R20 `roadmap.go:180` → not a conflict because phase files don't touch `issues:`.
- R21 `sync_watch.go:93` → see R11.
- R22 `verify.go:113` → not a conflict because `init` and D3 use the same `RepoRoot`.
