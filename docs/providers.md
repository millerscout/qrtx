# Tracker providers

qrtx keeps the project backlog in an issue tracker (see [tracker-protocol.md](tracker-protocol.md)). With no configuration it uses the built-in `file` backend, `sdd/issues.jsonl`. To use a hosted tracker, install its plugin and run `qrtx tracker setup`.

| Provider | Install | Status |
|---|---|---|
| `file` | built into `qrtx` | first-party |
| `github` | `go install github.com/millerscout/qrtx/cmd/qrtx-tracker-github@latest` | first-party |
| `jira` | `go install github.com/millerscout/qrtx/cmd/qrtx-tracker-jira@latest` | first-party (Jira Cloud) |
| GitLab, Linear, Azure DevOps | planned (`tracker-providers-more`) | — |
| anything else | any `qrtx-tracker-<name>` on `PATH` that passes `qrtx tracker check --allow-writes` | community |

## One-step setup

```bash
qrtx init                                   # skills, constitution, merge driver
qrtx init --tracker github --repo acme/shop --token-command "gh auth token" --yes
```

`qrtx init` is safe to re-run: steps that are already done print `unchanged`, and a skill you edited locally is reported as `skipped`. `qrtx tracker setup <backend>` does only the tracker part:

```bash
qrtx tracker setup github --repo acme/shop --token-command "gh auth token"
qrtx tracker setup jira --url https://acme.atlassian.net --project SHOP \
  --email-env JIRA_EMAIL --token-env JIRA_API_TOKEN
```

Setup runs these steps in order:
1. It writes the `tracker:` block into `.sdd.yaml`, keeping your other keys and comments.
2. It shows the credentials binding it wants to add, and asks before writing it (`--yes` skips the question).
3. It checks that the plugin is on `PATH`, or prints the `go install` line.
4. It runs a read-only `qrtx tracker check`.

Without a terminal (agents, CI), a missing value is an error that names the flag; setup never guesses.

## Credentials

Tokens never go in `.sdd.yaml`. That file is committed, and a repository you clone could point a plugin at its own server, or at another repository on the same host. Instead, each user keeps a credentials file:

- `$QRTX_CREDENTIALS`, else
- `$XDG_CONFIG_HOME/qrtx/credentials.yaml` or `~/.config/qrtx/credentials.yaml` (`%AppData%\qrtx\credentials.yaml` on Windows).

```yaml
hosts:
  api.github.com:       { token_command: "gh auth token", scopes: [acme/shop, acme/web] }
  acme.atlassian.net:   { email_env: JIRA_EMAIL, token_env: JIRA_API_TOKEN, scopes: [SHOP] }
```

The rules:
- **Hosts:** a host is the exact API host, with its port when the URL has one: `api.github.com`, not `github.com`. GitHub Enterprise Server uses `HOST` from `https://HOST/api/v3`. A host with no binding gets an `auth` error, and no request is sent.
- **Scopes:** `scopes` lists the repos (GitHub) or project keys (Jira) the credential may be used for. `*` allows any. A repo or project outside the list gets an `auth` error naming it.
- **Sources:** a binding names where the secret comes from, never the secret itself. `token_env` and `email_env` are environment variables. `token_command` is a command that prints a token. Literal `token`/`password` keys are rejected.
- **Short-lived tokens:** `token_command` runs with `QRTX_CRED_HOST` and `QRTX_CRED_SCOPE` set. An OIDC exchange, a GitHub App installation-token minter or a zero-trust broker can use them to mint a short-lived token scoped to that one repository or project. Built-in sources for those are planned (`tracker-credentials-federated`). SSH certificates don't apply: GitHub's and Jira's REST APIs don't accept SSH auth.
- **Redirects:** a redirect to another scheme, host or port is refused, so credentials never follow one.
- **Plain http:** allowed only for a loopback host (tests).

## GitHub

```yaml
tracker:
  backend: github
  config:
    repo: acme/shop              # required
    api_url: https://api.github.com   # or https://HOST/api/v3 (GitHub Enterprise Server)
    started_label: in progress   # default
    api_version: 2022-11-28      # X-GitHub-Api-Version
```

| GitHub | qrtx category |
|---|---|
| open | unstarted |
| open with `started_label` | started |
| closed, `state_reason: completed` | completed |
| closed, `not_planned` or `duplicate` | canceled |

How each operation works:
- **Started:** moving an issue to `started` adds the label, creating it if the repository doesn't have it.
- **Hidden marker:** `client_ref` and the `sdd` links live in a hidden marker on the last line of the issue body (`<!-- sdd:v1 {…} -->`), which `description` never shows. Issue links (`blocks`, …) and priority live in a second marker line just before it (`<!-- sdd-prec:v1 {…} -->`).
- **Idempotency:** a create first looks at your 100 newest issues for the same `client_ref`, and looks again 3 seconds later before creating. GitHub's issue list shows a new issue only 3–4 seconds after it is created, so a create retried within about 3 seconds of the first can still duplicate. qrtx's own retries come from its outbox on a later command, well outside that window. The issue search API isn't used because it lags even more. `qrtx tracker from-deferred` also skips bullets whose issue already exists, so re-running it weeks later creates no duplicates.
- **Listing:** pull requests are skipped, and listing pages through any number of issues.
- **Retries:** comments and creates are never retried by the plugin. qrtx's outbox retries them, and the `client_ref` check keeps a retried create from duplicating.

## Jira Cloud

```yaml
tracker:
  backend: jira
  config:
    url: https://acme.atlassian.net   # required
    project: SHOP                     # required
    issue_type: Task                  # default
    canceled_resolutions: ["Won't Do", "Won't Fix", Duplicate, Declined, "Cannot Reproduce"]
  status_map:                         # optional: exact target status per category
    started: In Progress
```

| Jira status category | qrtx category |
|---|---|
| new (To Do) | unstarted |
| indeterminate (In Progress) | started |
| done | completed, or canceled when the resolution is in `canceled_resolutions` |

How each operation works:
- **Transitions:** issues move only through the workflow's available transitions. The provider takes the transition reaching the `status_map` status when one is set, otherwise the first whose target is in the right category. When none fits, the error is `invalid_transition`, with the reachable statuses in `allowed`.
- **Cancelling:** `canceled` uses a done transition and sets the first `canceled_resolutions` value when the transition screen has a Resolution field.
- **API version:** the plugin uses REST v3. Descriptions and comments are sent as Atlassian Document Format (paragraphs, line breaks, inline code and links).
- **Idempotency:** a create carries the label `sdd-cref-<client_ref>`, and a create first searches for that label, three times over about 6 seconds, because Jira's search index lags. That label is hidden from `labels`.
- **sdd links:** the `sdd` links are stored in the issue property `sdd`. Issue links (`blocks`, …) and priority are stored in the issue property `sdd-precedence`.

## Writing a provider

See "Writing a provider with the kit" in [tracker-protocol.md](tracker-protocol.md). Run `qrtx tracker check --backend <name> --allow-writes` against a scratch project before publishing. Against a plugin it waits 10 seconds before retrying the create, as a real retry would (`QRTX_CHECK_RETRY_DELAY` changes the wait).

Already run against real services: GitHub (2026-11-26). Jira Cloud is tested only against the API fake so far.
