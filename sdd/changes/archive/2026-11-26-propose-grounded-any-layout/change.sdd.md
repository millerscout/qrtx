```change
name: propose-grounded-any-layout
status: complete
packages: []
```

## Why

Aurora's propose search (Auto-improve) climbs `ScoreChangeManifest`. Since the engine left Aurora, that rubric still only credits an Observed path that starts with one of Aurora's top-level folders, so a plan written in any other repository, this one included, can never earn the grounding points however well it read the code. The rubric also cannot tell a plan whose tasks name files it read from one whose tasks name files it invented. The chosen approach is to ground on any real file under the repository root, report how many task paths the plan actually read, and pin both with golden rows so a later search cannot raise the score by loosening the rubric. Alternatives go in Decisions.

## User request

Split from Aurora change `one-switch-richer-propose` (Aurora repo, `sdd/changes/one-switch-richer-propose/`). The user asked to change the SDD engine, not only Aurora, while increasing the sdd-propose feature and proposal quality, and to compare with opsx-explore and opsx-propose for a plan that reads the repository before it writes tasks. Auto-improve is the one switch that drives this search.

## Acceptance

- A plan in a repository whose code sits at the root (like this one) gets the Observed grounding points from `sdd change score` when it cites a real source file, and gets none for a made-up file, a directory, a path outside the repository, its own plan files under `sdd/`, or a dependency manifest such as `go.mod`.
- `sdd change score` shows how many paths named in Tasks were also cited in Observed, so a plan that invents files reads worse than one that read them.
- The existing manifest-quality golden rows keep their score windows, and new rows fail if a later edit loosens the grounding rule.

## Requirements

- Q1 Observed grounding SHALL credit any cited file that exists under the repository root, whatever its top-level folder.
- Q2 Observed grounding SHALL NOT credit a missing file, a directory, a path that resolves outside the repository root (symlinks included), a path under `sdd/`, or a dependency manifest (D5).
- Q3 `sdd change score` and `sdd change finish-propose` SHALL both report the count of task paths cited in Observed and the count of task paths, without changing `score`.
- Q4 Existing manifest-quality golden rows SHALL keep their score windows, and existing JSON keys from `sdd change score` SHALL stay.

## Scenarios

- S1 (Q1) WHEN Observed cites `manifest_quality.go` at the root of a fixture repo that has that file THEN `sdd change score` reports `score` of at least 95 for an otherwise good plan.
- S2 (Q1) WHEN Observed cites `docs/read.md` (the `intent-grounded` fixture) THEN the row still scores at least 95.
- S3 (Q2) WHEN Observed cites only a directory (`docs/` in a tree that has it), only the plan's own `sdd/changes/<slug>/change.sdd.md`, or only `go.mod` THEN the golden row scores at most 94.
- S6 (Q2) WHEN a unit test writes `outside.go` next to the temp repo root and Observed cites `../outside.go`, its absolute path, or an in-repo symlink to it THEN `ObservedGrounded` is false; an in-repo file reached through a root that is itself a symlink (macOS `/var` → `/private/var`) is still grounded.
- S4 (Q3) WHEN Tasks name two files in checkbox prose (text before `verify:`) and Observed cites one of them THEN `sdd change score` and `finish-propose` JSON both have `tasks_paths_total` 2 and `tasks_paths_observed` 1, and `score` is the same as before the fields existed; WHEN the same steps sit in a roadmap phase file THEN the counts are the same, and `phases/NN-x.md` roadmap lines are not counted.
- S5 (Q4) WHEN `go test . -run 'TestSddChangeManifestQualityGoldenJSONL|TestScoreChangeManifest|TestFailureRestatementScore|TestProductEchoScore'` runs after the change THEN every existing row and test passes with its window unchanged.

## Decisions

