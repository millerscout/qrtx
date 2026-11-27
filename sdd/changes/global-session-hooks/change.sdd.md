```change
name: global-session-hooks
status: planning
packages: []
```

## Why

The user wants every Claude Code session on this machine to take part in qrtx sync (heartbeat, claims, inbox) without installing hooks repository by repository. Claude Code reads user-wide hooks from `~/.claude/settings.json`, so qrtx should be able to install its sync hooks there. A user-wide hook also fires in sessions that have nothing to do with qrtx: a home directory, a scratch folder, any git repository or Go module. Today a hook-mode heartbeat there creates sync state (`.sdd/` or `<git common dir>/sdd/`) and may start a daemon. So the hooks must stay silent and do nothing outside qrtx projects. The approach: a `--global` target for the existing hook installer, and a guard on hook-mode sync commands that checks for the method's own files.

## User request

"Every Claude Code session on this machine runs qrtx sync via hooks installed globally in `~/.claude/settings.json`." As handed over by the session that started this one: `qrtx skills install --hooks --global` merges the sync hooks into `~/.claude/settings.json`, idempotently. Every hook-mode command (`sync heartbeat|begin|end|inbox --hook`) exits 0 silently and creates no state unless the hook's cwd resolves to a qrtx project (root holds `sdd/changes`, `sdd/constitution.md` or `.sdd.yaml`; `.git` or `go.mod` alone don't count). The non-hook CLI keeps today's behaviour. Install replaces legacy `sdd sync …` hook entries, so old and new hooks don't both fire. Docs in `docs/sync.md` and the CHANGELOG.

## Acceptance

- `qrtx skills install --hooks --global` adds the five sync hooks to `~/.claude/settings.json`, keeps everything else in that file, prints `updated <path> (qrtx sync hooks)`, and a re-run prints `unchanged <path> (qrtx sync hooks)` and leaves the file byte-identical.
- In a Claude Code session started in a folder that is not inside a qrtx project (home, a scratch dir outside any qrtx project, a plain git repo, a Go module), even when `$QRTX_REPO` is exported, the sync hooks print nothing, exit 0, create no `.sdd/` or `.git/sdd/`, start no daemon and write nothing to `$CLAUDE_ENV_FILE`.
- In a qrtx project, the same hooks heartbeat, register the session and show the inbox as they do today.
- Legacy `sdd sync …` entries in the file being written are replaced by the `qrtx` form. When the current project's own `.claude/settings{,.local}.json` still holds sync hooks, the global install says so, naming each file, so the user can remove the duplicates.
- `qrtx sync heartbeat|claim|inbox|status` without `--hook` behave exactly as before.

## Requirements

- Q1 `qrtx skills install --hooks --global` SHALL merge the sync hooks into `<claude config dir>/settings.json`, where the config dir is `$CLAUDE_CONFIG_DIR` when set and `~/.claude` otherwise, through `skills.InstallHooks`, keeping every other key and hook, and a re-run SHALL change nothing and report `unchanged`.
- Q2 `--global` SHALL install hooks only: it SHALL NOT copy skills, touch `.gitignore` or write anything under the current project, and SHALL be rejected together with `--shared` or `--dir`.
- Q3 `--global` SHALL refuse to write a hook command that points at a temporary build (an executable under the OS temp dir or a `go-build` directory, as with `go run`), and SHALL tell the user to install qrtx first.
- Q4 A hook-mode sync command (`sync heartbeat|begin|end|inbox --hook`) SHALL exit 0 with no output and create no state, start no daemon and write nothing to `$CLAUDE_ENV_FILE` unless both the root resolved from the hook's cwd (the process cwd when the hook gives none, ignoring `$QRTX_REPO`) and the root sync would use hold `sdd/changes`, `sdd/constitution.md` or `.sdd.yaml`.
- Q5 In a qrtx project, hook-mode commands SHALL behave as before: heartbeat extends the lease, begin registers the session and exports `QRTX_SESSION`, end releases claims, inbox prints events.
- Q6 Sync commands run without `--hook` SHALL keep today's behaviour in any directory, qrtx project or not.
- Q7 Installing into any settings file SHALL replace legacy hooks whose command runs `sdd` (bare or a path ending in `/sdd`, quoted or not) with the same sync arguments, and leave other hooks alone.
- Q8 `--global` SHALL print a `note:` for each of the current project's `.claude/settings.json` and `.claude/settings.local.json` that still holds a sync hook (qrtx or legacy sdd), and SHALL NOT edit those files.

