# sdd

Spec-driven development for coding agents. Before an agent writes code, `sdd` has it write a plan you can review: what you asked for, how you'll check it, the cases that can fail, the decisions made, and the tasks. A command-line tool checks that the plan is complete and consistent. Apply then works through the tasks, and archive files the finished change and records its promises next to the code.

```
sdd/changes/<slug>/change.sdd.md      the plan for one change
sdd/changes/<slug>/phases/*.md        task lists, one per phase
<package>/contract.sdd.md             optional: a package's exported API and promised behaviour
```

## Quick start

```bash
go install github.com/millerscout/sdd/cmd/sdd@latest
cd your-project
sdd skills install          # writes .claude/skills/sdd-{propose,apply,archive}
```

Then, in Claude Code (or any agent that reads `.claude/skills`):

```
/sdd-propose add a dark-mode toggle to the settings page
/sdd-apply dark-mode-toggle
/sdd-archive dark-mode-toggle
```

You can also use the CLI on its own:

```bash
sdd change new dark-mode-toggle        # scaffold the manifest
sdd change propose-context --change dark-mode-toggle
sdd change verify --change dark-mode-toggle
sdd change review-packet --change dark-mode-toggle
sdd change finish-propose --change dark-mode-toggle
```

See [`examples/tic-tac-toe`](examples/tic-tac-toe) for a complete planned change.

## How it works

1. **Propose** (`/sdd-propose`). The agent writes the user request, acceptance checks, requirements (`Q1 … SHALL …`), scenarios (`S1 (Q1) WHEN … THEN …`), decisions with rejected alternatives, risks, deploy and rollback notes, and tasks that cite the scenarios they cover. `sdd change review-packet` produces a summary for a second reader, and `finish-propose` refuses an uncovered scenario, an unanswered review hint or a leftover template line.
2. **Apply** (`/sdd-apply`). The agent implements accepted phases in order and checks off each task when its test passes. `finish-apply` checks scenario coverage, contract drift and the `Validate` commands.
3. **Archive** (`/sdd-archive`). The change moves to `sdd/changes/archive/`, and its requirements and scenarios are appended to the `## Behaviour` section of each touched package's contract. The next proposal for that package sees those promises.

## Commands

| Command | What it does |
|---|---|
| `sdd change new <slug>` | Create `sdd/changes/<slug>/change.sdd.md` from the template |
| `sdd change list` / `status --change <slug>` | Active changes; task progress and open phase as JSON |
| `sdd change propose-context --change <slug> [--json]` | Overlapping changes, related code, package graph, earlier promises |
| `sdd change verify` / `score --change <slug>` | Check the manifest; score how complete the spec is |
| `sdd change review-packet --change <slug> [--json]` | Claims, cited code and mechanical hints for a second reader |
| `sdd change finish-propose` / `finish-apply --change <slug>` | The gates at the end of propose and apply |
| `sdd change run-validate --change <slug>` | Run unchecked `Validate` commands (allowlisted) |
| `sdd change archive <slug> [--run-validate] [--quick-check]` | Archive a complete change |
| `sdd change lint-active` | Check every active change |
| `sdd graph` | Write `.sdd-graph.json`: import edges between packages that have contracts |
| `sdd verify-ast [--package <dir>]` | Compare `contract.sdd.md` exports with the Go code |
| `sdd sync-from-code [--package <dir>] [--drifted-only]` | Rewrite contract exports from the code |
| `sdd skills list` / `install [--dir <dir>] [--force]` | Install the bundled agent skills |

## Configuration

An optional `.sdd.yaml` at the repository root:

```yaml
validate_allow:            # extra command prefixes run-validate may execute
  - scripts/check.sh
quick_check: make test     # run by `change archive --quick-check`
verify_packages:           # default packages for verify-ast and sync-from-code
  - internal/store
```

`SDD_REPO` overrides the repository root (otherwise the nearest `go.mod`, or the working directory). See [docs/config.md](docs/config.md).

## Languages and builds

Manifests work in any project. Contracts, drift checks and the graph read Go code. With cgo enabled (the default when a C compiler is present), the review packet also finds definitions in TypeScript, JavaScript, Python, Rust and Swift via tree-sitter. `CGO_ENABLED=0 go install …` builds a pure-Go binary that indexes Go only.

## Embedding

`sdd` is also a Go library. A host can plug in a code index, project context (open issues, known fixes) and LLM classifiers; see [docs/embedding.md](docs/embedding.md). Only the APIs listed in the package doc are stable before v1.

## License

Apache-2.0. See [LICENSE](LICENSE).
