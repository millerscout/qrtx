```change
name: tracker-standard
status: complete
packages: []
```

## Why

sdd only tracks tasks inside a change (`## Tasks`, `phases/*.md`). Ideas, bugs and split-off work that are not a change yet have nowhere to live: `## Deferred` only names slugs, and nothing tracks them. Teams already keep that work in Jira, GitHub Issues or Linear. Others have no tracker at all. The user wants an sdd standard that any tracker can plug into, not a Markdown-only backlog.

Research (Nov 2026) found no adopted neutral standard:
- spec-kit and Kiro drive vendor MCP servers.
- Beads and git-bug compile each tracker into the binary.
- ai-sdlc's `IssueTracker@v1` is a TypeScript draft that loads adapters in-process.

The chosen approach:
- sdd defines a small, versioned data model and operation set (`sdd.adapter/v1`, kind `tracker`), with names aligned to ai-sdlc and with the Linear/omnitrack status categories.
- It ships a built-in `file` backend (`sdd/issues.jsonl`) as the zero-config default and reference implementation.
- It runs any other backend as an exec plugin (`sdd-tracker-<name>` on `PATH`).
- It wires the change lifecycle (start, archive, deferred) to the tracker through a crash-safe outbox.

The design input is the approved plan, kept outside the repo at `~/.claude/plans/the-sdd-has-tasklist-replicated-spark-of-673f3df8-0d8d-4244-8e52-3d09ffe54aeb.md`. Alternatives go in Decisions.

## User request

The user asked whether sdd has a task list or a way to keep a todo list like issues. When the answer was "only per change", they asked for a backlog. They then said:
- It must be an sdd standard that can plug in Jira, Confluence, GitHub Issues or any other application, rather than being tied to Markdown.
- With no external tracker, sdd must still have a default.
- Every item must carry which repo, project and change it belongs to.
- The design must keep the path open to generating Gherkin from scenarios, running e2e tests and screenshots from it, and feeding a product wiki.
- First-party providers should exist, and the design should be checked against what already exists online.

This change is the first of three. Providers and the leased sync daemon follow as their own changes (see Deferred).

## Acceptance

- In a repo with no tracker configured:
  - `sdd tracker add "Dark-mode toggle"` prints a new issue ID.
  - `sdd tracker list` shows the issue.
  - `sdd/issues.jsonl` holds one JSON line per issue in the documented schema.
- `sdd tracker start <id> dark-mode` creates the change:
  - The issue's title and description become its User request.
  - The issue ID appears under `issues:` in the change manifest.
  - The issue moves to `started` and records the repo, project and change it belongs to.
- When that change is archived, the issue moves to `completed`. If another active change still links the issue, it stays open and gets a comment instead (when the backend supports comments). Issues that `from-deferred` created for later work are not closed by the archive.
- A failed tracker call never stops an archive and never loses the update: `sdd tracker outbox` shows the pending write, and the next tracker command sends it.
- Two branches can each add issues, even with the same title, and merge without duplicate IDs. A 30-bit hash collision between branches is re-IDed by the merge driver instead of conflicting.
- An executable named `sdd-tracker-<name>` on `PATH` that speaks the documented protocol works as a backend once `.sdd.yaml` sets `tracker.backend: <name>`. `sdd tracker check --backend <name>` reports which operations pass.
- `docs/tracker-protocol.md` and the JSON Schemas under `docs/schema/tracker/v1/` are enough for someone to write a plugin without reading sdd's Go code.

## Requirements

