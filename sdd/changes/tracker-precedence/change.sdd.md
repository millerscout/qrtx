```change
name: tracker-precedence
status: planning
packages: []
```

## Why

The tracker already has `priority` (0–4) and `links` typed `blocks|relates|duplicates` in its schema (`tracker/issue.go`, `docs/schema/tracker/v1/issue.json`), but no command sets a link and nothing reads one. In practice, "the rename goes before the release" lived only in an agent's private plan. The product backlog listed both as unrelated `unstarted` items, so nothing stopped the release from being started first. Making `blocks` real, settable and enforced at `start`, puts the order where every session and the user can see it.

## User request

After seeing the backlog miss the rename-before-release order, the user asked to "track precedence when we have conflicts", and approved a plan:
- a command to link issues (`blocks`, `relates`, `duplicates`) and unlink them;
- `tracker start` refuses an issue that an unfinished issue blocks, unless `--force`;
- `tracker list` marks blocked items and sorts by priority;
- `propose-context` shows the order between overlapping changes.

They accepted deferring claim checks (`tracker-precedence-claims`) and native GitHub/Jira links (`tracker-native-links`). For priority they chose `tracker add --priority N` and `tracker priority <id> N`, stored on every backend.

## Acceptance

- `qrtx tracker link A blocks B` records the link on the file backend, GitHub and Jira, and on plugins that declare `update_issue`. `qrtx tracker unlink A blocks B` removes it.
- `qrtx tracker add --priority 1 …` and `qrtx tracker priority B 2` set priority on every backend, including the GitHub product backlog.
- `qrtx tracker start B <slug>` exits non-zero and names A while A is open (not completed or canceled), and creates nothing. `--force` starts B anyway and says it was forced. Once A is closed, `start B` works without `--force`.
- `qrtx tracker list` (with or without `--status`/`--change`) shows `blocked by A` next to B, and orders by priority (1 first, none last), keeping backend order among equals.
- When two active changes overlap and their linked issues are related by `blocks`, `qrtx change propose-context` names which change goes first, or warns when the tracker can't be reached.

## Requirements

- Q1 `tracker link` and `tracker unlink` SHALL add and remove a typed link on the source issue with a direct `update_issue`, on every backend that declares it, SHALL be idempotent, and SHALL report a backend that did not store the link.
- Q2 Priority SHALL be settable at `add` and later, on every backend, and SHALL appear in `get`, `list` and `--json` as the schema's `priority`.
- Q3 `tracker start` SHALL refuse an issue that an open issue blocks, naming every such blocker and creating nothing, unless `--force` is given, in which case it SHALL start and say it was forced. It SHALL fail closed when the blocker listing is incomplete, and SHALL skip the check with a warning when the backend cannot list issues.
- Q4 `tracker list` SHALL mark each blocked issue with its open blockers whatever filter is set, and SHALL order by priority without reordering equal priorities.
- Q5 `change propose-context` SHALL report, for each overlapping active change whose linked issues are related by `blocks` to this change's, which change goes first, and SHALL warn instead when the tracker is unavailable.
- Q6 Links and priority SHALL survive writes by older plugins and binaries, which know nothing of them.

## Scenarios

- S1 (Q1) WHEN `tracker link A blocks B` runs twice on the file backend THEN A's `links` holds exactly one `{blocks, B}`, and `tracker unlink A blocks B` removes it.
- S2 (Q1,Q2) WHEN `link A blocks B` and `priority B 2` run against the GitHub and Jira providers (fake APIs) THEN `get A` lists the link, and `get B` shows priority 2.
- S3 (Q3) WHEN B is blocked by A in `started` and `tracker start B b-slug` runs THEN the exit is non-zero, the message names A, and no change dir `b-slug` exists.
- S4 (Q3) WHEN the same start runs with `--force` THEN B starts and the output says `forced` and names A.
- S5 (Q3) WHEN A is completed or canceled THEN `tracker start B b-slug` works without `--force`.
- S6 (Q4) WHEN the backlog holds B (blocked by open A), C (priority 1) and D (no priority), and `tracker list --status backlog` runs while A is `started` THEN C comes first, D comes after the prioritised items, and B's line ends with `blocked by A`, although A itself is filtered out.
- S7 (Q5) WHEN active changes `x` (linking A) and `y` (linking B) share a package and A blocks B THEN `propose-context --change y` prints `goes first: x (A blocks B)` for that overlap. With the tracker unreachable, it prints a warning instead.
- S8 (Q1,Q3) WHEN a plugin declares `update_issue` but drops `add_links` THEN `link` exits non-zero saying the backend did not store it. WHEN a plugin does not declare `list_issues` THEN `start` warns that blockers were not checked and starts.
- S9 (Q3) WHEN the blocker listing stops at the page cap THEN `start` refuses, saying the listing was incomplete and that `--force` overrides.
- S10 (Q6) WHEN an issue with a link and a priority is updated by an older GitHub or Jira plugin (an `AddRefs` on `start`) THEN `get` with the new plugin still shows the link and the priority.

