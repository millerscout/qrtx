package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func countEvents(t *testing.T, gitSdd, kind string) int {
	t.Helper()
	raw, _ := os.ReadFile(filepath.Join(gitSdd, "events.jsonl"))
	return strings.Count(string(raw), `"kind":"`+kind+`"`)
}

func TestWatch(t *testing.T) {
	repo := newGitRepo(t, nil)
	fastSync(t, repo, "1h", "")
	if out, err := sddAt(t, repo, nil, "change", "new", "demo"); err != nil {
		t.Fatal(out)
	}
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-q", "-m", "demo")
	wt := filepath.Join(t.TempDir(), "wt")
	mustGit(t, repo, "worktree", "add", "-q", "-b", "feature", wt)
	gitSdd := filepath.Join(repo, ".git", "sdd")
	stopDaemonCleanup(t, repo)
	sddAt(t, repo, nil, "sync", "heartbeat", "--session", "A")
	sddAt(t, wt, nil, "sync", "heartbeat", "--session", "B")
	waitFor(t, "daemon", 3*time.Second, func() bool { return syncStatus(t, repo).Daemon != nil })
	// Let the first snapshot happen: it must be silent.
	time.Sleep(600 * time.Millisecond)
	if n := countEvents(t, gitSdd, "task_progress"); n != 0 {
		t.Fatalf("first snapshot emitted %d events", n)
	}
	manifest := filepath.Join(wt, "sdd", "changes", "demo", "change.sdd.md")
	flip := func(from, to string) {
		raw, _ := os.ReadFile(manifest)
		_ = os.WriteFile(manifest, []byte(strings.Replace(string(raw), from, to, 1)), 0o644)
	}
	flip("- [ ] 1.1 First implementation task", "- [x] 1.1 First implementation task")
	waitFor(t, "task_progress", 5*time.Second, func() bool { return countEvents(t, gitSdd, "task_progress") == 1 })
	out, _ := sddAt(t, repo, nil, "sync", "inbox", "--session", "A")
	if !strings.Contains(out, "demo/1.1 done in wt (feature)") {
		t.Fatalf("A's inbox: %q", out)
	}
	// A change made while no daemon runs is reported once on restart.
	if _, err := sddAt(t, repo, nil, "sync", "stop"); err != nil {
		t.Fatal(err)
	}
	flip("- [x] 1.1 First implementation task", "- [ ] 1.1 First implementation task")
	sddAt(t, repo, nil, "sync", "heartbeat", "--session", "C")
	waitFor(t, "restart event", 5*time.Second, func() bool { return countEvents(t, gitSdd, "task_progress") == 2 })
	time.Sleep(500 * time.Millisecond)
	if n := countEvents(t, gitSdd, "task_progress"); n != 2 {
		t.Fatalf("restart produced %d events, want exactly one more", n-1)
	}
	// Moving the change away (as archive does) emits nothing.
	_ = os.MkdirAll(filepath.Join(wt, "sdd", "changes", "archive"), 0o755)
	_ = os.Rename(filepath.Join(wt, "sdd", "changes", "demo"), filepath.Join(wt, "sdd", "changes", "archive", "2026-11-26-demo"))
	time.Sleep(700 * time.Millisecond)
	if n := countEvents(t, gitSdd, "task_progress"); n != 2 {
		t.Fatalf("archive move emitted events: %d", n)
	}
}