- Q1 The `file` backend SHALL store each issue as one JSON line in `sdd/issues.jsonl`, sorted by ID with keys in sorted order, and SHALL rewrite only the line of the issue it changes.
- Q2 Issue IDs created by the `file` backend SHALL be derived from a per-create `client_ref` (repo, change, title and a random nonce), and SHALL stay unique when two branches add issues independently: when the merge driver finds one ID holding two different `client_ref`s, it SHALL keep both issues and give the incoming one an 8-character ID.
- Q3 Creating an issue SHALL be idempotent per `client_ref`: a retried create returns the issue the first attempt made, in the `file` backend and in any exec plugin that passes `tracker check`.
- Q4 The `sdd tracker merge-driver` SHALL merge two versions of `sdd/issues.jsonl` per issue ID, keep issues added on both sides, and exit non-zero with conflict markers only when both sides changed the same field of the same issue to different values, or one side deleted an issue the other edited.
- Q5 sdd SHALL run `sdd-tracker-<name>` found on `PATH` as a backend: it sends one `sdd.adapter/v1` JSON request on stdin per operation and reads one JSON response on stdout. It enforces a timeout, default 30s. It rejects a backend name that does not match `^[a-z0-9-]+$`.
- Q6 sdd SHALL call `capabilities` before any other operation on an exec backend. It SHALL refuse a plugin that shares no protocol version with it, and SHALL not call an operation the plugin does not declare.
- Q7 Every remote write from the lifecycle SHALL first be appended to the outbox under the shared state directory, then sent. A write SHALL leave the outbox only after the backend confirms it. This covers start, archive and from-deferred. Each entry SHALL record the repo root that queued it, and only commands run from that root SHALL drain it. Each `sdd tracker` command drains before and after its own work.
- Q8 `sdd change archive` SHALL complete the archive when a tracker call fails or times out. It SHALL print a warning naming the pending outbox entries. The next `sdd tracker` command SHALL retry them.
- Q9 Archiving a change SHALL transition a linked issue to `completed` only when no other active change links the same issue. Otherwise it SHALL leave the issue's status unchanged. A change links an issue only through its manifest `issues:` list. Unless `comments: off`, archive SHALL queue a comment, and the drain SHALL drop a queued comment the backend does not declare.
- Q10 `sdd tracker start <ref> <slug>` SHALL make the change link the issue in both directions: it creates the change if it does not exist. It fills `## User request` only while that section is still the template text. It adds the ref to the manifest's `issues:` list. It queues a transition to `started` and an update that adds an `SDDRef` (repo, project, change, branch) to the issue.
- Q11 `sdd tracker from-deferred --change <slug>` SHALL create one issue per bullet under `## Deferred`. It SHALL use a deterministic `client_ref` (change and bullet text) so that a re-run, including one made while earlier creates are still pending, creates no duplicate. It SHALL NOT add those issues to the manifest `issues:` list, and SHALL tag their ref with `role: deferred`.
- Q12 The `Issue` JSON written by sdd and accepted from plugins SHALL match `docs/schema/tracker/v1/issue.json`: the required fields are `id`, `title` and `status.category`, the optional fields include `client_ref` and `sdd` (refs with an optional `role`), where the category is one of backlog, unstarted, started, completed or canceled. Unknown top-level fields and `extra` SHALL survive a read-modify-write unchanged.
- Q13 Existing behaviour SHALL stay as it is when no tracker command runs, and when a change links no issue. This covers manifests without `issues:`, `.sdd.yaml` without `tracker:`, the stable `ArchiveChange(repoRoot, slug) error` signature, and `propose-context` JSON keys.
- Q14 The `.sdd.yaml` `tracker:` block SHALL accept `backend`, `config`, `status_map`, `mirror` and `comments`; an unknown `mirror` value other than `change` SHALL be rejected, only when a tracker backend is resolved (not by `LoadRepoConfig`), with a message that says only `change` is implemented. `comments: off` SHALL suppress archive comments.
- Q15 `sdd change propose-context` SHALL include up to 20 open issues as `backlog` when a tracker is reachable, and SHALL add a warning instead of failing when it is not.

## Scenarios

