```change
name: go-minimum
status: complete
packages: []
```

## Why

`go.mod` says `go 1.22.0`, and Go 1.22 is end of life. On macOS 26, a pure-Go binary that Go 1.22's internal linker builds with cgo enabled aborts before `main` with `dyld: missing LC_UUID load command`. That covers the test binaries and the `qrtx-tracker-*` plugins. CI hides it with `GOFLAGS=-ldflags=-linkmode=external` in the macOS/1.22/cgo=1 job (commit 74a6eda). That workaround does nothing for a user who builds qrtx with Go 1.22. Go 1.24 is the first release whose internal linker writes LC_UUID, so raising the minimum to Go 1.24 fixes the problem itself and lets CI test the real minimum with no workaround.

## User request

Raise the Go minimum so pure-Go qrtx binaries work on macOS 26 without the CI external-link workaround. Bump the `go` line in `go.mod`, test the new minimum in the CI matrix instead of `1.22.x`, remove the `GOFLAGS` workaround and its comment, and record the change and the reason in the CHANGELOG.

## Acceptance

- `go.mod` declares the new minimum, and `go mod tidy` leaves `go.mod` and `go.sum` unchanged under both the minimum toolchain and the latest one.
- With no `GOFLAGS`, `CGO_ENABLED=1 go test ./...` passes on macOS 26 under the minimum toolchain.
- The CI matrix tests the new minimum and `stable` with no `GOFLAGS` external-link workaround, and all 8 jobs pass.
- The CHANGELOG `## Unreleased` section says the minimum Go changed and why.

## Requirements

- Q1 The module SHALL declare Go 1.24 as its minimum, and building qrtx with that toolchain on macOS 26 with cgo on SHALL produce binaries that start.
- Q2 CI SHALL test the declared minimum and `stable` on Linux and macOS, with cgo on and off, and without linker flags that users don't use.
- Q3 Builds on newer toolchains SHALL keep working: `go mod tidy`, `gofmt`, `go vet` and `go test` stay clean on the latest Go.
- Q4 Users SHALL be able to see the new minimum and its reason in CHANGELOG.md and CONTRIBUTING.md.

## Scenarios

- S1 (Q1) WHEN `GOTOOLCHAIN=go1.24.13 CGO_ENABLED=1 go test ./...` runs on macOS 26 with `GOFLAGS` empty THEN every package passes, the tracker plugin tests included, and no dyld abort appears.
- S2 (Q1) WHEN a user with Go 1.22 or 1.23 as their default toolchain and `GOTOOLCHAIN=auto` builds qrtx THEN the go command uses Go 1.24 or later because of the `go 1.24.0` line, and with `GOTOOLCHAIN=local` it refuses with "go.mod requires go >= 1.24.0" rather than producing a binary that aborts.
- S3 (Q2) WHEN CI runs on the PR THEN the matrix lists `1.24.x` and `stable`, `ci.yml` contains no `GOFLAGS` or `linkmode`, and all 8 jobs (2 OS × 2 Go × 2 cgo) pass.
- S4 (Q3) WHEN `go mod tidy` runs under Go 1.24.13 and under the local Go 1.27.1 THEN `git diff --exit-code go.mod go.sum` is clean (no `toolchain` line added).
- S5 (Q3) WHEN `gofmt -l .`, `go vet ./...` and `go test ./...` run under Go 1.27.1 THEN `gofmt` prints nothing and vet and tests exit 0.
- S6 (Q4) WHEN a reader opens CHANGELOG.md THEN `## Unreleased` says the minimum is Go 1.24 because Go 1.22's internal linker omits LC_UUID and macOS 26 aborts those binaries; CONTRIBUTING.md says Go 1.24 or newer.

## Decisions

- D1 Minimum is Go 1.24 (`go 1.24.0`). Rejected: Go 1.23 because its internal linker still writes no LC_UUID (otool shows none on 1.23.0 and 1.23.12). Its binaries start here only because they are stamped `minos 11.0`, while Go 1.22's cgo build was stamped `minos 26.0`. That leniency depends on dyld's rules and on the build host, and Go 1.23 is end of life too.
- D2 Write `go 1.24.0` with no `toolchain` line. Rejected: a `toolchain go1.24.13` line because it would force a download for users who already have any 1.24 release, and the bug is fixed in 1.24.0.
- D3 Update CONTRIBUTING.md's "Go 1.22 or newer" line in the same change. Rejected: leaving it because it would state a minimum that `go.mod` no longer allows.
- D4 Leave the `go 1.22` in `graph_module_test.go` alone. Rejected: bumping it because it is the content of a throwaway demo `go.mod` that the test writes, not this module's minimum.

## Risks

- A user pinned to Go 1.22 or 1.23 with `GOTOOLCHAIN=local` can no longer build qrtx → the CHANGELOG says so; both releases are end of life, and the default `GOTOOLCHAIN=auto` fetches 1.24.
- The `go 1.24` language version turns on vet checks gated on it (printf with a non-constant format) → `go vet ./...` with `go 1.24.0` ran clean before planning (see Observed); V1.2 runs it again.

Deploy: n/a (build metadata and CI only; no data persists and no process format changes). After archive, push branch `go-minimum` and open a PR to `main`; it is done when all 8 CI jobs pass. No tag.

Rollback: n/a (nothing persists); revert the commit to restore `go 1.22.0` and the workaround.

## Observed

