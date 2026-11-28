# Phase JSON goldens and fuzzing

- [x] 3.1 Add `cli/json_golden_test.go`, driving the in-process CLI in temp repos. It normalizes times, pids, generated ids, `<path>:<pid>` session ids, and temp paths in their raw, `/private/var`-resolved and JSON-escaped forms, then compares against `testdata/golden/{sync_status,sync_inbox,tracker_list,tracker_check,propose_backlog,events_line}.json`. `QRTX_UPDATE_GOLDEN=1` rewrites them. verify: `go test ./cli/ -run Golden` passes; renaming a JSON tag locally makes it fail with a diff; `QRTX_UPDATE_GOLDEN=1` then the rerun passes (revert after). covers S9
{: id="phase-03-task-1"}
- [x] 3.2 Add the fuzz targets:
  - `FuzzMergeIssues` (root): it must not panic. When there is no error and no conflict, every output line is valid JSON, sorted by id. With `c = Merge(x,x,x)`, `Merge(c,c,c) == c`.
  - `FuzzMarker` (`tracker/`): `DecodeMarker(EncodeMarker(d, m))` returns `m` (after normalizing nil/empty slices and UTF-8) and the trimmed description of `DecodeMarker(d)`.
  - `FuzzADF` (`providers/jira/`): `MarkdownToADF(s)` is a valid doc (type `doc`, version 1, no empty text nodes), and `ADFToMarkdown(arbitrary bytes)` never panics.
  - `FuzzSetManifestIssues` (root): it must not panic. When the input parses and the refs have no backtick or newline, the output parses and its issues equal the refs.

  Seed corpora go under each package's `testdata/fuzz/`. A crasher seed is committed only together with its fix. verify: each target runs for 10s with no crasher. covers S10
{: id="phase-03-task-2"}