## Decisions

- D1 Keep links and priority in a qrtx slot that older plugins leave alone: on GitHub a second body marker `<!-- sdd-prec:v1 {...} -->`, which the old `DecodeMarker` leaves in the description it rewrites; on Jira a separate issue property `sdd-precedence`; on the file backend the schema's own `links` and `priority` fields. `toIssue` maps the slot into `Issue.Links` and `Issue.Priority`. Rejected: extending the existing `sdd` marker and `sdd` property, because older plugins rewrite those whole on every `AddRefs` and would erase links (review R4).
- D2 `UpdateIssueInput` gains `add_links`, `remove_links` and `priority`. These are copied field by field in `tracker_exec.go` and `providers/kit/provider.go` (which copy the update fields one by one) and applied by every backend's `UpdateIssue`. Rejected: native GitHub issue dependencies and Jira issue links in this change, because they differ per backend and plan level (deferred to `tracker-native-links`).
- D3 A link lives on the source issue (`A.links` has `{blocks, B}`), and blockers of B are found by listing open issues (`backlog`, `unstarted`, `started`) on B's own backend (`impl`) and comparing normalized refs (`ParseIssueRef`). Rejected: also writing a `blocked_by` entry on B, because two writes per link can half-fail.
- D4 `start` checks blockers after `GetIssue` and before `NewChange` (`tracker_lifecycle.go` between l.173 and l.176). It fails closed when the listing hits the page cap and skips with a warning when the backend lacks `list_issues`. `--force` is reported in the output only and not persisted. Rejected: persisting a `forced` role, because the schema allows only `""`/`deferred` roles and `sameLink` would then keep two refs per change (review R9).
- D5 `link`, `unlink` and `priority` call `impl.UpdateIssue` directly and check the returned issue. Rejected: queueing them through the outbox like `start`, because the re-read would run before the deferred drain and always report "not stored" (review R10).
- D6 `tracker list` keeps the issue schema in `--json` (its `links` and `priority` already show both), and the text output gains `blocked by`. When a filter is set, blockers come from a second open-issues listing. Rejected: a computed `blocked_by` JSON field, because the list output is the issue schema.
- D7 Phase 2 resolves links with one `GetIssue` per linked ref of both changes, under the same 10s budget as `proposeBacklog` (`tracker_open.go`), and degrades to a warning. Rejected: reading links from manifests, because links live on issues and not in manifests (review R12).
- D8 Claims are unchanged in this change. Rejected: making `sync claim` read the tracker, because hooks have a one-second budget and must not call remote backends. The user accepted deferring this to `tracker-precedence-claims`.

## Risks

- A GitHub user editing the issue body by hand removes the `sdd-prec` marker → links and priority go with it, as with the existing `sdd` marker. `get` shows what remains.
- `start` lists open issues on every call → one paged `list_issues` per start, fails closed at the page cap, and `--force` is the escape.
- A plugin that ignores the new fields → `link` reports it (S8), and nothing is lost silently.

Deploy: ship `qrtx`, `qrtx-tracker-github` and `qrtx-tracker-jira` together. Older plugins ignore the new update fields and leave the `sdd-prec` slot alone (S10).

Rollback: use the previous binaries. The `sdd-prec` marker and the `sdd-precedence` property stay on issues unread, and the file backend's `links`/`priority` are already in the v1 schema.

## Observed

