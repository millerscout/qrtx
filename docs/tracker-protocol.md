# Tracker protocol (`sdd.adapter/v1`, kind `tracker`)

sdd keeps a project's backlog in an issue tracker. The tracker can be the built-in `file` backend (`sdd/issues.jsonl`), or any program that implements this protocol: Jira, GitHub Issues, Linear, GitLab, or your own. This document and the JSON Schemas in [`schema/tracker/v1/`](schema/tracker/v1/) are the whole contract. You do not need to read sdd's Go code to write a plugin.

The protocol is **unstable until sdd v1**.

## Choosing a backend

```yaml
# .sdd.yaml
project: shop                  # optional; names this project in issue refs
tracker:
  backend: github              # file (default) | <name> → sdd-tracker-<name> on PATH
  config: { repo: owner/name } # passed verbatim to the plugin; never secrets
  status_map:                  # optional: exact native status per category
    started: In Progress
    completed: Done
  comments: summary            # summary | minimal | off (archive comments)
  timeout: 30                  # seconds per plugin call
```

- **Names:** a backend name matches `^[a-z0-9-]+$`, and sdd finds the program as `sdd-tracker-<name>` on `PATH` only. sdd never runs a path from inside the repository.
- **Credentials:** a plugin reads its credentials from its own environment or from the CLI it wraps. sdd never puts secrets in the request.
- **Mirror:** `mirror` is reserved. Only `change` (one issue per change) is implemented.

## Calling convention

sdd starts the plugin once per operation, as `sdd-tracker-<name> <op>`, and writes one JSON request to its stdin:

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
- **Timeout:** sdd kills a call after the timeout (30s by default).

### Error codes

| Code | Meaning | Does sdd retry a queued write? |
|---|---|---|
| `not_found` | the issue does not exist | no |
| `unsupported` | the operation is not implemented | no |
| `auth` | credentials are missing or rejected | yes |
| `invalid` | the arguments are wrong | no |
| `invalid_transition` | the workflow cannot reach the category; set `allowed` to the native statuses it can reach | no |
| `rate_limited` | try again later; set `retry_after` in seconds | yes |
| `unavailable` | network or server failure | yes |

## Operations

sdd always calls `capabilities` first, and never calls an operation the plugin does not list in `ops`.

| Op | `args` | `result` |
|---|---|---|
| `capabilities` | `{}` | `{"protocols":["sdd.adapter/v1"],"interfaces":["tracker@v1"],"ops":[…],"link_types":[…]}` |
| `list_issues` | `{"filter":{"categories":[…],"labels":[…],"assignee":"","text":"","change":"","limit":0,"cursor":""}}` | `{"issues":[…],"cursor":"…"}` |
| `get_issue` | `{"id":"…"}` | an issue |
| `create_issue` | `{"title","description","category","labels","sdd":[…],"client_ref"}` | the issue |
| `update_issue` | `{"id","title","description","labels","add_sdd":[…]}`; absent fields are left alone | the issue |
| `transition_issue` | `{"id","category","native"}`; `native` is set when `status_map` names one | the issue |
| `comment` | `{"id","markdown","client_ref"}` | `{}` |

### Version negotiation

`protocols` lists every envelope version the plugin speaks. sdd uses the newest version both sides know. When there is none, sdd stops with a message that names both lists.

### Idempotent create (MUST)

sdd queues every write in a local outbox and may resend it after a crash or timeout. `create_issue` carries a `client_ref`.
- **Before creating,** a plugin MUST look for an existing issue with that `client_ref` and return it if one exists.
- **Where to keep it:** store the `client_ref` in the sdd marker (below) or in a backend field.
- **Checking it:** `sdd tracker check --allow-writes` creates twice with the same `client_ref` and fails the plugin if two issues come back.

`comment` carries a `client_ref` too. Plugins SHOULD skip a comment whose `client_ref` is already on the issue.

### Pagination

`list_issues` returns at most `limit` issues (all of them when `limit` is 0) and an opaque `cursor` when more exist. sdd sends the cursor back unchanged. Plugins handle the vendor's rate limits themselves, and return `rate_limited` only when they give up.

## The issue

See [`schema/tracker/v1/issue.json`](schema/tracker/v1/issue.json). The required fields are `id`, `title` and `status.category`.

- **Status categories:** `status.category` is one of `backlog`, `unstarted`, `started`, `completed` and `canceled`, the same model as Linear's state types. `status.name` keeps the native status, for example "In Review".
- **Unknown fields:** a plugin keeps backend fields in `extra`. Unknown top-level keys and `extra` MUST survive a read-modify-write unchanged.
- **`sdd` refs:** each entry ties the issue to a repo, a project and a change, and can add `role`, `phase`, `tasks`, `scenarios`, `branch` and `commit`. `role: "deferred"` marks work a change split off for later. Archiving that change does not close it.

### Keeping `sdd` refs in a backend that has no field for them

A plugin MUST round-trip `sdd` and `client_ref`. When the backend has no custom field for them, add a machine block at the end of the description and strip it again on read:

```markdown
<!-- sdd:v1 {"client_ref":"3f9c0a1b2c3d","sdd":[{"repo":"github.com/acme/shop","change":"dark-mode"}]} -->
```

Labels such as `sdd:change/dark-mode` and `sdd:project/shop` are optional extras that let people filter in the tracker's own UI.

## Lifecycle

| sdd command | Tracker writes (queued in the outbox, then sent) |
|---|---|
| `sdd tracker start <ref> <slug>` | `transition_issue(started)` and `update_issue(add_sdd)` |
| `sdd change archive <slug>` | `comment` (unless `comments: off`; dropped if the plugin has no `comment`), then `transition_issue(completed)` only when no other active change lists the issue in its `issues:` |
| `sdd tracker from-deferred --change <slug>` | `create_issue` per `## Deferred` bullet, with `role: deferred` |

- **Where the outbox lives:** in `<git common dir>/sdd/`, or in `.sdd/` outside git.
- **Draining:** each `sdd tracker` command drains the outbox before and after its own work.
- **Watching it:** `sdd tracker outbox` shows what is pending.

## Checking a plugin

```bash
sdd tracker check --backend <name>                 # read-only: capabilities, list_issues
sdd tracker check --backend <name> --allow-writes  # also create (twice), get, update, transition, comment
```

## Alignment with other work

- **ai-sdlc:** the operation names follow ai-sdlc's `IssueTracker@v1` (`listIssues`, `getIssue`, `createIssue`, `updateIssue`, `transitionIssue`), so a bridge to it is a thin mapping.
- **Linear and omnitrack:** the status categories match Linear's state types and omnitrack's normalized categories.

## Planned: Gherkin and test evidence tags

A later sdd change exports scenarios as Gherkin. It will tag each scenario so that test results and screenshots can be traced back to issues:

```gherkin
@sdd:dark-mode/S3 @req:Q1 @issue:github:123 @project:shop
Scenario: …
```
