# Phase Jira provider

- [x] 4.1 Before coding, check on developer.atlassian.com:
  - the transitions response fields (`to.name`, `to.statusCategory.key`) and the POST body;
  - the comment ADF body;
  - the entity property size limit;
  - the `/search/jql` request and response fields.

  Record what was confirmed or differs in this change's Observed. Then add `providers/jira/fakeapi` (an importable package), an `httptest` fake that records requests and can hide new issues from search for N queries, of: search/jql with `nextPageToken`, issue get/create/edit, transitions GET/POST, comment POST, and issue property GET/PUT. verify: Observed lists the checked pages with dates, and the fake smoke test pages two results. covers S9
{: id="phase-04-task-1"}
- [x] 4.2 Add `providers/jira/adf.go`: Markdown → ADF (paragraphs, hard breaks, inline code, links) and ADF → text. verify: `go test ./providers/jira/ -run TestADF` round-trips the four constructs and reads an unknown node type as its text content. covers S9
{: id="phase-04-task-2"}
- [x] 4.3 Implement `providers/jira`:
  - **Config:** `url`, `project`, `issue_type` and `canceled_resolutions`.
  - **Mapping:** category from `statusCategory` plus the resolution.
  - **Transitions:** per D8, preferring the `native` argument. `canceled` with no `native` uses a done-category transition and sets the first `canceled_resolutions` value when the transition's fields accept a resolution. Otherwise it returns `invalid_transition` with `allowed`.
  - **Idempotency and refs:** the `sdd-cref-` label and a JQL lookup retried three times over about 6s (D7). The `sdd-cref-` labels are hidden on read and kept on a labels replace. The `sdd` property holds the refs, and `ListIssuesPage` filters on it for `change`.
  - **Credentials:** from `kit.Credentials(host of url, project)`.
  - **Bodies:** comments in ADF.
  - **Binary:** `cmd/sdd-tracker-jira/main.go`.

  verify: `go test ./providers/jira/ -count=1` runs `kittest.Run` against the fake, plus the In Review-only `invalid_transition` case, the Done-with-resolution canceled read-back, and hidden or kept `sdd-cref-` labels. covers S9,S10
{: id="phase-04-task-3"}
- [x] 4.4 Test a retried create for Jira, the same as task 3.3, plus a search that hides the new issue for two queries. verify: `go test ./providers/jira/ -run TestCreateRetry`. covers S11
{: id="phase-04-task-4"}
