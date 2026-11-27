# Phase docs and release notes

- [x] 6.1 Write `docs/providers.md`: install lines; `sdd init` and `sdd tracker setup` examples; config keys and status mapping tables for GitHub and Jira; the credentials file and why the repo cannot bind hosts; first-party vs community providers. Add a "Writing a provider with the kit" section and a "Credentials" section to `docs/tracker-protocol.md`. verify: `grep -n "credentials.yaml" docs/providers.md docs/tracker-protocol.md` and `grep -n "kittest.Run" docs/tracker-protocol.md` find the sections. covers S3,S6
{: id="phase-06-task-1"}
- [x] 6.2 Update `README.md` (Quick start uses `sdd init`; commands table gains `init` and `tracker setup`), `docs/embedding.md` (the `tracker` package and aliases), `doc.go` (unstable list gains the kit) and `CHANGELOG.md`. verify: `go test ./... -count=1` passes and `grep -n "sdd init" README.md` shows the quick start. covers S1,S14
{: id="phase-06-task-2"}
