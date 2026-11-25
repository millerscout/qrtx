```change
name: failure-decided
status: planning
packages: []
```

## Why

The chosen approach is a regression that fails on the reported failure and passes after the fix. The approach set aside is leaving the plan on an open question.

## User request

the search drops results

## Acceptance

- A regression test fails on the reported failure and passes after the fix.

## Observed

- `docs/read.md` is in this tree.

## Tasks

- [ ] 1.1 Add a regression test that fails on the reported failure and passes after the fix.

## Validate

- [ ] V1.1 go test ./internal/sdd/ -count=1
