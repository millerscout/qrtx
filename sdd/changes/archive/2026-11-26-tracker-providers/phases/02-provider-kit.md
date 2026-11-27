# Phase provider kit and credentials

- [x] 2.1 Add `providers/kit`:
  - `Serve(kinds map[string]Handler)` reads one envelope, rejects a wrong `protocol` or an undeclared `kind` with `invalid`, calls the kind's handler, writes one response and returns the exit code.
  - `TrackerHandler(p Provider)` adapts a `Provider` (the `IssueTracker` methods, with `ListIssuesPage(f) ([]Issue, cursor string, error)` in place of `ListIssues`, plus `Init(config json.RawMessage) error`) to the op table, matching the argument shapes `execTracker` sends (`filter`, `id`, `add_sdd`, `native`, `client_ref`).
  - A plain Go error is returned as `invalid`, and an `*AdapterError` is returned as is.

  verify: `go test ./providers/kit/ -run 'TestServe'` covers a v2 envelope, a `wiki` envelope to a tracker-only provider, a generic `wiki` handler, a plain error, and an `*AdapterError`. covers S2,S16
{: id="phase-02-task-1"}
- [x] 2.2 Add `kit.Credentials(host, scope)`:
  - **File path:** `$SDD_CREDENTIALS`, else `os.UserConfigDir()` on Windows, else `$XDG_CONFIG_HOME` or `~/.config`, plus `/sdd/credentials.yaml`.
  - **Bindings:** `hosts: {<host[:port]>: {token_env, email_env, token_command, scopes: [owner/name | KEY | "*"]}}`. Matching is exact on host and port. A literal `token` or `password` key is rejected. `token_command` runs via `sh -c` (`cmd /C` on Windows) with a 10s timeout and `SDD_CRED_HOST`/`SDD_CRED_SCOPE` set, and its stdout is trimmed.
  - **Errors:** an unbound host, a scope missing from `scopes`, or an empty variable is `auth`, naming what is unbound and the file path.

  verify: `go test ./providers/kit/ -run 'TestCredentials'` (with a temp `SDD_CREDENTIALS`) covers an unbound host, a different port, a scope outside the list, `*`, a literal token, an empty env var, an env var, and a command echoing `SDD_CRED_HOST`/`SDD_CRED_SCOPE`. covers S3,S4
{: id="phase-02-task-2"}
- [x] 2.3 Add `kit.HTTP`, built from a base URL and a scope. The URL must be `https`, or `http` only when the bound host is loopback. It:
  - looks up credentials for the URL's host before any request, so an unbound host never receives a request;
  - sets the auth header and `User-Agent`;
  - maps statuses per Q4, including `x-ratelimit-remaining: 0` with `x-ratelimit-reset`, and `Retry-After`;
  - retries a 5xx or 429 once, only for GET/PATCH/PUT or requests marked safe, and only when the wait fits before `SDD_TRACKER_DEADLINE` (default 20s);
  - refuses every redirect that changes scheme, host or port (D14).

  verify: `go test ./providers/kit/ -run 'TestHTTP'` (TestHTTP and TestHTTPRedirects) uses `httptest` servers for:
  - a 503 followed by a 200;
  - a 429 with `Retry-After: 7` and a deadline 3s away (no wait);
  - a 401;
  - a POST with a 503 (one request);
  - a host or port mismatch where the server sees no request;
  - a scope mismatch;
  - a 302 to another server and a 302 to http, where the target sees no request.

  covers S3,S5,S18
{: id="phase-02-task-3"}
- [x] 2.4 Add `providers/kit/kittest.Run(t, p Provider, config json.RawMessage)`, which calls `p.Init(config)` and runs `tracker.Conformance` in-process with writes on, failing on each failed op. verify: `go test ./providers/kit/... -run 'TestKittest'` passes against an in-memory provider and fails against one that breaks idempotent create. covers S6
{: id="phase-02-task-4"}
