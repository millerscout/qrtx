```change
name: intent-bad-missing-acceptance
status: planning
packages: []
```

## Why

Golden fixture: missing Acceptance fails verify.

## User request

Operator wants failing manifests caught.

## Observed

- `internal/sdd/change_validation.go` requires Acceptance.

## Assumptions

- No acceptance section on purpose.

## Tasks

- [ ] 1.1 Example task

## Validate

- [ ] V1.1 go test ./internal/sdd/... -run TestManifestQuality

## Non-goals

- Passing verify.
