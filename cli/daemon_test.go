package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/millerscout/qrtx"
)

// daemonEnv is the environment for built-binary runs: daemons allowed.
func daemonEnv(root string, extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "QRTX_NO_DAEMON=") || strings.HasPrefix(kv, "CI=") || strings.HasPrefix(kv, "QRTX_SESSION=") || strings.HasPrefix(kv, qrtx.EnvRepo+"=") {
			continue
		}
		env = append(env, kv)
	}
	return append(append(env, qrtx.EnvRepo+"="+root), extra...)
}

func qrtxAt(t *testing.T, root string, extra []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(qrtxBinary(t), args...)
	cmd.Dir = root
	cmd.Env = daemonEnv(root, extra...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func syncStatus(t *testing.T, root string) qrtx.SyncStatus {
	t.Helper()
	out, err := qrtxAt(t, root, nil, "sync", "status", "--json")
	if err != nil {
		t.Fatalf("status: %v %s", err, out)
	}
	var st qrtx.SyncStatus
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("status json: %v\n%s", err, out)
	}
	return st
}

// stopDaemonCleanup stops any daemon a test started and fails if it lingers.
func stopDaemonCleanup(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = qrtxAt(t, root, nil, "sync", "stop")
		if st := syncStatus(t, root); st.Daemon != nil {
			t.Errorf("daemon pid %d still running after stop", st.Daemon.PID)
		}
	})
}

