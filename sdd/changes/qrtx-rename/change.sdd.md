```change
name: qrtx-rename
status: planning
packages: []
```

## Why

The product plan (`sdd-product/docs/product-plan.md`) calls for a real product name, because "sdd" is generic and probably can't be trademarked. The user picked **qrtx** and already renamed the GitHub repository to `millerscout/qrtx`. "sdd" still says exactly what the method is (spec-driven development), so the user wants the files that belong to the method to keep it. The tool is what gets the new name.

## User request

"We should rename the project sdd to qrtx." The user then narrowed it: "we still may use the files 'sdd' because they make sense as spec driven development". They also confirmed the split: tracker plugins and environment variables become qrtx, and the `sdd-*` skills keep their names. They chose to do the rename on a branch off `tracker-fixes`.

## Acceptance

- `go install github.com/millerscout/qrtx/cmd/qrtx@latest` installs a `qrtx` binary. Every command, usage line, doc and bundled skill says `qrtx …`.
- A repository already set up with sdd keeps working with no edits to its files: `.sdd.yaml`, `sdd/changes/`, `sdd/issues.jsonl`, `*.sdd.md`, issues already in GitHub and Jira, and the `sdd-*` skills.
- Tracker plugins are found as `qrtx-tracker-<name>`, and settings are read from `QRTX_*` environment variables.
- Re-running the setup commands moves an existing clone over: `qrtx skills install --hooks` and `qrtx init` leave the hooks and the merge driver calling `qrtx`, with no duplicate entries, and `qrtx skills install --force` replaces old skill copies.

## Requirements

- Q1 The tool SHALL be named qrtx everywhere it is the tool: Go module `github.com/millerscout/qrtx`, root package `qrtx`, `cmd/qrtx`, `CLIName`, usage and error text, docs, README and the bundled skills' command lines.
- Q2 Tracker plugin lookup SHALL resolve `qrtx-tracker-<name>` on PATH, and the bundled plugins SHALL be built as `cmd/qrtx-tracker-github` and `cmd/qrtx-tracker-jira`.
- Q3 Every environment variable the tool reads or sets SHALL use the `QRTX_` prefix.
- Q4 The method's files and data formats SHALL keep their sdd names: `.sdd.yaml`, `sdd/`, `*.sdd.md`, the ```` ```sdd ```` block, the `sdd-*` skills, `sdd.adapter/v1`, `sdd-…` issue ids, the `sdd` refs field, the `<!-- sdd:v1 -->` marker, the Jira `sdd` property, the `sdd-issues` merge driver and `.git/sdd/`.
- Q5 `qrtx skills install --hooks` SHALL replace Claude Code hooks that call the old `sdd` binary with the `qrtx` form, and SHALL leave every other hook alone.
- Q6 Credentials SHALL live in `<config dir>/qrtx/credentials.yaml`, and the old `<config dir>/sdd/credentials.yaml` SHALL still be read when the new file is missing.
- Q7 `run-validate` SHALL still allowlist validate commands that start with `sdd ` or `./sdd `, as written in manifests from before the rename.

## Scenarios

- S1 (Q1) WHEN `qrtx` runs with no arguments THEN stderr starts with `Usage: qrtx`, and `git grep` finds no `millerscout/sdd`, `cmd/sdd` or `sdd <subcommand>` outside `sdd/changes/` and the released CHANGELOG sections.
- S2 (Q2) WHEN `.sdd.yaml` sets `tracker.backend: github` (or `jira`) and the built `qrtx-tracker-github` (or `-jira`) is on PATH THEN `qrtx tracker check --backend github --allow-writes` passes against the fake API.
- S3 (Q3) WHEN `QRTX_REPO` names a directory THEN qrtx commands use it as the repository root, and `git grep 'SDD_[A-Z]'` finds only links to Aurora's `docs/SDD_VS_OPSX_BASELINE.md`.
- S4 (Q4) WHEN the existing goldens and fixtures run against the renamed code THEN regenerating them with `QRTX_UPDATE_GOLDEN=1` leaves `testdata/` byte-for-byte unchanged, and `qrtx change verify` passes on `examples/tic-tac-toe`.
- S5 (Q5) WHEN `.claude/settings.local.json` holds `sdd sync begin --hook` and `"/usr/local/bin/sdd" sync heartbeat --hook --quiet` next to an unrelated hook THEN `InstallHooks(path, "qrtx")` removes both old entries, adds the `qrtx sync …` set, keeps the unrelated hook and any `sdd` command with other arguments, and a second run reports unchanged.
- S6 (Q6) WHEN only `<config>/sdd/credentials.yaml` exists THEN `CredentialsPath` returns it; WHEN `<config>/qrtx/credentials.yaml` also exists THEN that file wins; WHEN neither exists THEN the qrtx path is returned.
- S7 (Q7) WHEN a validate line reads `sdd change verify --change x` or `SDD_REPO=. sdd change verify --change x` THEN `allowedValidateCommand` allows it, as it does `qrtx …` and `./qrtx …`.

