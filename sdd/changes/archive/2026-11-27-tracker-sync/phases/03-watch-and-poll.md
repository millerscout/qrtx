# Phase local watch and remote polling

- [x] 3.1 Add the watch:
  - it lists worktrees with `git worktree list --porcelain`, mapping each to its repo root by the same relative subpath;
  - each tick it stats `sdd/changes/*/change.sdd.md` and `phases/*.md`;
  - for a changed mtime it re-parses with `tasksmd`, keying tasks per D5, and diffs against `state.watch`;
  - it appends `task_progress` with change, task, done, worktree and branch.

  The persisted snapshot is the baseline, new paths are silent, and vanished files are dropped silently (Q9). Watch entries are keyed by root. verify: `go test ./cli/ -run TestWatch` uses a temp repo with `git worktree add`, a daemon running, and a task checked in worktree A, and expects one `task_progress` event and B's inbox showing it. It also checks a task changed while the daemon was stopped (one event on restart), and an archive emitting no `done=false`. covers S10,S21
{: id="phase-03-task-1"}
- [x] 3.2 Add remote polling for non-`file` backends, every `sync.interval`, in its own goroutine, once per live root:
  - it lists up to 200 issues through `OpenTracker(root)` without holding the sync lock, then merges the result under a short `withSync`;
  - it diffs `updated_at` against `state.remote`;
  - it appends `remote_change` with the issue ref and title, and sets `last_poll`.

  The first poll is silent. verify: `go test . -run TestPoll` uses the fake plugin with `interval` 200ms, changes F-1's `updated_at`, and expects exactly one event, with none from the first or an unchanged poll. covers S11
{: id="phase-03-task-2"}