## Scenarios

- S1 (Q1) WHEN `HOME` is a temp dir whose `.claude/settings.json` holds `permissions` and an unrelated `PreToolUse` hook and `qrtx skills install --hooks --global` runs twice THEN the first run prints `updated <HOME>/.claude/settings.json (qrtx sync hooks)` and the file holds the five sync hooks plus the old keys; the second prints `unchanged …` and the file is byte-identical. WHEN `CLAUDE_CONFIG_DIR` is set THEN `<CLAUDE_CONFIG_DIR>/settings.json` is written instead.
- S2 (Q2) WHEN `--global` runs with the cwd in a temp project THEN no `.claude/` directory and no `.gitignore` appear in that project. WHEN `--global` is combined with `--shared` or `--dir x` THEN the command exits 1 naming the conflict and writes nothing.
- S3 (Q3) WHEN `qrtx` is not on `PATH` and the running executable is under the temp dir THEN `--global` exits 1 with a message naming `go install github.com/millerscout/qrtx/cmd/qrtx@latest`, and the settings file is not created.
- S4 (Q4) WHEN `sync heartbeat --hook`, `begin --hook`, `inbox --hook --brief` and `end --hook` run with hook stdin `cwd` set to an empty temp dir, and then to a temp dir holding only `.git` (a `git init` repo) and `go.mod` THEN each exits 0 with empty stdout and stderr, no `.sdd/` or `.git/sdd/` exists afterwards, and `$CLAUDE_ENV_FILE` is still empty. WHEN `$QRTX_REPO` points at a qrtx project and the hook's cwd is an empty temp dir THEN the hooks are equally silent and that project's state gets no session. MODIFIES tracker-sync/S1 WHEN a hook-mode heartbeat runs outside a qrtx project THEN nothing happens, instead of creating state.
- S5 (Q5) WHEN the same hook commands run with `cwd` in a temp git repo holding `.sdd.yaml` (and, separately, only `sdd/changes/`) THEN heartbeat creates `state.json` with the hook's session, begin appends `export QRTX_SESSION=<id>` to `$CLAUDE_ENV_FILE`, and `sync status --json` lists the session as live. MODIFIES tracker-sync/S18 WHEN `begin --hook` runs THEN the export happens only inside a qrtx project.
- S6 (Q6) WHEN `qrtx sync heartbeat --session x` runs (no `--hook`) in a temp dir that is not a qrtx project THEN it still creates state and prints `x: lease until …`, as before.
- S7 (Q7) WHEN the global settings file holds `"/Users/me/go/bin/sdd" sync heartbeat --hook --quiet` and `sdd sync begin --hook` next to an unrelated `sdd change list` hook THEN `--global` removes both sync entries, adds the `qrtx sync …` set, and keeps `sdd change list`.
- S8 (Q8) WHEN `--global` runs in a project whose `.claude/settings.local.json` holds `"/x/sdd" sync heartbeat --hook --quiet` THEN stdout has a `note:` line naming that file, and the file is byte-identical afterwards. WHEN the project has no sync hooks THEN no note is printed.

## Decisions

