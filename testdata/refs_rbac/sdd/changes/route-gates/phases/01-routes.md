# Phase 1 — Unauthenticated routes

- [ ] 1.1 Wrap each route with `withAuthAndPermission`. Mapping: `pipelines` GET → `records.read`, POST → `custom_field.manage.*`; `companies` POST → `company.add`; `tasks/[id]/skip` POST → `task.skip`. covers S1 — verify: `npm run test:rbac`.