## Decisions

- D1 Rename the tool and keep the method's files as sdd. Rejected: renaming everything (`.qrtx.yaml`, `qrtx/changes/`) because the user said the sdd files name the method, and moving them would force every repository and fallback-read code to change.
- D2 Keep wire and remote-data names (`sdd.adapter/v1`, the `<!-- sdd:v1 -->` marker, the `sdd` refs field, the Jira `sdd` property, `sdd-cref-`, `sdd-…` ids). Rejected: renaming them because issues already in GitHub and Jira carry them, and changing them would orphan those links or require migrating remote data.
- D3 Keep `.git/sdd/` and the lock and daemon file names. Rejected: `.git/qrtx/` because an old `sdd` binary still called by stale hooks and a new `qrtx` binary would then keep separate leases and daemons for one repository and hand out conflicting claims.
- D4 Rename the env vars and plugin prefix with no `SDD_*` or `sdd-tracker-*` fallback. Rejected: reading both because the user confirmed the split and the user's own repositories are the only installs. The CHANGELOG lists each break.
- D5 Make `InstallHooks` drop old `sdd` hook commands whose arguments match a qrtx hook. Rejected: leaving them because re-running setup would then leave both hooks in place, one of them calling a binary that may be gone.
- D6 Read the old credentials file when the new one is missing. Rejected: requiring a manual move because that silently drops tracker access, which goes against Rule 2 of the starter constitution ("No silent data loss").
- D7 Accept the `sdd ` prefix in `run-validate`. Rejected: rejecting it because manifests are sdd files the user kept, and in-flight plans (in Aurora too) would otherwise fail validation.
- D8 Back-fill this plan after the code was written. Rejected: reverting and redoing because the user asked for the plan before commit, and the code is unchanged by planning. Rule 1's "before any code changes" is recorded as not met (C1).

## Risks

- Aurora imports `github.com/millerscout/sdd v0.4.2` and sets `CLIName` → Aurora keeps building against v0.4.2 until a qrtx tag exists. Its move is a separate change in Aurora.
- Stale `sdd …` skill copies in other repositories → the CHANGELOG says to run `qrtx skills install --force`, since a plain install reports them as local edits.
- Token commands that read `$SDD_CRED_HOST` break → the CHANGELOG names `QRTX_CRED_HOST` and `QRTX_CRED_SCOPE`.

Deploy: tag a qrtx release, install `qrtx` and the `qrtx-tracker-*` plugins, then in each repository run `qrtx init` and `qrtx skills install --hooks --force`. The old `sdd` binary can stay installed until every clone has re-run setup, because both use the same `.git/sdd/` state.

Rollback: reinstall the old `sdd` binary and plugins. Repository files are unchanged, and `.git/sdd/` is shared. Hooks that were already moved to `qrtx` need `sdd skills install --hooks` and manual removal of the `qrtx` entries.

## Observed

- `config.go`: `CLIName = "sdd"`, `EnvRepo = "SDD_REPO"`, `EnvRunValidate = "SDD_RUN_VALIDATE"`. Usage and help text go through `CLIName` (`cli/cli.go`, `cli/sync.go`, `cli/tracker.go`).
- `tracker_exec.go` `bin()` and `cli/setup.go:173` build `sdd-tracker-` + name, and `adapter.go` validates names against the same pattern.
- `skills/hooks.go` `InstallHooks` appends a hook unless the exact command string already exists, so a changed binary name would add a second set.
- `cli/tracker.go` `registerMergeDriver` sets `merge.sdd-issues.driver` to `os.Executable()` and rewrites it when the path differs, so re-running init with `qrtx` moves it over.
- `providers/kit/credentials.go` `CredentialsPath` used `<config>/sdd/credentials.yaml`.
- `finish.go` `allowedValidateCommand` allowlists `CLIName + " "` and `"./" + CLIName + " "`.
- `tracker/marker.go` `markerPrefix = "<!-- sdd:v1 "`, `providers/jira/jira.go` `propKey = "sdd"` and `tracker_file.go` `"sdd-" + enc` are stored in remote issues and in `sdd/issues.jsonl`.
- `tracker_state.go` keeps tracker and sync state in `<git common dir>/sdd`.
- Baseline before the rename: `go test ./...` passed on `tracker-fixes` (f00aabb). `gofmt -l .` reports `tracker_recent.go`, and `sdd verify-ast` reports drift in the `testdata/pkgdrift` fixture; both are already on `tracker-fixes`.
- After the rename: `go vet ./...` and `go test -count=1 ./...` pass. `QRTX_UPDATE_GOLDEN=1 go test ./...` changes nothing under `testdata/`. `QRTX_REPO=examples/tic-tac-toe go run ./cmd/qrtx change verify --change tic-tac-toe` prints OK.
- In a scratch repository, `qrtx init` registers the merge driver with the `qrtx` path, and `qrtx skills install --hooks` replaced a `sdd sync begin --hook` entry. Over a `tracker-fixes` copy of `sdd-apply/SKILL.md`, `qrtx skills install` reports "differs from the bundled skill; pass --force".

