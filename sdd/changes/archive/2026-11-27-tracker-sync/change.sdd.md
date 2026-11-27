```change
name: tracker-sync
status: complete
packages: []
```

## Why

Several agent sessions often work in one repository at once: different Claude Code sessions, worktrees and terminals. Today none of them knows what the others are doing:
- two sessions can pick up the same issue or the same task;
- a task checked off in one worktree is invisible to the others until a merge;
- remote tracker changes are seen only when someone runs `sdd tracker list`;
- pending tracker writes wait in the outbox until the next `sdd tracker` command.

The user asked for a sync session with these properties:
- it lasts an hour;
- each heartbeat while working extends it by another hour;
- it closes when idle and reopens on the next use;
- local sessions get updates on the same tasks.

The chosen approach:
- **Shared files:** session leases, claims and an append-only event log live in the shared state directory, which all worktrees use (`tracker_state.go`). Every command reads and writes them under the existing lock, so they work with no daemon.
- **Optional daemon:** a small daemon, started on demand, adds what needs a timer: lease expiry, outbox draining, a watch on task checkboxes in every worktree, and polling the remote tracker. It exits when no lease is live.
- **Claude Code hooks (opt-in):** they send heartbeats while the agent works and show the inbox on each prompt.

The design input is `~/.claude/plans/the-sdd-has-tasklist-replicated-spark-of-673f3df8-0d8d-4244-8e52-3d09ffe54aeb.md` §8, as refined here. Alternatives go in Decisions.

## User request

While designing the backlog standard, the user asked:
- for a sync session that lasts one hour, with each heartbeat while working adding another hour;
- for the session to close when idle and reopen afterwards, to help sync tasks;
- for different local sessions to get updates on the same tasks being worked on.

`tracker-standard` and `tracker-providers` deferred this work to this slug. The user then asked to propose `tracker-sync`.

## Acceptance

- **Heartbeat and lease:** while a Claude Code session works in a repo (with `sdd skills install --hooks`), its heartbeat keeps a one-hour lease alive. `sdd sync status` shows the session, its lease end, and a running sync daemon.
- **Closing and reopening:** when every session stops for an hour (or `sync.lease`), their claims are released, pending tracker writes are flushed, and the daemon exits. The next heartbeat or claim starts it again.
- **Claims:** sessions use the same id in hooks and in the agent's own commands, because a `SessionStart` hook exports `SDD_SESSION`. When session A claims an issue or a task, session B's attempt to claim it fails and names A. B's next prompt shows the claim in its inbox.
- **Shared progress:** a task checked off in worktree A appears in session B's inbox within seconds, as change, task, worktree and branch, without any commit or merge.
- **Remote changes:** a change made in the remote tracker (GitHub or Jira) appears in the inbox within the poll interval while a session is live.
- **One daemon for several roots:** the daemon serves every repo root (worktree or monorepo subproject) that shares the git directory. It drains each root's outbox, polls each root's tracker, and keeps claims and events apart per root.
- **No daemon:** everything except the timer-driven parts works with no daemon (CI, `--no-daemon`, `SDD_NO_DAEMON=1`). An expired lease is still treated as expired.

## Requirements

