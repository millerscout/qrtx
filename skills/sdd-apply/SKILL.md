---
name: sdd-apply
description: Implement tasks from an SDD change manifest. Use when the user runs /sdd-apply or asks to build an active SDD slug.
---

Implement tasks from `sdd/changes/<slug>/change.sdd.md`.

**CLI:** run `sdd …` with your shell tool from the project root, with no `SDD_REPO` and no `cd` elsewhere. `sdd/changes/` lives in the open project so that project's git tracks it. Allow long timeouts for test commands. If `sdd` is not on `PATH`, stop and tell the user to install it (`go install github.com/millerscout/sdd/cmd/sdd@latest`).

**Steps**

1. Resolve slug — confirm manifest exists; `sdd change status --change <slug>`.
2. Read the spec before coding. When the manifest has these sections, read **Scenarios** (`- S<n> WHEN … THEN …`), **Decisions** (`- D<n> … Rejected: …`), **Risks** (including any `Deploy:` and `Rollback:` lines) and **Review**.
   - For each task or Validate line that cites `covers S<n>`, write or extend the test that proves that scenario's THEN, and mark the line `- [x]` only when that test passes. `finish-apply` fails with `scenarios not covered by a checked task: S…` while any scenario is cited only by unchecked lines.
   - Never implement against a Decision, and never use an approach the Decision lists as `Rejected:`. If a Decision proves wrong during apply, stop and ask the user; do not silently change the design.
   - Follow the `Deploy:` line when you deploy, and keep the `Rollback:` path working. Treat **Review** findings as constraints on the code.
3. Implement — read **User request**, **Acceptance**, **Validate**, and `open_phase` from `sdd change status`. A `review` phase is not open. Implement the phase only when its roadmap line says `accepted`. Read the roadmap and that phase file. Run the test named in the phase, then mark its tasks `- [x]`. When the test passes, the phase is complete: read status again and implement the next open phase in the same apply. Keep going until no phase is open. A failed test stays on that phase; do not start the next one, and do not ask the user to continue.
4. Complete **Validate** (`V1.x`) harnesses before `status: complete` (use backticks around shell commands in manifest lines for `run-validate`).
5. **Finish apply** — `sdd change finish-apply --change <slug>` (verify, scenario coverage, verify-ast when `packages:` set, run-validate for pending V1.x). Apply is **not** done until exit 0.
6. Suggest `/sdd-archive <slug>` when all tasks checked and status **complete**.

**Guardrails:** stay scoped to the manifest; follow the project's own docs (README, CONTRIBUTING, AGENTS.md, CLAUDE.md) for build, test and deploy.