- D1 Guard in `runSync`, after the root is re-resolved from the hook's cwd and before `ResolveSession` or any write, for `--hook` with `heartbeat|begin|end|inbox`, using two new predicates: `qrtx.IsProject(root)` (the root itself holds a marker) and `qrtx.InProject(dir)` (the root found by walking up from `dir` with `RepoRoot`'s markers, ignoring env, is a project). Rejected: changing `RepoRoot`'s markers because the non-hook CLI (Q6) and every other command depend on today's resolution; rejected: guarding inside `Heartbeat`/`BeginSession` because the library calls are also used by non-hook commands and tests.
- D2 The guard checks only the root `RepoRoot` resolves from the hook's cwd, not any ancestor. Rejected: walking up to the nearest qrtx marker because hook-mode and CLI-mode would then pick different roots and sessions would sync under a different scope than the agent's own `qrtx` commands. Consequence: a session started in a subdirectory with its own `go.mod` (or `.git`) inside a qrtx repo resolves to that subdirectory and stays silent, as listed in Non-goals.
- D3 In hook mode the hook's own cwd must lie in a qrtx project even when `$QRTX_REPO` is set, and the root `$QRTX_REPO` names must be one too; which root sync then uses is unchanged (`$QRTX_REPO` still wins). Rejected: checking only the final root because a `QRTX_REPO` exported in a shell profile would then make every session on the machine heartbeat and print inbox events into that repo; rejected: exempting `$QRTX_REPO` entirely for the same reason.
- D4 `--global` installs hooks only (Q2). Rejected: also installing skills into `~/.claude/skills` because that directory already holds differently-worded `sdd-*` skills, which a plain install reports as local edits and `--force` would overwrite; skills stay per project.
- D5 Legacy entries are replaced in the file being written, and project files that still hold sync hooks get a `note:` (Q8). Rejected: having `--global` edit the current project's settings files because a user-wide install that rewrites a repository's (possibly committed) `.claude/settings.json` is surprising, and other clones aren't reached anyway.
- D6 The settings dir honours `$CLAUDE_CONFIG_DIR`. Rejected: always `~/.claude` because Claude Code reads user settings from `$CLAUDE_CONFIG_DIR` when it is set, so hooks written elsewhere would never fire.
- D7 Refuse `--global` for a temporary executable (Q3). Rejected: writing the temp path because `hookCommand` falls back to `os.Executable()`, and under `go run` that file is deleted after the run, which would break every session's hooks machine-wide.

## Risks

- Per-repo hooks (here: `mimile/sdd` and `mimile/sdd-product` `.claude/settings.local.json` call `"/Users/aurora/go/bin/sdd" sync …`) keep firing next to the global ones → `--global` prints a note naming the file (Q8); the CHANGELOG and `docs/sync.md` say to remove per-repo sync hooks after a global install.
- Existing hook users in a repo that has only `go.mod`/`.git` lose sync silently → such a repo was never a qrtx project; `qrtx init` (which writes `sdd/constitution.md` unless a constitution is configured, `cli/setup.go:261`) or `qrtx change new` (which creates `sdd/changes/`) turns it on again. The CHANGELOG says so.
- A per-repo `"/abs/qrtx" sync …` and a global `qrtx sync …` differ as strings, so both run even if Claude Code deduplicates identical commands → harmless (heartbeat is debounced, the inbox cursor advances once per event), and the Q8 note points them out.

Deploy: install the new binary (`go install …/cmd/qrtx@latest`) first, then run `qrtx skills install --hooks --global`, then remove per-repo sync hooks the note names. Old per-repo hooks keep working meanwhile because the state directory is shared.

Rollback: remove the five `qrtx sync` entries from `~/.claude/settings.json` (other keys are untouched by the install), and re-run `qrtx skills install --hooks` per repository if per-repo hooks are wanted again. Sync state is unchanged by this change.

## Observed

