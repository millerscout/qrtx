```change
name: sync-marks
status: complete
packages: []
```

## Why

qrtx's sync notices reach every agent session's prompt. In a leadhype session one burst read `1.1 done`, `V1.1 done`, `1.1 done` again, then `1.2` to `1.4`, which looks like something is wrong. The notices should say each thing once, in task order, and lose none.

## User request

The user found strange marks in the qrtx notices of a leadhype session. They chose: report only task files, keep truncated notices unread, stop echoing a session's own ticks, drop stale watch entries, and list a burst in task order.

## Acceptance

- A task checked once appears once for the worktree where its box changed.
- Ticks found in one scan of one file are listed in task order (1.2 before 1.10).
- Events announced by `…and N more` stay unread, and a later `sync inbox` from the same worktree shows them while who shares that worktree is unchanged.
- A session that is alone in its worktree when it reads is not told about ticks made in that worktree.
- Watch entries of deleted files leave `state.json`.

## Requirements

- Q1 The watcher SHALL read a change's task files only: `phases/*.md` when the change has a `phases/` folder, else `change.sdd.md`, the same rule `change status` counts by.
- Q2 `task_progress` events found in one scan of one file SHALL be logged in task id order, comparing numbers as numbers.
- Q3 `sync inbox --brief` text output SHALL print at most 10 events and leave the rest unread. `--json` and the output without `--brief` SHALL stay complete.
- Q4 A session's inbox SHALL leave out `task_progress` of its own worktree unless another live session works in that worktree when the inbox is read. `sync watch` SHALL still show it.
- Q5 The watcher SHALL drop the entry of a file that no longer exists, and keep entries of existing files that belong to other project roots.

## Scenarios

- S1 (Q1) WHEN a phased change has an index line `1.1`, a `V1.1` line and a phase task `1.1`, and all three are checked THEN exactly one `task_progress` for `1.1` is logged and none for `V1.1`. WHEN a change without `phases/` has a task checked THEN it is logged. MODIFIES tracker-sync/Q9 WHEN a phased change's `change.sdd.md` box is checked THEN no event is logged.
- S2 (Q2) WHEN tasks 1.1 to 1.12 and V1 of one file are checked between two scans THEN the log lists 1.1, 1.2, up to 1.12 in that order, then V1.
- S3 (Q3) WHEN 25 events arrive and `sync inbox --brief` runs THEN it prints 10 and `…and 15 more`, and the next `sync inbox` prints the other 15. MODIFIES tracker-sync/S6 WHEN `--brief` cuts the list THEN the cursor stops before the first event left out instead of at the log's end.
- S4 (Q4) WHEN a task is checked in a worktree and a session alone there reads its inbox THEN the event is omitted and another worktree's session sees it. WHEN a second live session shares the worktree at read time THEN both see it. MODIFIES tracker-sync/Q6 WHEN the daemon logs a tick of the reader's own unshared worktree THEN the inbox omits it.
- S5 (Q5) WHEN a watched file no longer exists and lies under no listed worktree THEN its entry is removed; an existing file under no listed worktree keeps its entry; no event is logged for either.

## Decisions

- D1 One helper, `taskFiles`, serves `ChangeProgress` and the watcher. Rejected: skipping only index lines in the watcher because two rules would drift again; the user chose to drop the `V*` notices of phased changes' `change.sdd.md`. This narrows tracker-sync/Q9.
- D2 `InboxLimit` holds the cursor and `Inbox` calls it unlimited. Rejected: changing `Inbox`'s signature because callers of the package would break.
- D3 Own ticks are recognised by worktree path. Rejected: stamping a session id on watch events because the daemon cannot know who edited a file.
- D4 Outside the listed worktrees an entry goes only on `os.IsNotExist`; inside them an entry the scan did not read goes, as before. Rejected: dropping outside entries on any stat error because another root's snapshot would be lost.
- D5 Order is set where events are emitted. Rejected: sorting in the inbox because `sync watch` reads the same log.
- D6 Whether a worktree is shared is decided when the inbox is read. Rejected: stamping it on the event at tick time because it changes the log format for a rare handover. This narrows tracker-sync/Q6.

