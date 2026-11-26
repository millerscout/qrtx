# File formats

## Change manifest: `sdd/changes/<slug>/change.sdd.md`

`sdd change new <slug>` writes the template. The first block is metadata:

````markdown
```change
name: <slug>
status: planning        # planning | complete
packages: []            # package dirs with a contract.sdd.md that this change touches
issues: ["file:sdd-k3f9qa"]  # optional: linked tracker issues, <backend>:<id>
```
````

Sections, in template order:

| Section | Contents |
|---|---|
| `## Why` | The user's problem and the chosen approach. |
| `## User request` | What the user asked for, in their terms. |
| `## Acceptance` | User-visible checks that would satisfy the requester. |
| `## Requirements` | `- Q<n> <subject> SHALL <behaviour>.` One per concern. |
| `## Scenarios` | `- S<n> (Q<n>) WHEN <condition> THEN <observable result>`. At least one per requirement. |
| `## Decisions` | `- D<n> <choice>. Rejected: <alternative> because <reason>.` or `- None: <reason>`. |
| `## Risks` | `- <risk> → <mitigation>`, then `Deploy:` and `Rollback:` lines (`n/a (<reason>)` when nothing persists). |
| `## Observed` | What was read or run, with paths and commands. Kept apart from assumptions. |
| `## Assumptions`, `## Non-goals`, `## Deferred` | Open assumptions, explicit boundaries, other slugs that own split work. |
| `## Tasks` | Checkbox tasks, or a roadmap of `### Phase` headings pointing at `phases/<nn-name>.md`. Each task ends with its check and may cite `covers S1,S2`. |
| `## Validate` | `- [ ] V<n> <command>` lines that `run-validate` executes, or `manual:` steps. |
| `## Constitution` | Only in repositories with a constitution: `- C<n> <rule> → <how this plan complies>` per rule the change touches, or `- None: <why no rule applies>`. |
| `## Review` | `- R<n> <claim ids> <conflict> → <fix>` for each finding from the review packet, or `- None: <what was checked>`. |

Roadmap phases carry an attribute line that apply reads:

```markdown
### Phase add the board
{: id="phase-01" status="review" file="phases/01-add-the-board.md"}
```

`status="review"` waits for the user; apply implements only `status="accepted"` phases.

`sdd change verify` rejects template `(TBD …)` text left in a required section, a scenario with no requirement and a requirement with no scenario. `finish-propose` and `finish-apply` add the coverage and review checks described in the skills.

## Constitution

A constitution is the project's standing rules: what every change must respect, however small. It is one or more Markdown files: `sdd/constitution.md` by default, or the files listed under `constitution:` in `.sdd.yaml`. `sdd constitution init` writes a starter file.

Each rule is a heading. In each file, sdd takes whichever of `##` and `###` appears more often (ties go to `##`), ignoring headings inside code fences. So both of these layouts work. One heading per rule:

```markdown
## Rule 1: Tests before code
## Rule 2: No silent data loss
```

Or rules grouped under sections (the `###` headings are the rules):

```markdown
## Purpose
## Rules
### Rule 1: Tests before code
### Rule 2: No silent data loss
### Rule 3: Public contracts stay compatible
```

A plan cites a rule by its full heading, or by the part before the first `:`, ` — ` or ` - ` (`Rule 1`, `Article IX`). `sdd constitution show` lists the rules, and `propose-context` and the review packet include them.

`sdd change new` adds a `## Constitution` section to the template when the repository has a constitution. `finish-propose` then requires that section, with:

- one or more `- C<n> <rule> → <how the plan complies>` lines, each naming a real rule; or
- a single `- None: <why no rule applies>`.

`verify` and `archive` do not check the section, so changes created before the constitution existed still archive.

## Package contract: `<package>/contract.sdd.md`

Markdown with one fenced `sdd` block:

````markdown
# store

```sdd
package: store
exports:
  - name: Save
    kind: func
    signature: "func Save(path string, data []byte) error"
scenarios:
  - id: atomic
    when: the process crashes during Save
    then: the file holds the previous or the new content, never a mix
```

## Behaviour

### 2026-11-24 add-store-backups

- add-store-backups/Q1 Save SHALL keep the previous file as a backup.
- add-store-backups/S1 (Q1) WHEN Save succeeds THEN the previous content is in store.bak
````

`sdd verify-ast` reports drift between `exports` and the package's exported Go declarations; `sdd sync-from-code` rewrites `exports` from the code. `## Behaviour` is appended by `sdd change archive`, with each id prefixed by the change slug.

## Issues: `sdd/issues.jsonl`

The built-in tracker backend keeps one issue per line, in the schema of [`schema/tracker/v1/issue.json`](schema/tracker/v1/issue.json):

```json
{"client_ref":"3f9c0a1b2c3d","id":"sdd-k3f9qa","sdd":[{"change":"dark-mode","project":"shop","repo":"github.com/acme/shop"}],"status":{"category":"started"},"title":"Dark-mode toggle","updated_at":"2026-11-26T12:00:00Z"}
```

- **Order:** lines are sorted by `id`, and each line has its keys in sorted order. sdd rewrites only the line of the issue it changes.
- **IDs:** an `id` is `sdd-` plus six characters of a hash, so two branches adding issues do not collide.
- **Merging:** `sdd tracker init` registers a merge driver that merges two branches issue by issue.
- **Hand edits:** you may edit the file by hand. sdd skips a malformed line with a warning (`sdd/issues.jsonl:<line>`) and refuses to rewrite the file until you fix it.

See [tracker-protocol.md](tracker-protocol.md) for other backends.