- `cli/cli.go:386-446` `runSkills install`: flags `--force`, `--hooks`, `--shared`, `--dir`; always runs `skills.Install` first, then `skills.InstallHooks(<root>/.claude/settings{,.local}.json, hookCommand())`, then `ensureGitignored` and a "clone only" note when not `--shared`.
- `cli/cli.go:502` `hookCommand()` returns `qrtx` when `exec.LookPath` finds it, else the quoted `os.Executable()`. `which qrtx` finds nothing on this machine (only `/Users/aurora/go/bin/sdd`), so a `go run` install would write the go-build temp path.
- `skills/hooks.go` `InstallHooks` already drops legacy `sdd` commands with the same args (`dropLegacy`, `isLegacyCommand` trims quotes and checks `filepath.Base == "sdd"`), returns `changed=false` without writing when every hook is present, and writes atomically via `path.tmp` + rename. So Q7 is test-only.
- `cli/sync.go:35-60` `runSync` reads hook JSON (`readHook`), resolves `root` with `qrtx.RepoRoot()`, and re-resolves from `hook.Cwd` (chdir and back) unless `$QRTX_REPO` is set.
- `verify.go:110-139` `RepoRoot`: `$QRTX_REPO` wins; else the nearest dir with `.sdd.yaml`, `go.mod` or `.git`; else the working directory.
- `sync.go:459` `Heartbeat` calls `withSync(..., create=true)` and may start the daemon; `BeginSession` (`sync.go:487`) heartbeats and appends to `$CLAUDE_ENV_FILE`; `EndSession` uses `create=false`; `Inbox` (`sync.go:584`).
- `tracker_state.go:15` state dir is `<git common dir>/sdd`, or `<root>/.sdd` outside git.
- `cli/sync_test.go` `TestSyncHeartbeatCLI` and `TestSyncInboxCLI` run `--hook` against bare temp roots via `QRTX_REPO`; they need a `.sdd.yaml` once the guard lands. `cli/main_test.go` sets `QRTX_NO_DAEMON=1`.
- Hooks on this machine: `~/.claude/settings.json` holds no sync hooks; `mimile/sdd/.claude/settings.local.json` and `mimile/sdd-product/.claude/settings.local.json` hold the five `"/Users/aurora/go/bin/sdd" sync …` entries.
- Baseline: `go test ./... -count=1` passes at `d48d725`.

## Assumptions

- Claude Code reads user hooks from `~/.claude/settings.json` (or `$CLAUDE_CONFIG_DIR/settings.json`) and passes the session's `cwd` in the hook stdin JSON for every event qrtx hooks on.
- Being silent includes stderr: a hook warning outside a project is noise in every unrelated session.
- `sdd/constitution.md` is `DefaultConstitutionFile`; a constitution configured elsewhere through `.sdd.yaml` is covered by the `.sdd.yaml` marker.

## Non-goals

- Editing per-repo `.claude/settings{,.local}.json` from `--global`, including the two mimile repos (D5).
- Syncing sessions started in a nested module or nested repo inside a qrtx project (D2). A folder with no root marker of its own inside a qrtx project resolves to that project and does sync, as today.
- Installing the hooks into the real `~/.claude/settings.json` as part of this change.
- An uninstall command.

## Deferred

## Tasks

- [ ] 1.1 Add `qrtx.IsProject(root)` (true when `root` holds `sdd/changes`, `DefaultConstitutionFile` or `RepoConfigFile`) and `qrtx.InProject(dir)`, and the hook guard in `runSync` after the hook-cwd re-resolution; give `TestSyncHeartbeatCLI` and `TestSyncInboxCLI` (bare temp roots with `--hook`) a `.sdd.yaml`. Guard tests `t.Chdir` to a temp dir so nothing resolves to this repo's live `.git/sdd`. verify: `go test ./cli -run 'TestSyncHook|TestSyncHeartbeatCLI|TestSyncInboxCLI' -count=1` covers S4,S5,S6
- [ ] 1.2 Add `--global` to `skills install`: settings path from `$CLAUDE_CONFIG_DIR` or `os.UserHomeDir()/.claude`, hooks only, reject `--shared`/`--dir`, refuse a temp executable, note project settings files that still hold sync hooks (new `skills.HasSyncHooks`). verify: `go test ./cli -run TestHooksGlobal -count=1` covers S1,S2,S3,S7,S8
- [ ] 1.3 Update `docs/sync.md` (global install, project guard, turning sync off) and the CHANGELOG Unreleased section. verify: `grep -n -- '--global' docs/sync.md CHANGELOG.md` shows both covers S1,S4

