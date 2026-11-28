```change
name: no-legacy-sdd
status: complete
packages: []
```

## Why

v0.5.0 renamed the tool to qrtx and kept three pieces of support for the old name, so setups made before the rename would keep working. The user's machine and repositories have now been moved over. The user wants no fallbacks: qrtx should behave as if the tool had always been called qrtx.

## User request

After `~/.config/sdd/credentials.yaml` was moved to `~/.config/qrtx/credentials.yaml`, the user said "no fallback". Asked which pieces of old-name support to remove, they chose all three: the credentials fallback, the hook replacement, and the `run-validate` allowlist for `sdd …`.

## Acceptance

- qrtx reads credentials only from `$QRTX_CREDENTIALS` or `<config dir>/qrtx/credentials.yaml`; a file under `<config dir>/sdd/` is ignored.
- `qrtx skills install --hooks` (project or `--global`) treats hooks that call `sdd` like any other hook: it neither removes them nor counts them as qrtx sync hooks.
- `qrtx change run-validate` rejects a validate command that starts with `sdd ` or `./sdd ` as not allowlisted, unless the repo allowlists it (`validate_allow`) or `QRTX_RUN_VALIDATE=1` is set.
- The docs name only the qrtx credentials path.

## Requirements

- Q1 `CredentialsPath` SHALL return `$QRTX_CREDENTIALS` when set, else `<config dir>/qrtx/credentials.yaml`, whether or not `<config dir>/sdd/credentials.yaml` exists.
- Q2 `InstallHooks` SHALL leave every hook that is not exactly one of its own qrtx sync commands untouched, including hooks that call `sdd`.
- Q3 `HasSyncHooks` SHALL report only hooks that run the sync commands through `qrtx` (bare, by path, or `qrtx.exe`).
- Q4 `allowedValidateCommand` SHALL allow only the built-in prefixes, which include `qrtx ` and `./qrtx ` but not `sdd ` or `./sdd `, plus the repo's `validate_allow` prefixes and the `QRTX_RUN_VALIDATE` escape.

## Scenarios

- S1 (Q1) WHEN only `<config>/sdd/credentials.yaml` exists THEN `CredentialsPath` returns `<config>/qrtx/credentials.yaml`, and a tracker command reports no binding for the host instead of using the old file.
- S2 (Q2) WHEN a settings file holds `sdd sync begin --hook` and `"/x/sdd" sync heartbeat --hook --quiet` THEN `qrtx skills install --hooks` adds the qrtx sync hooks and keeps both sdd entries byte-for-byte; a second run reports unchanged.
- S3 (Q3) WHEN a settings file's only sync hook is `"/x/sdd" sync heartbeat --hook --quiet` THEN `HasSyncHooks` reports false, so `--global` prints no "also runs sync hooks" note for it.
- S4 (Q4) WHEN a validate line is `sdd change verify --change x` or `SDD_REPO=. sdd change verify --change x` THEN `allowedValidateCommand` rejects it, while `qrtx change verify --change x` and `./qrtx change verify --change x` stay allowed; WHEN the repo's `validate_allow` lists `sdd ` THEN it is allowed.

## Decisions

- D1 Remove all three fallbacks and the `sdd` match in `isSyncCommand`. Rejected: keeping the hook migration because the user chose "All three".
- D2 Leave old `sdd` hooks in place, not delete them. Rejected: deleting any hook that mentions `sdd` because qrtx would then be editing hooks it does not own, which is the very migration being removed.
- D3 Add a "Removed" CHANGELOG entry under a new `## Unreleased` and leave the v0.5.0 entry as released history. Rejected: editing the v0.5.0 notes because that release did ship the fallbacks.
- D4 Fix `docs/providers.md`, which still names `<config>/sdd/credentials.yaml` from before v0.5.0. Rejected: leaving it because it would send users to a path that is no longer read.

## Risks

