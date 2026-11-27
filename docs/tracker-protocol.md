# Tracker protocol (`sdd.adapter/v1`, kind `tracker`)

qrtx keeps a project's backlog in an issue tracker. The tracker can be the built-in `file` backend (`sdd/issues.jsonl`), or any program that implements this protocol: Jira, GitHub Issues, Linear, GitLab, or your own. This document and the JSON Schemas in [`schema/tracker/v1/`](schema/tracker/v1/) are the whole contract. You do not need to read qrtx's Go code to write a plugin.

The protocol is **unstable until qrtx v1**.

## Choosing a backend

```yaml
# .sdd.yaml
project: shop                  # optional; names this project in issue refs
tracker:
  backend: github              # file (default) | <name> → qrtx-tracker-<name> on PATH
  config: { repo: owner/name } # passed verbatim to the plugin; never secrets
  status_map:                  # optional: exact native status per category
    started: In Progress
    completed: Done
  comments: summary            # summary | minimal | off (archive comments)
  timeout: 30                  # seconds per plugin call
```

- **Names:** a backend name matches `^[a-z0-9-]+$`, and qrtx finds the program as `qrtx-tracker-<name>` on `PATH` only. qrtx never runs a path from inside the repository.
- **Credentials:** qrtx never puts secrets in the request. A plugin resolves them per host and repo/project from the user-level credentials file (see Credentials below), never from `.sdd.yaml`.
- **Mirror:** `mirror` is reserved. Only `change` (one issue per change) is implemented.

## Calling convention

qrtx starts the plugin once per operation, as `qrtx-tracker-<name> <op>`, and writes one JSON request to its stdin:

```json
{"protocol":"sdd.adapter/v1","kind":"tracker","op":"get_issue","config":{"repo":"owner/name"},"args":{"id":"123"}}
```

The plugin writes one JSON response to stdout and exits:

```json
{"result": { …issue… }}
{"error": {"code": "not_found", "message": "issue 123 does not exist"}}
```

- **Exit code:** 0 with `result`, or 1 with `error`. Anything else on stdout, or a missing response, counts as a failure.
- **stderr:** shown to the user; use it for diagnostics.
- **Timeout:** qrtx kills a call after the timeout (30s by default).

### Error codes

| Code | Meaning | Does qrtx retry a queued write? |
|---|---|---|
| `not_found` | the issue does not exist | no |
| `unsupported` | the operation is not implemented | no |
| `auth` | credentials are missing or rejected | yes |
| `invalid` | the arguments are wrong | no |
| `invalid_transition` | the workflow cannot reach the category; set `allowed` to the native statuses it can reach | no |
| `rate_limited` | try again later; set `retry_after` in seconds | yes |
| `unavailable` | network or server failure | yes |

## Operations

qrtx always calls `capabilities` first, and never calls an operation the plugin does not list in `ops`.

| Op | `args` | `result` |
|---|---|---|
| `capabilities` | `{}` | `{"protocols":["sdd.adapter/v1"],"interfaces":["tracker@v1"],"ops":[…],"link_types":[…]}` |
| `list_issues` | `{"filter":{"categories":[…],"labels":[…],"assignee":"","text":"","change":"","limit":0,"cursor":""}}` | `{"issues":[…],"cursor":"…"}` |
| `get_issue` | `{"id":"…"}` | an issue |
| `create_issue` | `{"title","description","category","labels","sdd":[…],"priority","client_ref","fresh"}` | the issue |
| `update_issue` | `{"id","title","description","labels","add_sdd":[…],"add_links":[…],"remove_links":[…],"priority"}`; absent fields are left alone | the issue |
| `transition_issue` | `{"id","category","native"}`; `native` is set when `status_map` names one | the issue |
| `comment` | `{"id","markdown","client_ref"}` | `{}` |

### Version negotiation

`protocols` lists every envelope version the plugin speaks. qrtx uses the newest version both sides know. When there is none, qrtx stops with a message that names both lists.

