package qrtx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func alsoSlugs(res StartResult) []string {
	var out []string
	for _, a := range res.AlsoActive {
		out = append(out, a.Slug)
	}
	return out
}

func TestStartFromIssueAlsoActive(t *testing.T) {
	t.Run("same worktree", func(t *testing.T) {
		root := t.TempDir()
		tr := openT(t, root)
		is, _ := tr.CreateIssue(CreateIssueInput{Title: "Dark mode"})
		if res, err := StartFromIssue(root, tr, "file:"+is.ID, "dark-mode"); err != nil || len(res.AlsoActive) != 0 {
			t.Fatalf("first start: %+v %v", res, err)
		}
		res, err := StartFromIssue(root, tr, "file:"+is.ID, "dark-mode-2")
		if err != nil {
			t.Fatal(err)
		}
		if got := alsoSlugs(res); len(got) != 1 || got[0] != "dark-mode" {
			t.Fatalf("AlsoActive %v", got)
		}
		// Re-starting the same slug is not a double start.
		res, err = StartFromIssue(root, tr, "file:"+is.ID, "dark-mode-2")
		if err != nil || strings.Join(alsoSlugs(res), ",") != "dark-mode" {
			t.Fatalf("restart same slug: %v %v", alsoSlugs(res), err)
		}
	})
	t.Run("archived change does not count", func(t *testing.T) {
		root := t.TempDir()
		tr := openT(t, root)
		is, _ := tr.CreateIssue(CreateIssueInput{Title: "Dark mode"})
		if _, err := StartFromIssue(root, tr, "file:"+is.ID, "dark-mode"); err != nil {
			t.Fatal(err)
		}
		arch := filepath.Join(changesRoot(root), archiveDirName, "2026-01-01-dark-mode")
		_ = os.MkdirAll(filepath.Dir(arch), 0o755)
		if err := os.Rename(changeDir(root, "dark-mode"), arch); err != nil {
			t.Fatal(err)
		}
		res, err := StartFromIssue(root, tr, "file:"+is.ID, "dark-mode-2")
		if err != nil || len(res.AlsoActive) != 0 {
			t.Fatalf("archived change counted: %+v %v", res, err)
		}
	})
	t.Run("sibling worktree", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "main")
		_ = os.MkdirAll(root, 0o755)
		gitIn(t, root, "init", "-q", "-b", "main")
		gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "base")
		wt := filepath.Join(base, "wt2")
		gitIn(t, root, "worktree", "add", "-q", wt, "-b", "other")
		tr := openT(t, root)
		is, _ := tr.CreateIssue(CreateIssueInput{Title: "Dark mode"})
		// The sibling worktree has an active change linking the issue.
		if err := NewChange(wt, "dark-mode"); err != nil {
			t.Fatal(err)
		}
		text := mustRead(t, changeFilePath(wt, "dark-mode"))
		text, err := setManifestIssues(text, []string{"file:" + is.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(changeFilePath(wt, "dark-mode"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := StartFromIssue(root, tr, "file:"+is.ID, "dark-mode-2")
		if err != nil {
			t.Fatal(err)
		}
		if len(res.AlsoActive) != 1 || res.AlsoActive[0].Slug != "dark-mode" || res.AlsoActive[0].Worktree == "" {
			t.Fatalf("sibling worktree not seen: %+v", res.AlsoActive)
		}
	})
}