- A machine that still has only `~/.config/sdd/credentials.yaml` loses tracker access → the CHANGELOG says to move the file to `~/.config/qrtx/`; this user's file is already there (`qrtx tracker check` passes in `sdd-product`).
- A repo whose hooks still call `sdd` keeps calling it, and fails once the old binary is removed → the CHANGELOG says to remove those hooks by hand; this machine has none (checked `~/.claude/settings*.json` and the project settings).
- An Aurora plan whose validate lines say `sdd …` is rejected → Aurora's `CLIName` is `aurora-llm sdd`, so its plans write `aurora-llm sdd …`, which is not an `sdd ` prefix and is unaffected by this change.

Deploy: ships with the next qrtx tag. Move credentials and remove old hooks before upgrading.

Rollback: reinstall v0.5.0 (`go install github.com/millerscout/qrtx/cmd/qrtx@v0.5.0`). No data changes.

## Observed

- `providers/kit/credentials.go` `CredentialsPath` returns `<dir>/sdd/credentials.yaml` when the qrtx file is missing and the sdd one exists; `providers/kit/kit_test.go` `TestCredentialsPathFallsBackToLegacySddDir` covers it.
- `skills/hooks.go` `InstallHooks` calls `dropLegacy`/`isLegacyCommand` to remove `sdd` hooks; `isSyncCommand` also accepts base name `sdd`, and `HasSyncHooks` uses it. `cli/cli.go:546` prints the "also runs sync hooks" note from `HasSyncHooks`.
- Tests that expect the old behaviour: `skills/skills_test.go` `TestHooksInstallReplacesLegacySdd` and the `/x/sdd` case in `TestHasSyncHooks`, and `cli/hooks_global_test.go` `TestHooksGlobal`, whose fixture holds `sdd sync begin --hook` and `"/Users/me/go/bin/sdd" sync heartbeat …`.
- `finish.go` `allowedValidateCommand` lists `"sdd "` and `"./sdd "`; `finish_test.go` `TestAllowedValidateCommand` asserts they are allowed.
- `docs/providers.md:41` names `$XDG_CONFIG_HOME/sdd/credentials.yaml` and `~/.config/sdd/credentials.yaml`.
- On this machine, `~/.config/qrtx/credentials.yaml` now holds the GitHub binding, `~/.config/sdd/credentials.yaml` is gone, and `qrtx tracker check` passes in `sdd-product`.

## Assumptions

- The user's repositories are the only installs, and they are already moved over.

## Non-goals

- Renaming the method's files or data names (`.sdd.yaml`, `sdd/`, `sdd.adapter/v1`, the `sdd-issues` merge driver, `.git/sdd/`). Those are the method's names, not fallbacks.

## Deferred

(none)

## Tasks

- [x] 1.1 Remove the `<config>/sdd/` fallback from `CredentialsPath`, replace its test with one that an sdd-only file is ignored, and fix `docs/providers.md`. verify: `go test ./providers/kit -run TestCredentialsPath -count=1` covers S1
- [x] 1.2 Delete `dropLegacy` and `isLegacyCommand`, drop `sdd` from `isSyncCommand`, update the comments, and change `TestHooksInstallReplacesLegacySdd`, `TestHasSyncHooks` and `TestHooksGlobal` to expect sdd hooks kept and not counted. verify: `go test ./skills ./cli -run 'TestHooks|TestHasSyncHooks' -count=1` covers S2,S3
- [x] 1.3 Remove `"sdd "` and `"./sdd "` from `allowedValidateCommand` and make `TestAllowedValidateCommand` assert they are rejected unless allowlisted. verify: `go test . -run TestAllowedValidateCommand -count=1` covers S4
- [x] 1.4 Add `## Unreleased` → `### Removed` to the CHANGELOG with the three removals and what to do before upgrading. verify: the entry names the credentials move and the hooks to remove covers S1,S2

## Validate

- [x] V1.1 `gofmt -l .` prints nothing, and `go vet ./... && go test -count=1 ./...` passes covers S1,S2,S3,S4

## Constitution

- C1 Rule 1 → planned here as `sdd/changes/no-legacy-sdd/` with `qrtx change new` and `propose-context`, passes `finish-propose` before code changes, then is applied and archived.

## Review

- R1 `cli/setup.go:249` `skills.InstallKeep` (uncited): it installs skill copies and never reads credentials, hooks or validate lines → not a conflict, because none of Q1–Q4 touch skill copies.