## Validate

- [ ] V1.1 `go vet ./... && go test ./... -count=1` covers S1,S2,S3,S4,S5,S6,S7,S8
- [ ] V1.2 `go run ./cmd/qrtx verify-ast` reports no new drift

## Constitution

- C1 Rule 1 → planned here as `sdd/changes/global-session-hooks/` with `qrtx change new` and `propose-context` before any code change, passed through `finish-propose`, applied against tasks 1.1–1.3 and archived with the CLI built from source (`go run ./cmd/qrtx`).

## Review

- R1 D1 vs `cli/sync.go` hook-cwd block: a guard placed before the re-resolution from `hook.Cwd` would judge the process cwd's root → D1 now places it after that block and before `ResolveSession`.
- R2 D3, A2, S4: with `$QRTX_REPO` pointing at a qrtx project, a guard on the final root alone lets unrelated sessions heartbeat into it, against D3's own rationale → D3 and Q4 now also require the hook cwd's own root to be a project; S4 gains that case.
- R3 S4, Q5 vs `cli/sync_test.go` `TestSyncHeartbeatCLI`/`TestSyncInboxCLI`: bare temp roots with `--hook` go silent under the guard → task 1.1 adds `.sdd.yaml` to them.
- R4 S4: the test process cwd is inside this repo, a qrtx project whose state is the live `.git/sdd` → task 1.1's guard tests `t.Chdir` to temp dirs.
- R5 K2 vs `cli/setup.go`: `.sdd.yaml` is written only by `--tracker` setup, not plain `qrtx init` → K2 now names `sdd/constitution.md` (written by `qrtx init`) and `qrtx change new`.
- R6 D4, Q2 vs `cli/cli.go` install path: `skills.Install` runs before the hooks branch → not a conflict because task 1.2 branches on `--global` before it; S2 checks no `.claude/` appears.
- R7 A2 vs `cli/sync.go`: `RepoRoot`, `gitToplevel` and `gitBranch` run read-only git before the guard → not a conflict because they write nothing.
- R8 `propose_context.go:78` `RepoRoot()` → not a conflict because `RepoRoot` is unchanged (D1).
- R9 `cli/setup.go:249` `skills.InstallKeep` → not a conflict because it is the `qrtx init` path, untouched by `--global`.
- R10 `sync.go:490` `Heartbeat` in `BeginSession` → not a conflict because the guard in `runSync` covers `begin --hook`, and non-hook callers (daemon tests) don't pass `--hook` (Q6).
- R11 A1 vs `skills/hooks.go`: the first write re-marshals the file with sorted keys → not a conflict because A1 promises kept content and a byte-identical re-run, which `changed=false` gives.
- R12 A4, Q7 vs `dropLegacy`: replacement covers only the written file → not a conflict because A4 and D5 say so; Q8 notes project files.
- R13 Q3 vs `hookCommand`: the refusal only matters when qrtx is off PATH and the executable is temporary → not a conflict because S3 describes that case.
- R14 A2 vs D2: a scratch dir inside a qrtx project resolves to it and syncs → A2 now says "outside any qrtx project"; Non-goals states the case.
- R15 S4 vs `soft()`: the guard returns before any soft-wrapped warning → not a conflict because stderr stays empty.
- R16 K3, M1: Claude Code's dedupe and `$CLAUDE_CONFIG_DIR` behaviour are external → K3 no longer relies on dedupe; M1 stays an assumption.
- R17 C1 vs Rule 1 naming → not a conflict because Rule 1 names the method and the plan runs its propose → apply → archive flow with the qrtx CLI.
