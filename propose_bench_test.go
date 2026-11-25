package sdd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanBench(t *testing.T) {
	root := filepath.Join("testdata", "fixtures", "sdd_plan_bench")
	rows, err := loadPlanJSONL[planScenario](filepath.Join(root, "scenarios.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 20 {
		t.Fatalf("scenarios=%d", len(rows))
	}
	baseRows, err := loadPlanJSONL[planBaseline](filepath.Join(root, "baseline.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]planBaseline{}
	for _, b := range baseRows {
		base[b.ID] = b
	}
	improved, regressed, unchanged := 0, 0, 0
	applyOK, applyFail, applySkipped := 0, 0, 0
	for _, row := range rows {
		var hits []string
		if row.Hit != "" {
			hits = []string{row.Hit}
		}
		band := proposeBand(row.Request, row.AcceptanceBullets, row.Overlap)
		domain := proposeDomain(row.Request, hits, "")
		if band != row.ExpectBand || domain != row.ExpectDomain {
			t.Fatalf("%s band=%s want %s domain=%s want %s", row.ID, band, row.ExpectBand, domain, row.ExpectDomain)
		}
		switch benchStatus(base[row.ID].Band, band, row.ExpectBand, base[row.ID].Domain, domain) {
		case "improved":
			improved++
		case "regressed":
			regressed++
		default:
			unchanged++
		}
		switch applyBenchStatus(domain, row.Paths) {
		case "apply_ok":
			applyOK++
		case "apply_fail":
			applyFail++
		default:
			applySkipped++
		}
	}
	summary := formatBenchCounts(improved, regressed, unchanged, applyOK, applyFail, applySkipped)
	t.Log(summary)
	if improved != 0 || regressed != 0 || unchanged != 20 {
		t.Fatal(summary)
	}
	if applyOK != 17 || applyFail != 1 || applySkipped != 2 {
		t.Fatal(summary)
	}
	if !strings.Contains(summary, "improved") || !strings.Contains(summary, "regressed") || !strings.Contains(summary, "unchanged") || !strings.Contains(summary, "apply_ok") || !strings.Contains(summary, "apply_fail") || !strings.Contains(summary, "apply_skipped") {
		t.Fatal(summary)
	}
	if benchStatus("", "context", "context", "", "internal/sdd") != "improved" {
		t.Fatal("empty baseline should count as improved")
	}
	if benchStatus("context", "simple", "context", "internal/sdd", "internal/sdd") != "regressed" {
		t.Fatal("wrong band against a stored baseline should regress")
	}
	if benchStatus("context", "context", "context", "internal/sdd", "internal/codegraph") != "regressed" {
		t.Fatal("domain drift against a stored baseline should regress")
	}
	if applyBenchStatus("internal/sdd", nil) != "apply_skipped" {
		t.Fatal("missing paths should skip apply")
	}
	if applyBenchStatus("internal/sdd", []string{"internal/sdd/finish.go"}) != "apply_ok" {
		t.Fatal("in-domain apply should pass")
	}
	if applyBenchStatus("internal/sdd", []string{"homepage/agent/app.js"}) == "apply_ok" {
		t.Fatal("homepage is outside internal/sdd")
	}
}

type cityArm struct {
	Arm    string `json:"arm"`
	Prompt string `json:"prompt"`
}

func TestCityBench(t *testing.T) {
	rows, err := loadPlanJSONL[cityArm](filepath.Join("testdata", "fixtures", "sdd_plan_bench", "city.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("arms=%d", len(rows))
	}
	var two cityArm
	for _, row := range rows {
		if row.Arm == "two-pass" {
			two = row
		}
	}
	phases, deferred, question := PlanRoadmap(two.Prompt)
	if len(phases) < 3 || question == "" {
		t.Fatalf("phases %d question %q", len(phases), question)
	}
	for _, phase := range phases {
		if strings.Contains(strings.ToLower(phase.Paragraph), "later") {
			t.Fatalf("later feature stayed in a phase: %s", phase.Paragraph)
		}
	}
	joined := strings.Join(deferred, "\n")
	if !strings.Contains(joined, "later") {
		t.Fatal("humans were not deferred")
	}
	reply, _, _ := renderRoadmap(phases, deferred, question)
	if strings.Contains(reply, ".txt") || strings.Contains(reply, ".go") {
		t.Fatalf("file before the answer %s", reply)
	}
}

type tictactoeArm struct {
	Arm        string `json:"arm"`
	Prompt     string `json:"prompt"`
	ExpectSlug string `json:"expect_slug"`
}

func stripSkillLoaderLine(prompt string) string {
	lines := strings.Split(prompt, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(line, "getSkillFileContent") {
			continue
		}
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func TestTictactoeBench(t *testing.T) {
	rows, err := loadPlanJSONL[tictactoeArm](filepath.Join("testdata", "fixtures", "sdd_plan_bench", "tictactoe.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("arms=%d", len(rows))
	}
	for _, row := range rows {
		body := stripSkillLoaderLine(row.Prompt)
		switch row.Arm {
		case "one-pass":
			if MultiParagraphRequest(body) {
				t.Fatal("one-pass tic tac toe should be single paragraph")
			}
			if row.ExpectSlug != "tic-tac-toe" {
				t.Fatalf("fixture expect_slug %q", row.ExpectSlug)
			}
		case "two-pass":
			phases, deferred, question := PlanRoadmap(body)
			if len(phases) < 3 || question == "" {
				t.Fatalf("phases %d question %q", len(phases), question)
			}
			joined := strings.Join(deferred, "\n")
			if !strings.Contains(joined, "replay") && !strings.Contains(joined, "e2e") {
				t.Log("deferred:", joined)
			}
			if row.ExpectSlug == "" {
				t.Fatal("missing expect_slug")
			}
		default:
			t.Fatalf("unknown arm %q", row.Arm)
		}
	}
}