### Idempotent create (MUST)

qrtx queues every write in a local outbox and may resend it after a crash or timeout. `create_issue` carries a `client_ref`.
- **Before creating,** a plugin MUST look for an existing issue with that `client_ref` and return it if one exists.
- **`fresh`:** when `true`, qrtx has never sent this `client_ref` before, so the plugin MAY skip that lookup. qrtx never sets it on a resend. A plugin that ignores it stays correct, only slower.
- **Where to keep it:** store the `client_ref` in the sdd marker (below) or in a backend field.
- **Checking it:** `qrtx tracker check --allow-writes` creates twice with the same `client_ref` and fails the plugin if two issues come back.

`comment` carries a `client_ref` too. Plugins SHOULD skip a comment whose `client_ref` is already on the issue.

### Pagination

`list_issues` returns at most `limit` issues (all of them when `limit` is 0) and an opaque `cursor` when more exist. qrtx sends the cursor back unchanged. Plugins handle the vendor's rate limits themselves, and return `rate_limited` only when they give up.

## The issue

See [`schema/tracker/v1/issue.json`](schema/tracker/v1/issue.json). The required fields are `id`, `title` and `status.category`.

- **Status categories:** `status.category` is one of `backlog`, `unstarted`, `started`, `completed` and `canceled`, the same model as Linear's state types. `status.name` keeps the native status, for example "In Review".
- **Unknown fields:** a plugin keeps backend fields in `extra`. Unknown top-level keys and `extra` MUST survive a read-modify-write unchanged.
- **`sdd` refs:** each entry ties the issue to a repo, a project and a change, and can add `role`, `phase`, `tasks`, `scenarios`, `branch` and `commit`. `role: "deferred"` marks work a change split off for later. Archiving that change does not close it.

- **Links and priority:** `add_links` and `remove_links` are lists of `{"type":"blocks|relates|duplicates","id":"…"}`, held on the source issue (`A.links` has `{blocks, B}`; `id` is a bare id on the same backend). Both are idempotent. `priority` is 0 (none) to 4 (low), 1 being urgent; 0 clears it. A plugin that declares `update_issue` SHOULD store them and return them as the issue's `links` and `priority`. `qrtx tracker link`, `unlink` and `priority` read the returned issue and report a backend that did not store the change. `qrtx tracker start` lists the open issues and refuses to start an issue that an open one blocks, so `list_issues` SHOULD return `links` too (without `list_issues` the check is skipped with a warning).

### Keeping `sdd` refs in a backend that has no field for them

A plugin MUST round-trip `sdd` and `client_ref`. When the backend has no custom field for them, add a machine block at the end of the description and strip it again on read:

```markdown
<!-- sdd:v1 {"client_ref":"3f9c0a1b2c3d","sdd":[{"repo":"github.com/acme/shop","change":"dark-mode"}]} -->
```

Keep links and priority apart from the `sdd` marker, because a plugin that predates them rewrites the whole `sdd` marker or property and would erase them. The first-party plugins use a second marker line before the `sdd` one, and the Jira plugin an issue property of its own:

```markdown
<!-- sdd-prec:v1 {"priority":2,"links":[{"type":"blocks","id":"124"}]} -->
<!-- sdd:v1 {"client_ref":"3f9c0a1b2c3d","sdd":[…]} -->
```

The `sdd:v1` marker stays the last line. Jira keeps `{"priority":…,"links":[…]}` in the issue property `sdd-precedence`.

Labels such as `sdd:change/dark-mode` and `sdd:project/shop` are optional extras that let people filter in the tracker's own UI.

## Lifecycle

| qrtx command | Tracker writes (queued in the outbox, then sent) |
|---|---|
| `qrtx tracker start <ref> <slug>` | `transition_issue(started)` and `update_issue(add_sdd)` |
| `qrtx change archive <slug>` | `comment` (unless `comments: off`; dropped if the plugin has no `comment`), then `transition_issue(completed)` only when no other active change lists the issue in its `issues:` |
| `qrtx tracker from-deferred --change <slug>` | `create_issue` per `## Deferred` bullet, with `role: deferred` |

