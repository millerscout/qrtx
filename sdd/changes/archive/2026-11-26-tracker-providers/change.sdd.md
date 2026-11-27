```change
name: tracker-providers
status: complete
packages: []
```

## Why

`tracker-standard` defined `sdd.adapter/v1` and the exec-plugin runner, but sdd ships no plugin for a real tracker, so teams on GitHub or Jira cannot use the backlog yet. Writing a plugin today also means re-implementing the envelope, error codes and HTTP error mapping.

Plugins also create a new risk: `.sdd.yaml` is committed. Without protection, a hostile repo could point a plugin at its own host and receive the user's token.

The chosen approach:
- move the protocol types into a dependency-free package;
- add a Go provider kit with host-bound credentials;
- ship `sdd-tracker-github` and `sdd-tracker-jira` on it;
- add `sdd init` and `sdd tracker setup`, so one command prepares a repository.

The design input is the approved plan at `~/.claude/plans/the-sdd-has-tasklist-replicated-spark-of-673f3df8-0d8d-4244-8e52-3d09ffe54aeb.md`. Alternatives go in Decisions.

## User request

After `tracker-standard`, the user asked to propose `tracker-providers`:
- the provider kit;
- the GitHub and Jira plugins first, with GitLab and Linear after;
- credentials bound to hosts;
- providers checked against what exists online.

While planning, they added that a CLI command should set up a repository or folder. They also said the later wiki work should offer both a generic HTTP adapter for custom product wikis (theirs stores pages in versioned GCS with its own API) and a wiki engine built into sdd. Only the kit's reusability for that later work is in scope here.

## Acceptance

- After `go install …/cmd/sdd-tracker-github@latest` and `sdd tracker setup github --repo owner/name --token-command "gh auth token" --yes`:
  - `sdd tracker add`, `list`, `start` and archive work against that GitHub repo.
  - Issues show `in progress` while started, and are closed as completed on archive.
- The same works for Jira after `sdd tracker setup jira --url https://acme.atlassian.net --project SHOP --email-env JIRA_EMAIL --token-env JIRA_API_TOKEN --yes`. Issues move through the project's own workflow transitions.
- When `.sdd.yaml` points a plugin at a host, repo or project the user has not bound in their user-level credentials file, every call fails with an `auth` error naming what is unbound and the file. The token is never sent, not even after a redirect.
- Credentials can come from a command that mints short-lived, narrowly scoped tokens (an OIDC exchange, a GitHub App installation token, a zero-trust broker). sdd tells the command which host and repo or project it is for.
- `sdd init` in a new folder installs the skills, writes a starter constitution, registers the merge driver (in git), and with `--tracker` configures the tracker. Re-running it changes nothing and says so.
- A Go developer can write a new provider by implementing the `IssueTracker` methods on top of `providers/kit` and testing it with `kittest.Run`, without importing the root `sdd` package.

## Requirements

