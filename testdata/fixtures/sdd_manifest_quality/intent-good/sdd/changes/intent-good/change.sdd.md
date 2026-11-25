```change
name: intent-good
status: planning
packages: []
```

## Why

Golden fixture: intent-first manifest passes verify and scores high.

## User request

Operator wants SDD propose quality measured with a deterministic rubric.

## Acceptance

- `sdd change verify` passes on fixture manifests marked good.
- Quick gate runs manifest quality golden without a model.

## Observed

- `internal/sdd/change_validation.go` enforces sections.
- `scripts/golden_regression.sh` runs SDD families.

## Assumptions

- Fixture lives under `scripts/fixtures/sdd_manifest_quality/`.

## Tasks

- [ ] 1.1 Wire rubric in `internal/sdd/manifest_quality.go`

## Validate

- [ ] V1.1 go test ./internal/sdd/... -run TestSddChangeManifestQualityGoldenJSONL

## Non-goals

- Live model propose replay.

## Deferred
