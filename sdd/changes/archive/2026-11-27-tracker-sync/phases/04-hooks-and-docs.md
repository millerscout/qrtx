# Phase Claude Code hooks, skills and docs

- [x] 4.1 Add `skills.InstallHooks(path string, sddPath string) (changed bool, err error)`:
  - it merges into the JSON settings file, keeping unknown keys and existing hooks;
  - it adds `UserPromptSubmit` (`sdd sync heartbeat --hook --quiet` and `sdd sync inbox --hook --brief`), `PostToolUse` (`matcher: "*"`, `async: true`, the heartbeat) `SessionStart` (`sdd sync begin --hook`) and `SessionEnd` (`sdd sync end --hook`);
  - an identical hook command is not added twice.

  Add `sdd skills install --hooks [--shared]`. verify: `go test ./skills/ ./cli/ -run TestHooks` covers an existing `PreToolUse` hook and `permissions` kept, a re-run byte-identical, and `--shared` writing `settings.json`. covers S13
{: id="phase-04-task-1"}
- [x] 4.2 Update the skills:
  - `sdd-apply` runs `sdd sync claim <issue ref or slug/task>` before working (stopping and reporting when another session holds it) and `--release` when done;
  - `sdd-propose` checks claims before `sdd tracker start`;
  - `sdd-archive` releases the change's claims.

  Update the docs: `docs/sync.md` (leases, claims, inbox, daemon, files, hooks), `README.md`, `docs/providers.md` and `CHANGELOG.md`. verify: `go test ./skills/ ./cli/` passes and `grep -n "sdd sync claim" skills/*/SKILL.md docs/sync.md` lists the lines. covers S13,S15
{: id="phase-04-task-2"}
