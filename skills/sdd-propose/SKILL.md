---
name: sdd-propose
description: Propose a new SDD-native change (planning only) under sdd/changes/<slug>/ in the open project. Use when the user runs /sdd-propose.
---

Propose a new **SDD-native** change under `sdd/changes/<slug>/`. Planning only — do not edit production implementation code in this workflow.

**Input**: Kebab-case slug after `/sdd-propose`, or a short description (derive slug from it).

**Planning boundary**: Author or update `change.sdd.md`, linked `contract.sdd.md` packages, and tasks. Stop when planning artifacts are coherent. Implementation is `/sdd-apply`.

**Where the plan lives:** `sdd/changes/<slug>/` is created in the open project so that project's git tracks it. The `sdd` CLI must be on `PATH` (`go install github.com/millerscout/sdd/cmd/sdd@latest`). Run every `sdd` command with your shell tool from the project root. Do not set `SDD_REPO` and do not `cd` elsewhere. If `sdd` is not on `PATH`, stop and tell the user how to install it.

```bash
sdd change list
```

Empty output and exit 0 means no active changes; scaffold next. Do not initialise a new project or edit product code in this workflow.

**Intent first (required before Discover)**

1. From the **full thread**, write the **user request** (what they wanted done or to experience).
2. Write **acceptance**: user-visible checks that would satisfy the requester (not “we changed file X”).
3. If they said “fix that” after a UX complaint, anchor acceptance on **their goal**, not only the last technical diagnosis.

**Steps**

1. **Slug** — From the **outcome**, not subsystem jargon; unused kebab-case (`sdd change list`).

2. **Discover** (read-only) — Code, tests, docs, existing `sdd/changes/`, contracts. Separate **Observed** from **Assumptions**. When the thread reports a failure, run the one check that failed and record the command and its result in **Observed** before tasks are written. A retelling is not that result. `finish-propose` rejects a failure-shaped **User request** whose **Observed** section has no path, backtick command, or HTTP or exit result. After step 5 scaffolds the slug, run `sdd change propose-context --change <slug>` (optional `--refresh-graph`; `--json` for agents). Treat overlap warnings as merge/defer signals when filling **Observed** and `packages:`. Read `existing_behaviour`: what earlier archived changes promised for the packages you touch. A new scenario that changes one of those promises says so, for example `- S4 (Q2) MODIFIES old-slug/S3 WHEN … THEN …`, and the change it makes goes in Decisions.
   - **Mappings from data, not docs:** when the change alters who can do what, or any other mapping kept as data (roles to permissions, plans to limits, flags to users), build the current mapping from where it is stored: seed files, migrations or the table. Docs and audits can be stale. Put the before and after for each party in **Observed**, and list every access or value a party loses in Decisions or Non-goals.
   - **Already done?** For each task you plan, check whether the code already does it, and cite where in **Observed**. A task whose behaviour already exists becomes a test-only task.

3. **Scope fit** — Primary tasks must meet **Acceptance**. Plumbing-only work either supports that or goes in a **second slug** named in **Deferred**. Do not plan only a transport tweak when acceptance needs a full answer to the user’s directive unless deferred is explicit.

4. **Clarify** — One question when acceptance or split would change; else assume in **Assumptions**. An unknown may stay open only if its answer would not change any scenario, decision or task. If it would change one, ask now instead of guessing, and do not write the plan around a guess.

5. **Scaffold** — run these two commands, then read `sdd/changes/<slug>/change.sdd.md`.
   ```bash
   sdd change new <slug>
   sdd change status --change <slug>
   ```