func waitFor(t *testing.T, what string, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func stateDirOf(root string) string { return filepath.Join(root, ".sdd") }

func fastSync(t *testing.T, root, lease string, extra string) {
	t.Helper()
	body := fmt.Sprintf("sync:\n  lease: %s\n  tick: 100ms\n  interval: 300ms\n%s", lease, extra)
	if err := os.WriteFile(filepath.Join(root, ".sdd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDaemonLifecycle(t *testing.T) {
	root := t.TempDir()
	fastSync(t, root, "2s", "")
	stopDaemonCleanup(t, root)
	out, err := qrtxAt(t, root, nil, "sync", "heartbeat", "--session", "A")
	if err != nil || !strings.Contains(out, "started sync daemon") {
		t.Fatalf("heartbeat: %v %s", err, out)
	}
	waitFor(t, "daemon up", 3*time.Second, func() bool { return syncStatus(t, root).Daemon != nil })
	if _, err := qrtxAt(t, root, nil, "sync", "claim", "demo/1.1", "--session", "A"); err != nil {
		t.Fatal(err)
	}
	// The only lease runs out: claims released, daemon exits by itself.
	waitFor(t, "daemon exit", 8*time.Second, func() bool { return syncStatus(t, root).Daemon == nil })
	logb, _ := os.ReadFile(filepath.Join(stateDirOf(root), "sync.log"))
	if !strings.Contains(string(logb), "exiting: no live sessions") {
		t.Fatalf("log:\n%s", logb)
	}
	if st := syncStatus(t, root); len(st.Claims) != 0 {
		t.Fatalf("claims after expiry: %+v", st.Claims)
	}
	// A new session starts a new daemon; stop ends it.
	if out, _ := qrtxAt(t, root, nil, "sync", "heartbeat", "--session", "B"); !strings.Contains(out, "started sync daemon") {
		t.Fatalf("restart: %s", out)
	}
	waitFor(t, "daemon up again", 3*time.Second, func() bool { return syncStatus(t, root).Daemon != nil })
	st := syncStatus(t, root)
	if st.Daemon.PID == 0 || len(st.Sessions) == 0 {
		t.Fatalf("status %+v", st)
	}
	start := time.Now()
	if out, err := qrtxAt(t, root, nil, "sync", "stop"); err != nil || !strings.Contains(out, "stopped") || time.Since(start) > 5*time.Second {
		t.Fatalf("stop: %v %s", err, out)
	}
	if syncStatus(t, root).Daemon != nil {
		t.Fatal("still running after stop")
	}
	if out, _ := qrtxAt(t, root, nil, "sync", "stop"); !strings.Contains(out, "not running") {
		t.Fatalf("second stop: %s", out)
	}
	if _, err := os.Stat(filepath.Join(stateDirOf(root), "stop")); err == nil {
		t.Fatal("stop request left behind")
	}
}

func TestDaemonStatus(t *testing.T) {
	root := t.TempDir()
	fastSync(t, root, "1h", "")
	stopDaemonCleanup(t, root)
	qrtxAt(t, root, nil, "sync", "heartbeat", "--session", "A")
	qrtxAt(t, root, nil, "sync", "claim", "github:7", "--session", "A")
	waitFor(t, "daemon", 3*time.Second, func() bool { return syncStatus(t, root).Daemon != nil })
	out, _ := qrtxAt(t, root, nil, "sync", "status", "--json")
	for _, k := range []string{`"daemon"`, `"pid"`, `"sessions"`, `"live": true`, `"claims"`, `"github:7"`, `"outbox"`, `"last_poll"`} {
		if !strings.Contains(out, k) {
			t.Fatalf("status lacks %s:\n%s", k, out)
		}
	}
}

// downPlugin is a shell tracker plugin whose answers depend on a mode file.
func downPlugin(t *testing.T, dir string) string {
	t.Helper()
	skipOnWindows(t, "sh-script plugins")
	bin := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
cat >/dev/null
echo "$1" >> %[1]q/log
if [ "$(cat %[1]q/mode 2>/dev/null)" = down ]; then echo '{"error":{"code":"unavailable","message":"down"}}'; exit 1; fi
case "$1" in
capabilities) echo '{"result":{"protocols":["sdd.adapter/v1"],"ops":["capabilities","list_issues","create_issue","get_issue"]}}' ;;
list_issues) echo '{"result":{"issues":[]}}' ;;
create_issue) echo '{"result":{"id":"F-1","title":"x","status":{"category":"backlog"}}}' ;;
*) echo '{"error":{"code":"not_found","message":"x"}}'; exit 1 ;;
esac
`, dir)
	if err := os.WriteFile(filepath.Join(bin, "qrtx-tracker-down"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestDaemonDrains(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()
	bin := downPlugin(t, dir)
	pathEnv := "PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")
	fastSync(t, root, "1h", "tracker:\n  backend: down\n")
	stopDaemonCleanup(t, root)
	_ = os.WriteFile(filepath.Join(dir, "mode"), []byte("down"), 0o644)
	if out, err := qrtxAt(t, root, []string{pathEnv}, "change", "new", "demo"); err != nil {
		t.Fatal(out)
	}
	p := filepath.Join(root, "sdd", "changes", "demo", "change.sdd.md")
	raw, _ := os.ReadFile(p)
	text := strings.Replace(string(raw), "(Optional — other slugs that own split acceptance.)", "- Export to CSV later.", 1)
	_ = os.WriteFile(p, []byte(text), 0o644)
	if out, err := qrtxAt(t, root, []string{pathEnv}, "tracker", "from-deferred", "--change", "demo"); err != nil || !strings.Contains(out, "1 pending") {
		t.Fatalf("queue: %v %s", err, out)
	}
	if _, err := qrtxAt(t, root, []string{pathEnv}, "sync", "heartbeat", "--session", "A"); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "mode"), []byte("up"), 0o644)
	waitFor(t, "daemon drain", 5*time.Second, func() bool {
		out, _ := qrtxAt(t, root, []string{pathEnv, "QRTX_NO_DAEMON=1"}, "sync", "status", "--json")
		return strings.Contains(out, `"outbox": 0`)
	})
	logb, _ := os.ReadFile(filepath.Join(dir, "log"))
	if !strings.Contains(string(logb), "create_issue") {
		t.Fatalf("plugin never got the create:\n%s", logb)
	}
}

func TestDaemonTrimsOnStart(t *testing.T) {
	root := t.TempDir()
	fastSync(t, root, "1h", "")
	stopDaemonCleanup(t, root)
	if _, err := qrtxAt(t, root, []string{"QRTX_NO_DAEMON=1"}, "sync", "heartbeat", "--session", "A"); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 1; i <= 6000; i++ {
		fmt.Fprintf(&b, `{"seq":%d,"at":%q,"session":"X","kind":"claim","key":"k"}`+"\n", i, now)
	}
	_ = os.WriteFile(filepath.Join(stateDirOf(root), "events.jsonl"), []byte(b.String()), 0o644)
	qrtxAt(t, root, nil, "sync", "heartbeat", "--session", "B")
	waitFor(t, "trim", 5*time.Second, func() bool {
		raw, _ := os.ReadFile(filepath.Join(stateDirOf(root), "events.jsonl"))
		return strings.Count(string(raw), "\n") == 5000
	})
}

func TestDaemonTwoRoots(t *testing.T) {
	repo := newGitRepo(t, nil)
	a, b := filepath.Join(repo, "a"), filepath.Join(repo, "b")
	for _, r := range []string{a, b} {
		_ = os.MkdirAll(r, 0o755)
		fastSync(t, r, "1h", "")
	}
	stopDaemonCleanup(t, a)
	qrtxAt(t, a, nil, "sync", "heartbeat", "--session", "A")
	qrtxAt(t, b, nil, "sync", "heartbeat", "--session", "B")
	if out, err := qrtxAt(t, a, nil, "sync", "claim", "demo/1.1", "--session", "A"); err != nil {
		t.Fatal(out)
	}
	if out, err := qrtxAt(t, b, nil, "sync", "claim", "demo/1.1", "--session", "B"); err != nil {
		t.Fatalf("same key in another root conflicted: %s", out)
	}
	gitDir := filepath.Join(repo, ".git", "sdd")
	// The daemon starts on its own time: wait for its first log line.
	var logb []byte
	waitFor(t, "daemon log", 3*time.Second, func() bool {
		logb, _ = os.ReadFile(filepath.Join(gitDir, "sync.log"))
		return strings.Contains(string(logb), "started (")
	})
	if strings.Count(string(logb), "started (") != 1 {
		t.Fatalf("want one daemon for both roots:\n%s", logb)
	}
	st := syncStatus(t, a)
	if st.Daemon == nil || len(st.Sessions) != 2 {
		t.Fatalf("status %+v", st)
	}
}

func TestDaemonAutostart(t *testing.T) {
	root := t.TempDir()
	fastSync(t, root, "1h", "")
	stopDaemonCleanup(t, root)
	// CI: lease updated, no daemon.
	if out, err := qrtxAt(t, root, []string{"CI=true"}, "sync", "heartbeat", "--session", "ci"); err != nil || strings.Contains(out, "started") {
		t.Fatalf("CI: %v %s", err, out)
	}
	st := syncStatus(t, root)
	if st.Daemon != nil || len(st.Sessions) != 1 {
		t.Fatalf("CI status %+v", st)
	}
	// Two heartbeats at once: one daemon.
	var wg sync.WaitGroup
	for _, s := range []string{"A", "B"} {
		wg.Add(1)
		go func(s string) { defer wg.Done(); qrtxAt(t, root, nil, "sync", "heartbeat", "--session", s) }(s)
	}
	wg.Wait()
	waitFor(t, "daemon", 3*time.Second, func() bool { return syncStatus(t, root).Daemon != nil })
	time.Sleep(300 * time.Millisecond)
	logb, _ := os.ReadFile(filepath.Join(stateDirOf(root), "sync.log"))
	if n := strings.Count(string(logb), "started ("); n != 1 {
		t.Fatalf("%d daemons started:\n%s", n, logb)
	}
	// A killed daemon is replaced by the next heartbeat. On Windows a killed
	// holder's lock file looks held until the 60s stale rule (filed for
	// sync-hardening: check whether the owner pid is alive).
	skipOnWindows(t, "detecting a killed lock holder before the 60s stale rule")
	pid := syncStatus(t, root).Daemon.PID
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Signal(syscall.SIGKILL)
	}
	waitFor(t, "daemon gone", 3*time.Second, func() bool { return syncStatus(t, root).Daemon == nil })
	if out, _ := qrtxAt(t, root, nil, "sync", "heartbeat", "--session", "C"); !strings.Contains(out, "started sync daemon") {
		t.Fatalf("replacement: %s", out)
	}
	waitFor(t, "replacement up", 3*time.Second, func() bool { return syncStatus(t, root).Daemon != nil })
}

func TestDaemonExitRace(t *testing.T) {
	root := t.TempDir()
	fastSync(t, root, "300ms", "")
	stopDaemonCleanup(t, root)
	for i := 0; i < 20; i++ {
		qrtxAt(t, root, nil, "sync", "heartbeat", "--session", fmt.Sprintf("s%d", i))
		time.Sleep(time.Duration(i%5) * 80 * time.Millisecond)
		// Invariant: a live session always has a daemon (give it a moment to start).
		st := syncStatus(t, root)
		live := false
		for _, s := range st.Sessions {
			live = live || s.Live
		}
		if live {
			ok := false
			for j := 0; j < 20 && !ok; j++ {
				ok = syncStatus(t, root).Daemon != nil
				if !ok {
					time.Sleep(25 * time.Millisecond)
				}
			}
			if !ok {
				// The lease may have just ended; only a still-live session counts.
				st2 := syncStatus(t, root)
				still := false
				for _, s := range st2.Sessions {
					still = still || s.Live
				}
				if still && st2.Daemon == nil {
					t.Fatalf("iteration %d: live session with no daemon", i)
				}
			}
		}
	}
}