- S1 (Q1) WHEN `sdd tracker add` runs three times and then `sdd tracker close` runs on the second issue THEN `sdd/issues.jsonl` has three lines sorted by `id`, and only the closed issue's line differs from before the close.
- S2 (Q2) WHEN two git branches from the same base each run `sdd tracker add "Same title"` and are merged with the registered driver THEN the file holds two issues with different IDs and the merge exits 0. WHEN a fixture gives both branches the same 6-character ID with different `client_ref`s THEN the merge exits 0 and the incoming issue has an 8-character ID.
- S3 (Q4) WHEN branch A changes issue X's title and branch B changes issue X's labels THEN the merge driver exits 0 and the result has both changes. WHEN both branches change X's title to different values THEN it exits 1 and writes both lines between conflict markers.
- S4 (Q4) WHEN the driver is not registered in a clone THEN git's line merge of two branches that each added an issue still succeeds without conflict, because each new line sorts to a different position. The test writes hand-made fixture lines whose new IDs are separated by an unchanged base line. Adjacent IDs are a known residual case (Risks).
- S5 (Q3, Q7) WHEN `from-deferred` runs against the exec helper, the helper persists the issue, signals a file and blocks, and the test kills the sdd child process THEN the next `sdd tracker list` drains the entry, and exactly one issue with that `client_ref` exists.
- S6 (Q5, Q6) WHEN `.sdd.yaml` sets `tracker.backend: fake` and a test helper binary `sdd-tracker-fake` is on `PATH` THEN `sdd tracker list --json` prints the helper's issues. WHEN the helper declares only `sdd.adapter/v2` THEN the command exits 1 with a message naming both versions.
- S7 (Q5) WHEN the helper sleeps past the timeout, writes non-JSON, or exits 1 with an `{"error":{"code":"auth"}}` body THEN the command exits 1 with a message naming the backend, the operation and the error code, and no partial state is written to `sdd/issues.jsonl`.
- S8 (Q5) WHEN `tracker.backend` is `../evil` or `a/b` THEN every tracker command that resolves a backend exits 1 before running anything, naming the invalid backend. `merge-driver` and `outbox --drop` still work.
- S9 (Q6) WHEN the helper's capabilities omit `comment` and a change linked to one of its issues is archived THEN no `comment` request reaches the helper, and the archive succeeds.
- S10 (Q8) WHEN the helper fails every call and `sdd change archive demo` runs THEN the change moves to `sdd/changes/archive/<date>-demo/`, the output names the pending entries, and `sdd tracker outbox` lists them. WHEN the helper recovers and `sdd tracker list` runs THEN the outbox is empty and the issue is `completed`. WHEN the archive destination already exists THEN the archive fails, the outbox holds no entry from that attempt, and a retry queues each entry once.
- S11 (Q9) WHEN issue X is linked by active changes `a` and `b` and `a` is archived THEN X keeps its category, and a comment is queued (sent only if the backend declares `comment`). WHEN `b` is archived next THEN X becomes `completed`.
- S12 (Q10) WHEN `sdd tracker start <id> demo` runs and `demo` does not exist THEN `sdd/changes/demo/change.sdd.md` exists, its User request holds the issue title and description, its manifest lists `issues: [file:<id>]`, and `sdd tracker get <id> --json` shows category `started` and an `sdd` entry with `change: demo`.
- S13 (Q10) WHEN `start` runs on an existing change whose User request is already written THEN the User request is unchanged and only the link and transition are added. WHEN `start` names an issue that does not exist THEN it exits 1 and creates no change directory.
- S14 (Q11) WHEN `from-deferred --change demo` runs twice on a Deferred section with two bullets THEN exactly two issues exist, each with `sdd[].change` = `demo` and `role: deferred`, and the manifest's `issues:` list is unchanged. WHEN the second run happens while the first run's creates are still pending in the outbox THEN after the drain there are still exactly two.
- S15 (Q12) WHEN an issue line holds `"extra":{"jira":{"customfield_1":"x"}}` and an unknown top-level key `"rank":3`, and the issue is closed THEN both survive byte-for-byte in the rewritten line. WHEN the Go types marshal a full `Issue` THEN the golden test's checks against `issue.json` (required keys, category enum) pass.
- S16 (Q12) WHEN `sdd/issues.jsonl` contains a malformed line 4 THEN `sdd tracker list` prints the other issues, warns `sdd/issues.jsonl:4`, and exits 0. Any write fails with that same location instead of rewriting the file.
- S17 (Q13) WHEN the existing test suite runs, and an archive runs in a repo without `tracker:` and without `issues:` THEN all existing tests pass, the archive output is unchanged, and no shared state directory is created.
- S18 (Q14) WHEN `.sdd.yaml` sets `tracker.mirror: tasks` THEN tracker commands exit 1 saying only `change` is implemented, while `sdd change verify` still exits 0. WHEN it sets `comments: off` and a linked change with a comment-capable backend is archived THEN no comment is queued.
- S19 (Q15) WHEN the configured backend fails during `sdd change propose-context --json` THEN the JSON still has every existing key, `backlog` is absent, and `warnings` names the tracker failure. WHEN the `file` backend has 25 open issues THEN `backlog` lists 20.
- S20 (Q6, Q12) WHEN `sdd tracker check` runs against the `file` backend THEN it works on a temporary copy, reports pass for capabilities, create, get, update, transition and list, and leaves `sdd/issues.jsonl` unchanged. WHEN it runs against an exec backend without `--allow-writes` THEN it runs only `capabilities` and `list_issues`.

