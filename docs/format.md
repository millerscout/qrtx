# File formats

## Change manifest: `sdd/changes/<slug>/change.sdd.md`

`sdd change new <slug>` writes the template. The first block is metadata:

````markdown
```change
name: <slug>
status: planning        # planning | complete
packages: []            # package dirs with a contract.sdd.md that this change touches
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
| `## Review` | `- R<n> <claim ids> <conflict> → <fix>` for each finding from the review packet, or `- None: <what was checked>`. |

Roadmap phases carry an attribute line that apply reads:

```markdown
### Phase add the board
{: id="phase-01" status="review" file="phases/01-add-the-board.md"}
```

`status="review"` waits for the user; apply implements only `status="accepted"` phases.

`sdd change verify` rejects template `(TBD …)` text left in a required section, a scenario with no requirement and a requirement with no scenario. `finish-propose` and `finish-apply` add the coverage and review checks described in the skills.

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
