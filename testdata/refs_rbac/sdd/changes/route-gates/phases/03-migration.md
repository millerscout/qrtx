# Phase 3 — Permission codes migration

- [ ] 3.1 Write `db/migrations/0004_codes.sql`. Insert `records.read` with `on conflict (code) do nothing`. covers S1 — verify: run it twice; the second run inserts 0 rows.