- **Direct writes:** `qrtx tracker link`, `unlink` and `priority` call `update_issue` at once, as `close` does, and check the issue it returns. They are not queued, because the re-read would run before the outbox drained.
- **Where the outbox lives:** in `<git common dir>/sdd/`, or in `.sdd/` outside git.
- **Draining:** each `qrtx tracker` command drains the outbox before and after its own work.
- **Watching it:** `qrtx tracker outbox` shows what is pending.

## Credentials

`.sdd.yaml` is committed, so it must not be able to steer a token. Plugins built with the kit read the user's `credentials.yaml` and use a credential only when all three hold:
- the file binds the exact API host (and port, when the URL has one);
- the binding's `scopes` lists the repo or project the request is for;
- no redirect changes the scheme, host or port.

Bindings name an environment variable or a `token_command`, never a token. `token_command` gets `QRTX_CRED_HOST` and `QRTX_CRED_SCOPE`, so an OIDC or zero-trust broker can mint a short-lived, narrowly scoped token. Plugins written without the kit SHOULD follow the same rules. See [providers.md](providers.md#credentials).

qrtx sets `QRTX_TRACKER_DEADLINE` (RFC 3339) in the plugin's environment: the moment qrtx stops waiting. A plugin should not start a retry it cannot finish by then.

## Writing a provider with the kit

Go providers use `github.com/millerscout/qrtx/providers/kit`. It imports only the protocol package `github.com/millerscout/qrtx/tracker` (standard library only), not the rest of qrtx.

```go
type Provider struct{ client *kit.Client }

func (p *Provider) Init(config json.RawMessage) error {
	// decode config, then:
	c, err := kit.NewClient(apiURL, project, "qrtx-tracker-acme", kit.BearerAuth)
	p.client = c
	return err
}
// Capabilities, ListIssuesPage (returns a cursor), GetIssue, CreateIssue
// (idempotent per client_ref), UpdateIssue, TransitionIssue, Comment …

func main() { kit.Main(kit.Tracker(&Provider{})) }
```

What the kit provides:
- **`kit.Serve`/`kit.Main`:** checks the envelope, dispatches by `kind` and `op`, and turns errors into adapter errors. A plain Go error becomes `invalid`, and an `*tracker.AdapterError` is returned as it is.
- **`kit.Client`:** maps HTTP statuses to error codes, retries a failed GET/PUT/PATCH once within the deadline, and refuses cross-host redirects.
- **`tracker.EncodeMarker`/`DecodeMarker`:** keep `client_ref` and `sdd` links in a description when the backend has no field for them.

Test with `kittest.Run(t, provider, config)` against a fake of the API. It runs the same conformance sequence as `qrtx tracker check --allow-writes`. The GitHub and Jira providers in `providers/` are worked examples, with their API fakes in `providers/*/fakeapi`.

## Checking a plugin

```bash
qrtx tracker check --backend <name>                 # read-only: capabilities, list_issues
qrtx tracker check --backend <name> --allow-writes  # also create (twice), get, update, transition, comment
```

## Alignment with other work

- **ai-sdlc:** the operation names follow ai-sdlc's `IssueTracker@v1` (`listIssues`, `getIssue`, `createIssue`, `updateIssue`, `transitionIssue`), so a bridge to it is a thin mapping.
- **Linear and omnitrack:** the status categories match Linear's state types and omnitrack's normalized categories.

## Planned: Gherkin and test evidence tags

A later qrtx change exports scenarios as Gherkin. It will tag each scenario so that test results and screenshots can be traced back to issues:

```gherkin
@sdd:dark-mode/S3 @req:Q1 @issue:github:123 @project:shop
Scenario: …
```
