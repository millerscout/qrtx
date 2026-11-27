package qrtx

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func setLineClassifier(t *testing.T, fn func(string, []string, map[string]string) (map[string]string, error)) {
	t.Helper()
	prev := LineClassifier
	LineClassifier = fn
	t.Cleanup(func() { LineClassifier = prev })
}

func stubDeterministicRoadmapClassify(t *testing.T) {
	t.Helper()
	setLineClassifier(t, nil)
	prev := classifyUncued
	classifyUncued = func(_ string, lines []string) map[string]string {
		m := map[string]string{}
		for _, line := range lines {
			switch {
			case laterParagraph(line):
				m[line] = "deferred"
			case sequenceParagraph(line):
				m[line] = "sequence"
			default:
				m[line] = "phase"
			}
		}
		return m
	}
	t.Cleanup(func() { classifyUncued = prev })
}

func TestBlockIDSurvivesAcceptanceRewrite(t *testing.T) {
	stubDeterministicRoadmapClassify(t)
	root := t.TempDir()
	slug := "block-ids"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteRoadmap(root, slug, "one feature in the city\nanother feature of the map\na third feature for power\n", ""); err != nil {
		t.Fatal(err)
	}
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := replaceSection(string(raw), "Acceptance", "- The player lasts the night and keeps the lights on through the storm")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `id="phase-01"`) || !strings.Contains(string(got), `id="needs-user"`) {
		t.Fatalf("blocks dropped\n%s", got)
	}
}

func TestFakeClassifierDefers(t *testing.T) {
	phases, deferred, _ := PlanRoadmap("build the droid yard\nmaintenance goes first\nkeep the lights on\n")
	if len(phases) != 2 {
		t.Fatalf("phases %d", len(phases))
	}
	if len(deferred) != 0 {
		t.Fatalf("sequence note was deferred %v", deferred)
	}
	if phases[0].Note != "maintenance goes first" {
		t.Fatalf("note %q", phases[0].Note)
	}
	tasks, _, _ := renderRoadmap(phases, deferred, "")
	if strings.Contains(tasks, "### Phase maintenance") || !strings.Contains(tasks, `id="phase-01-note"`) {
		t.Fatalf("note block %s", tasks)
	}
}

func TestAnswerFileIgnoresPhasePath(t *testing.T) {
	prev := classifyUncued
	classifyUncued = func(string, []string) map[string]string {
		return map[string]string{}
	}
	t.Cleanup(func() { classifyUncued = prev })
	got := lastAnswerFile("see phases/01-the-game.md then drawn in room.txt")
	if got != "room.txt" {
		t.Fatalf("file %q", got)
	}
}

func TestLineCriteriaGolden(t *testing.T) {
	raw, err := json.Marshal(lineCriteria("paragraph"))
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("testdata/fixtures/sdd_plan_bench/line_criteria.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != strings.TrimSpace(string(golden)) {
		t.Fatalf("criteria %s\nwant %s", raw, golden)
	}
	if !strings.Contains(string(raw), "abstain") {
		t.Fatal("abstain dropped")
	}
}

func TestGoldfishOnClassifierMiss(t *testing.T) {
	setLineClassifier(t, nil)
	prevC := classifyUncued
	classifyUncued = nil
	prev := SecondReader
	SecondReader = func(prompt string) (string, error) {
		if !strings.Contains(prompt, "abstain") {
			t.Fatalf("prompt %s", prompt)
		}
		return "choice: deferred\nchoice: phase\n", nil
	}
	t.Cleanup(func() {
		classifyUncued = prevC
		SecondReader = prev
	})
	_, deferred, _ := PlanRoadmap("the humans arrive next year\nbuild the yard\nkeep the lights\n")
	if len(deferred) != 1 || !strings.Contains(deferred[0], "next year") {
		t.Fatalf("deferred %v", deferred)
	}
}

func TestSecondGoldfishOnDisagreement(t *testing.T) {
	var posts int
	setLineClassifier(t, func(_ string, lines []string, criteria map[string]string) (map[string]string, error) {
		posts++
		if _, ok := criteria["abstain"]; !ok {
			t.Fatalf("criteria %v", criteria)
		}
		return map[string]string{lines[0]: "phase"}, nil
	})
	prevC := classifyUncued
	classifyUncued = nil
	calls := 0
	prev := SecondReader
	SecondReader = func(prompt string) (string, error) {
		calls++
		if calls == 1 {
			return "choice: deferred\n", nil
		}
		if !strings.Contains(prompt, "phase") || !strings.Contains(prompt, "deferred") {
			t.Fatalf("decide prompt %s", prompt)
		}
		return "choice: deferred\n", nil
	}
	t.Cleanup(func() {
		classifyUncued = prevC
		SecondReader = prev
	})
	phases, deferred, _ := PlanRoadmap("build the yard")
	if posts != 1 {
		t.Fatalf("classifier calls %d", posts)
	}
	if calls != 2 {
		t.Fatalf("goldfish calls %d", calls)
	}
	if len(phases) != 0 || len(deferred) != 1 {
		t.Fatalf("phases %d deferred %v", len(phases), deferred)
	}
}

func TestAcceptanceBodyKeepsValidate(t *testing.T) {
	stubDeterministicRoadmapClassify(t)
	root := t.TempDir()
	slug := "section-ids"
	if err := NewChange(root, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteRoadmap(root, slug, "one feature in the city\nanother feature of the map\na third feature for power\n", ""); err != nil {
		t.Fatal(err)
	}
	path := changeFilePath(root, slug)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := replaceSection(string(raw), "Acceptance", "The player lasts the night\n## not a heading\nmore")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `id="validate"`) {
		t.Fatalf("validate id dropped\n%s", got)
	}
	if !strings.Contains(sectionMarkdown(string(got), "Acceptance"), "## not a heading") {
		t.Fatalf("acceptance ended early\n%s", sectionMarkdown(string(got), "Acceptance"))
	}
}
