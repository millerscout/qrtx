# Changelog

## v0.2.0 — 2026-11-25

### Added

- **Project constitutions.** A constitution is the project's standing rules: `sdd/constitution.md`, or the files listed under `constitution:` in `.sdd.yaml`. Each `##` or `###` heading is a rule. See [docs/format.md](docs/format.md#constitution).
- `sdd constitution show [--json]` lists the rules; `sdd constitution init [--force]` writes a starter `sdd/constitution.md`.
- In a repository with a constitution, `sdd change new` adds a `## Constitution` section. `finish-propose` requires it, with `- C<n> <rule> → <how the plan complies>` lines that name real rules, or a single `- None: <reason>`.
- `propose-context` and the review packet list the rules; `C<n>` lines are review claims.
- The `sdd-propose` skill has a new step 5c (Constitution); `sdd-apply` and `sdd-archive` follow the rules.
- Library: `LoadConstitution`, `ConstitutionFiles`, `InitConstitution`, `ConstitutionDoc`, `RepoConfig.Constitution`.

### Compatibility

- Repositories without a constitution behave exactly as in v0.1.0.
- `verify` and `archive` do not check the Constitution section, so changes written before a constitution existed still archive.
- New JSON fields (`constitution` in `propose-context --json` and `review-packet --json`) are omitted when empty.

## v0.1.0 — 2026-11-24

First release.

- `sdd` CLI: `change new|list|status|propose-context|verify|score|review-packet|finish-propose|finish-apply|run-validate|archive|lint-active`, `graph`, `verify-ast`, `sync-from-code`, `skills list|install`.
- Change manifests with requirements, scenarios, decisions, risks, deploy and rollback, validate, and a second-reader review packet.
- Package contracts (`contract.sdd.md`) with Go export drift checks and living behaviour appended on archive.
- `.sdd.yaml`: `validate_allow`, `quick_check`, `verify_packages`.
- Bundled agent skills: `sdd-propose`, `sdd-apply`, `sdd-archive`.
- Go library with pluggable `CodeIndex`, `ContextProvider`, `LineClassifier` and `SecondReader`; `TextIndex` as the built-in code index.
- Pure-Go build with `CGO_ENABLED=0`; with cgo, tree-sitter indexes TypeScript, JavaScript, Python, Rust and Swift too.