5a. **Requirements, then scenarios** — First fill `## Requirements` with one `- Q<n> <subject> SHALL <behaviour>.` bullet per concern from the categories below that this request can actually hit: each concern is a promise the change makes. Then fill `## Scenarios` with `- S<n> (Q<n>) WHEN <condition> THEN <observable result>` bullets, at least one per requirement, each naming the requirement it proves. Leave out the categories the request cannot hit, and do not write "n/a" bullets. Spend scenarios on how the change behaves over its life, not on its command-line surface. finish-propose rejects a requirement with no scenario and a scenario with no requirement.
   - **Misfire on normal use:** the new mechanism must leave healthy use near its limit alone (a long but live stream is not reclaimed, a valid old file is not rejected).
   - **Steady state kept:** the everyday path the change touches still works under crash and concurrent use (normal saves stay atomic and serialized, not only during the new operation).
   - **During the operation:** reads, writes and new arrivals while it runs; what each sees.
   - **Every party, every state:** for each client or process, each state it can be in (queued, holding, finished) crossed with cancel, crash, timeout or disconnect.
   - **Crash or failure midway:** what state is left, and how it recovers or is rolled back (including the rollback itself failing).
   - **After the operation:** backup retention, cleanup, finalize, the undo window, re-running it.
   - **Operator view:** what status, log or health shows mid-operation and after a recovery.
   - **Existing data and other writers:** older formats, unknown newer versions, other programs using the same data.
   - **Output contract:** what must stay stable for consumers.
   - **Authorization:** who may call each new or changed path, ownership scope (own records vs any), clients that bypass row-level security (service-role or admin keys), and the difference between reaching a route and seeing its data.
   - **Surface:** empty input, unknown flag, env unset or invalid, feature disabled. Write at most one scenario for these together, unless the request is itself about the CLI or config.

   Each THEN must be observable without reading code (output, exit code, file state, UI). Before writing a scenario, read the code it depends on (struct tags, existing filters, error helpers), and record what you read in **Observed**.

5b. **Decisions and risks** — Under `## Decisions`, write one `- D<n> <choice>. Rejected: <alternative> because <reason>.` bullet for each real choice. Write `- None: <reason>` only when the request leaves no choice. Under `## Risks`, write `- <risk> → <mitigation>`, then a `Deploy:` line and a `Rollback:` line.
   - `Deploy:` gives the order in which binaries, scripts and data move, and which old version must keep working during that window: for example, ship the reader that understands both formats, then the writer, then run the migration. It reads `n/a (<reason>)` when nothing persists and no other process is involved.
   - `Rollback:` says how to undo the change if it fails midway, or reads `n/a (<reason>)` when nothing persists. finish-propose requires a `Deploy:` line whenever `Rollback:` is not n/a.

5c. **Constitution** — Run `sdd constitution show`. If it prints `no constitution`, skip this step. Otherwise read every rule file it lists; those rules bind every change. Under `## Constitution`, write one `- C<n> <rule> → <how this plan complies>` line for each rule the change touches, citing the rule by its heading or the part before its colon (`Rule 1`, `Article IX`). Point at the task, Validate line or Decision that satisfies it. Write a single `- None: <why no rule applies>` only when no rule touches the change. A rule the plan cannot meet is not an exception to write down: ask the user, and record their answer as a Decision. finish-propose rejects a plan without this section, a line that names no rule, and a line without `→`.

6. **Plan** — `change.sdd.md` holds the roadmap: one `### Phase` per feature the request names, marked `review` or `accepted`, plus the path of that phase’s task file. Checkbox tasks live in `sdd/changes/<slug>/phases/<nn-name>.md`. A feature the request says comes later goes under **Deferred**. The first pass writes headings and one question. Before the answer, do not invent outcomes. After the answer, write the outcome bullets, copy them to Assumptions, and do not add a product the request did not name. The second pass writes the steps and the check for that phase, not a copy of the heading.

   Each task line ends with its own check (`verify: <command or observable result>`). Do not leave all checking to a final task. Every scenario id is cited by a Validate line or a task (`covers S1,S2`). finish-propose rejects an uncited scenario.

6a. **Consistency pass** — Before finishing, compare every Assumption and Decision with every Acceptance bullet and Scenario, and with the code cited in Observed. Typical misses: reusing a struct whose `omitempty` tag drops a field that an acceptance check says is always present; a `Rollback:` that restores a backup while a scenario promises no write made during the change is lost; a scenario that promises no starvation while a decision accepts it; a store helper whose save also bumps a timestamp the plan relies on staying fixed. Each contradiction you find becomes a Decision that resolves it, or a changed Acceptance bullet.