## Assumptions

- The user's own repositories (Aurora, sdd-product and the mimile workspace) are the only installs, so dropping the old env-var and plugin names breaks nobody else.
- The `sdd-product` repository keeps its name for now. That's the user's call.

## Non-goals

- Renaming `.sdd.yaml`, `sdd/`, `*.sdd.md`, the `sdd-*` skills, or the wire and remote-data names in Q4.
- Changing history: `sdd/changes/archive/` and the released CHANGELOG sections stay as written.
- Fixing the existing gofmt finding in `tracker_recent.go` and the `testdata/pkgdrift` fixture drift.

## Deferred

- Aurora's update to `github.com/millerscout/qrtx` and its `CLIName` and env vars, planned in Aurora after a qrtx tag.
- Refreshing the skill copies in `mimile/.claude/skills/` and `sdd-product/.claude/skills/`.

## Tasks

- [x] 1.1 Move the module to `github.com/millerscout/qrtx`, the root package to `qrtx`, `cmd/sdd` to `cmd/qrtx`, and `CLIName` to `"qrtx"`; update every import and `qrtx.` qualifier. verify: `go build ./... && go vet ./...` covers S1
- [x] 1.2 Rename `cmd/sdd-tracker-*` to `cmd/qrtx-tracker-*` and the `sdd-tracker-` lookup prefix to `qrtx-tracker-`. verify: `go test ./cli -run TestProviderBinaries -count=1` covers S2
- [x] 1.3 Rename every `SDD_*` env var the code reads or sets to `QRTX_*`, including CI. verify: `git grep -nE 'SDD_[A-Z]' -- . ':!sdd/changes' ':!CHANGELOG.md'` lists only `docs/SDD_VS_OPSX_BASELINE.md` links and the old-manifest fixture in `finish_test.go` covers S3
- [x] 1.4 Replace `sdd <subcommand>` text and tool prose with `qrtx` in code, docs, README, skills, the example and the CHANGELOG's Unreleased section; leave the Q4 names alone. verify: `QRTX_UPDATE_GOLDEN=1 go test ./... && git diff --exit-code -- testdata` covers S1,S4
- [x] 1.5 Make `InstallHooks` drop old `sdd` hook commands. verify: `go test ./skills -run TestHooksInstall -count=1` covers S5
- [x] 1.6 Move credentials to `<config>/qrtx/` and read the old file when the new one is missing. verify: `go test ./providers/kit -run TestCredentialsPathFallsBackToLegacySddDir -count=1` covers S6
- [x] 1.7 Allowlist `sdd ` and `./sdd ` in `run-validate`. verify: `go test . -run TestAllowedValidateCommand -count=1` covers S7
- [x] 1.8 Add the "Renamed to qrtx" CHANGELOG entry with the migration steps and the names kept on purpose. verify: the Unreleased section lists Q2, Q3, Q5, Q6 and the Q4 names covers S1

## Validate

- [x] V1.1 `go vet ./... && go test -count=1 ./...` covers S1,S2,S3,S5,S6,S7
- [x] V1.2 `QRTX_UPDATE_GOLDEN=1 go test ./... && git diff --exit-code -- testdata` covers S4
- [x] V1.3 `QRTX_REPO=examples/tic-tac-toe go run ./cmd/qrtx change verify --change tic-tac-toe` covers S4

## Constitution

- C1 Rule 1 → the rename is planned here as `sdd/changes/qrtx-rename/` with `qrtx change new` and `propose-context`, passes `finish-propose` and `finish-apply`, and is archived. The plan was written after the code (D8): the user asked for it to be back-filled before commit, so Rule 1's "before any code changes" was not met for this change.

## Review

- R1 A4, `cli/setup.go:249` `skills.InstallKeep`: `qrtx init` keeps an old skill copy as a local edit, so re-running setup alone does not move the skills over → A4 is narrowed to name `qrtx skills install --force` for skill copies, matching K2 and the Deploy line.
