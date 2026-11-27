# Phase order between overlapping changes

- [x] 2.1 For each overlap in `propose_context.go`, `GetIssue` the linked refs of both changes under the 10s budget (D7). Add `goes_first` to `PackageOverlap`, print `goes first: <slug> (<a> blocks <b>)`, and warn instead when the tracker is unavailable. verify: `go test . -run TestProposeContextOrder -count=1`. covers S7