6b. **Cold review**: the author's own consistency pass misses contradictions, so a second reader checks the plan. This is the last planning step, after every other section is written.
   1. Run `sdd change review-packet --change <slug>`. It prints numbered claims, the cited code, mechanical hints, and other code that uses the same on-disk data or symbols (scripts writing the same files, other callers of a constructor). Code marked `(uncited)` was never read by the plan. Open it before deciding a design still holds: a lock per instance does not protect files that another process or a second instance also writes. Two hints need special care:
      - `setting-unwired`: a new setting whose package is built in more than one way. Find the path production actually uses (the server's own constructor call), and make sure the setting and its off switch reach it.
      - `all-callers`: a claim that something holds for every caller. The packet shows each call site; check them one by one. A release that happens when a response body closes is not a `defer`.
      - `soft-mitigation-vs-promise`: a risk handled only by a warning, a log or a deferral while a check promises no loss. A warning does not prevent the loss: prevent it, or narrow the promise and name the case in Non-goals.
      - `used-before-created`: a new identifier (a code, key or symbol) is used by a phase before the phase that creates it. Move the creating task earlier, or say what the earlier phase does until it exists.
      - `unknown-ref`: an identifier the code does not have, or a pattern such as `x.manage.*` used where the code compares one exact value. Name the exact existing value, or add a task that creates it.
      - `existing-siblings`: a new identifier next to existing ones of the same family (`records.read` beside `records.view.any`). Reuse one, or say in a Decision why a new one is needed and how it relates to the old ones.
      - `uncovered-sites`: a task replaces or removes something used in files the plan never names, or its check searches a narrower path than the real sites. Cover every site, or widen the check.
   2. Give the packet to a reader with fresh context and no stake in the plan. Where you can start a subagent, give it the packet text and tell it to read the cited files and answer in the packet's format. Where you cannot, re-read only the packet and the cited files, not your own notes.
   3. Apply each fix to the plan: a new or changed Decision, Acceptance bullet or Scenario, or a `needs user:` task. A fix may not defer or merely warn about a case that an Acceptance bullet, or a scenario promising no loss, still promises. Either handle the case, or narrow that bullet and name the case in Non-goals or Deferred. finish-propose rejects a `→ deferred` fix on a promised case that Non-goals and Deferred do not mention.
   4. Write `## Review` with one `- R<n> <claim ids> <conflict> → <fix>` line per finding. Every hint in the packet needs its own R line, even when it is not a conflict (`→ not a conflict because …`). Write `- None: <what was checked>` only when the reader found nothing and the packet had no hints. finish-propose rejects an unanswered hint.

7. **Verify contracts** when packages touched: `sdd verify-ast`.

8. **Finish propose** — run `sdd change finish-propose --change <slug>`. Propose is **not** done until this exits 0 (runs propose-context warnings, **verify**, and **score** JSON).

9. **Summarize** — Slug, acceptance one-liner, deferred slugs, next task; suggest `/sdd-apply`.

**No plannable goal**: answer in prose; offer slug + goal; scaffold after confirm.

**Score bands**

Read `score:` and `domain:` from propose-context.

- `simple`: the request names one file. One task and one validate line. No `### Phase` and no question.
- `context`: the request names no file. The reply asks one question about the open task. The only task in that pass is `needs user:` for that question. Do not run `finish-propose`.
- `goldfish`: the request itself states two checks, or another change overlaps. Ask the same way as `context`. The second reader still waits until no task is `needs user:`.
- A phase is one planned feature. The full list can be long. It is split across phases, not piled into one phase. The old 1-to-3 cap is not a storage limit.
- Before the answer, do not invent outcomes. After the answer, write the outcome bullets, copy them to Assumptions, and do not add a product the request did not name. Do not finish while a task starts with `needs user:`. Each phase stays `review` until the user accepts it. Apply implements a phase only when the roadmap line says `accepted`.
- `sdd` classifies each paragraph and each file token. A line that already says `later`, `accept`, or a file name is classified by rule. `goes first` or `first thing` is a sequence note on the previous phase, not a phase and not Deferred. Other lines go to the classifiers the host configures, if any; with none, each becomes a phase. Do not add a phase the classifier did not return.

**Guardrails**

- Manifest must show whether the **original request** would be addressed.
- The project's constitution (`sdd constitution show`) is binding; its own contributor docs (README, CONTRIBUTING, AGENTS.md, CLAUDE.md) say how to build and test.