## Decisions

- D1 Use one generic envelope, `{"protocol":"sdd.adapter/v1","kind":"tracker","op":…,"config":…,"args":…}`, with binaries named `sdd-<kind>-<name>`. Rejected: a tracker-only protocol name, because the planned wiki adapter would then need a second envelope, discovery path and check harness.
- D2 Use exec plugins found on `PATH`, one process per call. Rejected: hashicorp/go-plugin, because it needs gRPC and protobuf for authors and adds dependencies; an MCP client inside sdd, because MCP fixes the transport but not the data model, and agents already have vendor MCP servers; providers compiled into `sdd`, because every new tracker would need an sdd release.
- D3 The status is `{name, category}`, with category one of backlog, unstarted, started, completed or canceled. Rejected: open/closed only, because it loses in-progress state; a fixed native list, because Jira workflows are per project.
- D4 The default backend is JSONL in `sdd/issues.jsonl`, chosen by the user on 2026-11-26. Rejected: a `backlog.md` checklist, because concurrent edits merge badly and it is not the schema; one file per issue, because it creates many small files.
- D5 The `file` backend's ID is `sdd-` plus 6 lowercase base32 characters of sha256(`client_ref`), lengthened to 8 when the 6-character form already exists with another `client_ref`. A cross-branch clash is resolved by the merge driver (Q2). Rejected: a max+1 counter (`B5`), because two branches would both take the next number.
- D6 Remote writes go through an outbox at `<git common dir>/sdd/outbox.jsonl` (from `git rev-parse --git-common-dir`, so all worktrees share it), or at `.sdd/outbox.jsonl` with a `.sdd/.gitignore` of `*` outside git. Each entry carries `repo_root`, and a drain sends only entries whose `repo_root` is the current `RepoRoot`, so other worktrees and sibling projects never apply each other's writes. The outbox is drained synchronously before and after the work of each `sdd tracker` command, and after archive. Rejected: calling the tracker directly from the lifecycle, because a crash between the local change and the remote call would lose the update.
- D7 Locking around the outbox and `sdd/issues.jsonl` writes: the lock files live in the state directory (D6), never under `sdd/`. On Unix it is `syscall.Flock` on a lock file. On Windows (build tag) it is an `O_CREATE|O_EXCL` lock file, retried for 10s, and taken over when its mtime is older than 60s. Rejected: adding `golang.org/x/sys` in this change, because a single-command lock does not need it; tracker-sync revisits this for the daemon.
- D8 `ArchiveChange` keeps its signature: it appends the outbox entries before the directory move, using the same local-disk step as `promoteBehaviour`. Each entry has a deterministic key (slug, issue, op), and an append skips a key already queued. When the move fails, the entries this call appended are removed. Archive queues a comment whenever `comments` is not `off`, without calling the backend; the drain checks `capabilities` and drops a comment the plugin does not declare. A failure to write the outbox aborts the archive before the move. The CLI drains the outbox after the move, and prints its warnings. Rejected: draining inside `ArchiveChange`, because the stable API cannot return warnings and a library call should not do network I/O.
- D9 `tracker check` against `file` uses a temporary copy. Against an exec backend, it is read-only unless `--allow-writes` is given. Rejected: always writing, because `check` against a real Jira project would create real issues.
- D10 The `file` backend does not declare `comment`. Archive history for file issues is the `sdd` refs list, plus the change's archived folder. Rejected: storing comments in `extra`, because `extra` is reserved for backend fields that pass through unchanged.
- D11 Merge driver behaviour: it ignores `updated_at` when comparing fields and keeps the newer value. When one ID holds two different `client_ref`s, it re-IDs the incoming issue to 8 characters. It is registered by `sdd tracker init`, which writes `.gitattributes` and runs `git config merge.sdd-issues.driver` with the absolute path from `os.Executable()`, so hosts that embed sdd under another `CLIName` work too. Rejected: registering it in `sdd skills install`, because installing skills should not change git config.
- D12 Credentials are never sent in the envelope; each plugin reads its own environment. Binding hosts to tokens (`~/.config/sdd/credentials.yaml`) belongs to the provider kit in `tracker-providers`. Rejected: building it here, because no first-party provider exists yet to enforce it.
- D13 The new Go API (`Issue`, `IssueTracker`, `SetIssueTracker`) and the protocol are listed as unstable in `doc.go` until v1. `SetIssueTracker` lets a host replace the configured backend. Rejected: adding them to the stable list now, because the providers change may still reshape the operations.

