```change
name: fixtures-survive-module-download
status: planning
packages: []
```

## Why

v0.4.0 shipped a golden fixture that holds a `go.mod`. Go treats any folder with a `go.mod` as a separate module and leaves it out of the parent module's download, so `go test github.com/millerscout/sdd` run from a consumer fails on the missing fixture. Aurora's golden gate runs that exact command, so Aurora cannot move to v0.4.0. The fix keeps the "a dependency manifest does not ground" check but moves the `go.mod` case into a test that writes the file at run time, and adds a guard so no fixture can hold a `go.mod` again.

## User request

Fix the v0.4.0 release so Aurora can `go get` it: the sdd tests must pass when run from the module cache, the way Aurora's `scripts/golden_regression.sh` runs them.

## Acceptance

- `go test github.com/millerscout/sdd` passes when run from a consumer against the new release, including `TestSddChangeManifestQualityGoldenJSONL`.
- Citing only `go.mod` (or another dependency manifest) still earns no Observed grounding.
- A test fails if anything under `testdata/` would be left out of the module download: a `go.mod`, a `vendor/` folder, a VCS folder, or a symlink.

## Requirements

- Q1 Every golden fixture SHALL be part of the published module download.
- Q2 Observed grounding SHALL still not credit a dependency manifest, `go.mod` included.

## Scenarios

- S1 (Q1) WHEN Aurora runs `go test github.com/millerscout/sdd -run TestSddChangeManifestQualityGoldenJSONL` against the new tag from its module cache THEN it passes, with no `lstat … no such file or directory`.
- S2 (Q1) WHEN a `go.mod`, a `vendor/` folder, a `.git`/`.hg`/`.bzr`/`.svn` folder or a symlink is added under `testdata/` THEN `TestTestdataSurvivesModuleZip` fails and names the path.
- S3 (Q2) WHEN Observed cites only `go.mod` (written at run time) or only `package.json` (the `cites-package-json` golden row) THEN `ObservedGrounded` is false and the row scores at most 94.

## Decisions

- D1 Replace golden row and fixture `cites-go-mod` with `cites-package-json` (same window, a root `package.json`), and test the `go.mod` case in `TestObservedGroundingEscapes` by writing `go.mod` into the temp repo. Rejected: renaming the fixture file (for example `go.mod.txt`) and renaming it back in the harness because `copyTree` is shared by every row and a per-file rename rule is a special case nobody will remember.
- D2 Add `TestTestdataSurvivesModuleZip`, which walks `testdata/` and fails on the markers Go's module zip leaves out (golang.org/x/mod/zip): a `go.mod` (nested module), a `vendor/` folder, a `.git`, `.hg`, `.bzr` or `.svn` folder, and any symlink or other non-regular file. Name rules Go rejects and `.gitattributes` `export-ignore` are not checked: Go reports bad names itself when it builds the zip, and the repo has no `.gitattributes`. Rejected: a CI-only check because the bug only shows from a consumer, and a unit test catches it at `go test ./...` time in this repo.

## Risks

- Another file the module zip drops slips into `testdata/` → D2's test catches the markers Go leaves out; a find on 2026-11-26 shows only `cites-go-mod/go.mod` today.

Deploy: tag the patch release on the archive commit and push; then Aurora runs `go get github.com/millerscout/sdd@<tag>` in its `one-switch-richer-propose` phase 3.

Rollback: n/a (test fixtures only); consumers stay on v0.3.1, the last tag before the broken v0.4.0.

## Observed

- From Aurora with sdd v0.4.0, `go test github.com/millerscout/sdd -count=1` failed: `TestSddChangeManifestQualityGoldenJSONL/cites-go-mod`, `change_manifest_quality_golden_test.go:42: lstat testdata/fixtures/sdd_manifest_quality/cites-go-mod: no such file or directory` (2026-11-26). The module cache's `testdata/fixtures/sdd_manifest_quality/` lists every fixture except `cites-go-mod`.
- `testdata/fixtures/sdd_manifest_quality/cites-go-mod/go.mod` is the only `go.mod` under `testdata/`.
- Aurora's `scripts/golden_regression.sh` runs `go test "$SDD_MODULE" -run TestSddChangeManifestQualityGoldenJSONL` (line 637), with `SDD_MODULE="github.com/millerscout/sdd"`.
- `manifest_quality.go` `dependencyManifests` lists `go.mod` and `package.json`; `manifest_quality_grounding_test.go` `TestObservedGroundingEscapes` writes its repo tree at run time.
- `sdd/constitution.md` Rule 1 requires this fix to be planned here.

## Assumptions

- No consumer reads the `cites-go-mod` fixture by name.

## Non-goals

- Changing the grounding rule itself.

## Deferred

- Aurora's version bump stays in Aurora's `one-switch-richer-propose` phase 3.

## Tasks

- [x] 1.1 Add `TestTestdataSurvivesModuleZip` (D2) in `manifest_quality_grounding_test.go`. verify: it fails on the current tree, naming `cites-go-mod/go.mod`; task 1.2 makes it pass. covers S2
- [x] 1.2 Replace the `cites-go-mod` row and fixture with `cites-package-json` in `testdata/fixtures/sdd_change_manifest_quality_golden.jsonl`, and add a `go-mod` case to `TestObservedGroundingEscapes` that writes `go.mod` into the temp repo and checks both `ObservedGrounded` false and `Score` at most 94. verify: V1.1. covers S3
- [x] 1.3 Add a CHANGELOG entry, and tag the next patch release on the archive commit (see Deploy). verify: in Aurora, after `go get github.com/millerscout/sdd@<tag>`, `go test github.com/millerscout/sdd -count=1` passes. covers S1

## Validate

- [x] V1.1 `go test ./... -count=1` covers S2,S3

## Constitution

- C1 Rule 1 → this fix is planned here before any code change; the CHANGELOG entry and tag are planned tasks (1.3), tagged on the archive commit.

## Review

- R1 K1, D2: `go.mod` is not the only thing the module zip drops; `vendor/` subfolders, VCS folders and symlinks are dropped too (none under `testdata/` today) → D2 and S2 widen the guard to all of them; Risks and D2 say what is not checked and why.
- R2 D1: a root `package.json` in a fixture → not a conflict because Go doesn't treat it as a module boundary, `copyTree` copies it, and `excludedFromGrounding` still blocks it by base name.
- R3 S3, task 1.2: the escape test checked only `ObservedGrounded`, not the score → 1.2 also asserts `Score` at most 94 for the `go-mod` case.
- R4 task 1.2: it cited S2, which task 1.1 owns → 1.2 covers S3, and 1.1 says 1.2 makes it pass.
- R5 C1 → not a conflict because Rule 1 wants the fix planned here, and planning the tag and CHANGELOG as tasks doesn't break it.
- R6 Rollback: "previous tag" is the broken v0.4.0 → Rollback names v0.3.1.
