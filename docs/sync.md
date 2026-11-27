# Sync: several sessions, one repository

When several agent sessions work in one repository (Claude Code sessions, worktrees, terminals), `sdd sync` lets them see each other:

- **Sessions and leases:** a session lives as long as its lease. A lease lasts one hour (`sync.lease`), and every heartbeat while the session works extends it. When every session has been idle past its lease, sync closes on its own: claims are released, pending tracker writes are flushed, and the daemon exits. The next heartbeat or claim opens it again.
- **Claims:** a claim says "I'm working on this", for an issue (`github:12`) or a task (`dark-mode/2.1`). A second session's claim fails and names who holds it, with their worktree and branch. Claims are advisory and local to this machine; other machines see work as taken through the tracker's assignee and "started" status.
- **Inbox:** each session sees what the others did since its last look: claims, releases, expired claims, tasks checked off in any worktree (no commit needed), and remote tracker changes.

## Quick start (Claude Code)

```bash
sdd skills install --hooks          # writes .claude/settings.local.json (--shared: settings.json)
```

The hooks:

| Event | Command | Why |
|---|---|---|
| `SessionStart` | `sdd sync begin --hook` | Registers the session, and exports `SDD_SESSION` (through `$CLAUDE_ENV_FILE`) so the agent's own `sdd` commands use the same session id |
| `UserPromptSubmit` | `sdd sync heartbeat --hook --quiet` | Keeps the lease alive while you work |
| `UserPromptSubmit` | `sdd sync inbox --hook --brief` | Puts what the other sessions did into the prompt's context (10 lines at most) |
| `PostToolUse` (async) | `sdd sync heartbeat --hook --quiet` | Keeps the lease alive during long agent runs, without delaying any tool |
| `SessionEnd` | `sdd sync end --hook` | Releases the session's claims at once; lease expiry still covers a crash |

The bundled skills claim and release work for you: `/sdd-apply` claims tasks and linked issues, `/sdd-propose` claims an issue it starts from, and `/sdd-archive` releases the change's claims. Where sync has never run, `claim` prints `sync not enabled; claim skipped` and does nothing.

Other agents, scripts or people call the same commands with `--session <id>` or `SDD_SESSION=<id>`.

## Commands

| Command | What it does |
|---|---|
| `sdd sync heartbeat [--session id] [--hook] [--quiet]` | Extends the session's lease. A repeat within `min(60s, lease/4)` only checks a marker file. It starts the daemon when none runs. It never fails the caller. |
| `sdd sync begin` / `end [--hook]` | Starts the session (and writes `SDD_SESSION` to `$CLAUDE_ENV_FILE`), or ends it and releases its claims |
| `sdd sync claim <key> [--release]` | Claims (or releases) an issue ref or `<slug>/<task id>`. Fails, naming the holder, when another live session holds it. |
| `sdd sync inbox [--json] [--brief] [--hook]` | Shows the other sessions' events since your last look |
| `sdd sync watch [--json]` | Streams new events |
| `sdd sync status [--json]` | Shows the daemon, sessions and their leases, claims, the outbox size, and the last remote poll |
| `sdd sync stop` | Stops the daemon (it flushes pending tracker writes first) |

## The daemon

`heartbeat`, `begin` and `claim` start a small background process (`sdd sync serve`) when none runs. There is one per git repository, and it serves every worktree and monorepo subproject. While any lease is live, it:

- expires leases every `sync.tick` (5s) and releases their claims;
- sends each repository root's pending tracker writes (the outbox), so they don't wait for your next `sdd tracker` command;
- watches `sdd/changes/*` task checkboxes in every worktree and reports each one that flips;
- polls each root's remote tracker every `sync.interval` (2m), and reports issues that changed.

When no lease is live, it flushes the outbox once more and exits. With `CI` set, `--no-daemon` or `SDD_NO_DAEMON=1`, no daemon is started; everything except the timer-driven parts still works, and expired leases are still treated as expired.

```yaml
# .sdd.yaml
sync:
  lease: 1h
  tick: 5s
  interval: 2m
```

## Files

Everything lives in the shared state directory: `<git common dir>/sdd/` (all worktrees share it), or `.sdd/` outside git. Nothing is committed.

| File | Contents |
|---|---|
| `state.json` | Sessions, claims, inbox cursors, the remote and watch snapshots |
| `events.jsonl` | The event log, trimmed to the newest 5000 events (or 7 days) when the daemon starts |
| `daemon.json`, `daemon.lock` | The running daemon |
| `sync.log` | The daemon's log, rotated at 1 MB |
| `hb-*` | Heartbeat markers for the debounce |

To turn sync off: remove the hooks from `.claude/settings.local.json`, run `sdd sync stop`, and delete those files.
