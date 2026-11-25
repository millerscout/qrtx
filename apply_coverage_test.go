package sdd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func editManifest(t *testing.T, root, slug string, edit func(string) string) {
	t.Helper()
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(edit(string(raw))), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFinishApplyScenarioCoverageAllCovered(t *testing.T) {
	root, slug := specManifest(t, goodScenarios, goodDecisions, goodRisks, "- [x] V1.1 `go test ./x` covers S1,S2")
	if err := FinishApply(root, slug, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestFinishApplyScenarioCitedOnlyByUncheckedTask(t *testing.T) {
	root, slug := specManifest(t, goodScenarios, goodDecisions, goodRisks, "- [x] V1.1 `go test ./x` covers S1")
	editManifest(t, root, slug, func(s string) string {
		return strings.Replace(s, "- [ ] 1.1 Add the list command", "- [ ] 1.1 Add the list command covers S2", 1)
	})
	err := FinishApply(root, slug, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "scenarios not covered by a checked task: S2") {
		t.Fatalf("want uncovered S2, got %v", err)
	}
	if strings.Contains(err.Error(), "S1") {
		t.Fatalf("S1 is covered by a checked line, got %v", err)
	}
}

func TestFinishApplyScenarioCoveredByCheckedPhaseLine(t *testing.T) {
	root, slug := specManifest(t, goodScenarios, goodDecisions, goodRisks, "- [x] V1.1 `go test ./x` covers S1")
	dir := filepath.Join(changeDir(root, slug), "phases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	phase := "# Phase x\n\n- [ ] 1.1 Unrelated task covers S1\n  - [x] 1.2 Reject unknown flags covers S2\n"
	if err := os.WriteFile(filepath.Join(dir, "01-x.md"), []byte(phase), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := FinishApply(root, slug, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestFinishApplyLegacyManifestWithoutScenarios(t *testing.T) {
	root, slug := specManifest(t, goodScenarios, goodDecisions, goodRisks, "- [x] V1.1 `go test ./x`")
	editManifest(t, root, slug, func(s string) string {
		s = replaceSection(s, "Scenarios", "")
		return strings.Replace(s, "## Scenarios\n", "", 1)
	})
	raw, err := os.ReadFile(changeFilePath(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	if hasSection(string(raw), "Scenarios") {
		t.Fatal("fixture still has a Scenarios section")
	}
	if err := FinishApply(root, slug, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}
