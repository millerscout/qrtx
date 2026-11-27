# qrtx

Spec-driven development for coding agents. Before an agent writes code, `qrtx` has it write a plan you can review: what you asked for, how you'll check it, the cases that can fail, the decisions made, and the tasks. A command-line tool checks that the plan is complete and consistent. Apply then works through the tasks, and archive files the finished change and records its promises next to the code.

```
sdd/changes/<slug>/change.sdd.md      the plan for one change
sdd/changes/<slug>/phases/*.md        task lists, one per phase
<package>/contract.sdd.md             optional: a package's exported API and promised behaviour
```

The tool is qrtx. The files, the `.sdd.yaml` config and the `sdd-*` skills keep the `sdd` name, because they belong to the method (spec-driven development), not to the tool.

## Quick start

```bash
go install github.com/millerscout/qrtx/cmd/qrtx@latest
cd your-project
qrtx init                    # skills, a starter sdd/constitution.md, the issues merge driver
```

`qrtx init` is safe to re-run. Add `--tracker github --repo owner/name --token-command "gh auth token"` (or `--tracker jira …`) to connect a tracker as well. See [docs/providers.md](docs/providers.md).

Then, in Claude Code (or any agent that reads `.claude/skills`):

```
/sdd-propose add a dark-mode toggle to the settings page
/sdd-apply dark-mode-toggle
/sdd-archive dark-mode-toggle
```

You can also use the CLI on its own:

```bash
qrtx change new dark-mode-toggle        # scaffold the manifest
qrtx change propose-context --change dark-mode-toggle
qrtx change verify --change dark-mode-toggle
qrtx change review-packet --change dark-mode-toggle
qrtx change finish-propose --change dark-mode-toggle
```

See [`examples/tic-tac-toe`](examples/tic-tac-toe) for a complete planned change.

## How it works

1. **Propose** (`/sdd-propose`). The agent writes the user request, acceptance checks, requirements (`Q1 … SHALL …`), scenarios (`S1 (Q1) WHEN … THEN …`), decisions with rejected alternatives, risks, deploy and rollback notes, and tasks that cite the scenarios they cover. `qrtx change review-packet` produces a summary for a second reader, and `finish-propose` refuses an uncovered scenario, an unanswered review hint or a leftover template line.
   If the project has a constitution, the plan also says under `## Constitution` how it meets each rule it touches, and `finish-propose` checks that every line names a real rule.
2. **Apply** (`/sdd-apply`). The agent implements accepted phases in order and checks off each task when its test passes. `finish-apply` checks scenario coverage, contract drift and the `Validate` commands.
3. **Archive** (`/sdd-archive`). The change moves to `sdd/changes/archive/`, and its requirements and scenarios are appended to the `## Behaviour` section of each touched package's contract. The next proposal for that package sees those promises.

## Commands

| Command | What it does |
|---|---|
| `qrtx change new <slug>` | Create `sdd/changes/<slug>/change.sdd.md` from the template |
| `qrtx change list` / `status --change <slug>` | Active changes; task progress and open phase as JSON |
| `qrtx change propose-context --change <slug> [--json]` | Overlapping changes, related code, package graph, earlier promises |
| `qrtx change verify` / `score --change <slug>` | Check the manifest; score how complete the spec is |
| `qrtx change review-packet --change <slug> [--json]` | Claims, cited code and mechanical hints for a second reader |
| `qrtx change finish-propose` / `finish-apply --change <slug>` | The gates at the end of propose and apply |
| `qrtx change run-validate --change <slug>` | Run unchecked `Validate` commands (allowlisted) |
| `qrtx change archive <slug> [--run-validate] [--quick-check]` | Archive a complete change |
| `qrtx change lint-active` | Check every active change |
| `qrtx graph` | Write `.sdd-graph.json`: import edges between packages that have contracts |
| `qrtx verify-ast [--package <dir>]` | Compare `contract.sdd.md` exports with the Go code |
| `qrtx sync-from-code [--package <dir>] [--drifted-only]` | Rewrite contract exports from the code |
| `qrtx skills list` / `install [--dir <dir>] [--force]` | Install the bundled agent skills |
| `qrtx constitution show [--json]` / `init [--force]` | List the project's rules; write a starter `sdd/constitution.md` |
| `qrtx init [--tracker <backend> …]` | Set up a repository or folder: skills, constitution, merge driver, tracker |
| `qrtx tracker setup github\|jira [flags]` | Configure a tracker and bind its credentials |
| `qrtx tracker add` / `list` / `get` / `close` | The project backlog: `sdd/issues.jsonl` by default, or any tracker plugin |
| `qrtx tracker start <ref> <slug>` | Turn an issue into a change and link them both ways |
| `qrtx tracker from-deferred --change <slug>` | Put a change's `## Deferred` work in the backlog |
| `qrtx sync heartbeat` / `claim` / `inbox` / `status` | Several sessions in one repo: leases, claims, and an inbox of what the others did ([docs/sync.md](docs/sync.md)) |
| `qrtx tracker outbox` / `check` / `init` | Pending tracker writes; plugin conformance check; register the `issues.jsonl` merge driver |

## Backlog and issue trackers

Work that is not a change yet lives in a tracker:

```bash
qrtx tracker init                         # once per clone: registers the issues.jsonl merge driver
qrtx tracker add "Dark-mode toggle"       # added file:sdd-k3f9qa
qrtx tracker start file:sdd-k3f9qa dark-mode
```

- **Default backend:** the built-in `file` backend, `sdd/issues.jsonl`, with one JSON issue per line. It is committed with your code, and branches merge cleanly.
- **Other backends:** GitHub and Jira have first-party plugins (`qrtx tracker setup github|jira`, see [docs/providers.md](docs/providers.md)). Anything else works through any `qrtx-tracker-<name>` on `PATH` that speaks [the tracker protocol](docs/tracker-protocol.md). Tokens live in your user-level credentials file, bound to a host and an allow-list of repos or projects, never in `.sdd.yaml`.
- **Links:** a change links its issues under `issues:`, and archiving it marks them completed. Each issue records the repo, project and change that worked on it.

## Several sessions at once

Run `qrtx skills install --hooks` once per clone. Claude Code sessions then keep a one-hour lease while they work, claim the tasks and issues they pick up, and see in each prompt what the other sessions claimed, checked off or changed in the tracker. A small daemon handles the timers and exits when every session is idle. See [docs/sync.md](docs/sync.md).

## Configuration

An optional `.sdd.yaml` at the repository root:

```yaml
validate_allow:            # extra command prefixes run-validate may execute
  - scripts/check.sh
quick_check: make test     # run by `change archive --quick-check`
verify_packages:           # default packages for verify-ast and sync-from-code
  - internal/store
constitution:              # rule files plans must comply with (default: sdd/constitution.md)
  - docs/RULES.md
```

`QRTX_REPO` overrides the repository root (otherwise the nearest `go.mod`, or the working directory). See [docs/config.md](docs/config.md).

## Languages and builds

Manifests work in any project. Contracts, drift checks and the graph read Go code. With cgo enabled (the default when a C compiler is present), the review packet also finds definitions in TypeScript, JavaScript, Python, Rust and Swift via tree-sitter. `CGO_ENABLED=0 go install …` builds a pure-Go binary that indexes Go only.

## Embedding

`qrtx` is also a Go library. A host can plug in a code index, project context (open issues, known fixes) and LLM classifiers; see [docs/embedding.md](docs/embedding.md). Only the APIs listed in the package doc are stable before v1.

## License

Apache-2.0. See [LICENSE](LICENSE).