## Risks

- A plugin ignores `client_ref` and creates duplicates on retry → the protocol doc makes it a MUST, and `tracker check --allow-writes` runs the create twice and fails the plugin when two issues come back.
- Git's default line merge, in a clone without the driver, conflicts when two new IDs sort next to each other → the conflict is visible and resolvable by hand; `sdd tracker init` registers the driver, and the README tells teams to run it once per clone.
- The outbox grows when a backend stays down → `sdd tracker outbox` shows the size and age, and `--drop <seq>` removes an entry after the user decides.
- A slow backend slows `propose-context` → its `list_issues` call uses a 10s timeout and degrades to a warning (S19).
- Windows lock takeover after 60s could break a live but slow holder → every operation under the lock is a single file rewrite, well under a second, so a 60-second-old lock is dead.

Deploy: tag a minor release; nothing migrates. Old sdd versions ignore `issues:` in manifests and the `tracker:` key in `.sdd.yaml` (yaml is decoded non-strictly), so a repo can mix versions.

Rollback: remove `tracker:` from `.sdd.yaml` and the `issues:` lines; `sdd/issues.jsonl` and the outbox are inert files that older versions never read. Pin the previous tag.

## Observed

- `change.go` `ChangeManifest` has `name`, `status` and `packages`, decoded with `yaml.Unmarshal` (non-strict) in `ParseChangeMarkdown`. `NewChange` writes the template, including `## Deferred` "(Optional — other slugs that own split acceptance.)".
- `change.go` `ArchiveChange(repoRoot, slug string) error` checks verify, `status: complete` and all tasks done. It then calls `promoteBehaviour` (`living.go`) before `os.Rename` into `sdd/changes/archive/<date>-<slug>`.
- `change.go` `ListChanges` skips `archive` and dot directories, so "other active change" in Q9 means `ListChanges` minus the one being archived.
- `cli/cli.go` `Run` dispatches `change`, `skills` and `constitution` by `args[0]` before its switch; `runConstitution` is the pattern for a new `runTracker`. `change archive` calls `sdd.ArchiveChange`, then prints `archived sdd/changes/<slug>`.
- `repoconfig.go` `RepoConfig` has `validate_allow`, `verify_packages`, `constitution` and `quick_check`. `LoadRepoConfig` treats a missing file as empty.
- `providers.go` defines `CodeIndex` and `ContextProvider` with `SetCodeIndex` and `SetContextProvider`; `docs/embedding.md` documents them. `doc.go` lists which APIs are stable before v1.
- `propose_context.go` `ProposeContextReport` has `issues` (from `ContextProvider.OpenIssues`), `known_fixes` and `warnings`. `warnings` is always present, so a tracker failure can be reported there.
- `roadmap.go` `replaceSection(text, heading, body)` and `change_validation.go` `sectionMarkdown(md, heading)` read and write sections; `parseRoadmap` reads `### Phase` lines with `{: id status file}`.
- `verify.go` `RepoRoot` walks up to `.sdd.yaml`, `go.mod` or `.git`. Nothing in the non-test code runs `git` today (`grep -rn 'exec.Command("git"'` finds nothing), so D6's `git rev-parse` is new.
- `skills/skills.go` embeds `sdd-*/SKILL.md` and installs them to `.claude/skills`. It writes nothing else.
- `go.mod` requires only `smacker/go-tree-sitter` and `gopkg.in/yaml.v3`.
- `.gitignore` has `/sdd` and `!/sdd/`, so `sdd/issues.jsonl` is tracked.
- `sdd constitution show` lists one rule, `Rule 1: sdd plans with sdd`.
- `sdd change propose-context --change tracker-standard --json` gave score `simple` and no overlaps, warnings or code hits (run 2026-11-26).
- Prior art read online on 2026-11-26:
  - ai-sdlc.io `IssueTracker@v1` operations: listIssues, getIssue, createIssue, updateIssue, transitionIssue, watchIssues; a TypeScript draft.
  - omnitrack and human's `internal/tracker` both use a status name plus a category.
  - The spec-kit Jira and Linear extensions are MCP-based and one-way.
  - Beads' `bd github sync` is compiled into the binary.