- Q1 Package `github.com/millerscout/sdd/tracker` SHALL hold the protocol types, `IssueTracker`, the envelope, `CanonicalJSON`, the marker helpers and `Conformance`, importing only the standard library; root `sdd` SHALL keep every existing exported tracker name, including `NewClientRef` and `NewNonce`, as an alias or wrapper with behaviour unchanged.
- Q2 `kit.Serve` SHALL read one envelope, reject a wrong `protocol` or `kind` with `invalid`, dispatch the op to the provider, and write exactly one `result` or `error` document with exit code 0 or 1.
- Q3 `kit.Credentials(host, scope)` SHALL return credentials only when the user-level credentials file (`$SDD_CREDENTIALS`, else the user config dir) binds that exact `host[:port]` and lists the scope (a GitHub `owner/name` or Jira project key) in the binding's `scopes`, or the binding allows `*`. Anything else SHALL be refused with `auth` naming what is unbound and the file. A binding that holds a literal token SHALL be rejected. Plain http SHALL be allowed only for a loopback binding. A `token_command` SHALL receive `SDD_CRED_HOST` and `SDD_CRED_SCOPE` in its environment.
- Q12 `kit.HTTP` SHALL refuse any redirect that changes scheme, host or port, and never forward credentials across one.
- Q4 `kit.HTTP` SHALL map HTTP outcomes to adapter codes (401/403 `auth`, 404 `not_found`, 400/422 `invalid`, 429 or exhausted rate limit `rate_limited` with `retry_after`, 5xx and network errors `unavailable`) and SHALL retry a 5xx or 429 at most once, only for requests safe to repeat (GET, PATCH, PUT, and a create guarded by `client_ref`, never a comment POST), and only when the wait fits before the deadline `execTracker` passes in `SDD_TRACKER_DEADLINE` (default 20s from start).
- Q5 `sdd-tracker-github` SHALL implement every tracker op against GitHub's REST API, map open/closed plus `state_reason` and the `started_label` to the five categories, keep `client_ref` and `sdd` refs in a body marker that `description` never shows, skip pull requests in `list_issues`, return a `cursor` for the next page, support the `change` filter by reading markers, and keep the started label when `update_issue` replaces labels.
- Q6 `sdd-tracker-jira` SHALL implement every tracker op against Jira Cloud REST v3, map `statusCategory` and canceled resolutions to the five categories, move issues only through the workflow's available transitions, preferring the target named by the `native` argument (which sdd fills from `status_map`). For `canceled` with no `native`, it SHALL use a done-category transition and set the first `canceled_resolutions` value as the resolution when the transition accepts one. It SHALL return `invalid_transition` with `allowed` when no transition reaches the category. It SHALL hide its `sdd-cref-*` labels on read, and keep them when `update_issue` replaces labels.
- Q7 Both providers SHALL make `create_issue` idempotent per `client_ref`. GitHub SHALL scan the creator's 100 newest issues (all states) for the marker. Jira SHALL search a `sdd-cref-<client_ref>` label set at create time, retrying the lookup up to three times over about 6s to ride out search index lag. A retried create SHALL return the first issue.
- Q13 `sdd tracker from-deferred` SHALL skip a bullet whose deterministic `client_ref` is already on an issue listed for that change (`IssueFilter.Change`), so a re-run weeks later does not depend on a provider's idempotency window.
- Q8 `sdd tracker setup <backend>` SHALL write the `tracker:` block into `.sdd.yaml` while keeping its other keys and comments, SHALL add the host binding to the user-level credentials file only after confirmation or `--yes`, SHALL never write a token value, and SHALL end with a read-only check.
- Q9 `sdd init` SHALL run skills install, constitution init (only when none exists), tracker init (inside git) and, with `--tracker`, tracker setup. A re-run SHALL change no file and report each step `unchanged`. A local skill that differs from the bundled one SHALL be reported `skipped` while the other skills still install.
- Q10 Setup commands SHALL prompt for a missing required value only when stdin is a terminal; otherwise they SHALL exit 1 naming the missing flag.
- Q11 The kit SHALL dispatch by envelope `kind`, so a later `wiki` kind can reuse `Serve`, `HTTP` and `Credentials` without changing them.

## Scenarios

