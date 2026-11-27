# Changelog

## Unreleased

### Removed

- **Support for the old `sdd` name** (change `no-legacy-sdd`). Before upgrading:
  - **Credentials:** `<config dir>/sdd/credentials.yaml` is no longer read. Move it to `<config dir>/qrtx/credentials.yaml`.
  - **Hooks:** `qrtx skills install --hooks` (project or `--global`) no longer replaces hooks that call `sdd`, and no longer counts them as sync hooks. Remove them by hand.
  - **Validate commands:** `run-validate` no longer allowlists `sdd …` or `./sdd …`. Change such lines to `qrtx …`, or list `sdd ` under `validate_allow`.

## v0.5.0 — 2026-11-27

### Changed

- **Renamed to qrtx.** The tool, the Go module (`github.com/millerscout/qrtx`) and the binary (`go install github.com/millerscout/qrtx/cmd/qrtx@latest`) are now qrtx; the root package is `qrtx`. The method keeps its name, spec-driven development, and so do its files: `.sdd.yaml`, `sdd/`, `*.sdd.md` and the `sdd-*` skills. Breaking:
  - **Tracker plugins** are found as `qrtx-tracker-<name>` on `PATH` (was `sdd-tracker-<name>`).
  - **Environment variables** are `QRTX_*` (was `SDD_*`), including `QRTX_REPO`, `QRTX_CREDENTIALS`, `QRTX_SESSION`, `QRTX_NO_DAEMON`, and `QRTX_CRED_HOST`/`QRTX_CRED_SCOPE` for token commands.
  - **Credentials** live in `<config dir>/qrtx/credentials.yaml`; an existing `<config dir>/sdd/credentials.yaml` is still read when the new file is missing.
  - **Installed skills** still tell agents to run `sdd …`; replace them with `qrtx skills install --force` (a plain install reports them as local edits).
  - **Hooks:** re-run `qrtx skills install --hooks` to replace hooks that call `sdd`, and `qrtx init` (or `qrtx tracker init`) to point the issues merge driver at the new binary.
  - **Unchanged on purpose:** the plugin protocol id (`sdd.adapter/v1`), issue ids (`sdd-…`), the tracker `sdd` refs field and `<!-- sdd:v1 … -->` marker, the Jira `sdd` property, the `sdd-issues` merge driver name and `.git/sdd/` state, so existing data and remote issues keep working.

### Fixed

- **Tracker review fixes** (change `tracker-review-fixes`).
  - **Merge driver:** `qrtx init` and `qrtx tracker init` register the issues merge driver as `qrtx tracker merge-driver …` when `PATH` resolves `qrtx` to the running binary, so upgrades behind that name keep merges working. From any other binary they record its path and print a `warning:`, and they never replace a working `PATH` driver with a temporary path.
  - **`tracker get`:** no longer repeats the context line without a change once the issue is started, and drops empty fields. `--json` is unchanged.
  - **`tracker start`:** warns on stderr when the issue is already linked from an active change in this or a sibling worktree. It still links the issue and exits 0.

### Added

- **Precedence between issues** (change `tracker-precedence`; unstable until v1). Ship `qrtx`, `qrtx-tracker-github` and `qrtx-tracker-jira` together; older plugins keep working and leave links and priority alone.
  - **Links and priority:** `qrtx tracker link <a> blocks|relates|duplicates <b>`, `unlink` and `priority <ref> <0-4>`, and `tracker add --priority N`, on the file backend, GitHub, Jira and plugins that declare `update_issue` (new `add_links`, `remove_links` and `priority` fields). They write directly and report a backend that did not store the change. GitHub keeps them in a second `<!-- sdd-prec:v1 … -->` body line and Jira in the `sdd-precedence` issue property, apart from the `sdd` refs.
  - **`tracker start`:** refuses an issue that an open issue blocks, naming the blockers and creating nothing; `--force` starts it and says so. It fails closed when the open-issue listing is incomplete, and warns and skips the check when the backend cannot list issues.
  - **`tracker list`:** orders by priority (1 first, none last, backend order among equals) and ends a blocked issue's line with `blocked by <refs>`, whatever the filter. `--json` keeps the issue schema.
  - **`change propose-context`:** names which of two overlapping changes goes first when their linked issues are related by `blocks` (`goes_first`), or warns when the tracker cannot be reached.
