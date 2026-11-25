package sdd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConstitutionRules(t *testing.T) {
	articles := "# Brain\n\n## Article IX — Evidence\n\n### Golden scenarios\n\n## Article X — Change process\n\n```bash\n## not a heading\n```\n\n## Article XI — Decision log\n"
	if got, want := constitutionRules(articles), []string{"Article IX — Evidence", "Article X — Change process", "Article XI — Decision log"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("articles: %q", got)
	}
	grouped := "# Golden\n\n## Purpose\n\n## Rules\n\n### Rule 1: Golden-first\n\n### Rule 2: One dataset\n\n### Rule 3: Append-only\n\n### Rule 12: Ceilings\n\n## Checklist\n"
	if got, want := constitutionRules(grouped), []string{"Rule 1: Golden-first", "Rule 2: One dataset", "Rule 3: Append-only", "Rule 12: Ceilings"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("grouped: %q", got)
	}
	tie := "## A\n\n### a1\n\n## B\n\n### b1\n"
	if got := constitutionRules(tie); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Fatalf("tie goes to ##: %q", got)
	}
}

func TestCitedRule(t *testing.T) {
	docs := []ConstitutionDoc{
		{Path: "a.md", Rules: []string{"Rule 1: Golden-first", "Rule 12: Ceilings"}},
		{Path: "b.md", Rules: []string{"Article X — Change process", "Article XI — Decision log"}},
	}
	cases := map[string]string{
		"C1 Rule 1 → adds a golden row":                 "Rule 1: Golden-first",
		"C2 Rule 12 → keeps the ceiling":                "Rule 12: Ceilings",
		"C3 Article X → test, deploy, verify":           "Article X — Change process",
		"C4 article xi → logs the decision":             "Article XI — Decision log",
		"C5 Golden-first → fixture before code":         "",
		"C6 Rule 1: Golden-first → fixture before code": "Rule 1: Golden-first",
		"C7 Rule 3 → nothing":                           "",
	}
	for line, want := range cases {
		if got := citedRule(line, docs); got != want {
			t.Errorf("%q cited %q, want %q", line, got, want)
		}
	}
}

func TestConstitutionFiles(t *testing.T) {
	root := t.TempDir()
	if files, err := ConstitutionFiles(root); err != nil || files != nil {
		t.Fatalf("none: %v %v", files, err)
	}
	writeFile(t, root, DefaultConstitutionFile, "# C\n\n## Rule 1: A\n")
	if files, err := ConstitutionFiles(root); err != nil || !reflect.DeepEqual(files, []string{DefaultConstitutionFile}) {
		t.Fatalf("default: %v %v", files, err)
	}
	writeFile(t, root, "docs/rules.md", "# R\n\n## Rule 9: B\n")
	writeFile(t, root, RepoConfigFile, "constitution:\n  - docs/rules.md\n")
	docs, err := LoadConstitution(root)
	if err != nil || len(docs) != 1 || docs[0].Path != "docs/rules.md" || docs[0].Rules[0] != "Rule 9: B" {
		t.Fatalf("configured list replaces the default: %+v %v", docs, err)
	}
	writeFile(t, root, RepoConfigFile, "constitution:\n  - docs/missing.md\n")
	if _, err := ConstitutionFiles(root); err == nil || !strings.Contains(err.Error(), "docs/missing.md") {
		t.Fatalf("missing listed file: %v", err)
	}
}

