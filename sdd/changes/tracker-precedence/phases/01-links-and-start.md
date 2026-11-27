# Phase links and a start that respects them

- [x] 1.1 Add `AddLinks`, `RemoveLinks` and `Priority` (`add_links`, `remove_links`, `priority`) to `UpdateIssueInput`, and `Priority` to `CreateIssueInput` if it is missing.
  - Copy them in `tracker_exec.go` and `providers/kit/provider.go`.
  - Apply them idempotently in the file backend.
  - Update `docs/schema/tracker/v1/request.json` and `docs/tracker-protocol.md`.
  verify: `go test . ./tracker ./providers/kit -run 'TestTrackerFile|TestIssuePrecedence|TestTrackerExec' -count=1`. covers S1,S8
- [x] 1.2 Add the `sdd-prec` body marker (GitHub) and the `sdd-precedence` property (Jira): read them into `Issue.Links`/`Issue.Priority` in `toIssue`, and write them in `CreateIssue`/`UpdateIssue`. Add fake-API tests that include an old-style `AddRefs` update keeping links and priority. verify: `go test ./providers/... ./tracker -count=1`. covers S2,S10
- [x] 1.3 Add `qrtx tracker link|unlink <a> <type> <b>`, `qrtx tracker priority <id> <n>` and `tracker add --priority <n>`, calling `UpdateIssue` directly and checking the returned issue (D5). verify: `go test ./cli -run 'TestTrackerLink|TestTrackerPriority' -count=1`. covers S1,S2,S8
- [x] 1.4 Put the blocker check in `StartFromIssue` between `GetIssue` and `NewChange` (D3, D4): open categories on `impl`, normalized refs, fail closed at the page cap, warn and skip without `list_issues`. Add the CLI `--force`. verify: `go test . ./cli -run 'TestStartBlocked|TestTrackerStartBlocked' -count=1`. covers S3,S4,S5,S8,S9
- [x] 1.5 Make `tracker list` order by priority (stable) and print `blocked by <refs>`, with a second open-issues listing when a filter is set (D6). verify: `go test ./cli -run TestTrackerListPrecedence -count=1`. covers S6