## Risks

- A session's hand edits in its own worktree are no longer reported to it → accepted by the user; `sync watch` still shows them.
- A burst larger than 10 drains over several prompts → `…and N more` names the command that shows all of it.
- An old daemon keeps the old behaviour until restarted → Deploy restarts it.

Deploy: install the new binary, then run `qrtx sync stop` in each repository with a running daemon; the next `begin`, `claim` or heartbeat outside the debounce window (at most 60s) starts the new one. `state.json` and `events.jsonl` keep their format, so old and new binaries can share them meanwhile.

Rollback: install the previous version and run `qrtx sync stop`; the old watcher snapshots `change.sdd.md` files again without logging events.

## Observed

- `sync_watch.go` at 2c50967 globbed both `sdd/changes/*/change.sdd.md` and `sdd/changes/*/phases/*.md`, while `ChangeProgress` in `change.go` reads the phase files only when `phases/` exists.
- A leadhype repository's `.git/sdd/events.jsonl`, seqs 16–21, logged `1.1`, `V1.1`, `1.1`, `1.2`, `1.3`, `1.4` for one commit of a phased change.
- `cli/sync.go` printed 10 events while `Inbox` in `sync.go` had moved the cursor past all of them.
- Watch events in `sync.go` carry no `Session`, so `Inbox` showed them to the session that made them.
- The prune in `sync_watch.go` matched listed worktree prefixes only; the same leadhype `state.json` held 469 of 1618 entries for missing files.
- `watchRoot` ranged over a Go map, and a scratch run listed `1.5`, `1.6`, `1.10`, `1.13` first.
- The code and tests for Q1, Q3, Q4 and Q5 were written before this plan (see Constitution).

## Assumptions

- Other sessions working in a worktree hold a live lease when a session there reads its inbox.

## Non-goals

- Ordering across different files or scans.
- De-duplicating a tick that a merge, pull or checkout repeats in another worktree.
- The `<side-sessions-event>` tags the user saw: the desktop app adds them, qrtx does not.

## Deferred

- The known bugs that `docs/testing.md` scenarios skip for `sync-hardening`.

## Tasks

### Phase 1: sync notices — accepted

Tasks: `phases/01-notices.md`.

## Validate

- [x] V1.1 `go test -count=1 ./...` passes. covers S1,S2,S3,S4,S5
- [x] V1.2 `go test -count=1 -tags e2e ./e2e/` passes. covers S1

## Constitution

- C1 Rule 1 → not fully met: the code, tests and docs for Q1, Q3, Q4 and Q5 preceded this plan; the user, told of this, asked to write the plan afterwards, and the commit message will say so. Q2 is coded only after `finish-propose` passes. `/sdd-apply` and `/sdd-archive` run before the push.

## Review

- R1 A3,Q4,S4,K2 sharing is judged at read time → D6; A3, A4, Q4 and S4 reworded.
- R2 D4,Q5 D4 claimed more than the prune does → D4 limited to entries outside listed worktrees.
- R3 A1,Q1,S1 a merge can repeat a tick in another worktree → A1 narrowed per worktree; named in Non-goals.
- R4 A2,Q2 A2 promised order across scans → A2 narrowed to one scan of one file.
- R5 D1,S2 `V*` still reported without `phases/` → D1 limited to phased changes.
- R6 S3,D2 cursor wording → S3 says before the first event left out.
- R7 Q1,Q4,S1,S4 narrowed promises of tracker-sync → S1 and S4 name Q9 and Q6; D1 and D6 record it.
- R8 DP,K3 a debounced heartbeat starts no daemon → DP corrected.
- R9 M1,Q4 the assumption was about the wrong sessions → M1 reworded.
- R10 C1 breach of Rule 1 accepted by the user → C1 reworded.
- R11 omitempty-vs-presence S1 `change` → not a conflict because the watcher always sets it.
- R12 omitempty-vs-presence S1 `task` → not a conflict because `watchTaskID` never returns empty.
- R13 uncited-writer `sdd/changes` RB → not a conflict because none writes watch state or the event log.