func TestValidateConstitutionSection(t *testing.T) {
	root := t.TempDir()
	plan := func(section string) string {
		return "## Why\n\nx\n\n" + section + "## Review\n\n- None: checked\n"
	}
	if err := validateConstitutionSection(root, "s", plan("")); err != nil {
		t.Fatalf("no constitution: %v", err)
	}
	writeFile(t, root, DefaultConstitutionFile, "# C\n\n## Rule 1: Tests before code\n\n## Rule 2: No silent data loss\n")
	bad := map[string]string{
		"missing section": "",
		"TBD":             "## Constitution\n\n- (TBD — fill)\n\n",
		"no bullet":       "## Constitution\n\nprose only\n\n",
		"no id":           "## Constitution\n\n- Rule 1 → adds a test\n\n",
		"no arrow":        "## Constitution\n\n- C1 Rule 1 adds a test\n\n",
		"unknown rule":    "## Constitution\n\n- C1 Rule 7 → whatever\n\n",
		"bare none":       "## Constitution\n\n- None\n\n",
		"none mixed":      "## Constitution\n\n- C1 Rule 1 → adds a test\n- None: also nothing\n\n",
	}
	for name, section := range bad {
		if err := validateConstitutionSection(root, "s", plan(section)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	good := []string{
		"## Constitution\n\n- C1 Rule 1 → V1.1 fails before the fix\n- C2 Rule 2: No silent data loss → nothing is stored\n\n",
		"## Constitution\n\n- None: the change only edits docs\n\n",
	}
	for _, section := range good {
		if err := validateConstitutionSection(root, "s", plan(section)); err != nil {
			t.Errorf("good section rejected: %v", err)
		}
	}
}

func TestNewChangeAddsConstitutionSectionOnlyWhenConfigured(t *testing.T) {
	plain := t.TempDir()
	if err := NewChange(plain, "a"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(changeFilePath(plain, "a"))
	if strings.Contains(string(raw), "## Constitution") {
		t.Fatal("no constitution: template should not have the section")
	}
	ruled := t.TempDir()
	if _, err := InitConstitution(ruled, false); err != nil {
		t.Fatal(err)
	}
	if err := NewChange(ruled, "b"); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(changeFilePath(ruled, "b"))
	text := string(raw)
	if i, j := strings.Index(text, "## Constitution"), strings.Index(text, "## Review"); i < 0 || j < i {
		t.Fatalf("Constitution section should come before Review:\n%s", text)
	}
	if !strings.Contains(sectionMarkdown(text, "Constitution"), "(TBD") {
		t.Fatal("new section should start as TBD")
	}
}

// finish-propose rejects a complete plan in a repo with a constitution until
// the plan has a Constitution section.
func TestFinishProposeConstitutionGate(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join("examples", "tic-tac-toe", "sdd", "changes", "tic-tac-toe", "change.sdd.md")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "sdd/changes/tic-tac-toe/change.sdd.md", string(raw))
	if err := FinishPropose(root, "tic-tac-toe", io.Discard, io.Discard); err != nil {
		t.Fatalf("without a constitution the example passes: %v", err)
	}
	if _, err := InitConstitution(root, false); err != nil {
		t.Fatal(err)
	}
	err = FinishPropose(root, "tic-tac-toe", io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "## Constitution") {
		t.Fatalf("want the constitution gate, got %v", err)
	}
	withSection := strings.Replace(string(raw), "## Review\n", "## Constitution\n\n- C1 Rule 1 → V1.1 and V1.2 check each scenario before the page is called done\n- C2 Rule 2 → nothing is stored; the page keeps no data\n\n## Review\n", 1)
	writeFile(t, root, "sdd/changes/tic-tac-toe/change.sdd.md", withSection)
	var out bytes.Buffer
	if err := FinishPropose(root, "tic-tac-toe", &out, io.Discard); err != nil {
		t.Fatalf("with the section: %v", err)
	}
	pkt, err := BuildReviewPacket(root, "tic-tac-toe")
	if err != nil {
		t.Fatal(err)
	}
	var cIDs []string
	for _, c := range pkt.Claims {
		if c.Section == "Constitution" {
			cIDs = append(cIDs, c.ID)
		}
	}
	if !reflect.DeepEqual(cIDs, []string{"C1", "C2"}) || len(pkt.Constitution) != 1 {
		t.Fatalf("packet claims %v constitution %+v", cIDs, pkt.Constitution)
	}
	if text := FormatReviewPacket(pkt); !strings.Contains(text, "## Constitution") || !strings.Contains(text, "Rule 2: No silent data loss") {
		t.Fatal("formatted packet should list the rules")
	}
}

func TestInitConstitution(t *testing.T) {
	root := t.TempDir()
	path, err := InitConstitution(root, false)
	if err != nil || path != filepath.Join(root, DefaultConstitutionFile) {
		t.Fatalf("init: %q %v", path, err)
	}
	docs, err := LoadConstitution(root)
	if err != nil || len(docs) != 1 || len(docs[0].Rules) != 3 {
		t.Fatalf("template rules: %+v %v", docs, err)
	}
	if _, err := InitConstitution(root, false); err == nil {
		t.Fatal("existing file without --force should fail")
	}
	if _, err := InitConstitution(root, true); err != nil {
		t.Fatalf("--force: %v", err)
	}
	writeFile(t, root, "docs/rules.md", "# R\n\n## Rule 1: A\n")
	writeFile(t, root, RepoConfigFile, "constitution: [docs/rules.md]\n")
	if _, err := InitConstitution(root, true); err == nil || !strings.Contains(err.Error(), "already lists") {
		t.Fatalf("configured list: %v", err)
	}
}
