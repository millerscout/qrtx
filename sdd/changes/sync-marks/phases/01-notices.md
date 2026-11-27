# Phase 1: sync notices

- [x] 1.1 `taskFiles` in `change.go` serves `ChangeProgress` and `watchRoot`. verify: `go test -run TestWatchTaskFiles .` covers S1
- [x] 1.2 `InboxLimit` in `sync.go`, used by `cli/sync.go` for brief text. verify: `go test -run 'TestSyncInbox' . ./cli/` covers S3
- [x] 1.3 The inbox omits own-worktree ticks. verify: `go test -run TestSyncInboxOwnTicks .` covers S4
- [x] 1.4 The watcher drops entries of missing files. verify: `go test -run TestWatchTaskFiles .` covers S5
- [x] 1.5 Sort each file's changed task ids before emitting; update `docs/sync.md` and `CHANGELOG.md`. verify: `go test -run TestWatchOrder .` covers S2
