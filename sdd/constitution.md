# Constitution

Standing rules for changes to `github.com/millerscout/qrtx`. Each `##` heading is
a rule; a plan's `## Constitution` section says how it meets each rule it touches
(`qrtx constitution show`).

## Rule 1: sdd plans with sdd

Every change to this repository starts as an sdd plan in this repository:
`sdd/changes/<slug>/change.sdd.md`, written with `/sdd-propose` (or
`qrtx change new` plus `propose-context`) and passed through
`qrtx change finish-propose` before any code changes. Apply it with `/sdd-apply`
and file it with `/sdd-archive`, so the finished change lands in
`sdd/changes/archive/` and its promises reach the package contract.

This holds when the request comes from another project. A project that needs a
qrtx change (a new flag, a bench row, a fixture) names it in its own plan as a
dependency; the work itself is planned here, not as task lines in that
project's plan. The other project picks it up with `go get` after the release.

Exempt: release tags, CHANGELOG entries for a change already archived here, and
typo-only edits to docs. Say which exemption applies in the commit message.