- S1 (Q1) WHEN `go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./tracker` runs THEN it prints only `github.com/millerscout/sdd/tracker`. WHEN `go test ./...` runs after the move THEN every existing test passes, with only the `validate`→`Validate` and `addRefs`→`AddRefs` call sites renamed.
- S2 (Q2) WHEN a provider binary gets `{"protocol":"sdd.adapter/v2",…}` or `kind: "wiki"` for a tracker-only provider THEN it prints an `invalid` error naming the field and exits 1. WHEN the provider returns a plain Go error THEN the response has code `invalid`, and WHEN it returns an `*AdapterError` THEN its code is kept.
- S3 (Q3) WHEN the credentials file binds only `127.0.0.1:<portA>` and `.sdd.yaml` points the provider at `http://localhost:<portB>` THEN every op fails with `auth` naming `localhost:<portB>` and the credentials path, and the fake on port B counts zero requests. WHEN the binding's `scopes` is `[o/r]` and `.sdd.yaml` sets `repo: attacker/x` on the bound host THEN every op fails with `auth` naming `attacker/x`, and no request reaches the fake.
- S18 (Q12) WHEN the bound fake answers 302 to another host, or to an http URL on its own host THEN the op fails with `unavailable` naming the redirect, and the redirect target receives no request.
- S4 (Q3) WHEN a binding is `{token: ghp_x}` THEN loading fails saying literal tokens are not allowed. WHEN a binding names `token_env: GITHUB_TOKEN` and that variable is empty THEN the call fails with `auth` naming the variable. WHEN a binding's `token_command` prints `$SDD_CRED_HOST/$SDD_CRED_SCOPE` THEN the fake receives that token for the bound host and repo.
- S5 (Q4) WHEN the fake API returns 503 once and then 200 THEN the op succeeds. WHEN it returns 429 with `Retry-After: 7` and `SDD_TRACKER_DEADLINE` is 3s away THEN the op fails at once with `rate_limited` and `retry_after` 7. WHEN it returns 401 THEN the code is `auth` with no retry. WHEN a comment POST gets 503 THEN it is not retried, and the fake records one comment request.
- S6 (Q5) WHEN `kittest.Run` drives the GitHub provider against the fake GitHub API THEN every conformance op passes, including the same-`client_ref` create returning one issue.
- S7 (Q5) WHEN the fake repo holds an open issue, an open issue labelled `in progress`, an issue closed as `completed`, an issue closed as `not_planned`, an issue closed as `duplicate`, and a pull request THEN `list_issues` returns five issues with categories unstarted, started, completed, canceled and canceled, and `description` never contains `<!-- sdd:v1`. WHEN 150 issues exist THEN the first `list_issues` page returns 100 plus a `cursor`, and sending the cursor returns the remaining 50. WHEN `update_issue` sets labels `[ui]` on a started issue THEN it reads back with labels `[ui]` and category `started`.
- S8 (Q5) WHEN `transition_issue(started)` runs and the repo has no `in progress` label THEN the provider creates the label and adds it. A second call that gets "already_exists" still succeeds.
- S9 (Q6) WHEN `kittest.Run` drives the Jira provider against the fake Jira API THEN every conformance op passes, and descriptions and comments are sent as ADF documents.
- S10 (Q6) WHEN an issue's available transitions reach only `In Review` (indeterminate) and `Done` (done) and the provider is asked for `canceled` with no `status_map` THEN it returns `invalid_transition` with `allowed` = [In Review, Done]. WHEN the available transitions include `Done` (done) and the transition accepts a resolution, and `transition_issue(canceled)` runs with no `native` THEN the provider uses `Done` with resolution `Won't Do`, and the issue reads back as `canceled`. WHEN the issue carries label `sdd-cref-abc` THEN `get_issue` shows no `sdd-cref-` label, and `update_issue` labels `[ui]` keeps it in the fake.
- S11 (Q7) WHEN the fake stores an issue and then stalls past the timeout, and the create is retried THEN both providers return that same issue, and the fake holds exactly one issue with that `client_ref`. WHEN the Jira fake hides a new issue from search for its first two queries THEN the retried create still finds it and creates nothing. WHEN `from-deferred` re-runs against GitHub after more than 100 newer issues exist THEN no duplicate is created.
- S12 (Q8, Q10) WHEN `sdd tracker setup github --repo o/r --token-command "gh auth token" --yes` runs in a temp repo whose `.sdd.yaml` already has `project: shop` and a comment THEN `.sdd.yaml` keeps both and gains the tracker block, the temp `SDD_CREDENTIALS` file gains `api.github.com` with `token_command` and `scopes: [o/r]`, and no token appears in either file. A second setup for `o/other` adds that scope to the same binding. WHEN it runs without `--repo` and stdin is not a terminal THEN it exits 1 naming `--repo` and writes nothing.
- S13 (Q8) WHEN setup runs without `--yes` and the confirmation is answered `n` THEN the credentials file is unchanged and the output says how to bind the host by hand. WHEN `sdd-tracker-github` is not on `PATH` THEN the output prints the exact `go install …/cmd/sdd-tracker-github@latest` line and the check is skipped.
- S14 (Q9) WHEN `sdd init --tracker github --repo o/r --token-command "gh auth token" --yes` runs in a fresh git repo THEN skills, `sdd/constitution.md`, the `.gitattributes` merge line, the git merge driver, the tracker block and the binding exist. WHEN it runs again THEN every step prints `unchanged`, and all those files are byte-identical.
- S15 (Q9) WHEN `sdd init` runs in a repo that already has a constitution listed in `.sdd.yaml` THEN the constitution step reports `unchanged` instead of failing. WHEN it runs outside git THEN the merge-driver step reports `skipped (not a git repository)`. WHEN one installed skill has local edits THEN that skill reports `skipped (local edits; use skills install --force)`, and the other skills install.
- S16 (Q11) WHEN a test provider declares kind `wiki` and gets a `wiki` envelope THEN `kit.Serve` dispatches it to the provider's generic handler, and a `tracker` envelope to it fails with `invalid`.
- S19 (Q13) WHEN a change's Deferred bullet already has an issue carrying its deterministic `client_ref`, and the provider does not deduplicate THEN `sdd tracker from-deferred` queues no create for that bullet, and the backend still holds exactly one issue for it.
- S17 (Q5, Q6, Q3) WHEN the built `sdd-tracker-github` and `sdd-tracker-jira` binaries run under `sdd tracker check --backend <name> --allow-writes` with `SDD_CREDENTIALS` pointing at a temp file bound to the fake's host THEN every op passes through the real exec path.

## Decisions

- D1 Providers live in this module as `cmd/sdd-tracker-<name>` binaries over `providers/<name>` packages. Rejected: separate Go modules, because two modules would have to be versioned together before v1.
- D2 The protocol moves to package `tracker`, and root `sdd` keeps aliases (`type Issue = tracker.Issue`). Rejected: having plugins import root `sdd`, because it pulls in yaml and, with cgo, go-tree-sitter for every plugin author.
- D3 Credentials are env var names (`token_env`, `email_env`) or a `token_command`, bound per exact API `host[:port]` (`api.github.com`, not `github.com`) with a `scopes` allow-list of repos or projects, in a user-level file. The user asked for OIDC or zero-trust over static tokens (2026-11-26). `token_command` gets `SDD_CRED_HOST`/`SDD_CRED_SCOPE`, so an OIDC exchange, a GitHub App installation-token minter or a zero-trust broker can mint a short-lived token scoped to that one repo or project. Built-in sources for those are Deferred (`tracker-credentials-federated`). SSH certificates do not apply: GitHub and Jira REST APIs do not accept SSH auth. Rejected: literal tokens in YAML, because they get copied and committed; bindings in `.sdd.yaml`, because the repo is the threat.
- D4 GitHub idempotency uses a body marker plus a scan of the authenticated user's 100 newest issues, all states. Amended 2026-11-26 after V1.5 found that real GitHub lists a new issue only 3–4s after it is created. The user chose both fixes: the provider scans again after 3s before creating, and `sdd tracker check` waits 10s before its retried create against plugin backends (`SDD_CHECK_RETRY_DELAY` overrides), as an outbox retry on a later command would. Rejected: `/search/issues`, because it allows 30 requests a minute, caps at 1,000 results and lags behind new issues.
- D5 GitHub "started" is a label (`started_label`, default `in progress`), which the provider creates when it is missing. Rejected: Projects v2 status, because it needs GraphQL and per-project field configuration.
- D6 Jira uses REST v3 with a minimal Markdown ↔ ADF converter (paragraphs, hard breaks, inline code, links). Rejected: v2 plain-string bodies, because Atlassian is moving off v2 and reports already show v3-only behaviour.
- D7 Jira idempotency is a `sdd-cref-<client_ref>` label set at create, found with JQL `labels = …` on `POST /rest/api/3/search/jql`. `sdd` refs live in the `sdd` issue entity property. Rejected: searching an entity property, because JQL needs an app-declared index for that; a description marker, because ADF has no HTML comments.
- D8 Jira transitions are chosen from the issue's available transitions: first the `status_map` target name, then the first target in the matching status category. Canceled is read from `done` plus a resolution in `canceled_resolutions`. Rejected: setting the status directly, because Jira only moves issues through workflow transitions.
- D9 `sdd tracker setup` edits `.sdd.yaml` through a `yaml.v3` `Node`, so comments and key order survive. Rejected: unmarshal and re-marshal, because that drops comments.
- D10 `sdd init` treats "already done" as success (`unchanged`), including a constitution already listed in `.sdd.yaml`, which `InitConstitution` reports as an error today. Rejected: changing `InitConstitution`, because it is stable API.
- D12 `kit.Provider` lists with `ListIssuesPage(f) ([]Issue, cursor string, error)`; the kit's `list_issues` returns `{"issues","cursor"}`, which `execTracker` already follows. Rejected: a plain `ListIssues`, because the kit could not page and a large repo would blow the timeout.
- D13 `execTracker` sets `SDD_TRACKER_DEADLINE` (RFC 3339) in the plugin's environment; the kit defaults to 20s when it is unset. Rejected: a `deadline` envelope field, because the protocol stays unchanged in this change.
- D14 Redirects: `kit.HTTP` sets `CheckRedirect` to refuse any change of scheme, host or port. Rejected: following them and re-resolving credentials, because a hostile server could bounce a request to another bound host.
- D11 `kit.Serve` dispatches on `kind` to a provider's declared kinds, and the tracker op table is one handler. Rejected: a tracker-only `Serve`, because the planned `wiki` kind would need a second kit.

## Risks

- Real GitHub or Jira behaviour differs from the fakes → the fakes are built from the documented responses; Validate V1.5 is a manual check against a scratch repo and a sandbox project; Observed records what was verified against official docs and what was not.
- Jira API details flagged unverified (transition fields, comment ADF quirks, property size) → task 4.1 starts by checking them on the live Atlassian reference pages and records any difference in Observed before coding.
- A `token_command` runs a user-chosen command → it comes only from the user-level file, never from the repo, and setup shows the exact line before writing it.
- A create retried within about 3s of the first on GitHub can still miss it, because the list lags 3–4s → outbox retries come on a later command, and the docs state the window.
- The 100-issue creator scan can miss an issue the same user created more than 100 issues ago → outbox retries are the main case and usually land within minutes. Deferred re-runs are deduplicated by sdd itself (Q13). The doc states the window.
- Jira search index lag can outlast the three lookups → the retry window is about 6s, and a later duplicate is still possible after a very long stall. Risks and the docs state it, and the label makes a duplicate easy to find.
- A `token_command` broker that ignores `SDD_CRED_SCOPE` hands out a broad token → the scope allow-list is still enforced before the command runs.

Deploy: tag a minor release. The root `sdd` API is unchanged (aliases). Users then install the plugins with `go install …/cmd/sdd-tracker-<name>@latest`, and older `sdd` binaries keep working with them because the protocol is unchanged.

Rollback: uninstall the plugin binaries, set `tracker.backend` back to `file` (or remove it), and remove the binding from the credentials file. Nothing in the repo depends on the providers.

## Observed

- Root `tracker.go` defines `Issue` (custom `MarshalJSON`/`UnmarshalJSON` keeping unknown keys), `IssueStatus`, `SDDRef`, `IssueFilter`, `CreateIssueInput`, `UpdateIssueInput`, `TrackerCaps` and `IssueTracker`, with unexported `validate`, `addRefs` and `canonicalJSON`. These are used by `tracker_file.go`, `tracker_merge.go`, `tracker_exec.go`, `tracker_outbox.go` and `tracker_lifecycle.go`.
- `adapter.go` defines `AdapterRequest`, `AdapterResponse`, `AdapterError` (unexported `retryable`), the error codes and `ValidBackendName`.
- `tracker_open.go` `CheckTracker` holds the conformance sequence: capabilities, list, create, a same-`client_ref` create, get, update with `AddRefs`, transition to started, list after create, comment, and a cleanup transition to canceled.
- `tracker_exec.go` `execTracker.run` sends `AdapterRequest{Protocol, Kind:"tracker", Op, Config, Args}` and passes `op` as `argv[1]`. It treats `{"error"}` with exit 1 as an adapter error. `ListIssues` sends `{"filter":…}` and reads `{"issues","cursor"}`; `UpdateIssue` sends `add_sdd`.
- `codeindex_treesitter.go` is `//go:build cgo` and imports go-tree-sitter. `go.mod` requires go-tree-sitter and `gopkg.in/yaml.v3`.
- `cmd/sdd/main.go` installs `TextIndex` and calls `cli.Run`. `cli/cli.go` `Run` dispatches `change`, `skills`, `constitution` and `tracker`. `cli/tracker.go` holds `runTracker`, with `init` writing `.gitattributes` and `git config merge.sdd-issues.driver` from `os.Executable()`.
- `skills/skills.go` `Install(dir, force)` returns `created`, `updated` or `unchanged` per skill.
- `constitution.go` `InitConstitution` errors when `.sdd.yaml` lists constitution files or when the file exists without `force`.
- `repoconfig.go` `TrackerConfig` has `backend`, `config`, `status_map`, `mirror`, `comments` and `timeout`. `LoadRepoConfig` does not validate it.
- `sdd change propose-context --change tracker-providers --json` gives score `simple` and no overlaps (run 2026-11-26).
- GitHub REST, verified on docs.github.com (2026-11-26):
  - Issues list, get, create and update are under `/repos/{owner}/{repo}/issues`, with `per_page` max 100. List results include pull requests, which carry a `pull_request` key.
  - `state_reason` values are `completed`, `not_planned`, `duplicate`, `reopened` and null.
  - Auth is `Authorization: Bearer`, and a `User-Agent` is required. `X-GitHub-Api-Version` defaults to `2022-11-28`.
  - Search allows 30 requests a minute and returns at most 1,000 results.
  - GitHub Enterprise Server's base URL is `/api/v3`.
