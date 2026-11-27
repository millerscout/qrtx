# Phase CI matrix

- [x] 1.1 Extend `.github/workflows/ci.yml`. First set `defaults.run.shell: bash` and add a `git config --global core.autocrlf false` step before checkout. Then:
  - add `windows-latest` to the `test` job, with Go `stable` and cgo 0 (the Go 1.22 and cgo combinations stay on Unix);
  - add a `race` job (Ubuntu, Go `stable`, `go test -race ./...`);
  - add a `fuzz` job (Ubuntu, runs each `Fuzz*` target for 10s and uploads `testdata/fuzz` on failure);
  - add an `e2e` job (Ubuntu and macOS, Go `stable`, `go test -tags e2e ./e2e/ -count=1`, retried once on failure, with both logs kept as artifacts).

  verify: `actionlint .github/workflows/*.yml` (or `gh workflow view` after push) parses, and the job list matches S1. covers S1
{: id="phase-01-task-1"}
- [ ] 1.2 Run the suite as Windows would, using `GOOS=windows go vet ./...` and `go test` on a Windows runner through the PR. Every test that fails only for platform reasons gets `skipOnWindows(t, "<feature>")`, a helper in each package's test files that checks `runtime.GOOS == "windows"`. Each skip is listed in Observed with its feature. verify: the `test (windows-latest)` job passes, and `grep -rn skipOnWindows --include=*_test.go .` matches the list in Observed. covers S2
{: id="phase-01-task-2"}
