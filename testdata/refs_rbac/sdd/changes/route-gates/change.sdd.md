```change
name: route-gates
status: planning
packages: []
```

## Why

Some routes check permissions through a direct RPC and some not at all.

## User request

Gate every route with a permission code.

## Acceptance

- A1 Every route returns 403 for a user without its code.

## Requirements

- Q1 every route SHALL authorize the caller with a permission code.

## Scenarios

- S1 (Q1) WHEN a user without the code calls a route THEN the response is 403.

## Decisions

- D1 Reads gate on a new `records.read` code. Writes gate on the existing codes (`company.add`, `task.skip`, `custom_field.manage.*`). Rejected: one catch-all `records.manage`, because it would hand every write to everyone.
- D2 New codes go in migration 0004: `records.read`.
- D3 Direct `supabase.rpc('user_has_permission')` calls in admin routes are replaced by the helper.

## Risks

- Gating open routes can lock users out → migration 0004 grants parity first.

Rollback: revert the code change; leave migration 0004 in place.

## Observed

- `src/lib/api/utils.ts` holds `checkPermission` and `withAuthAndPermission`.

## Assumptions

- The local database is available.

## Non-goals

- UI hiding.

## Deferred

- None.

## Phases

### Phase 1 — Unauthenticated routes

status: review
tasks: sdd/changes/route-gates/phases/01-routes.md

### Phase 2 — Admin helper

status: review
tasks: sdd/changes/route-gates/phases/02-admin.md

### Phase 3 — Permission codes migration

status: review
tasks: sdd/changes/route-gates/phases/03-migration.md

## Tasks

Tasks live in `phases/NN-*.md`.

## Validate

- [ ] V1.1 Acceptance 1: `npm run test:rbac` — covers S1

## Review

- None: not reviewed yet.