- Jira Cloud, from Atlassian docs and community reports (2026-11-26):
  - The legacy `/rest/api/3/search` is reported removed in favour of `/rest/api/3/search/jql`, which pages with `nextPageToken`/`isLast`.
  - v3 descriptions and comment bodies are ADF. Basic auth is email plus API token, and 429 carries `Retry-After`.
  - Entity properties store JSON, but JQL needs an app-declared index to search them.
  - V1.5 against real GitHub (2026-11-26, private repo `millerscout/sdd-tracker-scratch`, account `millerscout` via `token_command: gh auth token --user millerscout`):
  - **First run:** every op passed except `create_issue (idempotent)`, which made issues #1 and #2.
  - **Lag measurement:** three creates in a row became visible in `GET /repos/…/issues?creator=…` after 2.68s, 2.72s and 4.07s.
  - **After the D4 amendment:** `sdd tracker check --allow-writes` passed every op (issue #6, about 25s), and the stray #2 was closed as not planned.
- Task 4.1 check, against Atlassian's OpenAPI spec `swagger-v3.v3.json` (2026-11-26). The rendered reference pages came back empty to the fetcher.
    - **Transitions:** GET takes `expand=transitions.fields` for the transition-screen fields. The example shows `transitions[].id`, `.name`, `.to.name` and `.to.statusCategory.key` (example keys `in-flight`, `completed`, so the provider accepts both these and `new`/`indeterminate`/`done`). POST takes `IssueUpdateDetails` (`transition`, `fields`, `update`) and returns 204.
    - **Search:** `GET /rest/api/3/search/jql` takes `jql`, `nextPageToken` (absent on the last page), `maxResults`, `fields` (default `id` only, so the provider names its fields) and `properties` (up to 5).
    - **Issue get:** `GET /rest/api/3/issue/{key}` accepts `properties`. Issue properties are at most 32768 characters, and PUT returns 200 or 201.
    - **Create:** returns `id`, `key` and `self`.
    - **Comment body:** the schema was not read. The provider sends ADF, as every v3 issue example shows.

## Assumptions

- The user's GitHub and Jira accounts can create labels: in GitHub, push access; in Jira, labels are free-form. Without that permission, `start` and `transition` return `auth`/`invalid`, which the outbox reports.
- `gh auth token` is how most GitHub users will bind credentials, and the docs lead with it. Teams that want OIDC or zero-trust credentials plug in a broker command until the federated sources land.
- Interactive prompting needs only a "stdin is a terminal" check. A `golang.org/x/term` dependency is not needed: `os.Stdin.Stat()` and `ModeCharDevice` are enough.

## Non-goals

- GitLab, Linear and Azure DevOps providers.
- The sync daemon, claims and hooks.
- GitHub Projects v2 fields, Jira custom fields, and attachments.
- Two-way sync and webhooks.
- The wiki contract, `sdd-wiki-http` and the built-in wiki engine. This change only makes the kit dispatch by `kind` (Q11).

## Deferred

- `tracker-providers-more`: GitLab, Linear and Azure DevOps providers on `providers/kit`.
- `tracker-credentials-federated`: built-in short-lived credential sources. These are a GitHub App installation token minted per repo (1h), a CI OIDC token exchange (GitHub Actions `id-token`), and Atlassian OAuth 2.0 with scoped tokens. They replace static tokens where available.
- `tracker-jira-live-check`: run V1.6 against a real Jira Cloud site once the user has one. Until then, Jira behaviour is verified only against the fake built from Atlassian's OpenAPI spec.
- `tracker-credentials-multi-account`: several bindings per host, each with its own scopes and source (for example `gh auth token --user <account>`), plus `sdd tracker setup github --gh-user`. Today one binding per host can pick the account in its `token_command` from `$SDD_CRED_SCOPE`.
- `tracker-sync`: the leased daemon (1h lease, heartbeat), claims, inbox and watch, and opt-in hooks.
- `wiki-adapter`: the `wiki@v1` contract with `sdd-wiki-http`, and a wiki engine built into sdd (object-store drivers `fs`/`gcs`/`s3` with versioning and conditional writes, section edits, generated chunks, PR drafts, and a pluggable LLM command). It comes after `gherkin-export` and `test-evidence`.

## Tasks

### Phase protocol package
{: id="phase-01" status="accepted" file="phases/01-protocol-package.md"}

### Phase provider kit and credentials
{: id="phase-02" status="accepted" file="phases/02-provider-kit.md"}

### Phase GitHub provider
{: id="phase-03" status="accepted" file="phases/03-github-provider.md"}

### Phase Jira provider
{: id="phase-04" status="accepted" file="phases/04-jira-provider.md"}

### Phase setup commands
{: id="phase-05" status="accepted" file="phases/05-setup-commands.md"}

### Phase docs and release notes
{: id="phase-06" status="accepted" file="phases/06-docs.md"}

## Validate

- [x] V1.1 `go test ./... -count=1 -run 'TestTrackerPackage|TestIssue|TestTracker|TestMarker'` covers S1
- [x] V1.2 `go test ./providers/... -count=1` covers S2,S3,S4,S5,S6,S7,S8,S9,S10,S11,S16,S18
- [x] V1.3 `go test ./... -count=1 -run 'TestTrackerSetup|TestInit|TestProviderBinaries|TestTrackerDeferred'` covers S11,S12,S13,S14,S15,S17,S19
- [x] V1.4 `go test ./... -count=1` covers S1
- [x] V1.5 manual: `sdd tracker check --allow-writes` against a scratch GitHub repo with real credentials bound passed every op on 2026-11-26 (see Observed). covers S6
- [ ] V1.6 manual: `sdd tracker check --backend jira --allow-writes` against a Jira sandbox project. Not run: the user has no Jira site yet, so Jira is verified only against `providers/jira/fakeapi` (Deferred). covers S9

## Constitution

- C1 Rule 1 → this change is planned here (`sdd change new`, `propose-context`, `finish-propose` before code), applied with `/sdd-apply` and archived with `/sdd-archive`; follow-up providers, sync and the wiki are named in Deferred as slugs in this repository.

## Review

- R1 S1, task 1.1: root tests call `validate`/`addRefs`, which cannot stay as methods on an alias → S1 allows renaming only those call sites, and task 1.1 edits `tracker_test.go` and `tracker_helper_test.go`.
- R2 task 1.1 verify: `go list -deps ./tracker` always lists the package itself → the check uses `-f '{{if not .Standard}}…'` and expects only `github.com/millerscout/sdd/tracker`.
- R3 task 1.2: `Conformance` needs `NewClientRef`/`NewNonce` from `tracker_file.go` → task 1.1 moves both, with root aliases (Q1).
- R4 Q5, D12: `IssueTracker.ListIssues` returns no cursor → `kit.Provider` uses `ListIssuesPage` with a cursor; the GitHub list test pages 150 issues (S7).
- R5 S3, S12: binding `github.com` would not match calls to `api.github.com` → D3: bind the exact API host; setup writes `api.github.com` (S12, task 5.2).
- R6 Q3, task 2.3: redirects could carry credentials to another host or downgrade to http → Q12, D14 and S18: refuse scheme, host and port changes, with tests in task 2.3.
- R7 D3: host-only bindings let a hostile `.sdd.yaml` write to another repo or project on the same host → the user chose an allow-list and prefers OIDC or zero trust (2026-11-26). Bindings now carry `scopes`, `token_command` gets `SDD_CRED_HOST`/`SDD_CRED_SCOPE` for short-lived scoped brokers, and built-in federated sources are Deferred (`tracker-credentials-federated`).
- R8 Q3: a portless loopback binding would serve any local port → bindings match `host[:port]` exactly, and http is allowed only for loopback bindings (Q3, S3).
- R9 Q7 vs `QueueDeferredIssues`: weeks-later deferred re-runs exceed the GitHub 100-issue window → Q13 and task 1.4: sdd deduplicates deferred bullets against the change's listed issues; Risks states the window.
- R10 Q7, D7: Jira search lags → the lookup is retried three times over about 6s, the fake hides new issues for two queries (S11, task 4.4), and the residual risk is in Risks.
- R11 Q6, D8: providers never see `status_map`, only `native` → Q6, D8 and S10 prefer the `native` argument.
- R12 S9 vs the conformance cleanup `TransitionIssue(canceled, "")`: Jira has no canceled category → Q6: canceled with no `native` uses a done-category transition plus a canceled resolution, so the cleanup passes, and S10 expects it.
- R13 S10: nothing set the resolution, so canceled would read back as completed → Q6 and task 4.3 set the first `canceled_resolutions` value when the transition accepts one, and S10 checks the read-back.
- R14 Q5, Q6: a labels replace would drop `sdd-cref-*` and the started label → providers keep internal labels on replace and hide `sdd-cref-*` on read (Q5, Q6, S7, S10).
- R15 Q4: retrying a comment POST could post a duplicate → retries are limited to GET/PATCH/PUT and guarded creates (Q4); S5 checks a single comment request.
- R16 Q4: the kit does not know sdd's timeout → D13: `execTracker` passes `SDD_TRACKER_DEADLINE` (task 1.4), the kit defaults to 20s, and a `Retry-After` past the deadline fails at once (S5).
- R17 S3: an `evil.example` fake cannot exist under httptest → S3 uses a host or port mismatch between loopback addresses, with the fake's request counter as the observable.
- R18 task 2.4: `kittest.Run` had no config → `Run(t, p, config)`.
- R19 tasks 3.1, 4.1 vs 5.4: `_test.go` fakes cannot be imported → the fakes are `providers/<name>/fakeapi` packages.
- R20 Q9, S14: `trackerInit` always re-registers, and `skills.Install` stops at the first differing skill → task 5.3a adds the unchanged path and `skills.InstallKeep`, and S15 covers a locally edited skill.
- R21 S12, S14: setup and init tests would write the developer's real credentials file → every setup and init test sets a temp `SDD_CREDENTIALS` (tasks 5.1–5.3).
- R22 hints [omitempty-vs-presence] `tracker.go:82`, `:103`, `:245` vs S13 → not a conflict, because "sdd" in S13 is the binary name in the printed `go install` line, not the JSON `sdd` field.
- R23 hints [omitempty-vs-presence] `tracker.go:83`, `:104`, `tracker_outbox.go:24` vs S11 → not a conflict, because the S11 create always carries a non-empty `client_ref` (`CreateIssueInput.ClientRef` is not omitempty), and the scenario counts issues in the fake.
- R24 C1 vs Rule 1 → not a conflict. Also checked: Go aliases for a struct with unexported fields (only `tracker.go` touches `unknown`), constants via `const X = tracker.X`, and the `execTracker` argument shapes that task 2.1 must accept.
- R25 hint [omitempty-vs-presence] `tracker.go:46` (`SDDRef.Change`) vs S19 → not a conflict, because deferred issues always carry a non-empty `change` (`QueueDeferredIssues` sets the slug), and S19 counts issues in the backend; it does not check that the field is present when empty.
