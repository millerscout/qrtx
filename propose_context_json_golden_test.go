package sdd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestProposeContextJSONGolden pins every key and code-hit source value of
// `propose-context --json`. Agents and benches read this payload, so a rename
// must show up here first. Set SDD_UPDATE_GOLDEN=1 to rewrite the file.
func TestProposeContextJSONGolden(t *testing.T) {
	rep := ProposeContextReport{
		Slug:   "tic-tac-toe",
		Score:  "context",
		Domain: "internal/game",
		CodeHits: []CodeHit{
			{Path: "internal/game/board.go", Line: 12, Source: sourceGraph},
			{Path: "internal/game/ai.go", Line: 3, Source: "embedding"},
		},
		Packages:          []string{"internal/game"},
		Overlaps:          []PackageOverlap{{OtherSlug: "city-survival-game", Shared: []string{"internal/game"}}},
		GraphNeighbors:    []GraphNeighbor{{From: "internal/game", To: "internal/render"}},
		Issues:            &IssuesSummary{Total: 4, ByRule: map[string]int{"tool_repeat": 3, "stall": 1}},
		KnownFixes:        []KnownFix{{Method: "edit", Summary: "guard empty board", Test: "TestEmptyBoard"}},
		Warnings:          []string{`request overlap with "city-survival-game"`},
		ExistingBehaviour: []ExistingBehaviour{{Package: "internal/game", Text: "A full board is a draw."}},
		Constitution:      []ConstitutionDoc{{Path: "CONSTITUTION.md", Rules: []string{"C1 Tests lock behaviour."}}},
	}
	var buf bytes.Buffer
	if err := WriteProposeContextJSON(&buf, rep); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "fixtures", "sdd_propose_context_json.golden.json")
	if os.Getenv("SDD_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("propose-context JSON changed:\n got %s\nwant %s", buf.String(), want)
	}
}
