---
name: sdd-archive
description: Archive a completed SDD change. Use when the user runs /sdd-archive.
---

Archive after all **Tasks** and **Validate** are `- [x]` and manifest `status: complete`.

**CLI:** run `sdd …` with your shell tool from the project root, with no `SDD_REPO` and no `cd` elsewhere. The archive stays in the open project's git. If `sdd` is not on `PATH`, stop and tell the user to install it (`go install github.com/millerscout/sdd/cmd/sdd@latest`).

**Steps**

1. `sdd change status --change <slug>` — confirm complete.
2. Optional: `sdd change run-validate --change <slug>` if V1.x not yet run.
3. Run the project's test suite when runtime code changed, plus any check the constitution (`sdd constitution show`) or the change's **Constitution** lines require before a change is done.
4. Archive: `sdd change archive <slug> --run-validate`. Add `--quick-check` to run the `quick_check` command from `.sdd.yaml` before the move (opt-in; may be slow).
5. Report path under `sdd/changes/archive/`, and the package contracts that gained a `## Behaviour` block.

**Living specs:** archive appends the change's Requirements and Scenarios, with ids prefixed by the slug, to the `## Behaviour` section of each touched package's `contract.sdd.md`. Touched packages are those in `packages:` plus the dirs of cited files that have a contract. It is idempotent, and the `sdd` export block is left untouched. Commit those contract edits with the archive move. The next `sdd change propose-context` shows them as `existing_behaviour`.

**Note:** `archive` always runs manifest **verify** inside the CLI; `--run-validate` executes unchecked allowlisted V1.x commands.
