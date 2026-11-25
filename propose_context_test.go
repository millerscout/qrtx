package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeOverlaps(t *testing.T) {
	all := map[string][]string{
		"draft": {"internal/sdd", "internal/foo"},
		"other": {"internal/sdd"},
		"alone": {"internal/bar"},
	}
	over, warn := computeOverlaps("draft", all["draft"], all)
	if len(over) != 1 || over[0].OtherSlug != "other" {
		t.Fatalf("over=%v", over)
	}
	if len(warn) != 1 || !strings.Contains(warn[0], "other") {
		t.Fatalf("warn=%v", warn)
	}
}

func TestProposeContextOverlapFixture(t *testing.T) {
	root := filepath.Join("testdata", "fixtures", "sdd_propose_context")
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
	rep, err := ProposeContext(root, "overlap-a", ProposeContextOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Overlaps) != 1 || rep.Overlaps[0].OtherSlug != "overlap-b" {
		t.Fatalf("overlaps=%v warnings=%v", rep.Overlaps, rep.Warnings)
	}
	if !strings.Contains(rep.Warnings[0], "internal/sdd") {
		t.Fatalf("warnings=%v", rep.Warnings)
	}
}

func TestProposeContextOverlapGoldenJSONL(t *testing.T) {
	path := filepath.Join("testdata", "fixtures", "sdd_propose_overlap_golden.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fixtureRoot := filepath.Join("testdata", "fixtures", "sdd_propose_context")
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ID           string `json:"id"`
			DraftSlug    string `json:"draft_slug"`
			ExpectOther  string `json:"expect_overlap_with"`
			ExpectShared string `json:"expect_shared_package"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		t.Run(row.ID, func(t *testing.T) {
			rep, err := ProposeContext(fixtureRoot, row.DraftSlug, ProposeContextOpts{})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, o := range rep.Overlaps {
				if o.OtherSlug == row.ExpectOther {
					for _, s := range o.Shared {
						if s == row.ExpectShared {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatalf("overlaps=%v", rep.Overlaps)
			}
		})
	}
}