## Assumptions

- The approved design (data model, envelope, file backend format, outbox, archive rule) is the contract for this change. Providers and the daemon do not change it.
- Agents call `sdd tracker …` through the shell, like other sdd commands. No MCP server is needed for this change.
- `git` is on `PATH` wherever sdd runs inside a git repo. Without it, D6's `.sdd/` fallback applies.

## Non-goals

- First-party GitHub, GitLab, Jira and Linear providers, the provider kit and host-bound credentials.
- The leased sync daemon, heartbeats, claims, inbox and watch, and Claude Code hooks.
- `mirror: phases|tasks` (the key is accepted and validated only), webhooks, and pulling remote changes in the background.
- Gherkin export, test evidence and the wiki adapter. The tag convention (`@sdd:<slug>/S<n>`, `@req:Q<n>`, `@issue:<backend>:<id>`, `@project:<p>`) is only written down in `docs/tracker-protocol.md`.
- Confluence, which belongs to the planned `wiki@v1` kind.

## Deferred

- `tracker-providers`: the `providers/kit`, `sdd-tracker-github` and `sdd-tracker-jira` first, then gitlab and linear, plus `~/.config/sdd/credentials.yaml` host binding.
- `tracker-sync`: the leased daemon (1h lease, heartbeat extends it), local claims, inbox and watch, and opt-in hooks.
- `gherkin-export`, `test-evidence` and `wiki-adapter`, in that order, after tracker-sync.

## Tasks

### Phase data model and protocol
{: id="phase-01" status="accepted" file="phases/01-data-model-and-protocol.md"}

### Phase file backend and tracker CLI
{: id="phase-02" status="accepted" file="phases/02-file-backend-and-cli.md"}

### Phase exec plugin backends
{: id="phase-03" status="accepted" file="phases/03-exec-plugins.md"}

### Phase lifecycle wiring through the outbox
{: id="phase-04" status="accepted" file="phases/04-lifecycle-outbox.md"}

## Validate

