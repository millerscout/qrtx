# Security

## Trust model

`sdd` treats the repository it runs in as trusted, the same way `make` trusts a Makefile:

- `sdd change run-validate`, `finish-apply` and `archive --run-validate` execute `Validate` lines from change manifests with `bash -lc` in the repository root. Only allowlisted command prefixes run (see [docs/config.md](docs/config.md)), unless `SDD_RUN_VALIDATE=1` is set. The allowlist limits accidents; it is not a sandbox. `go test` and `go run` execute repository code.
- `change archive --quick-check` runs the `quick_check` command from `.sdd.yaml`.

Do not run these commands on a checkout you do not trust, and review manifest `Validate` sections in pull requests from others.

`sdd` itself makes no network calls. Hosts that embed it may plug in providers that do.

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's "Report a vulnerability" (Security → Advisories) on this repository, rather than in a public issue.
