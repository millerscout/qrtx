# Configuration

## `.sdd.yaml`

Optional, at the repository root. Every key is optional.

| Key | Type | Used by | Default |
|---|---|---|---|
| `validate_allow` | list of command prefixes | `change run-validate`, `finish-apply`, `archive --run-validate` | none beyond the built-ins |
| `quick_check` | shell command | `change archive --quick-check` | unset; the flag fails |
| `constitution` | list of Markdown files | `constitution show`, `change new`, `propose-context`, `review-packet`, `finish-propose` | `sdd/constitution.md` when it exists |
| `verify_packages` | list of package dirs | `verify-ast`, `sync-from-code`, `StatusForRepo` when no package is named | every directory with a `contract.sdd.md` |

### Which `Validate` commands run

`run-validate` executes an unchecked `- [ ] V… <command>` line only when the command, after any leading `VAR=value` assignments, starts with one of:

- `go test `, `go run `
- `python3 -m unittest `, `python3 scripts/`
- `sdd ` or `./sdd ` (the CLI name)
- a `validate_allow` prefix

Lines marked `manual:` are skipped. Set `SDD_RUN_VALIDATE=1` to run any command. Manifests are trusted input, like a Makefile; see [SECURITY.md](../SECURITY.md).

## Environment

| Variable | Effect |
|---|---|
| `SDD_REPO` | Repository root. Otherwise the nearest directory with `go.mod`, else the working directory. |
| `SDD_RUN_VALIDATE=1` | `run-validate` runs commands that are not allowlisted. |
| `SDD_PACKET_PLANS`, `SDD_PACKET_OUT` | Test-only: `TestReviewPacketSnapshot` writes review packets for a glob of plans, to compare two builds. |

## Files `sdd` writes

- `sdd/changes/<slug>/` — new changes; `sdd/changes/archive/<date>-<slug>/` after archive.
- `<package>/contract.sdd.md` — `## Behaviour` appended on archive; exports rewritten by `sync-from-code`.
- `.sdd-graph.json` — written by `sdd graph` and `propose-context --refresh-graph`.
- `.claude/skills/sdd-*/SKILL.md` — written by `sdd skills install`.
- `sdd/constitution.md` — written by `sdd constitution init`.
