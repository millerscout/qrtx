# Changelog

## Unreleased

### Added

- **Project backlog and pluggable issue trackers** (change `tracker-standard`; unstable until v1).
  - `sdd tracker add|list|get|close|start|from-deferred|outbox|check|init|merge-driver`.
  - **Built-in `file` backend:** `sdd/issues.jsonl`, with hash IDs, sorted canonical lines and a per-issue merge driver.
  - **Plugin backends:** any `sdd-tracker-<name>` on `PATH` that speaks `sdd.adapter/v1`. The contract is in `docs/tracker-protocol.md` and `docs/schema/tracker/v1/`.
  - **Manifest field `issues:`:** links tracker issues to a change. `change archive` comments on each linked issue and completes it, unless another active change still links it. The writes go through a crash-safe outbox, so a tracker failure never blocks the archive.
  - **`.sdd.yaml` keys:** `project:` and `tracker:` (`backend`, `config`, `status_map`, `mirror`, `comments`, `timeout`).
  - **`propose-context` JSON:** gains `backlog`, up to 20 open issues.

## v0.4.2 — 2026-11-26

### Added

- Plan bench row `daylist-break` (Aurora's day-list deep-regression request): `goldfish`, domain `scripts`, apply_ok. `TestPlanBench` now expects 21 rows, unchanged 21, apply_ok 18, apply_fail 1, apply_skipped 2.

## v0.4.1 — 2026-11-26

### Fixed

- `go test github.com/millerscout/sdd` failed from a consumer's module cache: the v0.4.0 golden fixture `cites-go-mod` held a `go.mod`, so Go treated it as a separate module and left it out of the module download. That row is now `cites-package-json`, and the `go.mod` case is tested with a file written at run time.

### Added

- `TestTestdataSurvivesModuleZip` fails on anything under `testdata/` that the module zip leaves out: a `go.mod`, a `vendor/` or VCS folder, or a symlink.

## v0.4.0 — 2026-11-26

### Changed

- Observed grounding in `ScoreChangeManifest` (`sdd change score`, `finish-propose`) now credits any regular file inside the repository root, not only paths under `internal/`, `scripts/`, `docs/` or `homepage/`. Root-level files such as `manifest_quality.go` now ground. It never credits a directory, a missing file, an absolute path, a path that leaves the root (symlinks resolved on both sides), a plan file under `sdd/`, or a dependency manifest (`go.mod`, `go.sum`, `package.json`, lockfiles and similar). Scores of plans that cite only Aurora-style paths are unchanged.

### Added

- `sdd change score` and `finish-propose` JSON gain `tasks_paths_total` and `tasks_paths_observed`: file paths named in task prose (before `verify:`, roadmap phase files included) and how many of them Observed also cites. They are reported and never added to `score`. Both commands now build their JSON from one helper, `sdd.ScoreJSON`.
- Golden rows `root-layout-grounded`, `cites-directory`, `self-cite` and `cites-go-mod` lock the grounding rule.

## v0.3.1 — 2026-11-26

### Fixed

- The contract graph walk (`sdd graph`, `.sdd-graph.json`) now skips every dot-directory (`.git`, `.claude/worktrees`, `.theia`, ...). Before, contracts in a hidden folder, such as a stale worktree copy, were listed as packages.

## v0.3.0 — 2026-11-25

### Changed (breaking for JSON readers)

- `propose-context --json`: the known-fixes list is now under `known_fixes` (was `fixledger`).
- Code hits from the code graph now have `"source": "graph"` (was `"kuzu"`). Embedding hits keep `"source": "embedding"`.
- The human `propose-context` output heads that section `known fixes:` (was `fixledger:`).

### Added

- A golden test pins every key and source value of `propose-context --json` (`testdata/fixtures/sdd_propose_context_json.golden.json`).

## v0.2.0 — 2026-11-25

### Added

- **Project constitutions.** A constitution is the project's standing rules: `sdd/constitution.md`, or the files listed under `constitution:` in `.sdd.yaml`. Each `##` or `###` heading is a rule. See [docs/format.md](docs/format.md#constitution).
- `sdd constitution show [--json]` lists the rules; `sdd constitution init [--force]` writes a starter `sdd/constitution.md`.
- In a repository with a constitution, `sdd change new` adds a `## Constitution` section. `finish-propose` requires it, with `- C<n> <rule> → <how the plan complies>` lines that name real rules, or a single `- None: <reason>`.
- `propose-context` and the review packet list the rules; `C<n>` lines are review claims.
- The `sdd-propose` skill has a new step 5c (Constitution); `sdd-apply` and `sdd-archive` follow the rules.
- Library: `LoadConstitution`, `ConstitutionFiles`, `InitConstitution`, `ConstitutionDoc`, `RepoConfig.Constitution`.

### Compatibility

- Repositories without a constitution behave exactly as in v0.1.0.
- `verify` and `archive` do not check the Constitution section, so changes written before a constitution existed still archive.
- New JSON fields (`constitution` in `propose-context --json` and `review-packet --json`) are omitted when empty.

## v0.1.0 — 2026-11-24

First release.

- `sdd` CLI: `change new|list|status|propose-context|verify|score|review-packet|finish-propose|finish-apply|run-validate|archive|lint-active`, `graph`, `verify-ast`, `sync-from-code`, `skills list|install`.
- Change manifests with requirements, scenarios, decisions, risks, deploy and rollback, validate, and a second-reader review packet.
- Package contracts (`contract.sdd.md`) with Go export drift checks and living behaviour appended on archive.
- `.sdd.yaml`: `validate_allow`, `quick_check`, `verify_packages`.
- Bundled agent skills: `sdd-propose`, `sdd-apply`, `sdd-archive`.
- Go library with pluggable `CodeIndex`, `ContextProvider`, `LineClassifier` and `SecondReader`; `TextIndex` as the built-in code index.
- Pure-Go build with `CGO_ENABLED=0`; with cgo, tree-sitter indexes TypeScript, JavaScript, Python, Rust and Swift too.
