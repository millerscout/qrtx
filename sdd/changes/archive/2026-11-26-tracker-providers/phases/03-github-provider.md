# Phase GitHub provider

- [x] 3.1 Add `providers/github/fakeapi` (an importable package, reused by the cli end-to-end test), an `httptest` fake of the GitHub REST subset that records every request:
  - issues list (with `creator`, `state`, `labels`, `sort`, `direction`, `per_page`, and `Link` paging), get, create, update and comments;
  - labels create, returning 422 `already_exists`;
  - `GET /user`;
  - one injectable stall per route, to test retries.

  verify: the fake's own smoke test lists a seeded pull request alongside issues. covers S7
{: id="phase-03-task-1"}
- [x] 3.2 Implement `providers/github`:
  - **Config:** `repo`, `api_url`, `api_version` and `started_label`.
  - **Mapping:** the status mapping per Q5, and the marker via `tracker.EncodeMarker`/`DecodeMarker`.
  - **Idempotent create:** the D4 scan of `creator=<login>&state=all&sort=created&direction=desc&per_page=100`. It is retried by the kit only through this guard.
  - **Credentials:** from `kit.Credentials(host of api_url, repo)`.
  - **List:** `ListIssuesPage` skips pull requests, turns `Link` into an opaque cursor, and applies the `change` filter by reading markers.
  - **Transition:** a transition sets `state`/`state_reason` and the started label, creating the label if it is missing.
  - **Other ops:** `update_issue` merges `add_sdd` into the marker and keeps the started label when labels are replaced, and `comment` posts an issue comment (never retried).
  - **Binary:** `cmd/sdd-tracker-github/main.go` calls `kit.Serve`.

  verify: `go test ./providers/github/ -count=1` runs `kittest.Run` against the fake, plus the six-item mapping check, 150-issue paging, a labels replace on a started issue, the started-label creation and the already-exists case. covers S6,S7,S8
{: id="phase-03-task-2"}
- [x] 3.3 Test a retried create: the fake stores the issue and then stalls past the timeout, and the retry returns the same issue with one stored. verify: `go test ./providers/github/ -run TestCreateRetry`. covers S11
{: id="phase-03-task-3"}