- Q1 A session SHALL be identified by `--session`, else (with `--hook`) the hook's stdin `session_id`, else `SDD_SESSION`, else `<worktree>:<parent pid>`. `sdd sync begin --hook` (the `SessionStart` hook) SHALL append `export SDD_SESSION=<session_id>` to `$CLAUDE_ENV_FILE` when that variable is set, so the agent's Bash commands carry the same id. `sdd sync heartbeat` SHALL set that session's `lease_until` to now plus `sync.lease` (default 1h) and record its worktree and branch.
- Q2 `sdd sync heartbeat` SHALL be cheap when repeated. A heartbeat within `min(60s, lease/4)` of the session's previous one SHALL only compare a marker file's mtime and exit 0, without taking the lock or starting anything. Hook commands SHALL wait at most 1s for the sync lock, then warn and exit 0. A heartbeat SHALL never fail the caller: on any error it prints to stderr and exits 0.
- Q3 Every sync read SHALL return empty results without creating the state directory when `state.json` does not exist. Otherwise it SHALL first expire sessions whose `lease_until` has passed, release their claims, and append one `claim_expired` event per released claim, whether or not a daemon runs.
- Q4 `sdd sync claim <key>` SHALL record the claim for the session when the key is free, or already held by that session. It SHALL fail with exit 1 naming the holder's session, worktree and branch when another live session holds it. `--release` SHALL drop the caller's claim. A key is an issue ref (`github:123`) or `<slug>/<task id>`, stored per repo root. A claim SHALL also set the session's lease, as a heartbeat does. When sync has never run in the repo (no `state.json`), `claim` SHALL print `sync not enabled; claim skipped` and exit 0 without creating anything, so skills can call it unconditionally.
- Q5 Every claim, release, expiry, task change and remote change SHALL be appended to `events.jsonl` in the shared state directory, with a sequence number, a time, the session, and repo/project/change/task context.
- Q6 `sdd sync inbox` SHALL print the events since this session's cursor that other sessions or the daemon caused, and then advance the cursor. A session's cursor SHALL be set to the log's end at its first heartbeat, claim or `begin`, so later events are shown even if its first inbox comes after them. `--brief` SHALL print at most 10 one-line summaries, plus a count of the rest.
- Q7 The sync daemon SHALL run as one process per shared state directory, guarded by a non-blocking lock it holds for its whole life. The lock file SHALL hold its owner's pid and a nonce, so a process never refreshes or removes a lock it does not own. It SHALL decide to exit and release `daemon.lock` while holding the sync lock. A heartbeat SHALL check for a running daemon under that same lock and start one when none runs. It SHALL be started detached by `sdd sync heartbeat` and `sdd sync claim` when not running, except in CI (`CI` set), with `--no-daemon`, or with `SDD_NO_DAEMON=1`.
- Q8 While at least one lease is live, the daemon SHALL expire leases every `sync.tick` (default 5s). In separate goroutines that never block expiry, it SHALL drain the outbox of every repo root recorded by a live session, and poll each root's tracker every `sync.interval` (default 2m, only for a non-`file` backend). The sync lock SHALL never be held across git, network or plugin calls. When no lease is live, it SHALL drain the outbox once more, log why, and exit 0.
- Q9 The daemon SHALL watch `sdd/changes/*/change.sdd.md` and `sdd/changes/*/phases/*.md` in every worktree of the repository by polling mtimes. It SHALL append a `task_progress` event when a task's checkbox changes. The persisted snapshot in `state.json` is the baseline, so changes made while no daemon ran are reported once at the next start. A new worktree, a new change, or a file seen for the first time SHALL be snapshotted silently, and a file that disappears (for example after archive) SHALL be dropped silently.
- Q10 Remote polling SHALL compare each listed issue's `updated_at` with the last seen value. It SHALL append `remote_change` for a changed or new issue, but not on the first poll.
- Q11 `sdd sync status [--json]` SHALL show the daemon (pid and start time, or not running), live and expired sessions with their lease ends, claims, outbox size and the last remote poll. `sdd sync stop` SHALL stop a running daemon, which drains the outbox before exiting. It SHALL write the stop request only while a daemon runs, and a starting daemon SHALL delete any stale stop request.
- Q12 `sdd skills install --hooks` SHALL merge Claude Code hooks into `.claude/settings.local.json` (`--shared` writes `.claude/settings.json` instead), keeping existing hooks and settings. The hooks are a heartbeat on `UserPromptSubmit`, an `async` heartbeat on `PostToolUse` (matcher `*`, so it never delays a tool), the brief inbox on `UserPromptSubmit` (whose stdout Claude Code adds to the prompt's context), `sdd sync begin --hook` on `SessionStart` (exporting `SDD_SESSION`, see Q1), and `sdd sync end --hook` on `SessionEnd` (which ends the session at once and releases its claims). A re-run SHALL change nothing.
- Q15 `sdd sync end` SHALL end the caller's session: it drops the lease, releases its claims, deletes the session's heartbeat marker, and appends `session_end`. A crashed or killed session is still covered by lease expiry (Q3).
- Q13 The event log SHALL be trimmed under the sync lock with an atomic rename. `sync watch` SHALL track the last seen seq, not a byte offset. The log SHALL be trimmed to the newest 5000 events (or 7 days) when the daemon starts, and a cursor that points before the trimmed start SHALL continue from the oldest kept event.
- Q14 Nothing in this change SHALL alter behaviour for users who never run `sdd sync`, install hooks or call `claim`. Existing tracker commands and the outbox keep working, and no state directory is created by other commands.

## Scenarios

- S1 (Q1, Q2) WHEN `sdd sync heartbeat --session A` runs twice within 60s THEN the first run writes `state.json` with A's `lease_until` about 1h ahead, and the second run takes no lock and leaves `state.json` byte-identical. WHEN it runs with `--hook` and stdin `{"session_id":"abc","cwd":"…"}` THEN the session is `abc`.
- S2 (Q2) WHEN the state directory cannot be written during a heartbeat THEN the command prints a warning to stderr and exits 0.
- S3 (Q3) WHEN session A holds claim `github:7` and its lease ended (injected clock) and session B runs `sdd sync inbox` with no daemon THEN the output includes `claim_expired github:7 (A)`, and B can claim `github:7`.
- S4 (Q4) WHEN A claims `demo/2.1` and B claims `demo/2.1` THEN B's command exits 1 naming A's session, worktree and branch. WHEN A claims it again THEN it exits 0 with no new event. WHEN A runs `--release` THEN B's claim succeeds.
- S5 (Q5, Q6) WHEN A claims `github:7` and B runs `sdd sync inbox` THEN B sees `A claimed github:7`. A second `inbox` from B prints nothing new. A's own inbox never shows A's claim.
- S6 (Q6) WHEN a new session C runs its first inbox after 50 earlier events THEN it prints nothing, and only later events appear. WHEN 25 events arrive and `--brief` runs THEN it prints 10 lines and `…and 15 more (sdd sync inbox)`.
- S7 (Q7) WHEN two `sdd sync heartbeat` commands start at once with no daemon running THEN exactly one daemon process holds the daemon lock afterwards. WHEN `CI=true` THEN no daemon starts, and the heartbeat still updates the lease.
- S8 (Q8) WHEN the only lease expires (test lease 2s, tick 200ms) THEN the daemon releases the claims, drains the outbox, writes `exiting: no live sessions` to `sync.log`, and its process exits. WHEN a heartbeat runs afterwards from a new session THEN a new daemon starts.
- S9 (Q8) WHEN the outbox holds a write for a recovered fake plugin and a lease is live THEN the daemon sends it within two ticks, with no `sdd tracker` command run.
- S10 (Q9) WHEN two worktrees of one repo have the change `demo` and task 1.1 is checked in worktree A THEN within two ticks the event log has `task_progress` with change `demo`, task `1.1`, done true and worktree A's path and branch. Session B's inbox shows it. The daemon's first snapshot produced no events.
- S11 (Q10) WHEN the fake plugin's issue F-1 changes `updated_at` between polls THEN one `remote_change F-1` event appears. The first poll and an unchanged poll add none.
- S12 (Q11) WHEN `sdd sync status --json` runs with a live daemon, two sessions (one expired) and a claim THEN the JSON lists `daemon.pid`, both sessions with `live` true/false, the claim, `outbox` and `last_poll`. WHEN `sdd sync stop` runs THEN the process exits within 5s after a final drain, and status says not running.
- S13 (Q12) WHEN `sdd skills install --hooks` runs where `.claude/settings.local.json` already has a `PreToolUse` hook and `permissions` THEN both are kept, the sdd hooks are added once, and a second run leaves the file byte-identical. `--shared` writes `.claude/settings.json` instead.
- S14 (Q13) WHEN the daemon starts with 6000 events THEN `events.jsonl` keeps the newest 5000, and a session whose cursor pointed at event 10 continues from the oldest kept event without error.
- S15 (Q14) WHEN the existing test suite runs, and `sdd tracker list` and `sdd change archive` run in a repo where no one ran `sdd sync` THEN all tests pass, and no `state.json`, `events.jsonl` or daemon appears.
- S17 (Q15) WHEN A holds `demo/2.1` and runs `sdd sync end --hook` with stdin `{"session_id":"A","hook_event_name":"SessionEnd"}` THEN A disappears from `sync status`, B's claim of `demo/2.1` succeeds, and B's inbox shows `session_end A`.
- S18 (Q1) WHEN `sdd sync begin --hook` runs with stdin `{"session_id":"abc"}` and `CLAUDE_ENV_FILE` pointing at a temp file THEN the file ends with `export SDD_SESSION=abc`, and a later `sdd sync claim demo/1.1` run with that environment is recorded for session `abc`.
- S19 (Q4, Q14) WHEN `sdd sync claim demo/1.1` runs in a repo where sync never ran THEN it prints `sync not enabled; claim skipped`, exits 0, and no state directory, `state.json` or daemon appears.
- S20 (Q7, Q8) WHEN sessions in two monorepo subprojects (each with its own `.sdd.yaml` tracker) share one git directory THEN one daemon drains both roots' outboxes and polls both trackers, and the same `demo/1.1` claim key in each root does not conflict.
- S21 (Q9) WHEN task 1.1 is checked while no daemon runs and a daemon then starts THEN exactly one `task_progress` event appears. WHEN `sdd change archive demo` moves the change THEN no `done=false` events appear.
- S22 (Q2, Q7) WHEN the daemon decides to exit at the same moment a heartbeat from a new session runs THEN afterwards either the daemon is still running or a new one has started (repeated 20 times in the test).
- S23 (Q4) WHEN session A in the main checkout claims `demo/1.1` and session B in another worktree of the same project claims `demo/1.1` THEN B's claim fails naming A. The same key in a subproject (`svc/`) succeeds.
- S16 (Q1, Q3, Q8) WHEN a session's process is killed without a heartbeat for longer than its lease THEN its claims are released by the next read or by the daemon tick, whichever comes first. No claim outlives its lease by more than one tick while a daemon runs, because expiry never waits on a drain or a poll.

## Decisions

- D1 **No socket.** Clients and the daemon share files in the state directory under the existing lock: `state.json` (sessions, claims, cursors, the remote cache and the watch snapshot), `events.jsonl`, and `sync.log` (rotated at 1 MB). `sdd sync watch` tails `events.jsonl`. Rejected: a Unix socket, because every command already works on files without a daemon, and a socket adds a second protocol and Windows AF_UNIX quirks.
- D2 Daemon identity is a non-blocking lock on `daemon.lock` (`syscall.Flock` with `LOCK_NB` on Unix; on Windows an `O_EXCL` file whose mtime the daemon refreshes every 30s, and which the existing 60s takeover treats as stale), plus `daemon.json` holding the pid and start time for status. Rejected: a pid file alone, because pids are reused and a crash leaves a stale file.
- D3 The daemon is spawned as `os.Executable() sync serve`, detached (`Setsid` on Unix; `DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP` on Windows), with stdout and stderr appended to `sync.log`. Rejected: running in the foreground of the hook, because hooks must return immediately.
- D4 Lease defaults: `sync.lease` 1h, `sync.tick` 5s, `sync.interval` 2m, set in `.sdd.yaml` `sync:`. The heartbeat debounce is `min(60s, lease/4)`, using a per-session marker file's mtime. Rejected: a heartbeat on every tool call without debounce, because `PostToolUse` fires on every tool call.
- D5 The local watch polls mtimes every tick across `git worktree list --porcelain`. A task's id is its leading token (`1.1`, `V1.2`, `2.3a`), and when a line has none, the hash of its text. Rejected: fsnotify, because it adds a dependency (go.mod today has only tree-sitter and yaml).
- D6 Remote polling lists up to 200 issues of the configured backend through `OpenTracker`, compares `updated_at` per issue id, and stores the last value in `state.json`. Rejected: webhooks, because they need a public endpoint.
- D7 Claims are advisory and local to the machine. Other machines see work as taken through the remote assignee and the `started` category, which `tracker start` already sets. Rejected: writing claims to the remote tracker, because each provider would need a new op.
- D8 A session's first inbox starts at the log's end. The cursor starts at the session's first heartbeat, claim or begin. Rejected: replaying history, because a new agent would get a wall of stale events in its first prompt.
- D12 One daemon serves every repo root that shares the state directory. Sessions record their root, and the daemon drains, polls and watches per live root. Claims and the remote cache are keyed by the root's project scope, which is its path inside the git worktree (`.` or `services/api`). Every worktree of a project therefore shares claims, while subprojects stay apart. Watch entries are keyed by absolute file path. Amended 2026-11-27 after V1.6: keying by `rootKey(root)` (the absolute path) made two worktrees of one project never conflict. Rejected: one daemon per root, because roots share the lock files and the event log.
- D13 The Claude session id reaches the agent's Bash commands through the `SessionStart` hook writing `export SDD_SESSION=…` to `$CLAUDE_ENV_FILE`, the documented way for `SessionStart` hooks to persist env vars. Rejected: parsing transcripts or `ppid`, because neither identifies the session. The manual check V1.6 confirms the variable reaches Bash on the real client.
- D14 Windows lock files carry `pid nonce`. Refresh and remove check that nonce, and the outbox drain re-takes its lock per entry so no single hold exceeds the 60s stale rule. Rejected: unconditional remove, because a stolen lock could then be deleted by its old holder.
- D11 Hook commands are `"$CLAUDE_PROJECT_DIR"`-independent. They call `sdd` on `PATH`, falling back to the absolute path from `os.Executable()` at install time, and read the session from stdin. `PostToolUse` is `async`, and `SessionEnd` only ends the session (well inside its 1.5s budget). Rejected: relying on `SessionEnd` alone, because a crash skips it, so lease expiry stays the safety net.
- D9 Hooks are opt-in and default to `settings.local.json`. Rejected: writing them into `.claude/settings.json` by default, because that is shared config every teammate gets.
- D10 `heartbeat`, `end` and `inbox --hook` exit 0 on internal errors, so agent prompts are never blocked; `claim` and `status` exit non-zero on errors, because callers act on the result. Rejected: failing hooks on errors, because a `UserPromptSubmit` hook that exits 2 blocks the user's prompt.

## Risks

- A detached daemon left running by a bug → it exits when no lease is live, `sync stop` and `sync status` exist, and a test kills a daemon and checks that the next heartbeat replaces it.
- A clock change makes leases look expired → leases are wall-clock with a 1h default, so a jump releases claims early at worst, and those are advisory.
- `events.jsonl` grows → the daemon trims it at start (Q13).
- A slow remote poll blocks expiry → polling runs in its own goroutine with the plugin timeout, and the tick loop never waits on it.
- Hook output floods the prompt → `--brief` caps it at 10 lines (Q6).

Deploy: tag a minor release. Old sdd binaries ignore `state.json`, `events.jsonl` and the `sync:` key. A team adopts sync by running `sdd skills install --hooks` per clone.

Rollback: remove the hooks from `.claude/settings.local.json`, run `sdd sync stop`, and delete `<git common dir>/sdd/{state.json,events.jsonl,daemon.*,sync.log}`. The outbox and tracker data are untouched.

## Observed

- `tracker_state.go`: `trackerStateDir` returns `<git common dir>/sdd`, shared by every worktree, or `.sdd/` outside git. `ensureTrackerStateDir` creates it lazily. `withStateLock(root, name, fn)` takes `<dir>/<name>.lock` through `lockFile`.
- `lock_unix.go` `lockFile` uses a blocking `syscall.Flock(LOCK_EX)`. `lock_windows.go` uses `O_EXCL` with a 10s retry and a 60s stale takeover. Neither has a non-blocking variant yet.
- `tracker_outbox.go` `DrainOutbox(root, t)` sends this root's entries under the `outbox` lock and does nothing when no outbox file exists. `cli/tracker.go` drains before and after each tracker command.
- `tracker_open.go` `OpenTracker` resolves the configured backend, and `ListIssues` takes an `IssueFilter` with `Limit`.
- V1.6, run 2026-11-27 with worktree `mimile/sdd-wt2` (branch `sync-check`) and `sync.lease: 2m`:
  - **Sessions:** A was the session "Set up sdd-product repo and backlog", running in `mimile/sdd`. B was run from this session in `sdd-wt2`. Both used explicit `--session` ids, because neither Claude session's folder had the hooks.
  - **Daemon:** `begin` started it (pid 62400), and A's 12 heartbeats every 31s kept A's lease alive.
  - **Bug:** B's claim of `sync-check/1.1` succeeded while A held it, because claims were keyed by absolute root path. Fixed with `syncScope` (D12, S23). After reinstalling, B's claim failed with "claimed by session A (worktree …/mimile/sdd, branch tracker-sync)".
  - **Task watch:** checking task 1.1 in `sdd-wt2` put `sync-check/1.1 done in sdd-wt2 (sync-check)` in A's inbox within one 5s tick.
  - **Not covered:** hook auto-registration and the `SDD_SESSION` export through `$CLAUDE_ENV_FILE`, because no Claude session ran in a folder with the hooks. The hook command itself was run with Claude's hook JSON on stdin and wrote `export SDD_SESSION=probe-1`.
- `tracker_state.go` `withStateLock` always calls `ensureTrackerStateDir` (MkdirAll), so read-only sync paths must check for `state.json` before locking. `lock_windows.go` removes the lock file unconditionally on unlock.
- `verify.go` `RepoRoot` stops at the nearest `.sdd.yaml`, `go.mod` or `.git`, so monorepo subprojects and worktrees are distinct roots that share one git common dir. `DrainOutbox` sends only entries whose `repo_root` matches.
- `cli/setup.go` `runInit` installs skills with `skills.InstallKeep`, so every user gets the updated `sdd-apply` skill, which is why `claim` must be a no-op where sync never ran.
- `tasksmd/tasksmd.go` `ParseTasksMD` returns `Items` with `Text` and `Done` per checkbox line. `Key` is unused today.
- `change.go` `ChangeProgress` parses `phases/*.md` when they exist, else the manifest.
- Claude Code hooks reference (code.claude.com/docs/en/hooks, read 2026-11-26 through the docs agent):
  - **Settings shape:** `{"hooks":{"<Event>":[{"matcher":"*","hooks":[{"type":"command","command":"…"}]}]}}`. `UserPromptSubmit` ignores matchers.
  - **Stdin:** the JSON has `session_id`, `cwd`, `hook_event_name` and `transcript_path`. No session env var is documented, so the session comes only from stdin.
  - **`UserPromptSubmit`:** stdout with exit 0 is added to Claude's context, capped at 10,000 characters. Its timeout is 30s.
  - **`PostToolUse`:** it cannot block. `"async": true` runs a command in the background.
  - **`SessionEnd`:** it exists, for side effects only, with a short 1.5s default budget. A crash skips it.
  - **Environment:** `CLAUDE_PROJECT_DIR` is exported to hooks.
- `skills/skills.go` `Install` and `InstallKeep` write only `.claude/skills/*/SKILL.md`. No hooks code exists.
- `repoconfig.go` `RepoConfig` has no `sync` key.
- `cli/cli.go` `Run` dispatches `change`, `skills`, `constitution`, `tracker` and `init`. There is no `sync` command.
- `sdd change propose-context --change tracker-sync --json` gives score `simple` and no overlaps (run 2026-11-26).

## Assumptions

- Agents other than Claude Code (Codex, Cursor, a person in a terminal) call `sdd sync heartbeat --session <id>` themselves, or set `SDD_SESSION`. Only Claude Code gets hooks in this change.
- One machine runs at most a few dozen sessions per repository, so `state.json` stays small and a whole-file rewrite under the lock is fine.

## Non-goals

- Sharing claims or events across machines.
- Remote conflict detection for pending outbox writes (remote changes are reported, not merged).
- Hooks for agents other than Claude Code.
- A UI.

## Deferred

- `tracker-sync-hooks-live-check`: run V1.7 in a Claude Code session whose folder has the hooks, to confirm `SDD_SESSION` reaches the agent's Bash through `$CLAUDE_ENV_FILE` and that the inbox appears in the prompt's context.
- `tracker-credentials-multi-account`, `tracker-jira-live-check`, `tracker-providers-more` and `tracker-credentials-federated`, as recorded in the archived `tracker-providers`.
- `gherkin-export`, `test-evidence` and `wiki-adapter`.

## Tasks

### Phase sessions, claims and events
{: id="phase-01" status="accepted" file="phases/01-sessions-claims-events.md"}

### Phase sync daemon
{: id="phase-02" status="accepted" file="phases/02-daemon.md"}

### Phase local watch and remote polling
{: id="phase-03" status="accepted" file="phases/03-watch-and-poll.md"}

### Phase Claude Code hooks, skills and docs
{: id="phase-04" status="accepted" file="phases/04-hooks-and-docs.md"}

## Validate

- [x] V1.1 `go test ./... -count=1 -run 'TestSync'` covers S1,S2,S3,S4,S5,S6,S16,S17,S18,S19,S23
- [x] V1.2 `go test ./... -count=1 -run 'TestDaemon'` covers S7,S8,S9,S12,S14,S20,S22
- [x] V1.3 `go test ./... -count=1 -run 'TestWatch|TestPoll'` covers S10,S11,S21
- [x] V1.4 `go test ./... -count=1 -run 'TestHooks'` covers S13
- [x] V1.5 `go test ./... -count=1` covers S15
- [x] V1.6 manual: two separate sessions in two worktrees with `sync.lease: 2m` (2026-11-27, see Observed). Results: a cross-worktree claim conflict names the holder (after the `syncScope` fix); task progress reached the other session's inbox within one tick; at 02:53:02 both leases expired, two `claim_expired` events were written and the daemon logged `exiting: no live sessions`; a later heartbeat started a new daemon. covers S3,S10
- [ ] V1.7 manual: in a Claude Code session whose folder has the hooks (`sdd skills install --hooks`), `echo $SDD_SESSION` prints the hook's session id, and a prompt's context shows the other session's events. Not run: the helper sessions ran in `mimile`, which has no hooks. The hook command was checked with Claude's hook JSON on stdin. covers S18

## Constitution

- C1 Rule 1 → planned here with `sdd change new`, `propose-context` and `finish-propose` before code, then applied with `/sdd-apply` and archived with `/sdd-archive`.

## Review

- R1 Q8, S9 vs `tracker_outbox.go` (`e.RepoRoot != me` keeps the entry): one daemon per git dir would never flush another worktree's or subproject's writes → D12 and Q8: the daemon drains and polls every live session's root; S20 covers it.
- R2 D6, Q9, Q10 single-root assumptions; claim keys collide across subprojects → D12: claims and watch entries are keyed by `rootKey(root)`, and per-root trackers are polled (S20).
- R3 task 1.1 and Q14 vs `withStateLock` always running MkdirAll → Q3: reads return empty without locking when `state.json` is missing; only heartbeat, begin and claim (once sync is enabled) create it.
- R4 Q2, D10, D11 vs the blocking `LOCK_EX`: hooks could wait without limit while the daemon holds the lock → Q2: hooks wait at most 1s, then warn and exit 0; Q8: the sync lock is never held across git, network or plugin calls.
- R5 D2 vs the Windows 60s takeover and unconditional remove → D14: lock files carry a pid and nonce checked before refresh or remove, and the drain re-locks per entry.
- R6 Q1, Q4, 4.2: claims run through the Bash tool would not know the Claude session id → D13 and Q1: a `SessionStart` hook (`sdd sync begin --hook`) exports `SDD_SESSION` via `$CLAUDE_ENV_FILE`; S18 covers it, and V1.6 confirms it on the real client.
- R7 Q4: a claim never set a lease, so it could be expired at once → Q4: a claim also sets the lease.
- R8 S8, D4: a 60s debounce makes a short-lease heartbeat a no-op → the debounce is `min(60s, lease/4)` (Q2, D4), S8 uses a new session, and `end` deletes the marker (Q15).
- R9 Q2, Q8: a heartbeat could race the daemon's exit and leave a live lease with no daemon → Q7: the exit decision and lock release happen under the sync lock, where heartbeats also check; S22 repeats the race.
- R10 Acceptance 2 vs Q7 and Q14 → the Acceptance bullet now says "the next heartbeat or claim".
- R11 task 2.2: a stale stop request would kill the next daemon → Q11: stop is written only while a daemon runs, and a starting daemon deletes stale ones.
- R12 D1 vs Q9 and Q10: restart, new-worktree and archive behaviour was unspecified → Q9: the persisted snapshot is the baseline, new paths are silent, and vanished files are dropped; S21 covers it.
- R13 Q13 trim racing appends and offset-based watch → Q13: trim under the sync lock with an atomic rename, and watch tracks seq.
- R14 Acceptance 3 vs Q6 and D8: a claim made before B's first inbox was never shown → Q6: the cursor is set at the first heartbeat, claim or begin.
- R15 S16 vs a synchronous drain in the tick → Q8: drain and poll run in their own goroutines, and expiry never waits.
- R16 tasks 2.2 and 2.3: detached daemons outlive tests → task 2.2 adds a test helper that runs `sync stop` on cleanup and fails the test if the pid does not exit.
- R17 hint [uncited-writer] `cli/setup.go` (`sdd init` installs the claiming skill), `doc.go`, `living.go`, `propose_context.go`, `roadmap.go` → `claim` is a silent no-op where sync never ran (Q4, S19), so Q14 holds for `sdd init` users; the other files only read `sdd/changes`, and archive moves are covered by Q9 and S21.
- R18 C1 vs Rule 1 → not a conflict: planned under `sdd/changes/tracker-sync` with `finish-propose` before code; no other rule is touched.
