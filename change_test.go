package qrtx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoRootUsesWorkingDirectoryWithoutGoMod(t *testing.T) {
	t.Setenv(EnvRepo, "")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	root, err := RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("root %s", root)
	}
}

func TestChangeLifecycle(t *testing.T) {
	root := t.TempDir()
	if err := ensureChangesStore(root); err != nil {
		t.Fatal(err)
	}
	if err := NewChange(root, "pilot-smoke"); err != nil {
		t.Fatal(err)
	}
	if err := NewChange(root, "Bad Slug"); err == nil {
		t.Fatal("expected invalid slug error")
	}
	slugs, err := ListChanges(root)
	if err != nil || len(slugs) != 1 || slugs[0] != "pilot-smoke" {
		t.Fatalf("list=%v err=%v", slugs, err)
	}
	sum, err := SummarizeChange(root, "pilot-smoke")
	if err != nil || sum.Status != "planning" || sum.TasksTotal < 1 {
		t.Fatalf("sum=%+v err=%v", sum, err)
	}
	m := ChangesMap(root, "pilot-smoke")
	if m["active_slug"] != "pilot-smoke" {
		t.Fatalf("active_slug=%v", m["active_slug"])
	}
	raw, err := os.ReadFile(changeFilePath(root, "pilot-smoke"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "Pilot smoke for archive.", 1)
	text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- Archive succeeds on a valid manifest.", 1)
	text = RemovePlaceholderSpecSections(text) // legacy manifest: no spec sections
	text = strings.Replace(text, "- [ ] 1.1 First implementation task", "- [x] 1.1 Pilot implementation", 1)
	text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [x] V1.1 go test ./internal/sdd/ -run TestChangeLifecycle", 1)
	text = strings.Replace(text, "- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible", "- [x] V1.2 manual: n/a", 1)
	text = strings.Replace(text, "status: planning", "status: complete", 1)
	if err := os.WriteFile(changeFilePath(root, "pilot-smoke"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ArchiveChange(root, "pilot-smoke"); err != nil {
		t.Fatal(err)
	}
	slugs, _ = ListChanges(root)
	if len(slugs) != 0 {
		t.Fatalf("expected no active changes, got %v", slugs)
	}
	m = ChangesMap(root, "pilot-smoke")
	if m["active_slug"] != "" {
		t.Fatalf("stale active_slug=%v", m["active_slug"])
	}
	if m["stale_pin"] != "pilot-smoke" {
		t.Fatalf("stale_pin=%v", m["stale_pin"])
	}
	arch := filepath.Join(changesRoot(root), archiveDirName)
	entries, err := os.ReadDir(arch)
	if err != nil || len(entries) != 1 {
		t.Fatalf("archive entries=%v err=%v", entries, err)
	}
}

func TestArchiveChangeRejectsIncomplete(t *testing.T) {
	root := t.TempDir()
	if err := NewChange(root, "incomplete"); err != nil {
		t.Fatal(err)
	}
	if err := ArchiveChange(root, "incomplete"); err == nil {
		t.Fatal("expected archive to fail on unchecked tasks")
	}
}
