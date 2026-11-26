```change
name: plan-bench-daylist-row
status: complete
packages: []
```

## Why

Aurora change `daylist-break-regression` (Aurora repo, `sdd/changes/daylist-break-regression/`) needs a plan-bench row for its day-list request, so a later propose change can be replayed against a saved planning baseline. The plan bench lives in this module, so per Rule 1 the row is planned here and Aurora picks it up with `go get`. Alternatives go in Decisions.

## User request

Add a day-list row to the plan bench so a later planning change can show where the plan missed and whether dogfood got better. Requested by Aurora change `daylist-break-regression`; moved here under Rule 1 and Aurora's Article X item 7.

## Acceptance

- The plan bench has a `daylist-break` row for the day-list request, and `TestPlanBench` reports it unchanged against its saved baseline and apply_ok.
- Every existing bench row keeps its band, domain and apply status.

## Requirements

- Q1 The plan bench SHALL hold a `daylist-break` row whose classified band and domain match its stored expectation and baseline.
- Q2 The existing 20 rows SHALL keep their band, domain and apply status.

## Scenarios

- S1 (Q1) WHEN `go test . -run TestPlanBench -v` runs THEN it logs 21 rows: improved 0, regressed 0, unchanged 21, apply_ok 18, apply_fail 1, apply_skipped 2.
- S2 (Q2) WHEN the row is added THEN no existing row's band, domain or apply status changes: apply_fail stays 1 and apply_skipped stays 2.

## Decisions

- D1 Append one row to `testdata/fixtures/sdd_plan_bench/scenarios.jsonl` and one to `baseline.jsonl`, and raise the counts in `propose_bench_test.go`. Rejected: a separate day-list bench file because the bench test reads one scenarios file and one baseline, and a second file would need a second loader for one row.
- D2 Request text is the Aurora plan's User request, verbatim; `acceptance_bullets` 2, `overlap` false, `hit` `scripts/daylist_quality.py`, `paths` `["scripts/fixtures/accomplish/daylist_good/daylist.py"]`, `expect_band` `goldfish`, `expect_domain` `scripts`. Rejected: a shortened request because the row is the baseline for that exact request, and a later replay must classify the same text.

## Risks

- The row's expectation is wrong and the bench goes red → a probe run on 2026-11-26 of `proposeBand` and `proposeDomain` on that request gave `goldfish`, `scripts` and `apply_ok` (Observed).

Deploy: after `/sdd-archive` commits the archived plan, tag a patch release on that commit; Aurora runs `go get github.com/millerscout/sdd@<tag>` in its own plan.

Rollback: n/a (test fixtures only; consumers pin the previous tag).

## Observed

- `propose_bench_test.go` `TestPlanBench` checks `len(rows) != 20`, `unchanged != 20`, and `applyOK != 17 || applyFail != 1 || applySkipped != 2`.
- `testdata/fixtures/sdd_plan_bench/scenarios.jsonl` and `baseline.jsonl` hold 20 rows each. Two `scripts` rows (`lance-cards`, `golden-script`) and one `scripts/fixtures` row (`paired-cases`) exist, all `simple` or `context` with an empty `hit`; none of the four `goldfish` rows has a `scripts/` hit.
- `propose_score.go` `proposeDomain` takes the first path in the request, then Observed, then the hit paths, and returns its directory. `propose_bench.go` `applyBenchStatus` returns apply_ok when every path is inside that domain (`inDomain`).
- A throwaway probe test (deleted after the run) of `proposeBand(request, 2, false)`, `proposeDomain(request, ["scripts/daylist_quality.py"], "")` and `applyBenchStatus` on the D2 row gave `goldfish`, `scripts` and `apply_ok` (2026-11-26).
- `sdd/constitution.md` Rule 1 requires sdd work to be planned in this repository, with other projects naming it as a dependency.

## Assumptions

- The day-list programs in Aurora do not need to exist for this row: the bench classifies text and paths, it does not open them.

## Non-goals

- Changing how propose classifies or scores a request.
- The day-list programs, scorer and miss file, which stay in Aurora's `daylist-break-regression`.

## Deferred

- Aurora's `go get` of the release and its own day-list work stay in Aurora change `daylist-break-regression`.

## Tasks

- [x] 1.1 Append the D2 row to `testdata/fixtures/sdd_plan_bench/scenarios.jsonl` and `{"id":"daylist-break","band":"goldfish","domain":"scripts"}` to `testdata/fixtures/sdd_plan_bench/baseline.jsonl`, then raise `propose_bench_test.go` to 21 rows, unchanged 21 and apply_ok 18. verify: V1.1. covers S1,S2
- [x] 1.2 Add a CHANGELOG entry with the change, and tag the next patch release on the archive commit (see Deploy). verify: `git tag --points-at HEAD` names the tag.

## Validate

- [x] V1.1 `go test . -count=1 -run TestPlanBench -v` covers S1,S2

## Constitution

- C1 Rule 1 → this plan is the sdd-side home for the row; Aurora's `daylist-break-regression` names it as a dependency and keeps only `go get` and its own tests. The CHANGELOG entry and tag are planned tasks (1.2), not exemptions, and the tag goes on the archive commit.

## Review

- R1 D2, K1, A1, S1: a second probe on a scratch copy, with the Aurora User request verbatim, gives `goldfish`, `scripts`, `apply_ok` and unchanged → not a conflict because the expected row matches what `proposeBand`, `proposeDomain` and `applyBenchStatus` return.
- R2 D1, S1, S2, Q2: `TestPlanBench` checks 20 / unchanged 20 / 17-1-2, and the other `sdd_plan_bench` readers (`city.jsonl`, `tictactoe.jsonl`, `line_criteria.json`) don't count scenarios or baseline → not a conflict; 21 / 21 / 18-1-2 is right.
- R3 Observed: `paired-cases` is a `scripts/fixtures` row, not a `scripts` row → Observed reworded; the point (no goldfish row with a `scripts/` hit) holds.
- R4 C1: the plan called its own tag a Rule 1 exemption, but exemptions cover commits outside a plan, and tagging before archive would miss the archive commit → C1 says the CHANGELOG entry and tag are planned tasks, and Deploy and task 1.2 tag on the archive commit.
