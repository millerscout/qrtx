package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/millerscout/qrtx"
)

func withHookStdin(t *testing.T, body string) {
	t.Helper()
	prev := hookStdin
	hookStdin = strings.NewReader(body)
	t.Cleanup(func() { hookStdin = prev })
}

func TestSyncHeartbeatCLI(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, qrtx.RepoConfigFile), nil, 0o644) // hooks act only in qrtx projects
	t.Setenv(qrtx.EnvRepo, root)
	hookJSON, _ := json.Marshal(map[string]string{"session_id": "abc", "cwd": root, "hook_event_name": "UserPromptSubmit"})
	withHookStdin(t, string(hookJSON)) // json.Marshal escapes Windows backslashes
	if out, _, err := run(t, "sync", "heartbeat", "--hook"); err != nil || !strings.Contains(out, "abc: lease until") {
		t.Fatalf("%q %v", out, err)
	}
	out, _, _ := run(t, "sync", "status", "--json")
	var st qrtx.SyncStatus
	if err := json.Unmarshal([]byte(out), &st); err != nil || len(st.Sessions) != 1 || st.Sessions[0].ID != "abc" || !st.Sessions[0].Live {
		t.Fatalf("status %s %v", out, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	// An unwritable state dir warns and still exits 0.
	r2 := t.TempDir()
	t.Setenv(qrtx.EnvRepo, r2)
	_ = os.MkdirAll(filepath.Join(r2, ".sdd"), 0o755)
	_ = os.Chmod(filepath.Join(r2, ".sdd"), 0o500)
	defer os.Chmod(filepath.Join(r2, ".sdd"), 0o755)
	_, errOut, err := run(t, "sync", "heartbeat", "--session", "x")
	if err != nil || !strings.Contains(errOut, "warning") {
		t.Fatalf("unwritable: %q %v", errOut, err)
	}
}

func TestSyncClaimCLI(t *testing.T) {
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	if out, _, err := run(t, "sync", "claim", "demo/1.1", "--session", "A"); err != nil || !strings.Contains(out, "sync not enabled; claim skipped") {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".sdd")); err == nil {
		t.Fatal("claim created the state dir")
	}
	run(t, "sync", "heartbeat", "--session", "A")
	run(t, "sync", "heartbeat", "--session", "B")
	if out, _, err := run(t, "sync", "claim", "demo/1.1", "--session", "A"); err != nil || !strings.Contains(out, "claimed demo/1.1 for A") {
		t.Fatalf("%q %v", out, err)
	}
	if _, _, err := run(t, "sync", "claim", "demo/1.1", "--session", "B"); err == nil || !strings.Contains(err.Error(), "claimed by session A") {
		t.Fatalf("conflict: %v", err)
	}
}

func TestSyncInboxCLI(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, qrtx.RepoConfigFile), nil, 0o644) // hooks act only in qrtx projects
	t.Setenv(qrtx.EnvRepo, root)
	run(t, "sync", "heartbeat", "--session", "A")
	run(t, "sync", "heartbeat", "--session", "B")
	for i := 0; i < 25; i++ {
		run(t, "sync", "claim", "task/"+string(rune('a'+i)), "--session", "A")
	}
	cwd, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	hook := `{"session_id":"B","cwd":` + string(cwd) + `}`
	withHookStdin(t, hook)
	out, _, err := run(t, "sync", "inbox", "--hook", "--brief")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 12 || !strings.HasPrefix(lines[11], "…and 15 more") {
		t.Fatalf("brief:\n%s", out)
	}
	withHookStdin(t, hook)
	if out, _, _ := run(t, "sync", "inbox", "--hook", "--brief"); strings.TrimSpace(out) != "" {
		t.Fatalf("nothing new should print nothing: %q", out)
	}
}

func TestSyncWatch(t *testing.T) {
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	run(t, "sync", "heartbeat", "--session", "A")
	run(t, "sync", "claim", "old/1", "--session", "A")
	watchStop = make(chan struct{})
	watchPoll = 20 * time.Millisecond
	defer func() { watchStop = nil; watchPoll = 500 * time.Millisecond }()
	var out lockedBuf
	done := make(chan error)
	go func() { done <- Run([]string{"sync", "watch"}, &out, &bytes.Buffer{}) }()
	time.Sleep(100 * time.Millisecond)
	run(t, "sync", "claim", "new/1", "--session", "A")
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(out.String(), "new/1") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	close(watchStop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s := out.String(); !strings.Contains(s, "A claimed new/1") || strings.Contains(s, "old/1") {
		t.Fatalf("watch output %q", s)
	}
}

type lockedBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func TestHooksCLI(t *testing.T) {
	root := t.TempDir()
	t.Setenv(qrtx.EnvRepo, root)
	if out, _, err := run(t, "skills", "install", "--hooks"); err != nil || !strings.Contains(out, "updated "+filepath.Join(root, ".claude", "settings.local.json")) {
		t.Fatalf("%q %v", out, err)
	}
	if out, _, _ := run(t, "skills", "install", "--hooks"); !strings.Contains(out, "unchanged "+filepath.Join(root, ".claude", "settings.local.json")) {
		t.Fatalf("re-run: %q", out)
	}
	if out, _, err := run(t, "skills", "install", "--hooks", "--shared"); err != nil || !strings.Contains(out, filepath.Join(root, ".claude", "settings.json")) {
		t.Fatalf("shared: %q %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "settings.json")); err != nil {
		t.Fatal("--shared did not write settings.json")
	}
}