- `go.mod:3` is `go 1.22.0`. `.github/workflows/ci.yml:18` has `go: ["1.22.x", "stable"]`, and lines 22–25 hold the LC_UUID comment and the `GOFLAGS: … '-ldflags=-linkmode=external' …` workaround. `CONTRIBUTING.md:14` says "Go 1.22 or newer".
- This Mac runs macOS 26.6.2 (`sw_vers`).
- `GOFLAGS= GOTOOLCHAIN=go1.22.12 CGO_ENABLED=1 go test -count=1 ./providers/kit` → `dyld[66138]: missing LC_UUID load command … signal: abort trap`, `FAIL github.com/millerscout/qrtx/providers/kit`.
- `GOFLAGS= GOTOOLCHAIN=go1.23.12 CGO_ENABLED=1 go test -count=1 ./providers/kit` → `ok` (1.764s), and the same with `go1.23.0` → `ok`. But `otool -l` on the `go test -c` binaries from 1.23.0 and 1.23.12 shows no `LC_UUID`. The binaries' `LC_BUILD_VERSION` is `minos 11.0 sdk 11.0` under 1.23.0 and `minos 26.0 sdk 26.5` under 1.22.12 (cgo=1). Run directly, the 1.22.12 binary aborts and the 1.23.0 one prints `PASS`.
- `GOFLAGS= GOTOOLCHAIN=go1.24.0 CGO_ENABLED=1 go test -count=1 ./providers/kit` → `ok` (1.476s), and `go1.24.13` (latest 1.24; `go1.24.14` is "toolchain not available") → `ok` (1.512s). `otool -l` shows one `LC_UUID` in the 1.24.0 and 1.24.13 binaries. `-ldflags=-v` on 1.24.13 printed no host-link line, so the internal linker wrote it.
- With `go.mod` edited to `go 1.24.0`, `GOFLAGS= go vet ./...` under Go 1.27.1 exited 0, and `GOTOOLCHAIN=local <go1.22.12 GOROOT>/bin/go build ./cmd/qrtx` printed `go: go.mod requires go >= 1.24.0 (running go 1.22.12; GOTOOLCHAIN=local)` and exited 1.
- `sdd/constitution.md` Rule 1 requires this change to be planned here.

## Assumptions

- GitHub's `macos-latest` runner runs macOS 26 or a dyld with the same rule, as the failure that prompted 74a6eda showed.
- `actions/setup-go@v5` resolves `1.24.x` to the latest 1.24 patch.

## Non-goals

- Tagging v0.5.1; the user decides that.
- Using language or library features newer than Go 1.22.

## Deferred

(none)

## Tasks

- [x] 1.1 Run `go mod edit -go=1.24.0`, then `go mod tidy` under `GOTOOLCHAIN=go1.24.13` and under the local Go 1.27.1. verify: `grep '^go \|^toolchain' go.mod` prints only `go 1.24.0`, and `git diff --exit-code go.sum` is clean after each tidy. covers S2,S4
- [x] 1.2 In `.github/workflows/ci.yml`, change `"1.22.x"` to `"1.24.x"` and delete the LC_UUID comment and the `GOFLAGS` line. verify: `grep -nE 'GOFLAGS|linkmode|1\.22' .github/workflows/ci.yml` prints nothing; after archive, all 8 CI jobs pass on the PR (see Deploy). covers S3
- [x] 1.3 Add `## Unreleased` to CHANGELOG.md with a Changed entry for the Go 1.24 minimum and its reason, and change CONTRIBUTING.md to "Go 1.24 or newer". verify: `grep -n 'Go 1.24' CHANGELOG.md CONTRIBUTING.md` shows both. covers S6

## Validate

- [x] V1.1 `GOFLAGS= GOTOOLCHAIN=go1.24.13 CGO_ENABLED=1 go test -count=1 ./...` passes on macOS 26. covers S1
- [x] V1.2 `gofmt -l .` prints nothing, and `go vet ./...` and `go test -count=1 ./...` pass under Go 1.27.1. covers S5
- [x] V1.3 The declared minimum itself: `GOFLAGS= GOTOOLCHAIN=go1.24.0 CGO_ENABLED=1 go test -count=1 ./...` passes on macOS 26, and `GOTOOLCHAIN=go1.24.0 go mod tidy` leaves `go.mod` and `go.sum` unchanged. covers S1,S4

## Constitution

- C1 Rule 1 → this change is planned here as `go-minimum` and passes `finish-propose` before `go.mod`, CI or docs change; it is applied with `finish-apply` and filed with `qrtx change archive` in a separate "Archive go-minimum" commit before the PR is pushed (Deploy). The CHANGELOG entry is task 1.3 of this change, so no exemption is claimed.

## Review

- R1 A1,A2,S1,S4,V1.1,D2: the plan calls go1.24.0 the minimum but tested the full suite and tidy only on go1.24.13, and CI's `1.24.x` resolves to the latest patch → V1.3 runs `go test ./...` and `go mod tidy` under go1.24.0 too.
- R2 C1, Tasks: C1 promised archiving but no task or Deploy step said when, and task 1.4 (open the PR) could not be checked before archive → C1 and Deploy say archive comes before the push; the PR and CI result move to Deploy and task 1.2's verify.
- R3 C1: Rule 1's CHANGELOG exemption covers only already-archived changes → not a conflict because the entry is task 1.3 of this change; C1 now says no exemption is claimed.
- R4 D4: remaining `1.22` mentions (`go.mod:3`, `CONTRIBUTING.md:14`, `ci.yml:18,22,25`, `graph_module_test.go:22`) → not a conflict because tasks 1.1–1.3 cover all but the test's demo `go.mod`, which D4 keeps on purpose.
