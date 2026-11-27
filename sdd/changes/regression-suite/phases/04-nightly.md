# Phase nightly workflow and soak

- [ ] 4.1 Add `.github/workflows/nightly.yml` (cron `0 6 * * *` plus `workflow_dispatch`):
  - it builds `qrtx` and `qrtx-tracker-github`;
  - when `TRACKER_SCRATCH_TOKEN` is set, it writes a temp credentials file binding `api.github.com` to scope `millerscout/sdd-tracker-scratch` with `token_env`, and runs `qrtx tracker check --allow-writes` in a temp repo configured for that repo;
  - otherwise it prints `::notice::TRACKER_SCRATCH_TOKEN not set; skipping real GitHub check`.

  verify: dispatch with and without the secret, per V1.6. covers S11
{: id="phase-04-task-1"}
- [x] 4.2 Add `e2e/soak_test.go`, gated by `QRTX_SOAK=<duration>`: 5 sessions do seeded random heartbeats, claims, releases and flips across two worktrees with lease 2s and tick 100ms; afterwards `checkInvariants(events, samples)` verifies Q9: releases are excluded from the daemon check, and the bounds are 1s. Add `TestSoakInvariants`, which feeds `checkInvariants` hand-made event logs and samples, one breaking each invariant, and expects each to be reported. The nightly workflow runs it with `QRTX_SOAK=5m`. verify: `go test -tags e2e ./e2e/ -run TestSoakInvariants` passes, and `QRTX_SOAK=30s go test -tags e2e ./e2e/ -run TestSoak` reports zero violations. covers S12
{: id="phase-04-task-2"}
- [x] 4.3 Add `docs/testing.md`, describing the test layers (unit, e2e, goldens, fuzz, nightly), how to run each, `QRTX_E2E_KNOWN_BUGS`, and how to update goldens. Add a CHANGELOG line. verify: `git diff main --stat` touches only the paths in S13. covers S13
{: id="phase-04-task-3"}
