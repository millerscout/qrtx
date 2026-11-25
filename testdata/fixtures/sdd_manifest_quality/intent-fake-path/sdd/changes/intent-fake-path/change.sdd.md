```change
name: intent-fake-path
status: planning
packages: []
```

## Why

Golden fixture: Observed cites a path that is not in this tree.

## User request

Operator wants a missing path to score like a note that names no file.

## Acceptance

- The score does not treat a missing path as a file that was read.

## Observed

- `internal/sdd/not-a-file.go` is not in this fixture tree.

## Assumptions

- The scorer resolves paths against the fixture root.

## Tasks

- [ ] 1.1 Leave the cited path out of the fixture tree.

## Validate

- [ ] V1.1 go test ./internal/sdd/ -run TestSddChangeManifestQualityGoldenJSONL

## Non-goals

- Starting a search.

## Deferred
