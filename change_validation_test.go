package sdd

import (
	"os"
	"strings"
	"testing"
)

func TestValidateChangeManifestRejectsScaffold(t *testing.T) {
	root := t.TempDir()
	if err := NewChange(root, "fresh"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChangeManifest(root, "fresh"); err == nil {
		t.Fatal("expected scaffold to fail verify")
	}
}

func TestValidateChangeManifestAcceptsMinimal(t *testing.T) {
	root := t.TempDir()
	if err := NewChange(root, "ok"); err != nil {
		t.Fatal(err)
	}
	path := changeFilePath(root, "ok")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "Enable feature X for operators.", 1)
	text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- Operator sees feature X in UI.", 1)
	text = RemovePlaceholderSpecSections(text) // legacy manifest: no spec sections
	text = strings.Replace(text, "- [ ] 1.1 First implementation task", "- [x] 1.1 Wire feature X", 1)
	text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [x] V1.1 go test ./internal/foo -run TestFeatureX", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChangeManifest(root, "ok"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateChangeManifestActiveRepoChange(t *testing.T) {
	root, err := RepoRoot()
	if err != nil {
		t.Skip(err)
	}
	path := changeFilePath(root, "sdd-minimal-regression-tasks")
	if _, err := os.Stat(path); err != nil {
		t.Skip("change not present")
	}
	if err := ValidateChangeManifest(root, "sdd-minimal-regression-tasks"); err != nil {
		t.Fatal(err)
	}
}

func TestSectionMarkdown(t *testing.T) {
	md := "## Why\n\nx\n\n## Acceptance\n\n- a\n\n## Tasks\n\n- b\n"
	got := sectionMarkdown(md, "Acceptance")
	if !strings.Contains(got, "- a") || strings.Contains(got, "## Tasks") {
		t.Fatalf("section=%q", got)
	}
}
