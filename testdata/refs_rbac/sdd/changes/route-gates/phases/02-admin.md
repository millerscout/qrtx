# Phase 2 — Admin helper

- [ ] 2.1 Check the claim that `admin/roles` still calls `user_has_permission` directly. Move each to `withAuthAndPermission`. covers S1 — verify: `grep -rn "user_has_permission" src/app/api/admin` prints nothing.
