# Phase sessions, claims and events

- [x] 1.1 Add `sync_state.go`:
  - `syncState` holds `sessions{id: {lease_until, worktree, branch, last_seen}}`, `claims{key: {session, worktree, branch, at}}`, `cursors{session: seq}`, `remote{id: updated_at}`, `watch{path: {mtime, tasks{id: done}}}` and `last_poll`.
  - `readSync(root, now)` returns empty state without locking or creating anything when `state.json` is missing.
  - `withSync(root, now, fn, wait)` loads `state.json` under the `sync` lock (hooks wait at most 1s via a try-lock loop), expires dead leases (releasing their claims and appending `claim_expired`), lets `fn` change the state, and writes it atomically.
  - `appendEvents` assigns `seq` from the log's last line.
  - Add `SyncConfig` (`lease`, `tick`, `interval`) under `sync:` in `RepoConfig`, with Q-defaults.

  - Claims and watch entries are keyed by `rootKey(root)`, and sessions record their root.

  verify: `go test . -run 'TestSyncState'` covers expiry releasing claims with one event per claim, an injected clock, a missing state file giving empty state with no directory created by reads, and the 1s hook lock timeout. covers S3,S16
{: id="phase-01-task-1"}
- [x] 1.2 Add `Heartbeat(root, session, opts)`, `BeginSession` and `EndSession`, with session resolution per Q1 and the `min(60s, lease/4)` marker-file debounce (`hb-<sha(session)>` in the state dir). The first heartbeat or begin sets the session's inbox cursor to the log end. `sdd sync begin --hook` also appends `export SDD_SESSION=<id>` to `$CLAUDE_ENV_FILE` when it is set. `EndSession` deletes the marker. `sdd sync heartbeat [--session id] [--hook] [--quiet] [--no-daemon]` and `sdd sync begin|end [--hook]` exit 0 on internal errors, with a warning on stderr. verify: `go test ./... -run 'TestSyncHeartbeat|TestSyncEnd'` covers the double heartbeat leaving `state.json` byte-identical, hook stdin, an unwritable state dir exiting 0, end releasing claims with a `session_end` event, and begin writing `CLAUDE_ENV_FILE`. covers S1,S2,S17,S18
{: id="phase-01-task-2"}
- [x] 1.3 Add `Claim(root, session, key, release bool)` and `sdd sync claim <key> [--release]`. An already-held claim is idempotent, a claim also sets the session's lease, and a conflict exits 1 naming the holder's session, worktree and branch. With no `state.json` it prints `sync not enabled; claim skipped` and exits 0. verify: `go test ./... -run TestSyncClaim` covers the S4 sequence (A re-claiming produces no new event), the S19 no-op in a fresh repo, and S18's `SDD_SESSION` claim. covers S4,S18,S19
{: id="phase-01-task-3"}
- [x] 1.4 Add `Inbox(root, session, limit)` and `sdd sync inbox [--json] [--brief] [--hook]`:
  - it skips the caller's own events, and a session with no cursor starts at the log end;
  - `--brief` caps output at 10 lines plus a "…and N more" line;
  - `--hook` reads the session from stdin and prints nothing when there is nothing new.

  Add `sdd sync watch [--json]`, which polls `events.jsonl` and prints events after the last seq it saw. verify: `go test ./... -run 'TestSyncInbox|TestSyncWatch'` covers the S5 and S6 cases, and watch printing an event appended after it starts. covers S5,S6
{: id="phase-01-task-4"}
