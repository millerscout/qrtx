```change
name: intent-observed-tbd
status: planning
packages: []
```

## Why

Golden fixture: Observed is still a placeholder.

## User request

Operator wants a placeholder Observed section to score below a plan that read a file.

## Acceptance

- The score stays below a plan whose Observed path exists.

## Observed

(TBD — repro, logs, code path. Separate from assumptions.)

## Assumptions

- A placeholder is not a file read.

## Tasks

- [ ] 1.1 Leave Observed as a placeholder.

## Validate

- [ ] V1.1 go test ./internal/sdd/ -run TestSddChangeManifestQualityGoldenJSONL

## Non-goals

- Starting a search.

## Deferred