- `tracker/issue.go`: `IssueLink{Type, ID}`, `Issue.Links` and `Issue.Priority` exist (l.79–104, omitempty). `UpdateIssueInput` (l.257) has only `Title`, `Description`, `Labels` and `AddRefs`. The `SDDRef` role enum is `""`/`deferred` (l.51; `docs/schema/tracker/v1/issue.json:49`), and `sameLink` compares the role.
- `tracker_exec.go:212-225` and `providers/kit/provider.go:91` copy update fields one by one, so new fields need explicit copies.
- `tracker_exec.go:168,182-193`: `ListIssues` stops after 50 pages without an error.
- `providers/github/github.go`: the `sdd` body marker via `tracker.EncodeMarker`, rewritten whole on `AddRefs` (l.356-361); `toIssue` (l.146-150) maps neither links nor priority; list pages over `state=all` (l.205-219).
- `providers/jira/jira.go`: the `sdd` property `sddProp{Refs}` (l.105), rewritten whole by `setRefs` (l.278-279); `toIssue` (l.149-168) sets no priority.
- `providers/github/fakeapi/fakeapi.go:252-256` and `providers/jira/fakeapi/fakeapi.go:302` store bodies and properties for tests.
- `tracker_lifecycle.go:165-176`: `StartFromIssue` resolves `impl` with `t.For(backend)`, calls `GetIssue`, then `NewChange`.
- `cli/tracker.go:52-70`: `list` (filters at l.53-58) prints in backend order; `add` (l.101-115) has no priority; `close` writes directly (l.146), while `start` queues through the outbox, drained after the command (l.47-49).
- `propose_context.go:24,113-128,208`: overlaps come from shared `packages:`. `tracker_open.go:174-199` `proposeBacklog` reads the tracker under a 10s budget and soft-fails.
- `docs/tracker-protocol.md:59`: qrtx never calls an op the plugin does not declare.

## Assumptions

- Priority uses the schema's scale: 1 urgent … 4 low, with 0 or missing meaning none and sorted last.

## Non-goals

- Native GitHub issue dependencies, Jira issue links and Jira's native priority field.
- Changing claims.
- Persisting that a start was forced.
- Enforcing `relates` or `duplicates` (they are stored and shown, not enforced).

## Deferred

- tracker-native-links: mirror `blocks` and priority to GitHub issue dependencies and Jira links and priority, where the API allows.
- tracker-precedence-claims: say in `sync claim` and the inbox when a claimed issue is blocked, without remote calls from hooks.

## Tasks

### Phase links and a start that respects them
{: id="phase-01" status="accepted" file="phases/01-links-and-start.md"}

### Phase order between overlapping changes
{: id="phase-02" status="accepted" file="phases/02-propose-context-order.md"}

## Validate

- [ ] V1.1 `go test ./... -count=1` covers S1,S2,S3,S4,S5,S6,S7,S8,S9,S10

## Constitution

- C1 Rule 1 → this change is planned here with `qrtx change new` and `finish-propose` before any code. Its phases are applied and archived with the same tool, and the deferred work gets its own slugs.

## Review

- R1 [omitempty-vs-presence] `tracker/issue.go:83` → not a conflict because omitempty only drops an empty slice: S1 sees `links` after `link`, and the key's absence after `unlink` is the expected result.
- R2 [omitempty-vs-presence] `tracker/issue.go:104` → not a conflict because `issueFields` serialises the same way as `Issue`.
- R3 Q1,S2,S8 vs `tracker_exec.go:212`, `providers/kit/provider.go:91`, `toIssue` → Observed is corrected, and tasks 1.1 and 1.2 now copy the fields in exec and kit and map them in both `toIssue` functions.
- R4 Q6,Deploy,Rollback: older plugins rewrite the whole `sdd` marker or property → D1 moves links and priority to a separate slot (`sdd-prec` marker, `sdd-precedence` property), and S10 was added.
- R5 D3,D4,S3: listing all categories hits the 50-page cap and fails open → only open categories are listed, `start` fails closed at the cap (S9), and the check sits between `GetIssue` and `NewChange` (D4, task 1.4).
- R6 D4: a plugin without `list_issues` would always need `--force` → `start` skips the check with a warning (Q3, S8).
- R7 D3: B may be on a non-default backend and `IssueLink.ID` is bare → blockers are listed on `impl` and compared as normalized refs (D3).
- R8 Q4,S6: filtered lists miss blockers → a second open-issues listing is used when a filter is set (D6, S6).
- R9 S4: the `forced` role isn't in the schema and splits `sameLink` → it is no longer persisted, only reported (D4, Non-goals).
- R10 S8: an outbox-queued link fails its own re-read → `link`, `unlink` and `priority` call `UpdateIssue` directly (D5).
- R11 A3,S6: only the file backend has priority → the user chose `--priority` stored in qrtx metadata on every backend (Q2, S2, D1, task 1.1).
- R12 Task 2.1: links are not in manifests → D7 resolves them with `GetIssue` under the 10s budget, with a warning offline (S7).
- R13 D8 vs User request: claims were dropped from this change → the user accepted deferring them to `tracker-precedence-claims`.
