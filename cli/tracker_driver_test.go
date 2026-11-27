package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/millerscout/qrtx"
)

// pathDriver is the merge driver written when PATH resolves to the binary.
func pathDriver() string { return qrtx.CLIName + " tracker merge-driver %O %A %B" }

// copyBin copies the test qrtx binary to dst.
func copyBin(t *testing.T, dst string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(qrtxBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// runBin runs bin in repo and returns its combined output.
func runBin(t *testing.T, bin, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = repo
	cmd.Env = gitEnv(repo)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", bin, args, err, out)
	}
	return string(out)
}

func driverOf(t *testing.T, repo string) string {
	t.Helper()
	return strings.TrimSpace(mustGit(t, repo, "config", "merge.sdd-issues.driver"))
}

// installed puts a qrtx symlink on PATH pointing at <dir>/<version>/qrtx and
// returns the symlink path.
func installed(t *testing.T, dir, version string) string {
	t.Helper()
	copyBin(t, filepath.Join(dir, version, "qrtx"))
	link := filepath.Join(dir, "bin", "qrtx")
	_ = os.MkdirAll(filepath.Dir(link), 0o755)
	_ = os.Remove(link)
	skipOnWindows(t, "symlinks")
	if err := os.Symlink(filepath.Join(dir, version, "qrtx"), link); err != nil {
		t.Fatal(err)
	}
	return link
}

func TestTrackerMergeDriverPath(t *testing.T) {
	x := line("sdd-mmmmmm", "X", `"client_ref":"x0",`)
	t.Run("PATH form survives an upgrade behind the PATH name", func(t *testing.T) {
		dir := t.TempDir()
		link := installed(t, dir, "v1")
		t.Setenv("PATH", filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
		repo := newGitRepo(t, []string{x})
		out := runBin(t, link, repo, "tracker", "init")
		if got := driverOf(t, repo); got != pathDriver() {
			t.Fatalf("driver %q, want %q\n%s", got, pathDriver(), out)
		}
		if strings.Contains(out, "warning:") {
			t.Fatalf("no warning expected for the PATH binary:\n%s", out)
		}
		mustGit(t, repo, "add", "-A")
		mustGit(t, repo, "commit", "-q", "-m", "init")
		branchEdit(t, repo, "a", []string{x, line("sdd-aaaaaa", "from a", `"client_ref":"ra",`)})
		branchEdit(t, repo, "b", []string{x, line("sdd-bbbbbb", "from b", `"client_ref":"rb",`)})
		// Upgrade: v2 behind the same PATH name, v1 gone.
		installed(t, dir, "v2")
		if err := os.RemoveAll(filepath.Join(dir, "v1")); err != nil {
			t.Fatal(err)
		}
		mustGit(t, repo, "merge", "-q", "--no-edit", "a")
		if out, err := gitRun(t, repo, "merge", "-q", "--no-edit", "b"); err != nil {
			t.Fatalf("merge after upgrade: %v\n%s", err, out)
		}
		if lines := issueLines(t, repo); len(lines) != 3 {
			t.Fatalf("want 3 issues: %v", lines)
		}
	})
	t.Run("off-PATH binary records its path and warns", func(t *testing.T) {
		dir := t.TempDir()
		copyBin(t, filepath.Join(dir, "other", "qrtx")) // a different qrtx on PATH
		t.Setenv("PATH", filepath.Join(dir, "other")+string(os.PathListSeparator)+os.Getenv("PATH"))
		repo := newGitRepo(t, []string{x})
		out := qrtxRun(t, repo, "tracker", "init")
		got := driverOf(t, repo)
		if !strings.Contains(got, qrtxBinary(t)) || got == pathDriver() {
			t.Fatalf("driver %q should name %s", got, qrtxBinary(t))
		}
		if !strings.Contains(out, "warning:") || !strings.Contains(out, qrtxBinary(t)) {
			t.Fatalf("want a warning naming the path:\n%s", out)
		}
	})
	t.Run("old absolute driver is upgraded, then unchanged", func(t *testing.T) {
		dir := t.TempDir()
		link := installed(t, dir, "v1")
		t.Setenv("PATH", filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
		repo := newGitRepo(t, []string{x})
		mustGit(t, repo, "config", "merge.sdd-issues.driver", `"/old/place/qrtx" tracker merge-driver %O %A %B`)
		out := runBin(t, link, repo, "tracker", "init")
		if got := driverOf(t, repo); got != pathDriver() || !strings.Contains(out, "registered") {
			t.Fatalf("driver %q out:\n%s", got, out)
		}
		if out := runBin(t, link, repo, "tracker", "init"); !strings.Contains(out, "unchanged") {
			t.Fatalf("re-run should be unchanged:\n%s", out)
		}
	})
	t.Run("PATH driver is kept when init runs off PATH", func(t *testing.T) {
		dir := t.TempDir()
		copyBin(t, filepath.Join(dir, "other", "qrtx"))
		t.Setenv("PATH", filepath.Join(dir, "other")+string(os.PathListSeparator)+os.Getenv("PATH"))
		repo := newGitRepo(t, []string{x})
		// As left by an earlier init from the PATH binary.
		if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte(qrtx.IssuesFile+" merge=sdd-issues\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, repo, "config", "merge.sdd-issues.driver", pathDriver())
		out := qrtxRun(t, repo, "tracker", "init")
		if got := driverOf(t, repo); got != pathDriver() {
			t.Fatalf("PATH driver replaced with %q\n%s", got, out)
		}
		if !strings.Contains(out, "unchanged") || !strings.Contains(out, "warning:") {
			t.Fatalf("want unchanged plus a warning:\n%s", out)
		}
	})
}
