// Package qrtx implements spec-driven development: changes are planned as
// manifests under sdd/changes/<slug>/change.sdd.md, implemented against their
// task lists, and archived once validated. Packages may carry a
// contract.sdd.md that is checked against the code.
//
// # Stability
//
// Until v1, only two groups are stable:
//
//   - Core: change lifecycle (NewChange, ListChanges, SummarizeChange,
//     ValidateChangeManifest, ScoreChangeManifest, ArchiveChange), propose
//     (ProposeContext, FinishPropose), apply (FinishApply, RunValidateTasks,
//     RunQuickCheck), review (BuildReviewPacket), contracts and drift
//     (BuildGraph, VerifyAST, SyncPackageFromCode), constitution
//     (LoadConstitution, ConstitutionFiles, InitConstitution), RepoRoot and
//     LoadRepoConfig.
//   - Host hooks: CodeIndex, ContextProvider, LineClassifier, SecondReader,
//     CLIName, DefaultProject, RepoEnv and RunValidateEnv.
//
// Everything else is unstable and may change in any v0 release. That
// includes the status and UI payload helpers (StatusForRepo, StatusMap,
// ChangesMap, PlanningMap, Merge*IntoStatus), the issue tracker API (Issue,
// IssueTracker, SetIssueTracker, OpenTracker, StartFromIssue, DrainOutbox,
// MergeIssues, SetTrackerConfig, the sdd.adapter/v1 types, and the tracker,
// providers/kit and provider packages) and the propose-agent helpers
// (ProposeBand, Regression*, WriteRoadmap, AcceptanceEcho and similar).
package qrtx
