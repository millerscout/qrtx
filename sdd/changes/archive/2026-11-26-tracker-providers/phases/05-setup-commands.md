# Phase setup commands

- [x] 5.1 Add `kit.BindHost(path, host string, b Binding) (changed bool, err error)` in `providers/kit` and `sdd.SetTrackerConfig(repoRoot string, c TrackerConfig) (changed bool, err error)` in root `sdd`.
  - `SetTrackerConfig` edits `.sdd.yaml` through a `yaml.v3` `Node`, keeping other keys and comments.
  - Both return `changed=false` when the content would be identical, and leave the file untouched.

  `BindHost` merges `scopes` into an existing binding. verify: `go test ./... -run 'TestSetTrackerConfig|TestBindHost'` (with a temp `SDD_CREDENTIALS`) covers comments and `project: shop` kept, a re-run byte-identical, a second scope merged, and a literal token refused. covers S12,S14
{: id="phase-05-task-1"}
- [x] 5.2 Add `sdd tracker setup <backend>` to `cli/tracker.go`:
  - **Flags:** github `--repo`, `--api-url`; jira `--url`, `--project`, `--issue-type`; for both, `--token-env`, `--email-env`, `--token-command` and `--yes`.
  - **Prompting:** only when stdin is a character device. Otherwise a missing required flag is an error naming it.
  - **Binding:** it binds the API host (`api.github.com` from `api_url`, or the Jira `url` host) with `scopes: [<repo or project>]`.
  - **Confirmation:** before writing the credentials file, it prints the path and the binding line. Answering `n` keeps the file and prints how to bind the host by hand.
  - **Plugin check:** it checks `exec.LookPath("sdd-tracker-<backend>")` and otherwise prints the `go install` line.
  - **Finish:** it runs the read-only `CheckTracker`.

  verify: `go test ./cli/ -run TestTrackerSetup`, with every test setting a temp `SDD_CREDENTIALS`, covers the S12 write, missing `--repo` without a terminal, `n` at the confirmation, and the plugin missing from `PATH`. covers S12,S13
{: id="phase-05-task-2"}
- [x] 5.3a Make `tracker init` (`cli/tracker.go` `trackerInit`) report `unchanged` when `.gitattributes` already has the line and `git config --get merge.sdd-issues.driver` already matches. Add `skills.InstallKeep(dir) []Result`, which reports a differing skill as `skipped` and continues. verify: `go test ./cli/ ./skills/ -run 'TestTrackerInitUnchanged|TestInstallKeep'`. covers S14,S15
{: id="phase-05-task-3a"}
- [x] 5.3 Add `sdd init [--tracker <backend> …setup flags] [--no-skills] [--no-constitution] [--yes]` in a new `cli/init.go`, wired into `Run` and its usage line. Each step prints `created`, `updated`, `unchanged` or `skipped (<reason>)`:
  - skills via `skills.InstallKeep(dir)`, where a differing local skill is `skipped (local edits; use skills install --force)`;
  - the constitution, only when `ConstitutionFiles` is empty, via `InitConstitution`;
  - `tracker init` when `git rev-parse` succeeds;
  - `tracker setup` when `--tracker` is given.

  verify: `go test ./cli/ -run TestInit`, with a temp `SDD_CREDENTIALS`, covers a fresh git repo, then a re-run with every step `unchanged` and files byte-identical, then a repo with a listed constitution, then a folder outside git, then one locally edited skill. covers S14,S15
{: id="phase-05-task-3"}
- [x] 5.4 Add an end-to-end test that builds `cmd/sdd`, `cmd/sdd-tracker-github` and `cmd/sdd-tracker-jira` into a temp `PATH`, starts the two fakes (exported from `providers/<name>/fakeapi` for reuse by cli tests), binds their loopback hosts in a temp `SDD_CREDENTIALS`, and runs `sdd tracker check --backend <name> --allow-writes`. verify: `go test ./cli/ -run TestProviderBinaries` passes for both. covers S17
{: id="phase-05-task-4"}
