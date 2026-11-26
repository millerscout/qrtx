```change
name: cites-directory
status: planning
packages: []
```

## Why

Golden fixture: Observed cites a file that exists in this tree.

## User request

Operator wants a proposal score only when the plan names a file that was read.

## Acceptance

- The score counts the Observed path when that file exists.

## Observed

- `docs/` is a folder in this fixture tree.

## Assumptions

- The scorer resolves paths against the fixture root.

## Tasks

- [ ] 1.1 Keep the cited file in the fixture tree.

## Validate

- [ ] V1.1 go test ./internal/sdd/ -run TestSddChangeManifestQualityGoldenJSONL

## Non-goals

- Starting a search.

## Deferred
