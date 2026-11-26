# Phase lifecycle wiring through the outbox

- [x] 4.1 Add `tracker_outbox.go`:
  - The state directory comes from `git rev-parse --git-common-dir` plus `/sdd`, or `.sdd/` with a `.gitignore` of `*`. It is created only when the first entry is written.
  - `outbox.jsonl` entries hold `{seq, key, repo_root, op, backend, args, client_ref, nonce, attempts, last_error, queued_at}` and are written under the lock. An append skips a `key` that is already queued.
  - `DrainOutbox` sends only the entries whose `repo_root` matches, in order. It removes each one only after the backend confirms it, keeps failed ones with `last_error`, and drops a `comment` the backend does not declare.
  - Every `sdd tracker` command drains before and after its work. `sdd tracker outbox [--json] [--drop <seq>]` shows or removes entries.

  verify: `go test . -run TestTrackerOutbox`:
  - `from-deferred` runs against the exec helper, which persists, signals a file and blocks; the test then kills the sdd child, and the next `list` leaves exactly one issue with that `client_ref`.
  - A second root sharing the git dir does not drain the first root's entries.

  covers S5
{: id="phase-04-task-1"}
- [x] 4.2 Add `Issues []string yaml:"issues,omitempty"` to `ChangeManifest`, and a helper that sets the `issues:` line inside the ```change block without touching the rest of the file. Add `sdd tracker start <ref> <slug>`, per Q10: `get` first, failing before creating anything; then `NewChange` when the change is missing, User request filled only while it is template text, the link, and queued `transition_issue(started)` plus `update_issue` adding the `SDDRef` (repo from `git remote get-url origin`, or the folder name; project; change; branch). verify: `go test . -run TestTrackerStart` covers a new change, an existing written change, and a missing issue. covers S12,S13
{: id="phase-04-task-2"}
- [x] 4.3 Make `ArchiveChange` queue, before the move and with deterministic keys, a `comment` for each issue in the manifest `issues:` list (unless `comments: off`, and without calling the backend), and `transition_issue(completed)` only when no other active change's `issues:` lists the issue. If the move fails, it removes the entries this call appended. Make the CLI `change archive` drain afterwards and print `tracker: <n> pending (sdd tracker outbox)` on failure, keeping exit 0. verify: `go test . -run TestTrackerArchive` covers the failing helper (archived, entries listed, drained on recovery), issue X linked by `a` and `b`, `comments: off`, a repo without `tracker:` producing byte-identical output and no state directory, and an existing archive destination leaving no entries and a retry queueing each once. covers S9,S10,S11,S17,S18
{: id="phase-04-task-3"}
- [x] 4.4 Add `sdd tracker from-deferred --change <slug>`. It creates one issue per `## Deferred` bullet with a deterministic `client_ref` (change and bullet text) and `role: deferred`, does not touch the manifest `issues:` list, and goes through the outbox. verify: `go test . -run TestTrackerDeferred` runs it twice on two bullets, including once with the first run's creates still pending, and gets two issues and an unchanged manifest. covers S14,S5
{: id="phase-04-task-4"}
- [x] 4.5 Add `Backlog []Issue json:"backlog,omitempty"` to `ProposeContextReport`, filled with up to 20 open issues from the configured backend with a 10s timeout. A failure adds to `warnings`. verify: `go test . -run TestProposeContextBacklog` covers 25 file issues giving 20, and the failing helper giving no `backlog` key, every old key and a warning. covers S19
{: id="phase-04-task-5"}
- [x] 4.6 Update the docs and skills:
  - `skills/sdd-propose/SKILL.md` accepts an issue ref (`/sdd-propose file:sdd-k3f9qa`) and runs `sdd tracker start`, and offers `from-deferred` at the end.
  - `skills/sdd-archive/SKILL.md` reports the outbox.
  - `README.md` gets a tracker section and `sdd tracker init`.
  - `docs/format.md` gets `issues:` and `sdd/issues.jsonl`.
  - `docs/embedding.md` gets `IssueTracker`.
  - `doc.go` lists the new API as unstable.
  - `CHANGELOG.md` gets an entry.

  verify: `go test ./skills/ ./cli/` passes and `grep -n "sdd tracker" README.md skills/*/SKILL.md` lists the new lines. covers S12
{: id="phase-04-task-6"}