- D1 Ground on any backticked or bare token with a `/` or a file extension that stats as a regular file inside the repository root, minus D5's exclusions. Rejected: a prefix list in `.sdd.yaml` because every repository would have to configure it before plans score fairly, and Aurora's prefixes would still be the silent default.
- D2 Reject absolute tokens; resolve each token with `filepath.Join`, then `filepath.EvalSymlinks` on both the target and the repository root, and require the resolved target to stay under the resolved root. Rejected: trusting the token because a plan could cite `../../go.mod` from a fixture and earn grounding for a file outside the repository.
- D3 Report task grounding as two new JSON keys and a `ManifestScore` field, not as points. Rejected: adding points because every golden window and Aurora's `TestProposeAutoresearchEval` baseline would move in the same release, and the search could not tell a rubric change from a better plan.
- D5 Never ground a path under `sdd/` (`changesDirName`, `change.go`) or a dependency manifest by base name: `go.mod`, `go.sum`, `package.json`, `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `Cargo.toml`, `Cargo.lock`, `pyproject.toml`, `poetry.lock`, `requirements.txt`, `Gemfile`, `Gemfile.lock` (user chose to exclude manifests 2026-11-26). Rejected: crediting any real file because every plan's own manifest and every Go repo's `go.mod` exist, so citing them earns +15 without reading code.
- D6 Task paths come from checkbox prose before `verify:` (`checkboxLines`, `taskProse`), and for roadmap plans from the phase files `parseRoadmap` (`roadmap.go`) lists; `phases/*.md` lines themselves are not task paths. Rejected: scanning the whole Tasks section because verify tails and phase-file links would count as invented files.
- D4 The rubric lock is golden rows in `testdata/fixtures/sdd_change_manifest_quality_golden.jsonl`, appended only. Rejected: a checksum over `manifest_quality.go` because it fails on harmless refactors and says nothing about which behaviour moved.

## Risks

- A wider grounding rule raises scores for plans in Aurora that cite a root file such as `go.mod` → the existing windows are minimums and maximums on fixtures, not on Aurora plans; Aurora pins the version it scores with in `go.mod`.
- A token like `v0.3.0` or `e.g.` looks like a file name → it only counts when it stats as a regular file, so a non-file never earns points.

Deploy: tag a minor release of `github.com/millerscout/sdd`; consumers pick it up with `go get`. Old consumers keep the old rubric until they bump.

Rollback: n/a (nothing persists; consumers pin the previous tag in `go.mod`).

## Observed

- `manifest_quality.go` `observedFileRE` is `(?:internal|scripts|docs|homepage)/[A-Za-z0-9_./-]+`, and `observedPathExists` only stats matches of that pattern, so a root file such as `manifest_quality.go` never grounds (read 2026-11-26).
- `manifest_quality.go` `ScoreChangeManifest` adds 15 points when `observedPathExists` is true, else 5 when Observed is non-empty and not TBD; `MaxScore` is 100. `ManifestScore.Depth` is reported and never added to `Score`, which is the pattern D3 follows.
- `cli/cli.go` `change score` encodes `slug`, `score`, `max_score`, `verify_ok`, `verify_err` and `spec_depth`; `finish.go` builds the same keys separately for `finish-propose`.
- The golden harness copies each fixture into `t.TempDir()` (`change_manifest_quality_golden_test.go`), so `../outside.go` in a fixture points at a temp sibling that does not exist and cannot prove the escape check; `os.Stat` follows symlinks. Every existing fixture's Observed cites `docs/read.md` or `internal/...` paths, so D1 and D5 move no existing window; `TestProductEchoScore` compares two manifests that both cite `docs/read.md`.
- `roadmap.go` `parseRoadmap` reads `### Phase` lines whose `file=` points at `phases/NN-x.md` under `sdd/changes/<slug>/`; the real steps live in those files.
- `propose_bench_test.go` `TestPlanBench` never calls `ScoreChangeManifest`, so it is not a check for this change.
- `change_manifest_quality_golden_test.go` `TestSddChangeManifestQualityGoldenJSONL` reads `testdata/fixtures/sdd_change_manifest_quality_golden.jsonl` (9 rows) against trees in `testdata/fixtures/sdd_manifest_quality/`. `intent-grounded` cites `docs/read.md` and expects at least 95; `intent-fake-path` cites a missing `internal/sdd/not-a-file.go` and expects at most 94.
- `propose_bench_test.go` `TestPlanBench` expects 20 rows, unchanged 20, apply_ok 17, apply_fail 1, apply_skipped 2.
- `change.go` scaffolds Acceptance as a single `(TBD …)` bullet with no count cap, and `skills/sdd-propose/SKILL.md` already says the old 1-to-3 cap is not a storage limit. The Aurora plan's "change the generator" step for that cap is already done here, so this change has no task for it.
- Aurora's `internal/ide/propose_autoresearch_test.go` `TestProposeAutoresearchEval` calls `sdd.ScoreChangeManifest`; Aurora pins `github.com/millerscout/sdd v0.3.1` (latest tag) in `go.mod`.
- This plan shows the gap: `sdd change score --change propose-grounded-any-layout` in this repository gives no grounding points although its Observed cites `manifest_quality.go`, which exists at the root (run 2026-11-26).
- `.gitignore` lists `/sdd` (the build output), which also ignores this repository's `sdd/` folder, so `sdd/changes/` here is not tracked by git (`git status --ignored` shows `!! sdd/`).

## Assumptions

- Aurora wires the new release in its own change `one-switch-richer-propose`; this change does not touch Aurora.
- Task paths are tokens in Tasks lines that would ground under D1's rule, before checking existence.

## Non-goals

- Changing the band or domain classifier, or the plan bench rows.
- Any Aurora UI, overnight script, or search program.
- A new explore command.

## Deferred

- Untracking the build output without hiding `sdd/` (`.gitignore` `/sdd`) belongs to its own slug.
- Aurora's switch, search program and version bump stay in Aurora's `one-switch-richer-propose`.

## Tasks

### Phase 1 — Ground on any repository file

- [x] 1.1 Append golden rows first, each with its tree under `testdata/fixtures/sdd_manifest_quality/` and `max_wall_ms` 2000: `root-layout-grounded` (`manifest_quality.go` at its root, Observed cites it, `expect_min_score` 95), `cites-directory` (Observed cites only `docs/`, which exists), `self-cite` (Observed cites only its own `sdd/changes/self-cite/change.sdd.md`) and `cites-go-mod` (Observed cites only a root `go.mod`), the last three `expect_max_score` 94. verify: `go test . -run TestSddChangeManifestQualityGoldenJSONL` fails on `root-layout-grounded` before 1.2. covers S1,S3
- [x] 1.1b Add `TestObservedGroundingEscapes` in `manifest_quality_test.go`: temp root, `outside.go` written beside it, Observed citing `../outside.go`, its absolute path, and an in-repo symlink to it (all not grounded), plus an in-repo file through a symlinked root (grounded). verify: fails before 1.2 on the symlink case. covers S6
- [x] 1.2 Replace `observedFileRE` and `observedPathExists` in `manifest_quality.go` with D1, D2 and D5's rule. verify: V1.1. covers S1,S2,S3,S6

### Phase 2 — Show whether tasks name files the plan read

- [x] 2.1 Add `TasksPathsTotal` and `TasksPathsObserved` to `ManifestScore`, filled in `ScoreChangeManifest` per D6 without changing `Score`, with `TestScoreChangeManifestTaskPaths` in `manifest_quality_test.go` for the two-files-one-observed case inline and in a roadmap phase file. verify: `go test . -run TestScoreChangeManifestTaskPaths`. covers S4
- [x] 2.2 Build the score JSON map in one helper used by both `cli/cli.go` `change score` and `finish.go`, adding `tasks_paths_total` and `tasks_paths_observed` and keeping every existing key, with a new `TestChangeScoreKeys` in `cli/cli_test.go` that checks old and new keys from both commands. verify: `go test ./cli/ -run TestChangeScoreKeys`. covers S4

### Phase 3 — Release

- [x] 3.1 Add a CHANGELOG entry naming the grounding rule and the two keys, and tag the next minor release. verify: V1.2. covers S5

## Validate

- [x] V1.1 `go test . -count=1 -run 'TestSddChangeManifestQualityGoldenJSONL|TestScoreChangeManifest|TestObservedGroundingEscapes|TestFailureRestatementScore|TestProductEchoScore'` covers S1,S2,S3,S5,S6
- [x] V1.2 `go test ./... -count=1` covers S4,S5

## Constitution

- C1 Rule 1 → this change was planned here (`finish-propose` before code) and applied with `/sdd-apply`; Aurora's `one-switch-richer-propose` names it as a dependency and keeps only the `go get`. Task 3.1's CHANGELOG entry and tag were planned tasks, not exemptions. The local tag `v0.4.0` (commit `78419ed`) was made before archive, so it does not include the archived plan; move it to the archive commit before pushing, or tag the archive commit as the next patch.

## Review

- R1 D4 [uncited-writer] `sdd/changes` (`doc.go`, `living.go`, `propose_context.go`, `roadmap.go`, `verify.go`) → not a conflict because none reads or writes the golden jsonl; three are comments, `propose_context.go` only reads, and `roadmap.go` writes phase files that `ScoreChangeManifest` only reads. `roadmap.go` does matter for task paths (R6).
- R2 Acceptance 3, Q4, S2: existing fixtures and `TestProductEchoScore` → not a conflict because every fixture cites `docs/read.md` or `internal/...` paths and the echo test compares two manifests that both ground on `docs/read.md`.
- R3 D1, Acceptance 1 and 3: any-real-file grounding lets a plan cite its own `change.sdd.md`, a phase file, or `go.mod` for +15 without reading code → D5 excludes `sdd/` and dependency manifests (user chose 2026-11-26); golden rows `self-cite` and `cites-go-mod` lock it.
- R4 D2, Q2: the lexical root check misses symlinks because `os.Stat` follows them, and resolving only the target would reject real files on macOS → D2 runs `EvalSymlinks` on target and root; S6 and task 1.1b test it in a unit test.
- R5 S3, task 1.1 `outside-root`: fixtures are copied into `t.TempDir()`, so that row would score ≤94 before and after 1.2 and prove nothing; no row covered the directory case → escape cases moved to unit test 1.1b (S6); `cites-directory` row added.
- R6 Q3, S4: roadmap plans keep real steps in phase files, and verify tails would count as task paths → D6: checkbox prose before `verify:`, phase files via `parseRoadmap`, `phases/*.md` lines skipped; S4 has a roadmap case.
- R7 S5, V1.2: `TestPlanBench` never calls `ScoreChangeManifest`, so it proves nothing for Q4 → S5 and V1.1 name the score tests; `TestPlanBench` dropped.
- R8 Q3, task 2.2: `finish.go` builds its own score JSON, so keys would drift → 2.2 shares one map builder and tests both commands.
- R9 Observed (`observedFileRE`, 15/5 points, 9 golden rows, cli score keys) → not a conflict: checked against `manifest_quality.go`, the golden jsonl and `cli/cli.go`.
