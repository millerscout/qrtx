package qrtx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specManifest builds a filled manifest from the scaffold, then applies the spec sections given.
func specManifest(t *testing.T, scenarios, decisions, risks, validate string) (string, string) {
	t.Helper()
	root := t.TempDir()
	slug := "spec-check"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(changeFilePath(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "Add --json to sessions list.", 1)
	text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- `app sessions list --json` prints one JSON document.", 1)
	text = strings.Replace(text, "- [ ] 1.1 First implementation task", "- [ ] 1.1 Add the list command", 1)
	text = replaceSection(text, "Scenarios", scenarios)
	text = replaceSection(text, "Decisions", decisions)
	text = replaceSection(text, "Risks", risks)
	text = replaceSection(text, "Validate", validate)
	text = replaceSection(text, "Review", "- None: test fixture, no claims conflict")
	text = removePlaceholderSection(text, "Requirements") // optional; fixtures leave it out
	if err := os.WriteFile(changeFilePath(root, slug), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, slug
}

const (
	goodScenarios = "- S1 WHEN no session is live THEN stdout is `{\"restore_to\":\"\",\"sessions\":[]}` and exit 0\n- S2 WHEN an unknown flag is passed THEN exit 2 and stdout is empty"
	goodDecisions = "- D1 Dedicated view struct. Rejected: marshal SessionState because restore_to is omitempty."
	goodRisks     = "- Field set becomes a contract → golden test on the exact keys.\n\nRollback: n/a (read-only command)"
	goodValidate  = "- [ ] V1.1 `cd cli && go test ./hybrid -run TestSessionsList` covers S1,S2"
)

func TestValidateSpecSectionsAcceptsFilled(t *testing.T) {
	root, slug := specManifest(t, goodScenarios, goodDecisions, goodRisks, goodValidate)
	if err := ValidateChangeManifest(root, slug); err != nil {
		t.Fatal(err)
	}
}

func TestValidateSpecSectionsRejects(t *testing.T) {
	cases := []struct {
		name, scenarios, decisions, risks, validate, want string
	}{
		{"tbd scaffold", "- (TBD — x)", goodDecisions, goodRisks, goodValidate, "Scenarios still contains TBD"},
		{"no then", "- S1 WHEN no session is live", goodDecisions, goodRisks, goodValidate, "S<n> WHEN … THEN …"},
		{"no id", "- WHEN a THEN b", goodDecisions, goodRisks, goodValidate, "S<n> WHEN … THEN …"},
		{"empty", "No bullets here.", goodDecisions, goodRisks, goodValidate, "has no"},
		{"duplicate", "- S1 WHEN a THEN b\n- S1 WHEN c THEN d", goodDecisions, goodRisks, goodValidate, "duplicate scenario id S1"},
		{"uncited", goodScenarios, goodDecisions, goodRisks, "- [ ] V1.1 `go test ./x` covers S1", "scenario S2 is not cited"},
		{"decision without alternative", goodScenarios, "- D1 Use a view struct.", goodRisks, goodValidate, "Rejected:"},
		{"risks without rollback", goodScenarios, goodDecisions, "- Contract burden → golden test.", goodValidate, "Rollback:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, slug := specManifest(t, tc.scenarios, tc.decisions, tc.risks, tc.validate)
			err := ValidateChangeManifest(root, slug)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestScenarioCitedByPhaseFile(t *testing.T) {
	root, slug := specManifest(t, goodScenarios, goodDecisions, goodRisks, "- [ ] V1.1 `go test ./x` covers S1")
	dir := filepath.Join(changeDir(root, slug), "phases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01-list.md"), []byte("- [ ] 1.1 Reject bad flags; verify: exit 2 test (covers S2)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChangeManifest(root, slug); err != nil {
		t.Fatal(err)
	}
}

func TestDecisionsNoneAllowed(t *testing.T) {
	root, slug := specManifest(t, goodScenarios, "- None: the request leaves no design choice.", goodRisks, goodValidate)
	if err := ValidateChangeManifest(root, slug); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptanceDoesNotSwallowScenarios(t *testing.T) {
	text := "## Acceptance\n\n- one check\n\n## Scenarios\n\n- S1 WHEN a THEN b\n\n## Observed\n\nx\n"
	acc := sectionMarkdown(text, "Acceptance")
	if strings.Contains(acc, "S1") {
		t.Fatalf("acceptance swallowed scenarios: %q", acc)
	}
}

func TestSpecDepthCounts(t *testing.T) {
	root, slug := specManifest(t, goodScenarios+"\n- S3 WHEN two sessions are live THEN both print oldest first", goodDecisions, goodRisks, goodValidate)
	sc, err := ScoreChangeManifest(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	d := sc.Depth
	if d.Scenarios != 3 || d.NegativeScenarios != 2 || d.UncoveredScenarios != 1 || d.Decisions != 1 || d.Risks != 1 || d.Rollback || !d.HasSpecSections {
		t.Fatalf("depth %+v", d)
	}
	if sc.VerifyOK {
		t.Fatal("uncovered S3 must fail verify")
	}
}

func TestRemovePlaceholderSpecSections(t *testing.T) {
	root := t.TempDir()
	if err := NewChange(root, "drop"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(changeFilePath(root, "drop"))
	text := RemovePlaceholderSpecSections(string(raw))
	for _, h := range specSectionHeadings {
		if hasSection(text, h) {
			t.Fatalf("%s left in\n%s", h, text)
		}
	}
	if strings.Contains(text, "Rollback:") {
		t.Fatalf("rollback placeholder left in\n%s", text)
	}
	for _, h := range []string{"Acceptance", "Observed", "Tasks", "Validate"} {
		if !hasSection(text, h) {
			t.Fatalf("%s removed\n%s", h, text)
		}
	}
	filled := replaceSection(string(raw), "Scenarios", goodScenarios)
	kept := RemovePlaceholderSpecSections(filled)
	if !hasSection(kept, "Scenarios") || hasSection(kept, "Decisions") {
		t.Fatalf("filled section must stay, placeholders go\n%s", kept)
	}
}

func TestExtractValidateCommandStripsCoverage(t *testing.T) {
	cases := map[string]string{
		"V1.1 go test ./internal/sdd -run TestX covers S1,S2": "go test ./internal/sdd -run TestX",
		"V1.1 go test ./internal/sdd (covers S3)":             "go test ./internal/sdd",
		"V1.1 `go test ./cli/hybrid` covers S1":               "go test ./cli/hybrid",
		"V1.1 go test ./internal/sdd -run TestScenarios":      "go test ./internal/sdd -run TestScenarios",
	}
	for in, want := range cases {
		got, skip, err := extractValidateCommand(in)
		if err != nil || skip || got != want {
			t.Fatalf("%q -> %q skip=%v err=%v, want %q", in, got, skip, err, want)
		}
	}
}

func TestDeployLineRequiredWithRealRollback(t *testing.T) {
	persist := "- Long migration → progress log.\n\nRollback: restore each project from its .v1 backup."
	root, slug := specManifest(t, goodScenarios, goodDecisions, persist, goodValidate)
	if err := ValidateChangeManifest(root, slug); err == nil || !strings.Contains(err.Error(), "Deploy:") {
		t.Fatalf("want Deploy error, got %v", err)
	}
	withDeploy := "- Long migration → progress log.\n\nDeploy: ship the reader first, then the writer, then run migrate; the old binary keeps reading v1 until then.\n\nRollback: restore each project from its .v1 backup."
	root, slug = specManifest(t, goodScenarios, goodDecisions, withDeploy, goodValidate)
	if err := ValidateChangeManifest(root, slug); err != nil {
		t.Fatal(err)
	}
	sc, err := ScoreChangeManifest(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	if !sc.Depth.Deploy || sc.Depth.Risks != 1 {
		t.Fatalf("depth %+v", sc.Depth)
	}
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range p.Claims {
		if c.ID == "DP" && strings.Contains(c.Text, "reader first") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no DP claim in %+v", p.Claims)
	}
}

func reqManifest(t *testing.T, reqs, scenarios string) (string, string) {
	t.Helper()
	root, slug := specManifest(t, scenarios, goodDecisions, goodRisks, goodValidate)
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(raw), "## Scenarios", "## Requirements\n\n"+reqs+"\n\n## Scenarios", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, slug
}

func TestRequirementsValidation(t *testing.T) {
	goodReqs := "- Q1 The list command SHALL print JSON for every live session.\n- Q2 An invalid invocation MUST fail without output."
	goodScen := "- S1 (Q1) WHEN no session is live THEN stdout is `{\"sessions\":[]}` and exit 0\n- S2 (Q2) WHEN an unknown flag is passed THEN exit 2 and stdout is empty"
	cases := []struct {
		name, reqs, scen, want string
	}{
		{"good", goodReqs, goodScen, ""},
		{"no shall", "- Q1 The list command prints JSON.", "- S1 (Q1) WHEN a THEN b\n- S2 (Q1) WHEN c THEN d", "SHALL"},
		{"scenario without requirement", goodReqs, "- S1 (Q1) WHEN a THEN b\n- S2 WHEN c THEN d", "scenario S2 names no requirement"},
		{"unknown requirement", goodReqs, "- S1 (Q1) WHEN a THEN b\n- S2 (Q3) WHEN c THEN d", "unknown requirement Q3"},
		{"requirement without scenario", goodReqs, "- S1 (Q1) WHEN a THEN b\n- S2 (Q1) WHEN c THEN d", "requirement Q2 has no scenario"},
		{"two requirements", goodReqs, "- S1 (Q1, Q2) WHEN a THEN b\n- S2 (Q2) WHEN c THEN d", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, slug := reqManifest(t, tc.reqs, tc.scen)
			err := ValidateChangeManifest(root, slug)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	root, slug := reqManifest(t, goodReqs, goodScen)
	sc, err := ScoreChangeManifest(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Depth.Requirements != 2 {
		t.Fatalf("depth %+v", sc.Depth)
	}
	p, err := BuildReviewPacket(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	if p.Claims[0].ID != "A1" || p.Claims[1].ID != "Q1" {
		t.Fatalf("claims %+v", p.Claims[:3])
	}
}
