package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/millerscout/sdd"
)

var (
	buildMu sync.Mutex
	built   = map[string]string{}
	binDir  string
)

// buildBin builds ../cmd/<name> once into a shared temp bin dir.
func buildBin(t *testing.T, name string) string {
	t.Helper()
	buildMu.Lock()
	defer buildMu.Unlock()
	if p, ok := built[name]; ok {
		return p
	}
	if binDir == "" {
		dir, err := os.MkdirTemp("", "sdd-bin-")
		if err != nil {
			t.Fatal(err)
		}
		binDir = dir
	}
	p := filepath.Join(binDir, name)
	if out, err := exec.Command("go", "build", "-o", p, "../cmd/"+name).CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	built[name] = p
	return p
}

// sddBinary builds cmd/sdd so git can run it as the merge driver.
func sddBinary(t *testing.T) string { return buildBin(t, "sdd") }

func gitEnv(repo string) []string {
	return append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_NOSYSTEM=1", sdd.EnvRepo+"="+repo)
}

func gitRun(t *testing.T, repo string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	cmd.Env = gitEnv(repo)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func mustGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := gitRun(t, repo, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

func sddRun(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command(sddBinary(t), args...)
	cmd.Dir = repo
	cmd.Env = gitEnv(repo)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sdd %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func newGitRepo(t *testing.T, issues []string) string {
	t.Helper()
	repo := t.TempDir()
	mustGit(t, repo, "init", "-q", "-b", "main")
	_ = os.MkdirAll(filepath.Join(repo, "sdd"), 0o755)
	writeIssues(t, repo, issues)
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "base")
	return repo
}

func writeIssues(t *testing.T, repo string, lines []string) {
	t.Helper()
	body := ""
	if len(lines) > 0 {
		body = strings.Join(lines, "\n") + "\n"
	}
	if err := os.WriteFile(filepath.Join(repo, "sdd", "issues.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func issueLines(t *testing.T, repo string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repo, "sdd", "issues.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
}

func line(id, title, extra string) string {
	return `{"id":"` + id + `",` + extra + `"status":{"category":"backlog"},"title":"` + title + `"}`
}

// branchEdit commits lines on a new branch from main, then returns to main.
func branchEdit(t *testing.T, repo, branch string, lines []string) {
	t.Helper()
	mustGit(t, repo, "checkout", "-q", "-b", branch, "main")
	writeIssues(t, repo, lines)
	mustGit(t, repo, "commit", "-q", "-am", branch)
	mustGit(t, repo, "checkout", "-q", "main")
}

func TestTrackerMerge(t *testing.T) {
	x := line("sdd-mmmmmm", "X", `"client_ref":"x0",`)
	t.Run("same title on two branches", func(t *testing.T) {
		repo := newGitRepo(t, []string{x})
		out := sddRun(t, repo, "tracker", "init")
		if !strings.Contains(out, "registered merge driver") {
			t.Fatal(out)
		}
		if drv := mustGit(t, repo, "config", "merge.sdd-issues.driver"); !strings.Contains(drv, "tracker merge-driver") {
			t.Fatalf("driver %q", drv)
		}
		mustGit(t, repo, "add", "-A")
		mustGit(t, repo, "commit", "-q", "-m", "init")
		mustGit(t, repo, "checkout", "-q", "-b", "a")
		sddRun(t, repo, "tracker", "add", "Same title")
		mustGit(t, repo, "commit", "-q", "-am", "a")
		mustGit(t, repo, "checkout", "-q", "main")
		sddRun(t, repo, "tracker", "add", "Same title")
		mustGit(t, repo, "commit", "-q", "-am", "main")
		if out, err := gitRun(t, repo, "merge", "-q", "--no-edit", "a"); err != nil {
			t.Fatalf("merge: %v\n%s", err, out)
		}
		lines := issueLines(t, repo)
		if len(lines) != 3 {
			t.Fatalf("want base + 2 issues: %v", lines)
		}
	})
	t.Run("same short id, different client_ref", func(t *testing.T) {
		repo := newGitRepo(t, []string{x})
		sddRun(t, repo, "tracker", "init")
		mustGit(t, repo, "add", "-A")
		mustGit(t, repo, "commit", "-q", "-m", "init")
		branchEdit(t, repo, "a", []string{x, line("sdd-zzzzzz", "from a", `"client_ref":"ra",`)})
		branchEdit(t, repo, "b", []string{x, line("sdd-zzzzzz", "from b", `"client_ref":"rb",`)})
		mustGit(t, repo, "merge", "-q", "--no-edit", "a")
		if out, err := gitRun(t, repo, "merge", "-q", "--no-edit", "b"); err != nil {
			t.Fatalf("merge: %v\n%s", err, out)
		}
		lines := issueLines(t, repo)
		if len(lines) != 3 {
			t.Fatalf("want 3 issues: %v", lines)
		}
		long := 0
		for _, l := range lines {
			var is sdd.Issue
			if err := json.Unmarshal([]byte(l), &is); err != nil {
				t.Fatal(err)
			}
			if len(is.ID) == len("sdd-")+8 && is.Title == "from b" {
				long++
			}
		}
		if long != 1 {
			t.Fatalf("incoming issue should be re-IDed to 8 chars: %v", lines)
		}
	})
	t.Run("different fields merge, same field conflicts", func(t *testing.T) {
		repo := newGitRepo(t, []string{x})
		sddRun(t, repo, "tracker", "init")
		mustGit(t, repo, "add", "-A")
		mustGit(t, repo, "commit", "-q", "-m", "init")
		branchEdit(t, repo, "title", []string{line("sdd-mmmmmm", "X renamed", `"client_ref":"x0",`)})
		branchEdit(t, repo, "labels", []string{line("sdd-mmmmmm", "X", `"client_ref":"x0","labels":["ui"],`)})
		branchEdit(t, repo, "title2", []string{line("sdd-mmmmmm", "X other", `"client_ref":"x0",`)})
		mustGit(t, repo, "merge", "-q", "--no-edit", "title")
		if out, err := gitRun(t, repo, "merge", "-q", "--no-edit", "labels"); err != nil {
			t.Fatalf("merge labels: %v\n%s", err, out)
		}
		got := issueLines(t, repo)
		if len(got) != 1 || !strings.Contains(got[0], "X renamed") || !strings.Contains(got[0], `"labels":["ui"]`) {
			t.Fatalf("want both changes: %v", got)
		}
		if _, err := gitRun(t, repo, "merge", "-q", "--no-edit", "title2"); err == nil {
			t.Fatal("title vs title should conflict")
		}
		raw, _ := os.ReadFile(filepath.Join(repo, "sdd", "issues.jsonl"))
		if !strings.Contains(string(raw), "<<<<<<<") || !strings.Contains(string(raw), "X other") {
			t.Fatalf("want conflict markers:\n%s", raw)
		}
	})
	t.Run("no driver, separated inserts", func(t *testing.T) {
		a := line("sdd-aaaaaa", "a", "")
		m := line("sdd-mmmmmm", "m", "")
		z := line("sdd-zzzzzz", "z", "")
		repo := newGitRepo(t, []string{a, m, z})
		branchEdit(t, repo, "one", []string{a, line("sdd-bbbbbb", "b", ""), m, z})
		branchEdit(t, repo, "two", []string{a, m, line("sdd-nnnnnn", "n", ""), z})
		mustGit(t, repo, "merge", "-q", "--no-edit", "one")
		if out, err := gitRun(t, repo, "merge", "-q", "--no-edit", "two"); err != nil {
			t.Fatalf("plain git merge: %v\n%s", err, out)
		}
		if n := len(issueLines(t, repo)); n != 5 {
			t.Fatalf("want 5 issues, got %d", n)
		}
	})
}

func TestTrackerCLI(t *testing.T) {
	root := t.TempDir()
	t.Setenv(sdd.EnvRepo, root)
	out, _, err := run(t, "tracker", "add", "Dark-mode toggle", "--description", "Settings page")
	if err != nil || !strings.HasPrefix(out, "added file:sdd-") {
		t.Fatalf("add: %q %v", out, err)
	}
	ref := strings.TrimSpace(strings.TrimPrefix(out, "added "))
	out, _, err = run(t, "tracker", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var list []sdd.Issue
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list) != 1 || list[0].Title != "Dark-mode toggle" {
		t.Fatalf("list: %s %v", out, err)
	}
	if len(list[0].Refs) != 1 || list[0].Refs[0].Repo == "" {
		t.Fatalf("add should record the repo: %+v", list[0].Refs)
	}
	if out, _, err = run(t, "tracker", "get", ref); err != nil || !strings.Contains(out, "Settings page") {
		t.Fatalf("get: %q %v", out, err)
	}
	if out, _, err = run(t, "tracker", "close", ref); err != nil || !strings.Contains(out, "completed") {
		t.Fatalf("close: %q %v", out, err)
	}
	if out, _, _ = run(t, "tracker", "list"); strings.TrimSpace(out) != "" {
		t.Fatalf("closed issue still listed as open: %q", out)
	}
	if out, _, _ = run(t, "tracker", "list", "--all"); !strings.Contains(out, "completed") {
		t.Fatalf("--all: %q", out)
	}
	if _, _, err := run(t, "tracker", "get", "file:sdd-nope00"); err == nil || !strings.Contains(err.Error(), "not_found") {
		t.Fatalf("missing issue: %v", err)
	}
}

func TestTrackerCheck(t *testing.T) {
	root := t.TempDir()
	t.Setenv(sdd.EnvRepo, root)
	if _, _, err := run(t, "tracker", "add", "keep me"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(root, "sdd", "issues.jsonl"))
	out, _, err := run(t, "tracker", "check")
	if err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	for _, op := range []string{"capabilities", "create_issue", "create_issue (idempotent)", "get_issue", "update_issue", "transition_issue", "list_issues"} {
		if !strings.Contains(out, "pass  "+op) {
			t.Fatalf("missing pass for %s:\n%s", op, out)
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, "sdd", "issues.jsonl"))
	if string(before) != string(after) {
		t.Fatal("check changed sdd/issues.jsonl")
	}
}

func TestTrackerCheckExec(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("needs sh")
	}
	root := t.TempDir()
	t.Setenv(sdd.EnvRepo, root)
	bin := t.TempDir()
	logf := filepath.Join(bin, "log")
	script := `#!/bin/sh
cat >/dev/null
echo "$1" >> "` + logf + `"
case "$1" in
capabilities) echo '{"result":{"protocols":["sdd.adapter/v1"],"ops":["capabilities","list_issues","get_issue","create_issue","update_issue","transition_issue"]}}' ;;
list_issues) echo '{"result":{"issues":[]}}' ;;
*) echo '{"error":{"code":"unsupported","message":"read-only fake"}}'; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "sdd-tracker-shfake"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = os.WriteFile(filepath.Join(root, ".sdd.yaml"), []byte("tracker:\n  backend: shfake\n"), 0o644)
	out, _, err := run(t, "tracker", "check")
	if err != nil {
		t.Fatalf("read-only check: %v\n%s", err, out)
	}
	raw, _ := os.ReadFile(logf)
	if got := strings.Fields(string(raw)); strings.Join(got, ",") != "capabilities,list_issues" {
		t.Fatalf("without --allow-writes only capabilities and list_issues may run: %v", got)
	}
}

func TestTrackerArchiveCLIReportsPending(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("needs sh")
	}
	root := t.TempDir()
	t.Setenv(sdd.EnvRepo, root)
	bin := t.TempDir()
	script := `#!/bin/sh
cat >/dev/null
case "$1" in
capabilities) echo '{"result":{"protocols":["sdd.adapter/v1"],"ops":["capabilities","transition_issue","comment"]}}' ;;
*) echo '{"error":{"code":"unavailable","message":"down"}}'; exit 1 ;;
esac
`
	_ = os.WriteFile(filepath.Join(bin, "sdd-tracker-down"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = os.WriteFile(filepath.Join(root, ".sdd.yaml"), []byte("tracker:\n  backend: down\n"), 0o644)
	if err := sdd.NewChange(root, "demo"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "sdd", "changes", "demo", "change.sdd.md")
	raw, _ := os.ReadFile(p)
	text := string(raw)
	text = strings.Replace(text, "(TBD — what the user wanted to accomplish or experience, from the conversation arc.)", "Ship demo.", 1)
	text = strings.Replace(text, "- (TBD — user-visible checks that satisfy the requester.)", "- demo works.", 1)
	text = sdd.RemovePlaceholderSpecSections(text)
	text = strings.Replace(text, "- [ ] 1.1 First implementation task", "- [x] 1.1 Do it", 1)
	text = strings.Replace(text, "- [ ] V1.1 (TBD — e.g. go test ./… -run Test…)", "- [x] V1.1 manual: done", 1)
	text = strings.Replace(text, "- [ ] V1.2 (optional) golden row in scripts/fixtures/ if behavior is user-visible", "- [x] V1.2 manual: n/a", 1)
	text = strings.Replace(text, "status: planning", "status: complete\nissues: [\"down:7\"]", 1)
	_ = os.WriteFile(p, []byte(text), 0o644)
	out, errOut, err := run(t, "change", "archive", "demo")
	if err != nil || !strings.Contains(out, "archived sdd/changes/demo") {
		t.Fatalf("archive: %q %q %v", out, errOut, err)
	}
	if !strings.Contains(errOut, "2 pending") || !strings.Contains(errOut, "tracker outbox") {
		t.Fatalf("stderr should name pending writes: %q", errOut)
	}
	out, _, err = run(t, "tracker", "outbox")
	if err != nil || !strings.Contains(out, "comment") || !strings.Contains(out, "transition_issue") {
		t.Fatalf("outbox: %q %v", out, err)
	}
}