- [x] V1.1 `go test ./... -count=1 -run 'TestIssue|TestTrackerLock|TestTrackerFile|TestTrackerMerge|TestTrackerSchema|TestAdapterEnvelope|TestTrackerCLI'` covers S1,S2,S3,S4,S15,S16
- [x] V1.2 `go test ./... -count=1 -run 'TestTrackerExec|TestTrackerCheck'` covers S6,S7,S8,S9,S20
- [x] V1.3 `go test ./... -count=1 -run 'TestTrackerOutbox|TestTrackerStart|TestTrackerArchive|TestTrackerDeferred|TestTrackerConfig|TestProposeContextBacklog'` covers S5,S10,S11,S12,S13,S14,S18,S19
- [x] V1.4 `go test ./... -count=1` covers S17

## Constitution

- C1 Rule 1 → this change is planned here with `sdd change new` and `propose-context`, and passes `finish-propose` before any code. It is applied with `/sdd-apply` and archived with `/sdd-archive`. The providers and sync work are named in Deferred as their own slugs in this repository, not as tasks elsewhere.

## Review

- R1 D8, Q6, Q8, S9, S10: knowing whether the backend declares `comment` needs `capabilities`, which is process I/O inside `ArchiveChange` → D8: archive queues comments unless `comments: off`, and the drain drops undeclared ones; Q9 and S11 reworded.
- R2 D8, D6: entries appended before `os.Rename` survive a failed move, and a retry duplicates them → D8: deterministic entry keys, skip already-queued keys, remove this call's entries when the move fails; S10 covers it.
- R3 D6, Q7, `verify.go` `RepoRoot`: the git common dir is shared by worktrees and sibling projects, so one root could drain another's writes into its own `sdd/issues.jsonl` → D6 and Q7: entries carry `repo_root`, and only matching roots drain them.
- R4 Q9, Q11, S14: archive would complete issues that `from-deferred` linked to the same change → Q9: links come only from the manifest `issues:` list; Q11 and S14: deferred issues are not added there and carry `role: deferred`; Acceptance updated.
- R5 Q11, S14, Q2: a second `from-deferred` run while the first run's creates are still pending would duplicate them → Q11: deterministic `client_ref` from change and bullet text; S14 covers the pending case.
- R6 Q2, Acceptance, D5: a 6-character hash clash across branches is invisible locally → Q2, D5 and D11: the merge driver re-IDs the incoming issue to 8 characters; S2 covers it with a fixture.
- R7 S4, D5: random nonces make sort positions uncontrollable in a test → S4 uses hand-made fixture lines separated by a base line.
- R8 tasks 2.3 and 2.4, D11: the root package cannot reach the CLI's `merge-driver`, and `init` must not hardcode `sdd` → D11 registers the absolute path from `os.Executable()`; task 2.3 builds `cmd/sdd` into a temp dir and lives in `./cli/`.
- R9 S5, task 4.1: with the in-process `file` backend there is no gap to kill in → S5 uses `from-deferred` against the exec helper, which persists the issue, signals a file and blocks before the kill.
- R10 Q14, S18, S8, task 1.4: validating `mirror` in `LoadRepoConfig` would break `verify` and `archive`, and S8 would break `merge-driver` → Q14: validation happens only when a backend is resolved; S8 and S18 exempt `merge-driver`, `outbox --drop` and non-tracker commands.
- R11 D7, Q1: a lock file next to `sdd/issues.jsonl` would show up as untracked → D7: lock files live in the state directory.
- R12 D6 vs task 4.1: "drain at the end" and "drain first" contradicted each other → D6: drain before and after each tracker command, and after archive.
- R13 S1: hash IDs do not keep insertion order → reworded to "the closed issue's line".
- R14 tasks 1.2 and 1.3: 1.2 checked against schemas that 1.3 writes → the envelope schema check moves into 1.3's verify.
- R15 Q3, Q12, D10: `client_ref` was missing from the schema → Q12 lists `client_ref` and `sdd` (with `role`) as optional fields.
- R16 hint [omitempty-vs-presence] S14 vs `propose_context.go:58` → not a conflict because S14 reads tracker issues through `sdd tracker`, not `ProposeContextReport.Issues`; the new `backlog` key is `omitempty`, and S19 relies on that.
- R17 C1 vs Rule 1 → not a conflict: C1 covers propose with `finish-propose`, apply, archive, and naming the Deferred slugs in this repository.
