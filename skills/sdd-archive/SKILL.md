---
name: sdd-archive
description: Archive a completed SDD change. Use when the user runs /sdd-archive.
---

Archive after all **Tasks** and **Validate** are `- [x]` and manifest `status: complete`.

**CLI:** run `qrtx …` with your shell tool from the project root, with no `QRTX_REPO` and no `cd` elsewhere. The archive stays in the open project's git. If `qrtx` is not on `PATH`, stop and tell the user to install it (`go install github.com/millerscout/qrtx/cmd/qrtx@latest`).

**Steps**

1. `qrtx change status --change <slug>` — confirm complete.
2. Optional: `qrtx change run-validate --change <slug>` if V1.x not yet run.
3. Run the project's test suite when runtime code changed, plus any check the constitution (`qrtx constitution show`) or the change's **Constitution** lines require before a change is done.
4. Archive: `qrtx change archive <slug> --run-validate`. Add `--quick-check` to run the `quick_check` command from `.sdd.yaml` before the move (opt-in; may be slow).
5. Report path under `sdd/changes/archive/`, and the package contracts that gained a `## Behaviour` block.
6. **Claims:** release this change's claims with `qrtx sync claim <key> --release` (keys `<slug>/<task id>` and the issue refs under `issues:`), so other sessions see the work as free. `sync not enabled; claim skipped` means there is nothing to release.
7. **Tracker:** when the manifest lists `issues:`, archive comments on each linked issue and marks it completed (unless another active change still links it). A tracker failure never stops the archive. If the output says `tracker: <n> pending`, report it and show `qrtx tracker outbox`; the next `qrtx tracker` command retries.

**Living specs:** archive appends the change's Requirements and Scenarios, with ids prefixed by the slug, to the `## Behaviour` section of each touched package's `contract.sdd.md`. Touched packages are those in `packages:` plus the dirs of cited files that have a contract. It is idempotent, and the `sdd` export block is left untouched. Commit those contract edits with the archive move. The next `qrtx change propose-context` shows them as `existing_behaviour`.

**Note:** `archive` always runs manifest **verify** inside the CLI; `--run-validate` executes unchecked allowlisted V1.x commands.