- **`qrtx skills install --hooks --global`** (change `global-session-hooks`): installs the sync hooks once, in `~/.claude/settings.json` (or `$CLAUDE_CONFIG_DIR/settings.json`), so every Claude Code session on the machine takes part in sync. It installs hooks only, replaces hooks that call the old `sdd` binary, reports `unchanged` on a re-run, and prints a `note:` when the current project's own settings still run sync hooks; remove those so each hook runs once. It refuses a temporary `go run` build when `qrtx` is not on `PATH`.
- **Sync hooks act only in qrtx projects:** `qrtx sync heartbeat|begin|end|inbox --hook` exit 0 silently and create no state unless the session's folder is in a project whose root holds `sdd/changes/`, `sdd/constitution.md` or `.sdd.yaml` (a `.git` or `go.mod` alone no longer counts, and `$QRTX_REPO` does not bypass it). A repository that used hooks without any of those files goes quiet; `qrtx init` or `qrtx change new` turns it back on. Commands without `--hook` are unchanged.
- **`qrtx sync`: coordination between agent sessions in one repository** (change `tracker-sync`; unstable until v1).
  - **Leases:** one-hour leases that heartbeats extend.
  - **Claims:** advisory claims on issues and `<slug>/<task>`.
  - **Inbox:** each session sees the others' claims, task checkboxes flipped in any worktree, and remote tracker changes.
  - **Daemon:** a small daemon started on demand expires leases, flushes the outbox, watches worktrees and polls trackers, and exits when every session is idle.
  - **Claude Code hooks:** `qrtx skills install --hooks` sets them up (`SessionStart`, `UserPromptSubmit`, async `PostToolUse`, `SessionEnd`).
  - **Skills:** the bundled skills claim and release work.
  - **Docs:** see `docs/sync.md`.
- **Faster `qrtx tracker add`:** a new add skips the plugin's duplicate lookup (`fresh` in `create_issue`), which saved about 3s per add on GitHub.
- **`qrtx tracker add --client-ref`:** after a timed-out add, qrtx prints the exact command that retries it without making a duplicate.
- **Fresh issues in `qrtx tracker list`:** an issue created in the last minute is listed even while the provider's list lags behind.
- **Clearer plugin timeouts:** plugins get a deadline a little before qrtx stops them, so a stalled request is reported by the plugin with what it was doing.
- **`qrtx skills install --hooks`:** adds `.claude/settings.local.json` to `.gitignore` unless git already ignores it, and says the hooks are local to the clone (`--shared` commits them).
- **Outbox draining:** only one process drains at a time, and the outbox lock is taken per entry, never across a tracker call.
- **Windows locks:** lock files record their owner, so a process never removes a lock it no longer owns.

- **First-party tracker plugins `sdd-tracker-github` and `sdd-tracker-jira`** (change `tracker-providers`; unstable until v1). Install them with `go install github.com/millerscout/sdd/cmd/sdd-tracker-{github,jira}@latest`.
  - **GitHub:** "in progress" is a label; `client_ref` and the sdd links live in a hidden body marker; pull requests are skipped; listing is paged.
  - **Jira Cloud:** REST v3 with ADF bodies; issues move through workflow transitions; cancelling sets a "Won't Do"-style resolution; idempotency uses a `sdd-cref-` label.
- **`qrtx init`** sets up a repository or folder: skills, constitution, merge driver and, with `--tracker`, the tracker. **`qrtx tracker setup github|jira`** writes the `.sdd.yaml` tracker block (keeping comments), binds credentials and runs a read-only check. Both are safe to re-run.
- **User-level `credentials.yaml`:** binds each tracker API host and an allow-list of repos or projects to a `token_env` or `token_command`. It never holds a literal token. `token_command` gets `QRTX_CRED_HOST`/`QRTX_CRED_SCOPE` for short-lived scoped tokens. Redirects to another host are refused.
- **Package `tracker`:** the protocol types, `Conformance` and the marker helpers, using only the standard library. Root `qrtx` keeps aliases.
- **Package `providers/kit` and `kittest`:** for writing providers.
- **Plugin environment:** qrtx passes `QRTX_TRACKER_DEADLINE` to plugins.
- **`from-deferred` duplicates:** `qrtx tracker from-deferred` skips bullets whose issue already exists.

- **Project backlog and pluggable issue trackers** (change `tracker-standard`; unstable until v1).
  - `qrtx tracker add|list|get|close|start|from-deferred|outbox|check|init|merge-driver`.
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
