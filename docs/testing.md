# Testing qrtx

qrtx is tested in layers. Each catches what the previous one can't.

| Layer | What it proves | Run it |
|---|---|---|
| Unit and package tests | Each function and command, with fake trackers and in-process CLI runs | `go test ./...` |
| Race detector | No data races in the daemon, outbox and lock code | `go test -race ./...` |
| JSON goldens | The JSON that agents and scripts read (`sync status`, `sync inbox`, `tracker list`, `tracker check`, the `propose-context` backlog, event lines) keeps its shape | `go test ./cli/ -run Golden` |
| Fuzzing | Merge, marker, ADF and manifest editing survive arbitrary input | `go test -run '^$' -fuzz FuzzMergeIssues -fuzztime 10s .` (see `ci.yml` for all four) |
| End-to-end (`e2e/`) | The built binaries, driven like Claude Code: hook JSON on stdin, `CLAUDE_ENV_FILE`, real git worktrees, a subproject, real daemons | `go test -tags e2e ./e2e/` |
| Soak (`e2e/`) | 5 sessions doing random sync work keep the invariants | `QRTX_SOAK=5m go test -tags e2e ./e2e/ -run 'TestSoak$'` |
| Nightly real GitHub | `qrtx tracker check --allow-writes` against a scratch repo | the `nightly` workflow (`workflow_dispatch` to run it now) |

CI (`.github/workflows/ci.yml`) runs the unit tests on Ubuntu, macOS and Windows. It also runs the race detector, a 10-second fuzz of each target, and the e2e suite on Ubuntu and macOS. The nightly workflow runs the real-GitHub check and the soak.

## Updating goldens

A golden failure means the JSON contract changed. If that is on purpose, regenerate the goldens and review the diff:

```bash
QRTX_UPDATE_GOLDEN=1 go test ./cli/ -run Golden
git diff testdata/golden/
```

Times, pids, generated ids and temp paths are replaced with placeholders, and arrays are sorted, so goldens only change when the shape does.

## Known-bug scenarios

An e2e scenario for a bug that isn't fixed yet skips with `known bug, fixed by <change>: <what>`. To see it fail, which proves the test catches the bug, run:

```bash
QRTX_E2E_KNOWN_BUGS=1 go test -tags e2e ./e2e/ -run <Scenario>
```

The change that fixes the bug removes the `knownBug` call, so the scenario then runs on every CI build.

## Fuzz crashers

A crasher found in CI is uploaded as the `fuzz-crashers` artifact. Commit its seed (`testdata/fuzz/<Target>/<hash>`) only together with the fix. Otherwise `go test ./...` runs it as a failing test.

## Windows

Tests that need `sh`, symlinks or Unix signals call `skipOnWindows(t, "<feature>")` and skip there with a `windows:` reason. The e2e package is Unix-only (`//go:build e2e && !windows`).

## Real GitHub secret

The nightly GitHub check needs the repository secret `TRACKER_SCRATCH_TOKEN`: a fine-grained token with Issues read and write on `millerscout/sdd-tracker-scratch` only. Without it, the job prints a notice and passes.
